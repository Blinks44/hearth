package projects

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestReadAdditionalConfigs(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		want    []Requirement
		wantErr string
	}{
		{
			name:  "go-version plain",
			files: map[string]string{".go-version": "# pin\n1.24.2\n"},
			want:  []Requirement{{Tool: "go", Want: "1.24.2", File: ".go-version"}},
		},
		{
			name:  "go-version with go prefix",
			files: map[string]string{".go-version": "go1.23.5\n"},
			want:  []Requirement{{Tool: "go", Want: "1.23.5", File: ".go-version"}},
		},
		{
			name:  "php-version plain",
			files: map[string]string{".php-version": "8.4.3\n"},
			want:  []Requirement{{Tool: "php", Want: "8.4.3", File: ".php-version"}},
		},
		{
			name:  "php-version with prefix",
			files: map[string]string{".php-version": "php-8.3.12\n"},
			want:  []Requirement{{Tool: "php", Want: "8.3.12", File: ".php-version"}},
		},
		{
			name:  "Pipfile full version beats minor",
			files: map[string]string{"Pipfile": "[requires]\npython_version = \"3.12\"\npython_full_version = \"3.12.7\"\n"},
			want:  []Requirement{{Tool: "python", Want: "3.12.7", File: "Pipfile", Key: "requires.python_full_version"}},
		},
		{
			name:  "Pipfile minor version",
			files: map[string]string{"Pipfile": "[requires]\npython_version = \"3.11\"\n"},
			want:  []Requirement{{Tool: "python", Want: "3.11", File: "Pipfile", Key: "requires.python_version"}},
		},
		{
			name:  "Pipfile without requires",
			files: map[string]string{"Pipfile": "[packages]\nrequests = \"*\"\n"},
			want:  []Requirement{{Tool: "python", File: "Pipfile"}},
		},
		{
			name:  "poetry.lock metadata python-versions",
			files: map[string]string{"poetry.lock": "[metadata]\nlock-version = \"2.0\"\npython-versions = \"^3.11\"\n"},
			want:  []Requirement{{Tool: "python", Want: "^3.11", File: "poetry.lock", Key: "metadata.python-versions"}},
		},
		{
			name:  "pyproject poetry fallback",
			files: map[string]string{"pyproject.toml": "[tool.poetry.dependencies]\npython = \"^3.12\"\n"},
			want:  []Requirement{{Tool: "python", Want: "^3.12", File: "pyproject.toml", Key: "tool.poetry.dependencies.python"}},
		},
		{
			name:  "deno.json",
			files: map[string]string{"deno.json": `{"tasks":{"dev":"deno run main.ts"}}`},
			want:  []Requirement{{Tool: "deno", File: "deno.json"}},
		},
		{
			name:  "deno.jsonc with comments and trailing comma",
			files: map[string]string{"deno.jsonc": "{\n  // task\n  \"tasks\": {\n    \"dev\": \"deno run main.ts\",\n  },\n}\n"},
			want:  []Requirement{{Tool: "deno", File: "deno.jsonc"}},
		},
		{
			name:    "deno.jsonc broken",
			files:   map[string]string{"deno.jsonc": "{broken"},
			wantErr: "read deno.jsonc",
		},
		{
			name: "pom.xml release priority",
			files: map[string]string{"pom.xml": `<project><properties>
				<maven.compiler.source>17</maven.compiler.source>
				<maven.compiler.release>21</maven.compiler.release>
			</properties></project>`},
			want: []Requirement{{Tool: "java", Want: "21", File: "pom.xml", Key: "properties.maven.compiler.release"}},
		},
		{
			name:  "pom.xml 1.8 normalized and placeholder skipped",
			files: map[string]string{"pom.xml": `<project><properties><java.version>${jdk.ver}</java.version><maven.compiler.source>1.8</maven.compiler.source></properties></project>`},
			want:  []Requirement{{Tool: "java", Want: "8", File: "pom.xml", Key: "properties.maven.compiler.source"}},
		},
		{
			name:  "pom.xml without java property",
			files: map[string]string{"pom.xml": `<project><modelVersion>4.0.0</modelVersion></project>`},
			want:  []Requirement{{Tool: "java", File: "pom.xml"}},
		},
		{
			name: "build.gradle toolchain",
			files: map[string]string{"build.gradle": `
// JavaLanguageVersion.of(11)
java {
    toolchain {
        languageVersion = JavaLanguageVersion.of(21)
    }
}`},
			want: []Requirement{{Tool: "java", Want: "21", File: "build.gradle", Key: "java.toolchain"}},
		},
		{
			name:  "build.gradle.kts jvmToolchain",
			files: map[string]string{"build.gradle.kts": "kotlin {\n    jvmToolchain(17)\n}\n"},
			want:  []Requirement{{Tool: "java", Want: "17", File: "build.gradle.kts", Key: "jvmToolchain"}},
		},
		{
			name:  "build.gradle sourceCompatibility VERSION_1_8",
			files: map[string]string{"build.gradle": "sourceCompatibility = JavaVersion.VERSION_1_8\n"},
			want:  []Requirement{{Tool: "java", Want: "8", File: "build.gradle", Key: "sourceCompatibility"}},
		},
		{
			name:  "build.gradle dynamic fallback",
			files: map[string]string{"build.gradle": "plugins { id 'java' }\n"},
			want:  []Requirement{{Tool: "java", File: "build.gradle"}},
		},
		{
			name:  "csproj single target framework with OS suffix",
			files: map[string]string{"App.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net8.0-windows10.0.19041.0</TargetFramework></PropertyGroup></Project>`},
			want:  []Requirement{{Tool: "dotnet", Want: ">=8.0", File: "App.csproj", Key: "TargetFramework"}},
		},
		{
			name:  "csproj multi-targeting picks highest modern net TFM",
			files: map[string]string{"Lib.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFrameworks>netstandard2.0;net8.0;net9.0;net48</TargetFrameworks></PropertyGroup></Project>`},
			want:  []Requirement{{Tool: "dotnet", Want: ">=9.0", File: "Lib.csproj", Key: "TargetFrameworks"}},
		},
		{
			name:  "csproj without TFM and without global.json still needs dotnet",
			files: map[string]string{"App.csproj": `<Project Sdk="Microsoft.NET.Sdk" />`},
			want:  []Requirement{{Tool: "dotnet", File: "App.csproj"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			for name, body := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p := Read(dir)
			if tt.wantErr != "" {
				if len(p.Errs) == 0 || !strings.Contains(p.Errs[0].Error(), tt.wantErr) {
					t.Fatalf("errs %v, want %q", p.Errs, tt.wantErr)
				}
				return
			}
			if len(p.Errs) > 0 {
				t.Fatalf("unexpected errs: %v", p.Errs)
			}
			if !reflect.DeepEqual(p.Requirements, tt.want) {
				t.Fatalf("requirements\n got %+v\nwant %+v", p.Requirements, tt.want)
			}
		})
	}
}

func TestReadCsprojDiscoveredByFind(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "api")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "Api.csproj"), []byte(`<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, nested := Find(root)
	if len(nested) != 1 || nested[0].Dir != sub {
		t.Fatalf("nested %+v", nested)
	}
	snap := &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "dotnet",
		Installations: []inventory.Installation{{
			Tool: "dotnet", OnPath: 1,
			Dotnet: &inventory.Dotnet{SDKs: []inventory.DotnetEntry{{Version: "9.0.102"}}},
		}},
	}}}
	st := Resolve(nested[0], snap, nil)
	if len(st) != 1 || !st[0].Checked || !st[0].Checks[0].Met {
		t.Fatalf("resolved %+v", st)
	}
}

func TestMonorepoVersionInheritance(t *testing.T) {
	outer := t.TempDir()
	// Stray config above the git root must never leak into the repository.
	if err := os.WriteFile(filepath.Join(outer, ".nvmrc"), []byte("16\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(outer, "mono")
	for _, rel := range []string{".git", "apps/docs", "apps/web", "services/api", "empty"} {
		if err := os.MkdirAll(filepath.Join(repo, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".nvmrc", "22\n")
	write("mise.toml", "[tools]\ngo = \"1.24.2\"\npython = \"3.12\"\n")
	write("apps/docs/package.json", "{}\n")
	write("apps/web/package.json", "{}\n")
	write("apps/web/.nvmrc", "20\n")
	write("services/api/go.mod", "module example.com/api\n\ngo 1.23\n")

	t.Run("docs inherits root nvmrc without counting as own version", func(t *testing.T) {
		p := Read(filepath.Join(repo, "apps", "docs"))
		wantFile := filepath.FromSlash("../../.nvmrc")
		want := []Requirement{{Tool: "node", Want: "22", File: wantFile}}
		if !reflect.DeepEqual(p.Requirements, want) {
			t.Fatalf("reqs %+v, want %+v", p.Requirements, want)
		}
		if len(p.Versions()) != 0 {
			t.Fatalf("inherited pin leaked into Versions(): %+v", p.Versions())
		}
		// Resolve recognizes inherited ../../.nvmrc for managers that switch on cd.
		snap := &inventory.Snapshot{Tools: []inventory.ToolState{{
			Tool: "node",
			Installations: []inventory.Installation{
				{Tool: "node", Version: "20.18.1", OnPath: 1, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm"}},
				{Tool: "node", Version: "22.12.0", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "fnm"}},
			},
			Managers: []inventory.Manager{{Name: "fnm", SwitchOnCD: true, Files: []string{".nvmrc"}}},
		}}}
		st := Resolve(p, snap, nil)
		if len(st) != 1 || st[0].Terminal == nil || st[0].Terminal.Version != "22.12.0" || !st[0].Checks[0].Met {
			t.Fatalf("resolve inherited .nvmrc: %+v", st)
		}
	})

	t.Run("web local nvmrc overrides root", func(t *testing.T) {
		p := Read(filepath.Join(repo, "apps", "web"))
		want := []Requirement{{Tool: "node", Want: "20", File: ".nvmrc"}}
		if !reflect.DeepEqual(p.Requirements, want) {
			t.Fatalf("reqs %+v, want %+v", p.Requirements, want)
		}
		if v := p.Versions(); len(v) != 1 || v[0].Want != "20" {
			t.Fatalf("Versions() %+v", v)
		}
	})

	t.Run("polyglot Go service inherits only go from root mise.toml", func(t *testing.T) {
		p := Read(filepath.Join(repo, "services", "api"))
		want := []Requirement{
			{Tool: "go", Want: "1.23", File: "go.mod", Key: "go"},
			{Tool: "go", Want: "1.24.2", File: filepath.FromSlash("../../mise.toml"), Key: "tools.go"},
		}
		if !reflect.DeepEqual(p.Requirements, want) {
			t.Fatalf("reqs %+v, want %+v", p.Requirements, want)
		}
	})

	t.Run("empty folder never inherits", func(t *testing.T) {
		if p := Read(filepath.Join(repo, "empty")); len(p.Files) != 0 || len(p.Requirements) != 0 {
			t.Fatalf("empty folder inherited: %+v", p)
		}
	})

	t.Run("git root never walks into parent directory", func(t *testing.T) {
		other := filepath.Join(outer, "standalone")
		if err := os.MkdirAll(filepath.Join(other, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(other, "package.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if p := Read(other); len(p.Requirements) != 0 {
			t.Fatalf("git root inherited from outer dir: %+v", p.Requirements)
		}
	})
}
