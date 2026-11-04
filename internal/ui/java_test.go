package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
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
			if !tt.HasText("no JDK was found") {
				t.Fatalf("texts %q", tt.Texts())
			}
		} else if !tt.HasText("Java is not installed") {
			t.Fatalf("texts %q", tt.Texts())
		}
	}
}
