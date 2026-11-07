//go:build !windows

package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestPathPlan(t *testing.T) {
	home := t.TempDir()
	bin, nvm := filepath.Join(home, "bin"), filepath.Join(home, ".nvm/versions/node/v16.20.2/bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	zprofile, zshrc := filepath.Join(home, ".zprofile"), filepath.Join(home, ".zshrc")
	src := func(file string, line int) inventory.PathSource { return inventory.PathSource{File: file, Line: line} }
	env := sysenv.Env{"HOME=" + home}
	tests := []struct {
		name            string
		zprofile, zshrc string
		entries         []inventory.PathEntry
		dirs            []string
		// file is the one the plan edits, .zshrc when empty; want what it
		// becomes, "" when there is no plan.
		file, want string
	}{
		{
			name:     "a repeat keeps the line that puts it first: the last before PATH",
			zprofile: "export PATH=\"$HOME/bin:$PATH\"\n",
			zshrc:    "alias ll='ls -l'\nexport PATH=\"$HOME/bin:$PATH\"\n",
			entries: []inventory.PathEntry{
				{Dir: bin, Sources: []inventory.PathSource{src(zprofile, 1), src(zshrc, 2)}},
				{Dir: "/usr/bin", Sources: []inventory.PathSource{src("/etc/paths", 1)}},
				{Dir: bin, Sources: []inventory.PathSource{src(zprofile, 1), src(zshrc, 2)}},
			},
			file: zprofile,
			want: "# hearth: repeats an earlier PATH entry · export PATH=\"$HOME/bin:$PATH\"\n",
		},
		{
			name:     "a repeat keeps the line that puts it first: the first after PATH",
			zprofile: "export PATH=\"$PATH:$HOME/bin\"\n",
			zshrc:    "alias ll='ls -l'\nexport PATH=\"$PATH:$HOME/bin\"\n",
			entries: []inventory.PathEntry{
				{Dir: "/usr/bin", Sources: []inventory.PathSource{src("/etc/paths", 1)}},
				{Dir: bin, Sources: []inventory.PathSource{src(zprofile, 1), src(zshrc, 2)}},
				{Dir: bin, Sources: []inventory.PathSource{src(zprofile, 1), src(zshrc, 2)}},
			},
			want: "alias ll='ls -l'\n# hearth: repeats an earlier PATH entry · export PATH=\"$PATH:$HOME/bin\"\n",
		},
		{
			name:     "a repeat brew shellenv puts first stays",
			zprofile: "eval \"$(/opt/homebrew/bin/brew shellenv)\"\n",
			zshrc:    "export PATH=\"/opt/homebrew/bin:$PATH\"\n",
			entries: []inventory.PathEntry{
				{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{src(zprofile, 1), src(zshrc, 1)}},
				{Dir: "/opt/homebrew/bin", Sources: []inventory.PathSource{src(zprofile, 1), src(zshrc, 1)}},
			},
		},
		{
			name:  "a folder that is not there",
			zshrc: "export PATH=\"$HOME/.nvm/versions/node/v16.20.2/bin:$PATH\"\n",
			entries: []inventory.PathEntry{
				{Dir: nvm, Missing: true, Sources: []inventory.PathSource{src(zshrc, 1)}},
				{Dir: "/usr/bin"},
			},
			want: "# hearth: no such folder · export PATH=\"$HOME/.nvm/versions/node/v16.20.2/bin:$PATH\"\n",
		},
		{
			name:  "only the problems asked for",
			zshrc: "export PATH=\"$HOME/.nvm/versions/node/v16.20.2/bin:$PATH\"\nexport PATH=\"$HOME/gone:$PATH\"\n",
			entries: []inventory.PathEntry{
				{Dir: filepath.Join(home, "gone"), Missing: true, Sources: []inventory.PathSource{src(zshrc, 2)}},
				{Dir: nvm, Missing: true, Sources: []inventory.PathSource{src(zshrc, 1)}},
			},
			dirs: []string{nvm},
			want: "# hearth: no such folder · export PATH=\"$HOME/.nvm/versions/node/v16.20.2/bin:$PATH\"\nexport PATH=\"$HOME/gone:$PATH\"\n",
		},
		{
			name:    "a line that adds other folders too stays",
			zshrc:   "export PATH=\"$HOME/.nvm/versions/node/v16.20.2/bin:$HOME/bin:$PATH\"\n",
			entries: []inventory.PathEntry{{Dir: nvm, Missing: true, Sources: []inventory.PathSource{src(zshrc, 1)}}},
		},
		{
			name:    "a line no longer where the scan saw it stays",
			zshrc:   "alias ll='ls -l'\n",
			entries: []inventory.PathEntry{{Dir: nvm, Missing: true, Sources: []inventory.PathSource{src(zshrc, 1)}}},
		},
		{
			name:  "a repeat from one line only is not known",
			zshrc: "export PATH=\"$HOME/bin:$PATH\"\n",
			entries: []inventory.PathEntry{
				{Dir: bin, Sources: []inventory.PathSource{src(zshrc, 1)}},
				{Dir: bin, Sources: []inventory.PathSource{src(zshrc, 1)}},
			},
		},
		{
			name:  "the system's repeat and files are left",
			zshrc: "export PATH=\"/usr/bin:$PATH\"\n",
			entries: []inventory.PathEntry{
				{Dir: "/usr/bin", Sources: []inventory.PathSource{src("/etc/paths", 1), src(zshrc, 1)}},
				{Dir: "/usr/bin", Sources: []inventory.PathSource{src("/etc/paths", 1), src(zshrc, 1)}},
				{Dir: "/nowhere", Missing: true, Sources: []inventory.PathSource{src("/etc/paths.d/x", 1)}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for file, text := range map[string]string{zprofile: tt.zprofile, zshrc: tt.zshrc} {
				if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p, err := PathPlan(tt.entries, tt.dirs, env, nil)
			if tt.want == "" {
				if !errors.Is(err, ErrNoFix) {
					t.Fatalf("err = %v, want ErrNoFix; steps %+v", err, p.Steps)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			file, before := zshrc, tt.zshrc
			if tt.file == zprofile {
				file, before = zprofile, tt.zprofile
			}
			if len(p.Steps) != 1 || p.Steps[0].Edit.Path != file || p.Steps[0].Edit.Before != before {
				t.Fatalf("steps %+v", p.Steps)
			}
			if got := p.Steps[0].Edit.After; got != tt.want {
				t.Errorf("after\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// A PATH plan checks a new terminal: the commands found where they were,
// the problems gone; one that does not pass puts the files back.
func TestPathPlanVerify(t *testing.T) {
	home := t.TempDir()
	zshrc := filepath.Join(home, ".zshrc")
	text := "export PATH=\"$HOME/gone:$PATH\"\n"
	if err := os.WriteFile(zshrc, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(home, "gone")
	entries := []inventory.PathEntry{{Dir: gone, Missing: true, Sources: []inventory.PathSource{{File: zshrc, Line: 1}}}, {Dir: "/usr/bin"}, {Dir: "/bin"}}
	for _, tt := range []struct {
		path string
		ok   bool
	}{
		{"/usr/bin:/bin", true},
		{"/bin:/usr/bin", false},
		{gone + ":/usr/bin:/bin", false},
	} {
		terminal := func(context.Context) (sysenv.Env, error) { return sysenv.Env{"PATH=" + tt.path}, nil }
		p, err := PathPlan(entries, nil, sysenv.Env{"HOME=" + home}, terminal)
		if err != nil {
			t.Fatal(err)
		}
		s := Execute(context.Background(), nil, p, &strings.Builder{}, func(State) {})
		data, _ := os.ReadFile(zshrc)
		if tt.ok && (s.Phase != Succeeded || string(data) == text) || !tt.ok && (s.Phase != NotVerified || string(data) != text || len(s.Restored) != 1) {
			t.Errorf("PATH %s: phase %v, err %q, restored %q, file %q", tt.path, s.Phase, s.Err, s.Restored, data)
		}
		if err := os.WriteFile(zshrc, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
