package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func catalogFixture() []skillcatalog.Skill {
	var out []skillcatalog.Skill
	for _, item := range []struct{ name, desc string }{
		{"frontend-design", "Дизайн интерфейсов с характером"}, {"skill-creator", "Создание и улучшение скиллов"}, {"webapp-testing", "Проверка веб-приложений с Playwright"}, {"docx", "Создание и редактирование документов Word"}, {"pdf", "Чтение, сборка и заполнение PDF"}, {"pptx", "Создание и редактирование презентаций"}, {"xlsx", "Таблицы, формулы и анализ данных"},
	} {
		out = append(out, skillcatalog.Skill{Name: item.name, Description: item.desc, Dir: item.name, Repository: "anthropics/skills", Commit: "1780c5e5a552fd4d1c9d6a7b0cab26b0c2bef064", Files: []skillcatalog.File{{Path: "SKILL.md", Data: []byte("---\nname: " + item.name + "\nlicense: Complete terms in LICENSE.txt\n---\n\n# Frontend Design\n\nGuidance for distinctive, intentional\nvisual design when building new UI\nor reshaping an existing one.")}, {Path: "LICENSE.txt", Data: []byte("Example license text")}}})
	}
	return out
}

func catalogApp() *App {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	a.Home = home
	a.LoadCatalog = func(repo skillcatalog.Repository, done func([]skillcatalog.Skill, map[string]string, error)) func() {
		done(catalogFixture(), map[string]string{"claude": home + "/.claude/skills", "codex": home + "/.agents/skills"}, nil)
		return func() {}
	}
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	a.openSkillCatalog(0)
	return a
}

func TestSkillCatalogPage(t *testing.T) {
	a := catalogApp()
	tt := openAI(t, a, skillsPage)
	hasTexts(t, tt, "Skill catalog", "frontend-design", "SKILL.md", "Install for", "Next: plan…")
	if err := tt.Click("LICENSE.txt"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	hasTexts(t, tt, "Example license text")
	a.skillCatalog.query = "no-such-skill"
	tt.Frame()
	hasTexts(t, tt, "Nothing matches")
	a.skillCatalog.query = ""
	a.skillCatalog.file = ""
	tt.Frame()
	a.Locale = "ru"
	i18n.SetLocaleFunc(func() string { return "ru" })
	defer i18n.SetLocaleFunc(nil)
	tt = ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "skill-catalog-ru")
	tt = ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "skill-catalog-narrow")
}

func TestSkillCatalogPropose(t *testing.T) {
	dir := t.TempDir()
	a := catalogApp()
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + dir, "USERPROFILE=" + dir}, nil))
	}
	tt := openAI(t, a, skillsPage)
	if err := tt.Click("Next: plan…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.started || a.op.plan.Steps[0].Skill == nil {
		t.Fatalf("no pending install: %+v", a.op)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "frontend-design")); !os.IsNotExist(err) {
		t.Fatal("preview installed files")
	}
	hasTexts(t, tt, "Repository and version", "Instructions and scripts", "LICENSE.txt")
}

func TestSkillCatalogLoading(t *testing.T) {
	a := NewApp()
	a.Snapshot = aiSnapshot()
	var callbacks []func([]skillcatalog.Skill, map[string]string, error)
	cancelled := 0
	a.LoadCatalog = func(repo skillcatalog.Repository, done func([]skillcatalog.Skill, map[string]string, error)) func() {
		callbacks = append(callbacks, done)
		return func() { cancelled++ }
	}
	tt := openAI(t, a, skillsPage)
	if err := tt.Click("Skill catalog"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(callbacks) != 1 || !a.skillCatalog.loading {
		t.Fatal("catalog did not start on click")
	}
	a.openSkillCatalog(1)
	callbacks[0](catalogFixture(), nil, nil)
	if !a.skillCatalog.loading || cancelled != 1 {
		t.Fatal("stale response was applied")
	}
	callbacks[1](nil, nil, errors.New("HTTP 429"))
	tt.Frame()
	hasTexts(t, tt, "Could not load the catalog", "HTTP 429")
}

func TestSkillCatalogSelections(t *testing.T) {
	a := catalogApp()
	tt := openAI(t, a, skillsPage)
	if err := tt.Click("Vercel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.skillCatalog.repo != 2 {
		t.Fatal("repository selection was lost")
	}
	if err := tt.Click("Install for"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.skillCatalog.agent != "codex" {
		t.Fatalf("agent selection was lost: %s", a.skillCatalog.agent)
	}
	hasTexts(t, tt, "~/.agents/skills/frontend-design")
}
