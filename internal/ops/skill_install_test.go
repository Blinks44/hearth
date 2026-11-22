package ops

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func installSkill() skillcatalog.Skill {
	return skillcatalog.Skill{Name: "example", Dir: "example", Repository: "anthropics/skills", Commit: "abc", Files: []skillcatalog.File{{Path: "SKILL.md", Data: []byte("---\nname: example\n---\nDo something.")}, {Path: "scripts/test.sh", Data: []byte("exit 42\n"), Executable: true}, {Path: "data.bin", Data: []byte{0, 255, 10}}}}
}

func TestSkillInstall(t *testing.T) {
	home := t.TempDir()
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home}
	sk := installSkill()
	p, err := SkillInstallPlan(sk, "claude", env, func(id string) string { return id })
	if err != nil {
		t.Fatal(err)
	}
	dest := p.Steps[0].Skill.Path
	if dest != filepath.Join(home, ".agents", "skills", "example") || p.Steps[0].Skill.Link != filepath.Join(home, ".claude", "skills", "example") {
		t.Fatal("installation must use common storage and a Claude link")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("plan created destination")
	}
	sk.Files[0].Data[0] = '!'
	state := Execute(t.Context(), nil, p, io.Discard, func(State) {})
	if state.Phase != Succeeded {
		t.Fatalf("state %+v", state)
	}
	found := ai.Skills(env)
	if len(found) != 1 || found[0].Name != "example" {
		t.Fatalf("not discovered: %+v", found)
	}
	data, err := os.ReadFile(filepath.Join(dest, "data.bin"))
	if err != nil || len(data) != 3 || data[1] != 255 {
		t.Fatalf("binary file %v %v", data, err)
	}
	if _, err := SkillInstallPlan(installSkill(), "claude", env, func(id string) string { return id }); err == nil {
		t.Fatal("existing destination accepted")
	}
}

func TestSkillInstallProtection(t *testing.T) {
	for _, tc := range []string{"destination appeared", "verification fails", "cancelled", "modified after install", "invalid path"} {
		t.Run(tc, func(t *testing.T) {
			home := t.TempDir()
			env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home}
			p, err := SkillInstallPlan(installSkill(), "claude", env, func(id string) string { return id })
			if err != nil {
				t.Fatal(err)
			}
			dest := p.Steps[0].Skill.Path
			ctx := t.Context()
			switch tc {
			case "destination appeared":
				if err := os.MkdirAll(dest, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dest, "keep"), []byte("user"), 0644); err != nil {
					t.Fatal(err)
				}
			case "verification fails":
				p.Verify = func(context.Context) (string, error) { return "", errors.New("failed") }
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "modified after install":
				p.Verify = func(context.Context) (string, error) {
					if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("user changes"), 0644); err != nil {
						t.Fatal(err)
					}
					return "", errors.New("failed")
				}
			case "invalid path":
				p.Steps[0].Skill.Skill.Files = append(p.Steps[0].Skill.Skill.Files, skillcatalog.File{Path: "../escape"})
			}
			state := Execute(ctx, nil, p, io.Discard, func(State) {})
			if state.Phase == Succeeded {
				t.Fatal("unsafe operation succeeded")
			}
			switch tc {
			case "destination appeared":
				if data, err := os.ReadFile(filepath.Join(dest, "keep")); err != nil || string(data) != "user" {
					t.Fatal("user file lost")
				}
			case "modified after install":
				if data, err := os.ReadFile(filepath.Join(dest, "SKILL.md")); err != nil || string(data) != "user changes" {
					t.Fatal("changed file lost")
				}
			default:
				if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "example")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("rollback left an agent link")
				}
				if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("destination remains: %v", err)
				}
			}
		})
	}
}

func TestSkillInstallSymlink(t *testing.T) {
	home := t.TempDir()
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home}
	p, err := SkillInstallPlan(installSkill(), "claude", env, func(id string) string { return id })
	if err != nil {
		t.Fatal(err)
	}
	dest := p.Steps[0].Skill.Path
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, dest); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	state := Execute(t.Context(), nil, p, io.Discard, func(State) {})
	if state.Phase == Succeeded {
		t.Fatal("followed a destination symlink")
	}
	files, err := os.ReadDir(outside)
	if err != nil || len(files) != 0 {
		t.Fatal("wrote outside destination")
	}
	if _, err := SkillInstallPlan(installSkill(), "claude", env, func(id string) string { return id }); err == nil {
		t.Fatal("plan accepted a symlink")
	}
}

func TestSkillInstallProject(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "work", "app")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home}
	p, err := SkillInstallPlanFor(installSkill(), "claude", project, env, func(id string) string { return id })
	if err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].Skill.Path != filepath.Join(project, ".agents", "skills", "example") ||
		p.Steps[0].Skill.Link != filepath.Join(project, ".claude", "skills", "example") {
		t.Fatalf("unexpected project install paths: %+v", p.Steps[0].Skill)
	}
	state := Execute(t.Context(), nil, p, io.Discard, func(State) {})
	if state.Phase != Succeeded {
		t.Fatalf("state = %+v", state)
	}
	found := ai.ProjectSkills(project)
	if len(found) != 1 || found[0].Name != "example" || found[0].Project != project {
		t.Fatalf("project skills = %+v", found)
	}
}
