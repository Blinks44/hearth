package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type VMDisk struct {
	Path      string
	Allocated int64
}

func readVMDisk(ctx context.Context, r run.Runner, env sysenv.Env, target Target) VMDisk {
	var paths []string
	switch target.Provider {
	case "Colima":
		name := target.ColimaProfile
		if name == "" || !filepath.IsLocal(name) || filepath.Base(name) != name {
			return VMDisk{}
		}
		instance := "colima"
		if name != "default" {
			instance += "-" + name
		}
		dir := env.Get("LIMA_HOME")
		if dir == "" {
			dir = filepath.Join(ColimaConfigDir(env), "_lima")
		}
		paths = []string{
			filepath.Join(dir, "_disks", instance, "datadisk"),
			filepath.Join(dir, instance, "disk"),
			filepath.Join(dir, instance, "diffdisk"),
		}
	case "Podman":
		paths = []string{podmanDisk(ctx, r, env, target.Endpoint)}
	default:
		paths = desktopDisks(env, target.Provider)
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if bytes, ok := allocatedSize(path, info); ok {
			return VMDisk{Path: path, Allocated: bytes}
		}
	}
	return VMDisk{}
}

func podmanDisk(ctx context.Context, r run.Runner, env sysenv.Env, endpoint string) string {
	paths := sysenv.LookPathAll("podman", env)
	if len(paths) == 0 {
		return ""
	}
	names, err := r.Run(ctx, run.Cmd{Args: []string{paths[0], "machine", "list", "--format", "{{.Name}}"}, Env: env})
	if err != nil || names.ExitCode != 0 || names.Truncated || len(strings.Fields(string(names.Stdout))) == 0 {
		return ""
	}
	args := []string{paths[0], "machine", "inspect", "--format", "{{json .}}", "--"}
	args = append(args, strings.Fields(string(names.Stdout))...)
	type machinePath struct{ Path string }
	machines, err := readRows[struct {
		ImagePath      machinePath
		ConnectionInfo struct{ PodmanSocket, PodmanPipe machinePath }
	}](ctx, r, run.Cmd{Args: args, Env: env})
	if err != nil {
		return ""
	}
	for _, m := range machines {
		if sameMachineSocket(endpoint, m.ConnectionInfo.PodmanSocket.Path) || sameMachineSocket(endpoint, m.ConnectionInfo.PodmanPipe.Path) {
			return m.ImagePath.Path
		}
	}
	return ""
}

func sameMachineSocket(endpoint, socket string) bool {
	if socket == "" {
		return false
	}
	if path, ok := strings.CutPrefix(endpoint, "unix://"); ok {
		if path == socket {
			return true
		}
		resolved, err := filepath.EvalSymlinks(path)
		other, otherErr := filepath.EvalSymlinks(socket)
		return err == nil && otherErr == nil && resolved == other
	}
	if path, ok := strings.CutPrefix(endpoint, "npipe:"); ok {
		return strings.EqualFold(strings.TrimLeft(path, "/"), strings.TrimLeft(filepath.ToSlash(socket), "/"))
	}
	return false
}

// ColimaDisk checks the file that belongs to a matched Colima profile.
func ColimaDisk(env sysenv.Env, profile string) (VMDisk, error) {
	disk := readVMDisk(context.Background(), nil, env, Target{Provider: "Colima", ColimaProfile: profile})
	if disk.Path == "" {
		return VMDisk{}, fmt.Errorf("cannot read VM disk for Colima profile %q", profile)
	}
	return disk, nil
}
