package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// The PATH page's startup tab lists the slowest lines of the startup
// files with their time and the rest, says what makes a known culprit
// fast, and offers to move from nvm to mise; a slow start is an issue
// whose fix is the same plan.
func TestPathStartup(t *testing.T) {
	ms := time.Millisecond
	rc := home + "/.zshrc"
	nvm := inventory.StartupLine{File: rc, Line: 12, Text: `[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"`, Took: 920 * ms}
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{
		Tools: []inventory.ToolState{{Tool: "node", Installations: []inventory.Installation{{Tool: "node", Version: "22.12.0", Source: inventory.Source{Manager: "nvm"}, Default: true, OnPath: 1}}}},
		Startup: &inventory.ShellStartup{Shell: "zsh", Took: 1420 * ms,
			Files: []inventory.StartupFile{{File: "/etc/zshrc", Took: 60 * ms}, {File: home + "/.zprofile", Took: 90 * ms}, {File: rc, Took: 1270 * ms}},
			Lines: []inventory.StartupLine{nvm,
				{File: rc, Line: 31, Text: `eval "$(pyenv init -)"`, Took: 210 * ms},
				{File: rc, Line: 45, Text: "compinit", Took: 140 * ms}}},
	}
	var built []ops.Plan
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		built = append(built, ops.Plan{Title: "built"})
	}

	a.router.Push(pathPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, locales.StartupLabel(), "zsh · slow")
	a.pathTab = startupTab
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "path-startup")
	hasTexts(t, tt, "0.92 s", "65% of the start", "line 12", "slow", locales.CulpritNvm(), locales.CulpritCompinit(),
		"0.15 s", "The rest, in 3 files", locales.StartupHow(), "1.42 s", "~/.zprofile")
	if err := tt.Click(locales.MoveToMise()); err != nil {
		t.Fatal(err)
	}
	if len(built) != 1 {
		t.Fatalf("Move to mise built %d plans", len(built))
	}

	found := issues.Find(a.Snapshot, nil, nil, time.Now())
	i := slices.IndexFunc(found, func(is issues.Issue) bool { return is.Kind == issues.SlowStartup })
	if i < 0 {
		t.Fatalf("no slow start issue in %+v", found)
	}
	title, explain := issueText(found[i], home)
	if title != "The terminal takes 1.42 s to start" || !strings.Contains(explain, "~/.zshrc:12 (0.92 s)") {
		t.Errorf("issue %q: %q", title, explain)
	}
	if fix := a.issueFix(found[i], found); fix == nil {
		t.Error("the issue has no fix")
	} else {
		fix()
		if len(built) != 2 {
			t.Errorf("the fix built %d plans", len(built)-1)
		}
	}
}
