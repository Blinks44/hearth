package projects

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

func TestParseGitRemote(t *testing.T) {
	tests := []struct {
		raw         string
		wantDisplay string
		wantURL     string
	}{
		{"git@github.com:Blinks44/hearth.git", "github.com/Blinks44/hearth", "https://github.com/Blinks44/hearth"},
		{"ssh://git@gitlab.corp.local:2222/team/sub/repo.git", "gitlab.corp.local/team/sub/repo", "https://gitlab.corp.local/team/sub/repo"},
		{"https://user:secret-token@github.com/org/app.git", "github.com/org/app", "https://github.com/org/app"},
		{"http://git.local:8080/group/proj", "git.local:8080/group/proj", "http://git.local:8080/group/proj"},
		{"file:///tmp/repo", "", ""},
		{"/local/path/repo", "", ""},
	}
	for _, tt := range tests {
		gotDisplay, gotURL := parseGitRemote(tt.raw)
		if gotDisplay != tt.wantDisplay || gotURL != tt.wantURL {
			t.Errorf("parseGitRemote(%q) = (%q, %q), want (%q, %q)", tt.raw, gotDisplay, gotURL, tt.wantDisplay, tt.wantURL)
		}
	}
}

func TestReadGitInfoFilesAndWorktree(t *testing.T) {
	root := t.TempDir()
	dotGit := filepath.Join(root, ".git")
	if err := os.MkdirAll(dotGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotGit, "HEAD"), []byte("ref: refs/heads/feat/skills\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = git@github.com:acme/web.git\n"
	if err := os.WriteFile(filepath.Join(dotGit, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	got := readGitInfo(root, nil)
	if got == nil {
		t.Fatal("readGitInfo returned nil")
	}
	if got.Branch != "feat/skills" || got.Detached {
		t.Fatalf("branch = %q detached=%v", got.Branch, got.Detached)
	}
	if got.Remote != "github.com/acme/web" || got.RemoteURL != "https://github.com/acme/web" {
		t.Fatalf("remote = %q (%q)", got.Remote, got.RemoteURL)
	}

	// Subfolder inherits parent .git.
	sub := filepath.Join(root, "packages", "ui")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if subInfo := readGitInfo(sub, nil); subInfo == nil || subInfo.Branch != "feat/skills" {
		t.Fatalf("subfolder git info = %+v", subInfo)
	}

	// Worktree .git file pointing to worktree gitdir + commondir.
	wt := t.TempDir()
	wtGitDir := filepath.Join(dotGit, "worktrees", "wt1")
	if err := os.MkdirAll(wtGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "HEAD"), []byte("0123456789abcdef\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtInfo := readGitInfo(wt, nil)
	if wtInfo == nil || wtInfo.Branch != "0123456" || !wtInfo.Detached || wtInfo.Remote != "github.com/acme/web" {
		t.Fatalf("worktree git info = %+v", wtInfo)
	}
}

func TestEnrichGitWithCLI(t *testing.T) {
	fake := &run.Fake{}
	fake.Stdout("## main...origin/main\n M package.json\n?? notes.txt\n",
		"/usr/bin/git", "--no-optional-locks", "-C", "/repo", "status", "--porcelain=v1", "-b")
	fake.Stdout("1760000000\x00feat: add tabs\n",
		"/usr/bin/git", "--no-optional-locks", "-C", "/repo", "log", "-1", "--format=%ct%x00%s")
	info := &GitInfo{}
	enrichGitWithCLI(context.Background(), fake, "/usr/bin/git", "/repo", []string{"PATH=/usr/bin"}, info)
	if info.Branch != "main" || !info.Dirty || info.Modified != 2 {
		t.Fatalf("status = %+v", info)
	}
	if info.LastCommit != "feat: add tabs" || !info.LastCommitAt.Equal(time.Unix(1760000000, 0)) {
		t.Fatalf("commit = %q at %v", info.LastCommit, info.LastCommitAt)
	}
}
