package ai

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

// A junction is a link: one to a skill is marked, one to nowhere stays
// visible as broken.
func TestSkillsJunction(t *testing.T) {
	root := t.TempDir()
	skills := filepath.Join(root, "skills")
	shared := filepath.Join(root, "shared", "pdf")
	gone := filepath.Join(root, "gone")
	for _, dir := range []string{skills, shared, gone} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(shared, "SKILL.md"), []byte("---\nname: pdf\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{"pdf": shared, "broken": gone} {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(skills, link), target).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	var found []foundSkill
	findSkills(skills, false, maxDepth, &found)
	if len(found) != 2 {
		t.Fatalf("%+v", found)
	}
	for _, f := range found {
		if wantBroken := filepath.Base(f.path) == "broken"; !f.link || f.broken != wantBroken {
			t.Fatalf("%+v", f)
		}
	}
	if b := brokenSkill(inventory.SkillPath{Path: filepath.Join(skills, "broken")}, ""); b.Dir != gone {
		t.Fatalf("target %q, want %q", b.Dir, gone)
	}
}
