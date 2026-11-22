package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
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
	a.LoadCatalog = func(repo skillcatalog.Repository, _ bool, done func([]skillcatalog.Skill, map[string]string, error)) func() {
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
	a.LoadCatalog = func(repo skillcatalog.Repository, _ bool, done func([]skillcatalog.Skill, map[string]string, error)) func() {
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

func TestSkillCatalogCacheDefaultAgentURLAndProject(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "myapp")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	// Only Codex is installed (Claude is not).
	a.Snapshot = &inventory.Snapshot{
		Agents: []inventory.Agent{
			{ID: "claude", Dir: dir + "/.claude"},
			{ID: "codex", Dir: dir + "/.codex", Installations: []inventory.Installation{{Tool: "codex", Version: "1.0", Path: "/bin/codex"}}},
		},
	}
	a.Projects = []projects.Project{{Dir: proj}}
	a.Home = dir
	var loads []struct {
		slug  string
		force bool
	}
	a.LoadCatalog = func(repo skillcatalog.Repository, force bool, done func([]skillcatalog.Skill, map[string]string, error)) func() {
		loads = append(loads, struct {
			slug  string
			force bool
		}{repo.Slug, force})
		done(catalogFixture(), map[string]string{"claude": dir + "/.claude/skills", "codex": dir + "/.agents/skills"}, nil)
		return func() {}
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + dir, "USERPROFILE=" + dir}, nil))
	}
	a.openSkillCatalog(0)
	tt := openAI(t, a, skillsPage)
	// Default agent must be the installed one (codex), not claude.
	if a.skillCatalog.agent != "codex" {
		t.Fatalf("default agent = %q, want codex", a.skillCatalog.agent)
	}
	// Switching to Vercel and back to Anthropic reuses in-memory cache (2 loads total, not 3).
	if err := tt.Click("Vercel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Anthropic"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(loads) != 2 {
		t.Fatalf("loads = %+v, expected Anthropic to hit memory cache", loads)
	}
	// Refresh forces a reload.
	if err := tt.Click("Refresh"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(loads) != 3 || !loads[2].force {
		t.Fatalf("refresh loads = %+v, want force=true", loads)
	}
	// Load custom GitHub repository URL.
	a.skillCatalog.customURL = "https://github.com/acme/custom-skills"
	tt.Frame()
	if err := tt.Click("Load"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(loads) != 4 || loads[3].slug != "acme/custom-skills" {
		t.Fatalf("custom loads = %+v", loads)
	}
	hasTexts(t, tt, "acme/custom-skills")
	// Select project scope and propose plan into project.
	if err := tt.Click("Scope"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("myapp"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.skillCatalog.project != proj {
		t.Fatalf("project scope = %q, want %q", a.skillCatalog.project, proj)
	}
	if err := tt.Click("Next: plan…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Steps[0].Skill == nil || a.op.plan.Steps[0].Skill.Path != filepath.Join(proj, ".agents", "skills", "frontend-design") {
		t.Fatalf("unexpected project install plan: %+v", a.op)
	}
}

func TestSkillsRemoveBrokenAndDiff(t *testing.T) {
	dir := t.TempDir()
	skA := filepath.Join(dir, ".claude", "skills", "pdf")
	skB := filepath.Join(dir, ".agents", "skills", "pdf")
	for _, p := range []string{skA, skB} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(skA, "SKILL.md"), []byte("---\nname: pdf\n---\nold\n"), 0o644)
	_ = os.WriteFile(filepath.Join(skB, "SKILL.md"), []byte("---\nname: pdf\n---\nnew\n"), 0o644)
	brokenLink := filepath.Join(dir, ".claude", "skills", "gone")
	_ = os.Symlink(filepath.Join(dir, "missing"), brokenLink)

	a := NewApp()
	a.Home = dir
	a.Snapshot = &inventory.Snapshot{
		Agents: []inventory.Agent{
			{ID: "claude", Dir: dir + "/.claude", Installations: []inventory.Installation{{Tool: "claude", Version: "1.0", Path: "/bin/claude"}}},
			{ID: "codex", Dir: dir + "/.codex", Installations: []inventory.Installation{{Tool: "codex", Version: "1.0", Path: "/bin/codex"}}},
		},
		Skills: []inventory.Skill{
			{Name: "pdf", Description: "Read PDFs", Dir: skA, Sum: "sum-a", Paths: []inventory.SkillPath{{Path: skA, Agents: []string{"claude"}}}},
			{Name: "pdf", Description: "Read PDFs", Dir: skB, Sum: "sum-b", Paths: []inventory.SkillPath{{Path: skB, Agents: []string{"codex"}}}},
			{Name: "gone", Dir: filepath.Join(dir, "missing"), Broken: true, Paths: []inventory.SkillPath{{Path: brokenLink, Link: true, Agents: []string{"claude"}}}},
		},
	}
	a.DiffSkills = func(dirA, dirB string, done func([]ai.SkillFileDiff, error)) {
		diffs, err := ai.SkillDiff(dirA, dirB)
		done(diffs, err)
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + dir, "USERPROFILE=" + dir}, nil))
	}

	tt := openAI(t, a, skillsPage)
	// Alphabetical order puts "gone" first, then "pdf".
	hasTexts(t, tt, "differs", "Remove broken link…")
	if err := tt.Click("Remove broken link…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Remove broken link gone" {
		t.Fatalf("unexpected broken link plan: %+v", a.op)
	}
	a.op = nil
	tt.Frame()

	// Select differing skill "pdf".
	if err := tt.Click("pdf"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	hasTexts(t, tt, "Copies of this skill have different contents.", "Compare copies…", "Remove…")

	// Open copy diff dialog.
	if err := tt.Click("Compare copies…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	hasTexts(t, tt, "Differences in pdf", "SKILL.md", "modified")
	if err := tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	// Propose removing managed skill.
	if err := tt.Click("Remove…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Remove skill pdf" {
		t.Fatalf("unexpected remove plan: %+v", a.op)
	}
}
