package ops

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

func TestBlock(t *testing.T) {
	const body = `export PATH="$HOME/.local/bin:$PATH"`
	block := BlockStart + "\n" + body + "\n" + BlockEnd + "\n"
	tests := []struct {
		name, text, body, want string
	}{
		{"into an empty file", "", body, block},
		{"after the user's lines", "alias ll='ls -l'\n", body, "alias ll='ls -l'\n\n" + block},
		{"after a last line without a break", "alias ll='ls -l'", body, "alias ll='ls -l'\n\n" + block},
		{"replaces the block where it is", "a\n" + BlockStart + "\nold\n" + BlockEnd + "\nb\n", body, "a\n" + block + "b\n"},
		{"removes the block it added", "a\n\n" + block, "", "a\n"},
		{"removes a block between lines", "a\n" + block + "b\n", "", "a\nb\n"},
		{"nothing to remove", "a\n", "", "a\n"},
		{"several lines", "", "one\ntwo\n", BlockStart + "\none\ntwo\n" + BlockEnd + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Block(tt.text, tt.body)
			if got != tt.want {
				t.Errorf("Block() =\n%q\nwant\n%q", got, tt.want)
			}
			if again := Block(got, tt.body); again != got {
				t.Errorf("applied again:\n%q\nwant it unchanged", again)
			}
		})
	}
}

func TestAddToBlock(t *testing.T) {
	activate, shims := `eval "$(mise activate zsh)"`, `eval "$(mise activate zsh --shims)"`
	tests := []struct {
		name, text, want string
	}{
		{"a new block", "alias ll='ls -l'\n", "alias ll='ls -l'\n\n" + BlockStart + "\n" + activate + "\n" + BlockEnd + "\n"},
		{"after what the block has", "a\n" + BlockStart + "\nexport X=1\n" + BlockEnd + "\nb\n",
			"a\n" + BlockStart + "\nexport X=1\n" + activate + "\n" + BlockEnd + "\nb\n"},
		{"a line it has stays once", BlockStart + "\n" + activate + "\n" + BlockEnd + "\n", BlockStart + "\n" + activate + "\n" + BlockEnd + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddToBlock(tt.text, activate)
			if got != tt.want {
				t.Errorf("AddToBlock() =\n%q\nwant\n%q", got, tt.want)
			}
			if again := AddToBlock(got, activate); again != got {
				t.Errorf("applied again:\n%q", again)
			}
		})
	}
	if got := AddToBlock("", activate, shims); got != BlockStart+"\n"+activate+"\n"+shims+"\n"+BlockEnd+"\n" {
		t.Errorf("two lines: %q", got)
	}
}

func TestCommentOut(t *testing.T) {
	text := "a\nexport PATH=/nowhere:$PATH\n# a comment\n" + BlockStart + "\nours\n" + BlockEnd + "\n"
	want := "a\n# hearth: no such folder · export PATH=/nowhere:$PATH\n# a comment\n" + BlockStart + "\nours\n" + BlockEnd + "\n"
	got := CommentOut(text, []int{2, 3, 5, 99}, "no such folder")
	if got != want {
		t.Errorf("CommentOut() =\n%q\nwant\n%q", got, want)
	}
	if again := CommentOut(got, []int{2}, "no such folder"); again != got {
		t.Errorf("applied again:\n%q", again)
	}
}

func TestDiff(t *testing.T) {
	before := "a\nb\nc\nd\n"
	after := "a\n# b\nc\nd\ne\n"
	want := []DiffLine{
		{Line: 2, Text: "b"},
		{Added: true, Line: 2, Text: "# b"},
		{Added: true, Line: 5, Text: "e"},
	}
	if got := Diff(before, after); !reflect.DeepEqual(got, want) {
		t.Errorf("Diff() = %+v, want %+v", got, want)
	}
	if got := Diff("a\n", "a\n"); len(got) != 0 {
		t.Errorf("same text differs: %+v", got)
	}
}

func TestSetJSON(t *testing.T) {
	tests := []struct {
		name, text, want string
	}{
		{"replaces the value where it is",
			"{\n  \"name\": \"app\",\n  \"packageManager\": \"pnpm@8.0.0\", \"private\": true\n}\n",
			"{\n  \"name\": \"app\",\n  \"packageManager\": \"pnpm@9.1.0\", \"private\": true\n}\n"},
		{"after the last key, indented as the others",
			"{\n\t\"name\": \"app\",\n\t\"scripts\": {\n\t\t\"dev\": \"vite\"\n\t}\n}\n",
			"{\n\t\"name\": \"app\",\n\t\"scripts\": {\n\t\t\"dev\": \"vite\"\n\t},\n\t\"packageManager\": \"pnpm@9.1.0\"\n}\n"},
		{"a key of a nested object is not the key", "{\"a\": {\"packageManager\": 1}}",
			"{\"a\": {\"packageManager\": 1}, \"packageManager\": \"pnpm@9.1.0\"}"},
		{"into an empty object", "{}\n", "{\n  \"packageManager\": \"pnpm@9.1.0\"\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SetJSON(tt.text, "packageManager", "pnpm@9.1.0")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("SetJSON() =\n%s\nwant\n%s", got, tt.want)
			}
			if again, err := SetJSON(got, "packageManager", "pnpm@9.1.0"); err != nil || again != got {
				t.Errorf("applied again:\n%s\n%v", again, err)
			}
		})
	}
	for _, text := range []string{"", "[1]", "{\"a\": "} {
		if _, err := SetJSON(text, "k", "v"); err == nil {
			t.Errorf("SetJSON(%q) did not fail", text)
		}
	}
}

func TestSetTOML(t *testing.T) {
	tests := []struct {
		name, text, want string
	}{
		{"replaces the line where it is",
			"# tools\n[tools]\nnode = \"20\" # old\npython = \"3.12\"\n",
			"# tools\n[tools]\nnode = \"22\"\npython = \"3.12\"\n"},
		{"after the table's last line",
			"[tools]\npython = \"3.12\"\n\n[env]\nA = \"1\"\n",
			"[tools]\npython = \"3.12\"\nnode = \"22\"\n\n[env]\nA = \"1\"\n"},
		{"a quoted key", "[tools]\n\"node\" = \"20\"\n", "[tools]\nnode = \"22\"\n"},
		{"the table at the end", "[env]\nA = \"1\"\n", "[env]\nA = \"1\"\n\n[tools]\nnode = \"22\"\n"},
		{"into an empty file", "", "[tools]\nnode = \"22\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SetTOML(tt.text, "tools", "node", "22")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("SetTOML() =\n%s\nwant\n%s", got, tt.want)
			}
			if again, err := SetTOML(got, "tools", "node", "22"); err != nil || again != got {
				t.Errorf("applied again:\n%s\n%v", again, err)
			}
		})
	}
	// A table written inline cannot take a [tools] after it.
	if _, err := SetTOML("tools = { python = \"3.12\" }\n", "tools", "node", "22"); err == nil {
		t.Error("an inline table did not fail")
	}
}

func TestApplyEdit(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	zshrc := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("backs up and writes", func(t *testing.T) {
		backup, err := applyEdit(FileEdit{Path: zshrc, Before: "a\n", After: "a\nb\n"}, now)
		if err != nil {
			t.Fatal(err)
		}
		if want := ".zshrc.hearth-2026-11-07-100000.bak"; filepath.Base(backup) != want {
			t.Errorf("backup %s, want %s", backup, want)
		}
		checkFile(t, zshrc, "a\nb\n", 0o600)
		checkFile(t, backup, "a\n", 0o600)
		if err := restoreEdit(FileEdit{Path: zshrc}, backup); err != nil {
			t.Fatal(err)
		}
		checkFile(t, zshrc, "a\n", 0o600)
	})

	t.Run("a file changed since the plan is left", func(t *testing.T) {
		_, err := applyEdit(FileEdit{Path: zshrc, Before: "something else\n", After: "x\n"}, now)
		if !errors.Is(err, ErrChanged) {
			t.Fatalf("err = %v, want ErrChanged", err)
		}
		checkFile(t, zshrc, "a\n", 0o600)
	})

	t.Run("a new file, and back", func(t *testing.T) {
		nvmrc := filepath.Join(dir, "app", ".nvmrc")
		backup, err := applyEdit(FileEdit{Path: nvmrc, After: "22\n", New: true}, now)
		if err != nil || backup != "" {
			t.Fatalf("backup %q, err %v", backup, err)
		}
		checkFile(t, nvmrc, "22\n", 0o644)
		if err := restoreEdit(FileEdit{Path: nvmrc}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(nvmrc); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the new file stays: %v", err)
		}
	})

	t.Run("through a link to the dotfiles", func(t *testing.T) {
		real := filepath.Join(dir, "dotfiles", "bashrc")
		if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(real, []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, ".bashrc")
		if err := os.Symlink(real, link); err != nil {
			t.Skip("no symlinks here:", err)
		}
		if _, err := applyEdit(FileEdit{Path: link, Before: "a\n", After: "b\n"}, now); err != nil {
			t.Fatal(err)
		}
		if target, err := os.Readlink(link); err != nil || target != real {
			t.Errorf("the link became %q, %v", target, err)
		}
		checkFile(t, real, "b\n", 0o644)
	})
}

// A plan that fails after an edit puts the file back as it was.
func TestExecuteRestoresEdits(t *testing.T) {
	dir := t.TempDir()
	zshrc := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Plan{Title: "Fix PATH", Steps: []Step{
		{Edit: &FileEdit{Path: zshrc, Before: "a\n", After: Block("a\n", "b")}},
		{Argv: []string{"zsh", "-lc", "true"}},
	}}
	f := &run.Fake{}
	f.Set(run.Result{ExitCode: 1}, nil, "zsh", "-lc", "true")
	got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {})
	if got.Phase != Failed || !reflect.DeepEqual(got.Restored, []string{zshrc}) {
		t.Fatalf("phase %d, restored %q", got.Phase, got.Restored)
	}
	checkFile(t, zshrc, "a\n", 0o644)

	f.Stdout("", "zsh", "-lc", "true")
	got = Execute(t.Context(), f, p, &strings.Builder{}, func(State) {})
	if got.Phase != Succeeded || len(got.Restored) != 0 {
		t.Fatalf("phase %d, err %q, restored %q", got.Phase, got.Err, got.Restored)
	}
	checkFile(t, zshrc, Block("a\n", "b"), 0o644)
}

func checkFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("%s holds %q, want %q", filepath.Base(path), data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeExe == "" && info.Mode().Perm() != mode {
		t.Errorf("%s has mode %v, want %v", filepath.Base(path), info.Mode().Perm(), mode)
	}
}
