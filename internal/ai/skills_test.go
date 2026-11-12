package ai

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// tree makes the files under root, each "path": "contents", and the
// symlinks, each "path": "target"; a test without symlinks on this OS is
// skipped.
func tree(t *testing.T, root string, files, links map[string]string) {
	t.Helper()
	for rel, data := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for rel, target := range links {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.FromSlash(target), p); err != nil {
			t.Skipf("no symlinks: %v", err)
		}
	}
}

func skillMD(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
}

// Readers of each skills directory, in Hearth's order.
var (
	claudeDir = []string{"claude", "cursor", "opencode", "grok"}
	sharedDir = []string{"codex", "copilot", "gemini", "cursor", "opencode", "grok", "kimi"}
)

// skill is what a test checks of a found skill; dir and paths are relative
// to the home.
type skill struct {
	name, description, dir string
	paths                  []inventory.SkillPath
	broken, err, synced    bool
}

func TestSkills(t *testing.T) {
	plan := skillMD("plan", "Plan work")
	tests := []struct {
		name         string
		files, links map[string]string
		want         []skill
	}{
		{"shared directory", map[string]string{".agents/skills/plan/SKILL.md": plan}, nil, []skill{
			{name: "plan", description: "Plan work", dir: ".agents/skills/plan",
				paths: []inventory.SkillPath{{Path: ".agents/skills/plan", Agents: sharedDir}}},
		}},
		// A skill kept in its own repository, linked for Codex and the
		// others reading ~/.agents/skills; Claude Code does not.
		{"shared directory linking a folder elsewhere", map[string]string{"dev/skills/plan/SKILL.md": plan},
			map[string]string{".agents/skills/plan": "../../dev/skills/plan"}, []skill{
				{name: "plan", description: "Plan work", dir: "dev/skills/plan",
					paths: []inventory.SkillPath{{Path: ".agents/skills/plan", Link: true, Agents: sharedDir}}},
			}},
		{"symlink from .claude/skills", map[string]string{".agents/skills/plan/SKILL.md": plan},
			map[string]string{".claude/skills/plan": "../../.agents/skills/plan"}, []skill{
				{name: "plan", description: "Plan work", dir: ".agents/skills/plan", paths: []inventory.SkillPath{
					{Path: ".claude/skills/plan", Link: true, Agents: claudeDir},
					{Path: ".agents/skills/plan", Agents: sharedDir},
				}},
			}},
		{"symlinked skills directory", map[string]string{"dotfiles/skills/plan/SKILL.md": plan},
			map[string]string{".claude/skills": "../dotfiles/skills"}, []skill{
				{name: "plan", description: "Plan work", dir: "dotfiles/skills/plan",
					paths: []inventory.SkillPath{{Path: ".claude/skills/plan", Link: true, Agents: claudeDir}}},
			}},
		{"broken symlink", nil, map[string]string{".claude/skills/gone": "../../skills/gone"}, []skill{
			{name: "gone", dir: "skills/gone", broken: true,
				paths: []inventory.SkillPath{{Path: ".claude/skills/gone", Link: true, Agents: claudeDir}}},
		}},
		{"copies with different content", map[string]string{
			".claude/skills/pdf/SKILL.md": skillMD("pdf", "Read PDFs"),
			".agents/skills/pdf/SKILL.md": skillMD("pdf", "Read and fill PDFs"),
		}, nil, []skill{
			{name: "pdf", description: "Read PDFs", dir: ".claude/skills/pdf",
				paths: []inventory.SkillPath{{Path: ".claude/skills/pdf", Agents: claudeDir}}},
			{name: "pdf", description: "Read and fill PDFs", dir: ".agents/skills/pdf",
				paths: []inventory.SkillPath{{Path: ".agents/skills/pdf", Agents: sharedDir}}},
		}},
		{"nested, hidden and not skills", map[string]string{
			".claude/skills/synced/acct/morning/SKILL.md": skillMD("morning", "Brief"),
			".claude/skills/.cache/x/SKILL.md":            plan,
			".claude/skills/notes/README.md":              "no skill",
			".claude/skills/README.md":                    "no skill",
		}, nil, []skill{
			{name: "morning", description: "Brief", dir: ".claude/skills/synced/acct/morning", synced: true,
				paths: []inventory.SkillPath{{Path: ".claude/skills/synced/acct/morning", Agents: claudeDir}}},
		}},
		// Claude keeps each organization's synced skills apart and reads
		// only the signed-in one's: the other folder is no copy.
		{"synced for two organizations", map[string]string{
			".claude.json": `{"oauthAccount":{"organizationUuid":"org-b"}}`,
			".claude/skills/synced/org-a_1/morning/SKILL.md": skillMD("morning", "Old brief"),
			".claude/skills/synced/org-b_2/morning/SKILL.md": skillMD("morning", "Brief"),
		}, nil, []skill{
			{name: "morning", description: "Brief", dir: ".claude/skills/synced/org-b_2/morning", synced: true,
				paths: []inventory.SkillPath{{Path: ".claude/skills/synced/org-b_2/morning", Agents: claudeDir}}},
		}},
		{"frontmatter without name, and none", map[string]string{
			".codex/skills/fold/SKILL.md": "---\ndescription: >\n  Folded\n  text\n---\n",
			".codex/skills/bare/SKILL.md": "# No frontmatter\n",
		}, nil, []skill{
			{name: "bare", dir: ".codex/skills/bare", err: true,
				paths: []inventory.SkillPath{{Path: ".codex/skills/bare", Agents: []string{"cursor"}}}},
			{name: "fold", description: "Folded text\n", dir: ".codex/skills/fold",
				paths: []inventory.SkillPath{{Path: ".codex/skills/fold", Agents: []string{"cursor"}}}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			tree(t, home, tt.files, tt.links)
			env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home}
			if got, want := summary(t, Skills(env), home), tt.want; !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}
}

// summary is what the tests check of skills, paths relative to root.
func summary(t *testing.T, skills []inventory.Skill, root string) []skill {
	t.Helper()
	rel := func(p string) string {
		r, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(r)
	}
	var out []skill
	for _, s := range skills {
		k := skill{name: s.Name, description: s.Description, dir: rel(s.Dir), broken: s.Broken, err: s.Err != "", synced: s.Synced}
		for _, p := range s.Paths {
			p.Path = rel(p.Path)
			k.paths = append(k.paths, p)
		}
		if !s.Broken && s.Sum == "" {
			t.Errorf("%s: no sum", s.Name)
		}
		out = append(out, k)
	}
	return out
}

// A project's skills, linked from .claude/skills to .agents/skills as many
// repositories do, are one skill per folder, with the project set.
func TestProjectSkills(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tree(t, dir, map[string]string{".agents/skills/plan/SKILL.md": skillMD("plan", "Plan work")},
		map[string]string{".claude/skills/plan": "../../.agents/skills/plan"})
	got := ProjectSkills(dir)
	want := []skill{{name: "plan", description: "Plan work", dir: ".agents/skills/plan", paths: []inventory.SkillPath{
		{Path: ".claude/skills/plan", Link: true, Agents: []string{"claude", "copilot", "cursor", "opencode", "grok"}},
		{Path: ".agents/skills/plan", Agents: []string{"codex", "copilot", "gemini", "cursor", "opencode", "devin", "grok", "kimi"}},
	}}}
	if s := summary(t, got, dir); !reflect.DeepEqual(s, want) {
		t.Errorf("got  %+v\nwant %+v", s, want)
	}
	if len(got) == 1 && got[0].Project != dir {
		t.Errorf("project %q, want %q", got[0].Project, dir)
	}
}

// Copies with the same files sum the same, whatever their folder and its
// hidden files; a changed file changes the sum.
func TestSum(t *testing.T) {
	root := t.TempDir()
	tree(t, root, map[string]string{
		"a/SKILL.md": "x", "a/ref/b.md": "y",
		"b/SKILL.md": "x", "b/ref/b.md": "y", "b/.DS_Store": "junk",
		"c/SKILL.md": "x", "c/ref/b.md": "z",
	}, nil)
	a, b, c := sum(filepath.Join(root, "a")), sum(filepath.Join(root, "b")), sum(filepath.Join(root, "c"))
	if a == "" || a != b || a == c {
		t.Errorf("a %q, b %q, c %q", a, b, c)
	}
}

func TestReadFrontmatter(t *testing.T) {
	tests := []struct {
		in, name, description string
		err                   bool
	}{
		{in: "---\nname: pdf\ndescription: Read PDFs\n---\n# pdf", name: "pdf", description: "Read PDFs"},
		{in: "\ufeff---\r\nname: pdf\r\ndescription: \"Quoted: `x` and \\\"y\\\"\"\r\n---\r\n", name: "pdf", description: "Quoted: `x` and \"y\""},
		{in: "---\nname: go\ndescription: Go\nmetadata:\n  version: \"1.2\"\n  requires:\n    - go\n---\n", name: "go", description: "Go"},
		{in: "# pdf\n", err: true},
		{in: "---\nname: pdf\n", err: true},
		{in: "---\nname: [pdf\n---\n", err: true},
	}
	for _, tt := range tests {
		fm, err := readFrontmatter([]byte(tt.in))
		if (err != nil) != tt.err || fm.Name != tt.name || fm.Description != tt.description {
			t.Errorf("%q: %+v, %v", tt.in, fm, err)
		}
	}
}
