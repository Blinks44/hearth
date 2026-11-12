package ui

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
)

// VS Code with its command, Cursor without one and extensions shared
// with VS Code, IntelliJ IDEA from Toolbox with its plugins and caches;
// old extensions and the caches lead to Cleanup.
func TestEditorsPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	const gb = 1_000_000_000
	ids := func(prefix string, n int) []string {
		var out []string
		for i := range n {
			out = append(out, fmt.Sprintf("%s.ext%d", prefix, i))
		}
		return out
	}
	vscodeExt := ids("x", 48)
	cursorExt := append(ids("x", 12), ids("c", 19)...)
	var old []string
	for i := range 14 {
		old = append(old, fmt.Sprintf("%s/.vscode/extensions/x.ext%d-0.%d.0", home, i, i))
	}
	idea := home + "/Applications/IntelliJ IDEA Ultimate.app"
	caches := home + "/Library/Caches/JetBrains/IntelliJIdea2024.3"
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{Tool: "editors", Editors: &inventory.EditorsState{Editors: []inventory.Editor{
		{ID: "vscode", Name: "VS Code", Version: "1.96.2", App: "/Applications/Visual Studio Code.app", Command: "code", CommandPath: "/usr/local/bin/code",
			CommandReal: "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code", Extensions: home + "/.vscode/extensions", Installed: vscodeExt, Superseded: old},
		{ID: "cursor", Name: "Cursor", Version: "0.44.11", App: "/Applications/Cursor.app", Command: "cursor", Extensions: home + "/.cursor/extensions", Installed: cursorExt},
		{ID: "IU", Name: "IntelliJ IDEA", Version: "2024.3", App: idea, Toolbox: true, Command: "idea", CommandPath: home + "/Library/Application Support/JetBrains/Toolbox/scripts/idea",
			Extensions: home + "/Library/Application Support/JetBrains/IntelliJIdea2024.3/plugins", Installed: ids("p", 13), Config: "IntelliJIdea2024.3", Caches: caches},
	}}}}}
	paths := map[string]inventory.Usage{
		home + "/.vscode/extensions": {Bytes: 1.4 * gb},
		home + "/.cursor/extensions": {Bytes: 1.1 * gb},
		home + "/Library/Application Support/JetBrains/IntelliJIdea2024.3/plugins": {Bytes: 0.4 * gb},
		caches: {Bytes: 1.0 * gb},
	}
	for _, p := range old {
		paths[p] = inventory.Usage{Bytes: 15_000_000}
	}
	a.Sizes = &inventory.Sizes{Taken: time.Now(), Caches: []inventory.Cache{{ID: "jetbrains", Path: caches}}, Paths: paths}
	a.router.Push(editorsPage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "editors")
	hasTexts(t, tt, "Editors", "Tools · editors and IDEs", "VS CODE 1.96.2", "Command code", "on PATH",
		"48 extensions", "~/.vscode/extensions", "1.4 GB", "Old versions of extensions", "left after updates · 14 folders", "210 MB", "210 MB · can be removed",
		"CURSOR 0.44.11", "Command cursor", "not on the PATH · installed from Cursor itself", "no", "31 extensions", "~/.cursor/extensions · 12 also in VS Code",
		"INTELLIJ IDEA 2024.3 · TOOLBOX", "Command idea", "Toolbox script · on the PATH", "13 plugins", "settings IntelliJIdea2024.3",
		"Index cache", "2 of 3", "code, idea", "92 extensions",
		"Hints", "Cursor without a command on the PATH", "cursor won't work in the terminal", "Open in Cleanup", "What Hearth leaves alone")
	// The fixture's paths are macOS ones: on Windows they show as they are.
	if filepath.Separator == '/' {
		hasTexts(t, tt, "/usr/local/bin/code → Visual Studio Code.app", "~/Library/Caches/JetBrains · rebuilt by itself")
	}
	a.editorsTab = 2 // commands
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("48 extensions") || !tt.HasText("Command cursor") {
		t.Errorf("commands tab: %q", tt.Texts())
	}
	if err := tt.Click("Open in Cleanup"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != cleanupPage {
		t.Fatal(a.router.Path())
	}
	for _, text := range []string{"Old extensions · VS Code", "JetBrains cache"} {
		if !tt.HasText(text) {
			t.Errorf("no %q in Cleanup: %q", text, tt.Texts())
		}
	}
}

// Russian names fit the narrow window.
func TestEditorsPageRussian(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{Tool: "editors", Editors: &inventory.EditorsState{Editors: []inventory.Editor{
		{ID: "cursor", Name: "Cursor", Version: "0.44.11", App: "/Applications/Cursor.app", Command: "cursor", Extensions: home + "/.cursor/extensions", Installed: []string{"a", "b", "c"}},
	}}}}}
	a.router.Push(editorsPage)
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "editors-ru")
	hasTexts(t, tt, "Редакторы", "Команда cursor", "не в PATH · ставится из самого Cursor", "3 расширения", "Cursor без команды в PATH")
	if tt.HasText("0 КБ") {
		t.Error("a size before the count")
	}
}
