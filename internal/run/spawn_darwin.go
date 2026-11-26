//go:build darwin

package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// macOS <sys/spawn.h> flags.
const (
	posixSpawnSetSigDef      int16 = 0x0004
	posixSpawnSetSigMask     int16 = 0x0008
	posixSpawnSetSID         int16 = 0x0400
	posixSpawnCloexecDefault int16 = 0x4000

	waitDelay = time.Second
)

// darwinSpawnAPI holds libSystem.B.dylib functions used to spawn opt-in
// daemon/VM launcher commands with macOS process responsibility disclaimed, so
// persistent background VMs (e.g. Colima / Virtualization.framework, Podman)
// are not attributed to Hearth in Activity Monitor.
type darwinSpawnAPI struct {
	spawn          func(pid *int32, path *byte, fa *uintptr, attr *uintptr, argv **byte, envp **byte) int32
	attrInit       func(attr *uintptr) int32
	attrDestroy    func(attr *uintptr) int32
	attrSetFlags   func(attr *uintptr, flags int16) int32
	attrSetSigDef  func(attr *uintptr, sigdefault *uint32) int32
	attrSetSigMask func(attr *uintptr, sigmask *uint32) int32
	faInit         func(fa *uintptr) int32
	faDestroy      func(fa *uintptr) int32
	faAddDup2      func(fa *uintptr, fd, newfd int32) int32
	faAddOpen      func(fa *uintptr, fd int32, path *byte, oflag int32, mode uint16) int32
	faAddChdir     func(fa *uintptr, path *byte) int32
	setDisclaim    func(attr *uintptr, disclaim int32) int32
}

var (
	spawnAPIOnce sync.Once
	spawnAPIVal  *darwinSpawnAPI
)

func loadSpawnAPI() *darwinSpawnAPI {
	spawnAPIOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return
		}
		var api darwinSpawnAPI
		if !registerSymbol(&api.spawn, lib, "posix_spawn") ||
			!registerSymbol(&api.attrInit, lib, "posix_spawnattr_init") ||
			!registerSymbol(&api.attrDestroy, lib, "posix_spawnattr_destroy") ||
			!registerSymbol(&api.attrSetFlags, lib, "posix_spawnattr_setflags") ||
			!registerSymbol(&api.attrSetSigDef, lib, "posix_spawnattr_setsigdefault") ||
			!registerSymbol(&api.attrSetSigMask, lib, "posix_spawnattr_setsigmask") ||
			!registerSymbol(&api.faInit, lib, "posix_spawn_file_actions_init") ||
			!registerSymbol(&api.faDestroy, lib, "posix_spawn_file_actions_destroy") ||
			!registerSymbol(&api.faAddDup2, lib, "posix_spawn_file_actions_adddup2") ||
			!registerSymbol(&api.faAddOpen, lib, "posix_spawn_file_actions_addopen") ||
			!registerSymbol(&api.setDisclaim, lib, "responsibility_spawnattrs_setdisclaim") {
			return
		}
		// macOS 26 standardized posix_spawn_file_actions_addchdir; macOS 10.15-25 provide _np.
		if !registerSymbol(&api.faAddChdir, lib, "posix_spawn_file_actions_addchdir") &&
			!registerSymbol(&api.faAddChdir, lib, "posix_spawn_file_actions_addchdir_np") {
			return
		}
		spawnAPIVal = &api
	})
	return spawnAPIVal
}

func registerSymbol(fptr any, lib uintptr, name string) bool {
	sym, err := purego.Dlsym(lib, name)
	if err != nil {
		return false
	}
	purego.RegisterFunc(fptr, sym)
	return true
}

func runPlatform(ctx context.Context, c Cmd, limit int) (Result, bool, error) {
	api := loadSpawnAPI()
	if api == nil {
		return Result{}, false, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Result{ExitCode: -1}, true, fmt.Errorf("run %s: %w", c.Args[0], ctxErr)
	}
	path, err := resolvePath(c.Args[0], c.Dir)
	if err != nil {
		return Result{}, true, fmt.Errorf("run %s: %w", c.Args[0], err)
	}

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return Result{}, true, fmt.Errorf("run %s: %w", c.Args[0], err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return Result{}, true, fmt.Errorf("run %s: %w", c.Args[0], err)
	}

	var (
		stdinR, stdinW *os.File
		stdinFile      *os.File
		stdinFd        int32 = -1
	)
	if c.Stdin != nil {
		if f, ok := c.Stdin.(*os.File); ok {
			if f != nil {
				stdinFile = f
				stdinFd = int32(f.Fd())
			}
		} else {
			stdinR, stdinW, err = os.Pipe()
			if err != nil {
				_ = stdoutR.Close()
				_ = stdoutW.Close()
				_ = stderrR.Close()
				_ = stderrW.Close()
				return Result{}, true, fmt.Errorf("run %s: %w", c.Args[0], err)
			}
			stdinFd = int32(stdinR.Fd())
		}
	}

	start := time.Now()
	proc, err := api.spawnProcess(path, c.Args, c.Env, c.Dir, [3]int32{stdinFd, int32(stdoutW.Fd()), int32(stderrW.Fd())})
	runtime.KeepAlive(stdinFile)
	_ = stdoutW.Close()
	_ = stderrW.Close()
	if stdinR != nil {
		_ = stdinR.Close()
	}
	if err != nil {
		_ = stdoutR.Close()
		_ = stderrR.Close()
		if stdinW != nil {
			_ = stdinW.Close()
		}
		return Result{Duration: time.Since(start)}, true, fmt.Errorf("run %s: %w", c.Args[0], err)
	}

	stopKill := context.AfterFunc(ctx, func() { _ = proc.Kill() })
	defer stopKill()

	stdoutCap := &capped{limit: limit}
	stderrCap := &capped{limit: limit}
	var outW, errW io.Writer = stdoutCap, stderrCap
	if c.Output != nil {
		outW = io.MultiWriter(stdoutCap, c.Output)
		errW = io.MultiWriter(stderrCap, c.Output)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(outW, stdoutR)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(errW, stderrR)
	}()

	var stdinDone chan struct{}
	if stdinW != nil {
		stdinDone = make(chan struct{})
		go func() {
			defer close(stdinDone)
			_, _ = io.Copy(stdinW, c.Stdin)
			_ = stdinW.Close()
		}()
	}

	ps, waitErr := proc.Wait()
	stopKill()

	// Wait for stdout/stderr readers to drain, but cap at waitDelay so a
	// grandchild holding a pipe open cannot stall Run indefinitely.
	ioDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(ioDone)
	}()

	timer := time.NewTimer(waitDelay)
	select {
	case <-ioDone:
		timer.Stop()
	case <-timer.C:
		_ = stdoutR.Close()
		_ = stderrR.Close()
		if stdinW != nil {
			_ = stdinW.Close()
		}
		<-ioDone
	}
	_ = stdoutR.Close()
	_ = stderrR.Close()
	if stdinW != nil {
		_ = stdinW.Close()
		// Best-effort wait so well-behaved Stdin readers finish cleanly before Run returns.
		stdinTimer := time.NewTimer(waitDelay)
		select {
		case <-stdinDone:
			stdinTimer.Stop()
		case <-stdinTimer.C:
		}
	}

	res := Result{
		Stdout:    stdoutCap.buf.Bytes(),
		Stderr:    stderrCap.buf.Bytes(),
		Truncated: stdoutCap.dropped || stderrCap.dropped,
		Duration:  time.Since(start),
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		res.ExitCode = -1
		return res, true, fmt.Errorf("run %s: %w", c.Args[0], ctxErr)
	}
	if waitErr != nil {
		return res, true, fmt.Errorf("run %s: %w", c.Args[0], waitErr)
	}
	res.ExitCode = ps.ExitCode()
	return res, true, nil
}

// resolvePath mirrors os/exec.Command's executable lookup: bare names are
// searched in Hearth's PATH via exec.LookPath; paths with a separator are used
// as-is (relative to dir when dir is set).
func resolvePath(name, dir string) (string, error) {
	if !strings.ContainsRune(name, filepath.Separator) {
		p, err := exec.LookPath(name)
		if err != nil {
			return "", err
		}
		if dir != "" && !filepath.IsAbs(p) {
			if abs, absErr := filepath.Abs(p); absErr == nil {
				p = abs
			}
		}
		return p, nil
	}
	return name, nil
}

func (api *darwinSpawnAPI) spawnProcess(path string, args, env []string, dir string, fds [3]int32) (*os.Process, error) {
	pathBuf, err := syscall.ByteSliceFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "posix_spawn", Path: path, Err: err}
	}
	var dirBuf []byte
	if dir != "" {
		dirBuf, err = syscall.ByteSliceFromString(dir)
		if err != nil {
			return nil, &os.PathError{Op: "posix_spawn", Path: dir, Err: err}
		}
	}
	if env == nil {
		env = os.Environ()
	} else {
		env = dedupEnv(env)
	}

	argvPtrs, argvBacking, err := toCStringArray(args)
	if err != nil {
		return nil, &os.PathError{Op: "posix_spawn", Path: path, Err: err}
	}
	envpPtrs, envpBacking, err := toCStringArray(env)
	if err != nil {
		return nil, &os.PathError{Op: "posix_spawn", Path: path, Err: err}
	}
	devNullBuf, err := syscall.ByteSliceFromString(os.DevNull)
	if err != nil {
		return nil, &os.PathError{Op: "posix_spawn", Path: os.DevNull, Err: err}
	}

	// Move any source fd that sits in 0..2 at a different slot above 2 so
	// earlier dup2/open actions on 0..2 cannot clobber it.
	var tempFds []int
	defer func() {
		for _, fd := range tempFds {
			_ = syscall.Close(fd)
		}
	}()
	for i := range fds {
		if fds[i] >= 0 && fds[i] <= 2 && fds[i] != int32(i) {
			duped, dupErr := unix.FcntlInt(uintptr(fds[i]), unix.F_DUPFD_CLOEXEC, 3)
			if dupErr != nil {
				return nil, &os.PathError{Op: "fcntl", Path: path, Err: dupErr}
			}
			tempFds = append(tempFds, duped)
			fds[i] = int32(duped)
		}
	}

	var attr uintptr
	if rc := api.attrInit(&attr); rc != 0 {
		return nil, &os.PathError{Op: "posix_spawnattr_init", Path: path, Err: syscall.Errno(rc)}
	}
	defer api.attrDestroy(&attr)

	flags := posixSpawnSetSID | posixSpawnCloexecDefault | posixSpawnSetSigDef | posixSpawnSetSigMask
	if rc := api.attrSetFlags(&attr, flags); rc != 0 {
		return nil, &os.PathError{Op: "posix_spawnattr_setflags", Path: path, Err: syscall.Errno(rc)}
	}
	var emptyMask uint32
	if rc := api.attrSetSigMask(&attr, &emptyMask); rc != 0 {
		return nil, &os.PathError{Op: "posix_spawnattr_setsigmask", Path: path, Err: syscall.Errno(rc)}
	}
	allSigs := ^uint32(0)
	if rc := api.attrSetSigDef(&attr, &allSigs); rc != 0 {
		return nil, &os.PathError{Op: "posix_spawnattr_setsigdefault", Path: path, Err: syscall.Errno(rc)}
	}
	if rc := api.setDisclaim(&attr, 1); rc != 0 {
		return nil, &os.PathError{Op: "responsibility_spawnattrs_setdisclaim", Path: path, Err: syscall.Errno(rc)}
	}

	var fa uintptr
	if rc := api.faInit(&fa); rc != 0 {
		return nil, &os.PathError{Op: "posix_spawn_file_actions_init", Path: path, Err: syscall.Errno(rc)}
	}
	defer api.faDestroy(&fa)

	for target := range int32(3) {
		if fds[target] < 0 {
			oflag := int32(syscall.O_WRONLY)
			if target == 0 {
				oflag = int32(syscall.O_RDONLY)
			}
			if rc := api.faAddOpen(&fa, target, &devNullBuf[0], oflag, 0); rc != 0 {
				return nil, &os.PathError{Op: "posix_spawn_file_actions_addopen", Path: os.DevNull, Err: syscall.Errno(rc)}
			}
		} else {
			if rc := api.faAddDup2(&fa, fds[target], target); rc != 0 {
				return nil, &os.PathError{Op: "posix_spawn_file_actions_adddup2", Path: path, Err: syscall.Errno(rc)}
			}
		}
	}
	if len(dirBuf) > 0 {
		if rc := api.faAddChdir(&fa, &dirBuf[0]); rc != 0 {
			return nil, &os.PathError{Op: "posix_spawn_file_actions_addchdir", Path: dir, Err: syscall.Errno(rc)}
		}
	}

	var pid int32
	syscall.ForkLock.RLock()
	rc := api.spawn(&pid, &pathBuf[0], &fa, &attr, &argvPtrs[0], &envpPtrs[0])
	syscall.ForkLock.RUnlock()

	runtime.KeepAlive(pathBuf)
	runtime.KeepAlive(dirBuf)
	runtime.KeepAlive(devNullBuf)
	runtime.KeepAlive(argvPtrs)
	runtime.KeepAlive(argvBacking)
	runtime.KeepAlive(envpPtrs)
	runtime.KeepAlive(envpBacking)

	if rc != 0 {
		return nil, &os.PathError{Op: "posix_spawn", Path: path, Err: syscall.Errno(rc)}
	}
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		return nil, fmt.Errorf("find spawned process %d: %w", pid, err)
	}
	return proc, nil
}

func toCStringArray(ss []string) (ptrs []*byte, backing [][]byte, err error) {
	ptrs = make([]*byte, len(ss)+1)
	backing = make([][]byte, len(ss))
	for i, s := range ss {
		b, bErr := syscall.ByteSliceFromString(s)
		if bErr != nil {
			return nil, nil, bErr
		}
		backing[i] = b
		ptrs[i] = &b[0]
	}
	return ptrs, backing, nil
}

// dedupEnv keeps the last value for each duplicate environment key, matching
// os/exec so callers can override variables via append(env, "KEY=value").
func dedupEnv(env []string) []string {
	out := make([]string, 0, len(env))
	saw := make(map[string]int, len(env))
	for _, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			out = append(out, kv)
			continue
		}
		if idx, dup := saw[key]; dup {
			out[idx] = kv
			continue
		}
		saw[key] = len(out)
		out = append(out, kv)
	}
	return out
}
