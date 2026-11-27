package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
)

func javaSnapshot() *inventory.Snapshot {
	mise := home + "/.local/share/mise"
	copy := func(version, manager, root, vendor string) inventory.Installation {
		return inventory.Installation{Tool: "java", Version: version, Path: root + "/bin/java", RealPath: root + "/bin/java", Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: manager, Root: root}, Java: &inventory.JDK{Home: root, Vendor: vendor}}
	}
	first := copy("21.0.5", "mise", mise+"/installs/java/temurin-21.0.5", "Eclipse Adoptium")
	first.Default = true
	old := copy("17.0.13", "brew", "/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home", "Homebrew")
	old.Source.Kind = inventory.KindPackageManager
	return &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "java", Installations: []inventory.Installation{
		{Tool: "java", Path: mise + "/shims/java", OnPath: 1, InGUI: 1, Source: inventory.Source{Manager: "mise", Shim: true}},
		first, copy("17.0.13", "mise", mise+"/installs/java/temurin-17.0.13", "Eclipse Adoptium"), old,
	}, Managers: []inventory.Manager{{Name: "mise", SwitchOnCD: true, Files: []string{"mise.toml", ".tool-versions"}, Shims: true, ShimsInGUI: true}}, Vars: map[string]string{"JAVA_HOME": old.Java.Home, "gradle": "/opt/homebrew/bin/gradle", "mvn": "/opt/homebrew/bin/mvn"}}}}
}

func openJava(a *App) *ui.Tester {
	a.Home = home
	a.router.Push("/tools/java")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	return tt
}

func TestJavaPage(t *testing.T) {
	a := NewApp()
	a.Snapshot = javaSnapshot()
	a.JavaVersions = &versions.Java{Lines: []versions.Cycle{{Cycle: "21", Latest: "21.0.6+7"}, {Cycle: "23", Latest: "23.0.2+7"}, {Cycle: "17", Latest: "17.0.14+7"}}}
	tt := openJava(a)
	screenshot(t, tt, "java")
	for _, text := range []string{"3 JDKs", "21.0.5", "17.0.13", "this is JAVA_HOME", "Version manager · mise", "How the terminal finds java", "Temurin 21.0.6+7", "Build tools", "gradle", "java and JAVA_HOME point to different JDKs"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("PATH"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("mvn") || !tt.HasText("JAVA_HOME") {
		t.Fatalf("PATH texts %q", tt.Texts())
	}
}

func TestJavaIndependentBuilders(t *testing.T) {
	orig := goos
	goos = "darwin"
	t.Cleanup(func() { goos = orig })

	a := NewApp()
	a.Shell = &fakeShell{}
	a.Snapshot = javaSnapshot()
	a.Snapshot.Tools[0].Managers[0].Activated = true
	a.Snapshot.Tools[0].Managers[0].Version = "2025.1.6"
	delete(a.Snapshot.Tools[0].Vars, "mvn")
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}, nil))
	}
	tt := openJava(a)
	screenshot(t, tt, "java-builders")
	hasTexts(t, tt, "gradle", "/opt/homebrew/bin/gradle", "Maven", "Install Maven")
	if err := tt.Click("Install Maven"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("mise use --global maven") {
		t.Fatalf("expected maven install plan: %q", tt.Texts())
	}
}

func TestJavaEmptyAndStub(t *testing.T) {
	for _, stub := range []bool{false, true} {
		a := NewApp()
		st := inventory.ToolState{Tool: "java"}
		if stub {
			st.Installations = []inventory.Installation{{Tool: "java", Path: "/usr/bin/java", OnPath: 1, Stub: true}}
		}
		a.Snapshot = &inventory.Snapshot{Tools: []inventory.ToolState{st}}
		tt := openJava(a)
		if stub {
			// A stub is no copy: nothing says where Java came from.
			if !tt.HasText("no JDK was found") || !tt.HasText("No version manager. Hearth will install Java through mise.") ||
				tt.HasText("Installed from ") || tt.HasText("Node") || a.version("java") != "" {
				t.Fatalf("texts %q", tt.Texts())
			}
		} else if !tt.HasText("Java is not installed") {
			t.Fatalf("texts %q", tt.Texts())
		}
	}
}

func TestJavaPatchUpdateShowsInstallButton(t *testing.T) {
	a := NewApp()
	a.Snapshot = javaSnapshot()
	// Installed default is 21.0.5; newer patch 21.0.13.1+2 should offer an install button
	// and compare numerically (13 > 5, not lexicographically).
	a.JavaVersions = &versions.Java{Lines: []versions.Cycle{
		{Cycle: "21", Latest: "21.0.13.1+2"},
		{Cycle: "17", Latest: "17.0.13+11"},
	}}
	tt := openJava(a)
	if !tt.HasText("Temurin 21.0.13.1+2") || !tt.HasText("JDK 21 · update for 21.0.5") {
		t.Fatalf("missing patch update row: %q", tt.Texts())
	}
	if !tt.HasText("Install Temurin 21.0.13.1+2") {
		t.Fatalf("expected patch update install button for 21.0.13.1+2: %q", tt.Texts())
	}
	if tt.HasText("Install Temurin 17.0.13+11") {
		t.Fatalf("did not expect install button for already-installed 17.0.13: %q", tt.Texts())
	}
	if compareJavaVersion("21.0.13.1+2", "21.0.5") <= 0 {
		t.Fatalf("expected 21.0.13.1+2 > 21.0.5")
	}
	if compareJavaVersion("17.0.13+11", "17.0.13") != 0 {
		t.Fatalf("expected 17.0.13+11 == 17.0.13")
	}
}
