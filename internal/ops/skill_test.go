package ops

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// skillHome is a home with skills in Claude Code's folder: pdf, and plan,
// which ~/.agents/skills has a copy of.
func skillHome(t *testing.T) (string, sysenv.Env) {
	t.Helper()
	home := t.TempDir()
	write := func(dir string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+filepath.Base(dir)+"\n---\nbody\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".claude", "skills", "pdf"))
	write(filepath.Join(home, ".claude", "skills", "plan"))
	write(filepath.Join(home, ".agents", "skills", "plan"))
	env := slices.Concat(sysenv.Env(os.Environ()), sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "CLAUDE_CONFIG_DIR=", "CODEX_HOME=", "COPILOT_HOME="})
	return home, env
}

func skillNamed(t *testing.T, env sysenv.Env, name string) []inventory.Skill {
	t.Helper()
	var out []inventory.Skill
	for _, s := range ai.Skills(env) {
		if s.Name == name {
			out = append(out, s)
		}
	}
	return out
}

func runPlan(t *testing.T, p Plan, env sysenv.Env) {
	t.Helper()
	p.Env = env
	if s := Execute(context.Background(), run.Exec{}, p, io.Discard, func(State) {}); s.Phase != Succeeded {
		t.Fatalf("%s: phase %v, %q, %q", p.Title, s.Phase, s.Err, s.Log)
	}
}

func title(id string) string { return id }

// A skill connects to Codex by a link in the folder Codex reads, which
// other agents read too; it disconnects by taking the link away, the
// folder staying.
func TestSkillLinkAndUnlink(t *testing.T) {
	home, env := skillHome(t)
	pdf := skillNamed(t, env, "pdf")[0]
	p, err := SkillLinkPlan(pdf, "codex", env, title)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".agents", "skills", "pdf")
	// Cursor reads ~/.claude/skills too: it has pdf already.
	if !slices.ContainsFunc(p.Notes, func(n Note) bool { return strings.Contains(n.Text, "copilot") && !strings.Contains(n.Text, "cursor") }) {
		t.Errorf("notes = %+v, want the other readers of ~/.agents/skills without pdf", p.Notes)
	}
	runPlan(t, p, env)
	pdf = skillNamed(t, env, "pdf")[0]
	if !sameSkillPath(pdf.Dir, link) {
		t.Fatalf("main folder = %s", pdf.Dir)
	}
	private := filepath.Join(home, ".claude", "skills", "pdf")
	if !slices.ContainsFunc(pdf.Paths, func(sp inventory.SkillPath) bool { return sameSkillPath(sp.Path, private) && sp.Link }) {
		t.Fatalf("private path must be a link: %+v", pdf.Paths)
	}
	if _, err := SkillLinkPlan(pdf, "codex", env, title); err == nil {
		t.Error("accepted existing destination")
	}
	if _, err := SkillUnlinkPlan(pdf, pdf.Dir, env, title); err == nil {
		t.Error("accepted removal of shared folder")
	}
	p, err = SkillUnlinkPlan(pdf, private, env, title)
	if err != nil {
		t.Fatal(err)
	}
	runPlan(t, p, env)
	if _, err := os.Stat(filepath.Join(pdf.Dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}

}

// Identical copies merge into one folder: the other copy moves to the
// backups and a link to the kept folder takes its place.
func TestSkillMerge(t *testing.T) {
	home, env := skillHome(t)
	copies := skillNamed(t, env, "plan")
	if len(copies) != 2 || copies[0].Sum != copies[1].Sum {
		t.Fatalf("copies: %+v", copies)
	}
	backups := filepath.Join(home, "backups")
	p, err := SkillMergePlan([][]inventory.Skill{copies}, backups, env, time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	runPlan(t, p, env)
	after := skillNamed(t, env, "plan")
	if len(after) != 1 || !sameSkillPath(after[0].Dir, filepath.Join(home, ".agents", "skills", "plan")) || len(after[0].Paths) != 2 {
		t.Fatalf("after the merge: %+v", after)
	}
	if _, err := os.Stat(filepath.Join(backups, "plan-2026-11-11-090000", "SKILL.md")); err != nil {
		t.Errorf("no backup of the copy: %v", err)
	}
	differ := []inventory.Skill{copies[0], copies[1]}
	differ[1].Sum = "other"
	if _, err := SkillMergePlan([][]inventory.Skill{differ}, backups, env, time.Now()); err == nil {
		t.Error("a merge of copies that differ")
	}
}

func TestSkillMergeCommonStorage(t *testing.T) {
	for _, name := range []string{"private copies", "shared link", "project copies", "shared copy last"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home}
			scope := ""
			base := home
			if name == "project copies" {
				base = filepath.Join(home, "repo")
				scope = base
			}
			roots := []string{filepath.Join(base, ".claude", "skills", "example"), filepath.Join(base, ".gemini", "skills", "example")}
			shared := filepath.Join(base, ".agents", "skills", "example")
			if name == "shared copy last" {
				roots[1] = shared
			}
			for _, root := range roots {
				if err := os.MkdirAll(root, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: example\n---\nbody"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if name == "shared link" {
				if err := os.MkdirAll(filepath.Dir(shared), 0755); err != nil {
					t.Fatal(err)
				}
				res, err := (run.Exec{}).Run(t.Context(), run.Cmd{Args: linkCommand(roots[0], shared), Timeout: time.Minute})
				if err != nil || res.ExitCode != 0 {
					t.Fatalf("create shared link: %v %s", err, res.Stderr)
				}
			}
			copies := readSkills(env, scope)
			slices.SortFunc(copies, func(a, b inventory.Skill) int { return strings.Compare(b.Dir, a.Dir) })
			p, err := SkillMergePlan([][]inventory.Skill{copies}, filepath.Join(home, "backups"), env, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Notes) == 0 || !strings.Contains(p.Notes[0].Text, ".agents") {
				t.Fatal("common path missing from preview")
			}
			runPlan(t, p, env)
			info, err := os.Lstat(shared)
			if err != nil || info.Mode()&linkMode != 0 {
				t.Fatalf("shared folder is not physical: %v", err)
			}
			after := readSkills(env, scope)
			if len(after) != 1 || !sameSkillPath(after[0].Dir, shared) {
				t.Fatalf("skills: %+v", after)
			}
			for _, root := range roots {
				if root != shared {
					info, err := os.Lstat(root)
					if err != nil || info.Mode()&linkMode == 0 {
						t.Fatalf("private folder is not a link: %s %v", root, err)
					}
				}
			}
		})
	}
}

func TestSkillCommonDestinationConflict(t *testing.T) {
	home, env := skillHome(t)
	sk := skillNamed(t, env, "pdf")[0]
	dest := filepath.Join(home, ".agents", "skills", "pdf")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("keep different content"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := SkillLinkPlan(sk, "codex", env, title); err == nil {
		t.Fatal("accepted occupied common destination")
	}
	data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || string(data) != "keep different content" {
		t.Fatal("changed existing common copy")
	}
	if _, err := os.Stat(filepath.Join(sk.Dir, "SKILL.md")); err != nil {
		t.Fatal("changed original before confirmation")
	}
}
