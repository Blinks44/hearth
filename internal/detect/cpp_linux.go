package detect

import (
	"context"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const ccCommand = "cc"

// exeName is the file of the command name.
func exeName(name string) string { return name }

// cppStub reports a stand-in compiler; Linux has none.
func cppStub(string) bool { return false }

// cppBase is Debian's and Ubuntu's build-essential, as dpkg knows it; nil
// on other distributions and without it.
func cppBase(ctx context.Context, r run.Runner, env sysenv.Env) *inventory.CppBase {
	res, err := r.Run(ctx, run.Cmd{Args: []string{"dpkg-query", "-W", "-f=${Status}\t${Version}", "build-essential"}, Env: env})
	if err != nil || res.ExitCode != 0 {
		return nil
	}
	// install ok installed	12.10ubuntu1; a removed package keeps its record.
	status, version, _ := strings.Cut(string(res.Stdout), "\t")
	if !strings.HasSuffix(status, " installed") {
		return nil
	}
	return &inventory.CppBase{ID: "build-essential", Name: "build-essential", Version: strings.TrimSpace(version)}
}
