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
	if len(p.Notes) != 1 || !strings.Contains(p.Notes[0].Text, "copilot") || strings.Contains(p.Notes[0].Text, "cursor") {
		t.Errorf("notes = %+v, want the other readers of ~/.agents/skills without pdf", p.Notes)
	}
	runPlan(t, p, env)
	pdf = skillNamed(t, env, "pdf")[0]
	if !slices.ContainsFunc(pdf.Paths, func(sp inventory.SkillPath) bool { return sp.Path == link && sp.Link }) {
		t.Fatalf("paths after linking: %+v", pdf.Paths)
	}
	if _, err := SkillLinkPlan(pdf, "codex", env, title); err == nil {
		t.Error("a plan for a place already taken")
	}
	if _, err := SkillUnlinkPlan(pdf, pdf.Dir, env, title); err == nil {
		t.Error("a plan that would delete the skill's own folder")
	}
	p, err = SkillUnlinkPlan(pdf, link, env, title)
	if err != nil {
		t.Fatal(err)
	}
	// Cursor keeps it by ~/.claude/skills: Codex loses it, Cursor does not.
	if !strings.Contains(p.Title, "codex") || strings.Contains(p.Title, "cursor") {
		t.Errorf("title %q, want the agents that lose it alone", p.Title)
	}
	runPlan(t, p, env)
	if _, err := os.Stat(filepath.Join(pdf.Dir, "SKILL.md")); err != nil {
		t.Errorf("the skill's folder went: %v", err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Error("the link stayed")
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
	if len(after) != 1 || after[0].Dir != copies[0].Dir || len(after[0].Paths) != 2 {
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
