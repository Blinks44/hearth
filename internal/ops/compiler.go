package ops

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// CompilerFound reports that st, C and C++'s state, has a compiler: the
// OS's toolset, or a cc the terminal runs that is not macOS's stand-in.
func CompilerFound(st inventory.ToolState) bool {
	return st.Cpp != nil && st.Cpp.Base != nil || inventory.TerminalCopy(st.Installations) >= 0
}

// CompilerPlan returns the plan that installs the OS's C and C++
// compiler: the Command Line Tools on macOS, the distribution's build
// package through managers on Linux, Visual Studio Build Tools through
// WinGet on Windows. ok is false when Hearth has no way here.
func CompilerPlan(managers []inventory.PackageManager, detect Detect) (p Plan, ok bool) {
	p, ok = compilerPlan(managers)
	if !ok {
		return Plan{}, false
	}
	p.Title = locales.CompilerInstallTitle()
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) { return compilerWaitFor(ctx, detect) }
	}
	return p, true
}

// compilerPoll is how far apart the check looks again while it waits for
// the installer; tests shorten it.
var compilerPoll = 5 * time.Second

// compilerWaitFor checks for the compiler until it is there or
// compilerWait is over: macOS's installer goes on in its own window after
// its command returned.
func compilerWaitFor(ctx context.Context, detect Detect) (string, error) {
	deadline := time.Now().Add(compilerWait)
	for {
		found, err := compilerCheck(ctx, detect)
		if err == nil || !time.Now().Before(deadline) {
			return found, err
		}
		select {
		case <-ctx.Done():
			return found, err
		case <-time.After(compilerPoll):
		}
	}
}

// compilerCheck finds C and C++ again and says what compiler there is.
func compilerCheck(ctx context.Context, detect Detect) (string, error) {
	st, err := detect(ctx)
	if err != nil {
		return "", err
	}
	if !CompilerFound(st) {
		return "", errors.New(locales.CompilerCheckMissing())
	}
	if b := st.Cpp.Base; b != nil {
		return locales.CompilerCheckFound(strings.TrimSpace(b.Name + " " + b.Version)), nil
	}
	in := st.Installations[inventory.TerminalCopy(st.Installations)]
	return locales.CompilerCheckFound(strings.TrimSpace(st.Cpp.Command + " " + in.Version)), nil
}
