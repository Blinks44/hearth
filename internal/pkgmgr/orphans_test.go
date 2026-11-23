package pkgmgr

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestAPTOrphans(t *testing.T) {
	bin := t.TempDir()
	aptPath := filepath.Join(bin, "apt")
	aptGetPath := filepath.Join(bin, "apt-get")
	fake := &run.Fake{}
	fake.Set(run.Result{Stdout: []byte(
		"Reading package lists...\n" +
			"The following packages will be REMOVED:\n" +
			"  libfoo1 libbar2:arm64\n" +
			"Remv libfoo1 [1.0-1]\n" +
			"Remv libbar2:arm64 [2.0-1]\n",
	)}, nil, aptGetPath, "--simulate", "autoremove")

	m := inventory.PackageManager{
		ID:   "apt",
		Path: aptPath,
		Packages: []inventory.SystemPackage{
			{Name: "git", Architecture: "arm64"},
			{Name: "libfoo1", Architecture: "arm64"},
			{Name: "libbar2", Architecture: "arm64"},
			{Name: "libbar2", Architecture: "amd64"},
		},
	}
	orphans(context.Background(), fake, nil, &m)
	if m.Packages[0].Orphan {
		t.Error("git marked orphan")
	}
	if !m.Packages[1].Orphan {
		t.Error("libfoo1 not marked orphan")
	}
	if !m.Packages[2].Orphan {
		t.Error("libbar2:arm64 not marked orphan")
	}
	if m.Packages[3].Orphan {
		t.Error("libbar2:amd64 wrongly marked orphan")
	}
}

func TestPacmanOrphans(t *testing.T) {
	fake := &run.Fake{}
	fake.Set(run.Result{Stdout: []byte("libunused\npcre-old\n")}, nil, "/usr/bin/pacman", "-Qdtq")

	m := inventory.PackageManager{
		ID:   "pacman",
		Path: "/usr/bin/pacman",
		Packages: []inventory.SystemPackage{
			{Name: "git"},
			{Name: "libunused"},
			{Name: "pcre-old"},
		},
	}
	orphans(context.Background(), fake, nil, &m)
	if m.Packages[0].Orphan || !m.Packages[1].Orphan || !m.Packages[2].Orphan {
		t.Errorf("pacman orphans = %+v", m.Packages)
	}

	// Exit code 1 means no orphans; must leave packages untouched.
	fakeNone := &run.Fake{}
	fakeNone.Set(run.Result{ExitCode: 1}, nil, "/usr/bin/pacman", "-Qdtq")
	m2 := inventory.PackageManager{ID: "pacman", Path: "/usr/bin/pacman", Packages: []inventory.SystemPackage{{Name: "git"}}}
	orphans(context.Background(), fakeNone, nil, &m2)
	if m2.Packages[0].Orphan {
		t.Error("git marked orphan when pacman -Qdtq exited 1")
	}
}
