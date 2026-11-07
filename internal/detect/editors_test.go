package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// extensions.json lists what is installed; a folder it does not list is
// an older version an update left.
func TestCodeExtensions(t *testing.T) {
	dir := filepath.Join(root(t), "extensions")
	writeFile(t, filepath.Join(dir, "extensions.json"), `[{"identifier":{"id":"golang.Go"},"version":"0.46.1","relativeLocation":"golang.go-0.46.1"},{"identifier":{"id":"esbenp.prettier-vscode"},"relativeLocation":"esbenp.prettier-vscode-11.0.0"}]`)
	for _, name := range []string{"golang.go-0.46.1", "golang.go-0.45.0", "esbenp.prettier-vscode-11.0.0", ".obsolete-dir"} {
		writeFile(t, filepath.Join(dir, name, "package.json"), "{}")
	}
	writeFile(t, filepath.Join(dir, ".obsolete"), "{}")
	installed, superseded := codeExtensions(dir)
	if len(installed) != 2 || installed[0] != "golang.go" || len(superseded) != 1 || filepath.Base(superseded[0]) != "golang.go-0.45.0" {
		t.Fatal(installed, superseded)
	}
	if installed, superseded := codeExtensions(filepath.Join(dir, "none")); installed != nil || superseded != nil {
		t.Fatal(installed, superseded)
	}
}

// Extensions left without the app still show the editor: what remains
// of it takes space.
func TestEditorsLeftExtensions(t *testing.T) {
	home := root(t)
	writeFile(t, filepath.Join(home, ".cursor", "extensions", "extensions.json"), `[{"identifier":{"id":"golang.go"},"relativeLocation":"golang.go-0.46.1"}]`)
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + filepath.Join(home, "bin")}
	r := &run.Fake{}
	st, err := Editors(t.Context(), r, env, env)
	if err != nil || len(r.Calls()) > 0 {
		t.Fatal(err, r.Calls())
	}
	var found bool
	for _, e := range st.Editors.Editors {
		if e.ID == "cursor" {
			found = e.CommandPath == "" && len(e.Installed) == 1
		}
		if e.ID == "vscode" && e.Extensions != "" {
			t.Fatal(e)
		}
	}
	if !found {
		t.Fatalf("%+v", st.Editors)
	}
}

func TestJetbrainsCommand(t *testing.T) {
	for _, tc := range []struct{ app, launcher, want string }{
		{"/Applications/GoLand.app", "../MacOS/goland", "goland"},
		{"/opt/idea", "bin/idea.sh", "idea"},
		{`C:\Program Files\JetBrains\IntelliJ IDEA`, "bin/idea64.exe", "idea"},
		{"/snap/intellij-idea-community/current", "bin/idea", "intellij-idea-community"},
	} {
		if got := jetbrainsCommand(tc.app, tc.launcher); got != tc.want {
			t.Errorf("%s %s: %q, want %q", tc.app, tc.launcher, got, tc.want)
		}
	}
}
