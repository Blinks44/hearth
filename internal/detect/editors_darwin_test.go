package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestEditorsDarwin(t *testing.T) {
	dir := root(t)
	saved := applications
	applications = filepath.Join(dir, "Applications")
	t.Cleanup(func() { applications = saved })
	home := filepath.Join(dir, "home")
	writeFile(t, filepath.Join(applications, "Visual Studio Code.app", "Contents", "Resources", "app", "package.json"), `{"name":"Code","version":"1.96.2"}`)
	bin := filepath.Join(dir, "bin")
	fakeExe(t, filepath.Join(bin, "code"))
	// Toolbox 2 puts the IDE in ~/Applications and its script in scripts.
	writeFile(t, filepath.Join(home, "Applications", "IntelliJ IDEA Ultimate.app", "Contents", "Resources", "product-info.json"),
		`{"name":"IntelliJ IDEA","version":"2024.3.1","productCode":"IU","dataDirectoryName":"IntelliJIdea2024.3","launch":[{"launcherPath":"../MacOS/idea"}]}`)
	writeFile(t, filepath.Join(applications, "Android Studio.app", "Contents", "Resources", "product-info.json"),
		`{"name":"Android Studio","productCode":"AI","launch":[{"launcherPath":"../MacOS/studio"}]}`)
	support := filepath.Join(home, "Library", "Application Support", "JetBrains")
	fakeExe(t, filepath.Join(support, "Toolbox", "scripts", "idea"))
	for _, plugin := range []string{"go", "rust", ".DS_Store"} {
		writeFile(t, filepath.Join(support, "IntelliJIdea2024.3", "plugins", plugin, "x"), "")
	}
	writeFile(t, filepath.Join(home, "Library", "Caches", "JetBrains", "IntelliJIdea2024.3", "index", "x"), "")
	env := sysenv.Env{"HOME=" + home, "PATH=" + bin}
	st, err := Editors(t.Context(), &run.Fake{}, env, env)
	if err != nil {
		t.Fatal(err)
	}
	eds := st.Editors.Editors
	if len(eds) != 2 {
		t.Fatalf("%+v", eds)
	}
	code, idea := eds[0], eds[1]
	if code.Version != "1.96.2" || code.CommandPath != filepath.Join(bin, "code") || code.Extensions != "" {
		t.Fatalf("%+v", code)
	}
	if idea.Name != "IntelliJ IDEA" || idea.Command != "idea" || !idea.Toolbox || idea.CommandPath != "" || len(idea.Installed) != 2 ||
		idea.Caches != filepath.Join(home, "Library", "Caches", "JetBrains", "IntelliJIdea2024.3") || idea.App != filepath.Join(home, "Applications", "IntelliJ IDEA Ultimate.app") {
		t.Fatalf("%+v", idea)
	}
}

// Homebrew's cask of IntelliJ IDEA CE links its command as idea-ce.
func TestEditorsCommunityCommand(t *testing.T) {
	dir := root(t)
	saved := applications
	applications = filepath.Join(dir, "Applications")
	t.Cleanup(func() { applications = saved })
	writeFile(t, filepath.Join(applications, "IntelliJ IDEA CE.app", "Contents", "Resources", "product-info.json"),
		`{"name":"IntelliJ IDEA CE","version":"2025.2.5","productCode":"IC","dataDirectoryName":"IdeaIC2025.2","launch":[{"launcherPath":"../MacOS/idea"}]}`)
	bin := filepath.Join(dir, "bin")
	fakeExe(t, filepath.Join(bin, "idea-ce"))
	env := sysenv.Env{"HOME=" + filepath.Join(dir, "home"), "PATH=" + bin}
	st, _ := Editors(t.Context(), &run.Fake{}, env, env)
	for _, e := range st.Editors.Editors {
		if e.ID == "IC" && (e.Command != "idea-ce" || e.CommandPath != filepath.Join(bin, "idea-ce")) {
			t.Fatalf("%+v", e)
		}
	}
}
