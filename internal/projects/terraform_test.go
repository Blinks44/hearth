package projects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestReadTerraformAndTofu(t *testing.T) {
	t.Run("versions.tf with comments, heredoc, and nested blocks", func(t *testing.T) {
		dir := t.TempDir()
		tf := `
# terraform { required_version = ">= 0.12" }
// terraform { required_version = ">= 0.13" }
/* terraform {
  required_version = ">= 0.14"
} */
locals {
  doc = <<-EOF
    terraform {
      required_version = ">= 0.15"
    }
  EOF
}

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = "~> 1.10"
}
`
		if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(tf), 0o644); err != nil {
			t.Fatal(err)
		}
		p := Read(dir)
		if len(p.Requirements) != 1 {
			t.Fatalf("Requirements = %+v, want 1", p.Requirements)
		}
		got := p.Requirements[0]
		if got.Tool != "terraform" || got.Want != "~> 1.10" || got.File != "versions.tf" || got.Key != "terraform.required_version" {
			t.Errorf("got = %+v", got)
		}
	})

	t.Run(".terraform-version and versions.tf together", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".terraform-version"), []byte("1.10.5\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("resource \"null_resource\" \"x\" {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p := Read(dir)
		if len(p.Requirements) != 1 || p.Requirements[0].Tool != "terraform" || p.Requirements[0].Want != "1.10.5" {
			t.Fatalf("Requirements = %+v", p.Requirements)
		}
	})

	t.Run("bare main.tf without required_version still declares terraform", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("resource \"null_resource\" \"x\" {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p := Read(dir)
		if len(p.Requirements) != 1 || p.Requirements[0].Tool != "terraform" || p.Requirements[0].Want != "" || p.Requirements[0].File != "main.tf" {
			t.Fatalf("Requirements = %+v", p.Requirements)
		}
	})

	t.Run("opentofu via .opentofu-version and .tf or .tofu", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".opentofu-version"), []byte("1.8.5\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte("terraform { required_version = \">= 1.8\" }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p := Read(dir)
		if len(p.Requirements) != 2 {
			t.Fatalf("Requirements = %+v, want 2", p.Requirements)
		}
		if p.Requirements[0].Tool != "tofu" || p.Requirements[0].Want != "1.8.5" {
			t.Errorf("req[0] = %+v", p.Requirements[0])
		}
		if p.Requirements[1].Tool != "tofu" || p.Requirements[1].Want != ">= 1.8" {
			t.Errorf("req[1] = %+v", p.Requirements[1])
		}
	})

	t.Run("mise.toml maps opentofu to tofu and terraform to terraform", func(t *testing.T) {
		dir := t.TempDir()
		toml := "[tools]\nterraform = \"1.10.5\"\nopentofu = \"1.8.5\"\n"
		if err := os.WriteFile(filepath.Join(dir, "mise.toml"), []byte(toml), 0o644); err != nil {
			t.Fatal(err)
		}
		p := Read(dir)
		if len(p.Requirements) != 2 {
			t.Fatalf("Requirements = %+v, want 2", p.Requirements)
		}
		if p.Requirements[0].Tool != "terraform" || p.Requirements[0].Want != "1.10.5" {
			t.Errorf("req[0] = %+v", p.Requirements[0])
		}
		if p.Requirements[1].Tool != "tofu" || p.Requirements[1].Want != "1.8.5" {
			t.Errorf("req[1] = %+v", p.Requirements[1])
		}
	})
}

func TestResolveTerraformAndTofu(t *testing.T) {
	snap := &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{
				Tool: "cli",
				Cli: &inventory.CliState{
					Tools: []inventory.Installation{
						{
							Tool: "terraform", Version: "1.9.8", OnPath: 1, Path: "/opt/homebrew/bin/terraform",
							Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"},
						},
						{
							Tool: "tofu", Version: "1.8.5", OnPath: 1, Path: "/opt/homebrew/bin/tofu",
							Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"},
						},
					},
				},
			},
			{
				Tool: "mise",
				MiseTools: []inventory.MiseTool{
					{
						Name: "terraform",
						Installations: []inventory.Installation{
							{
								Tool: "terraform", Version: "1.10.5", Path: "/mise/installs/terraform/1.10.5",
								Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise", Version: "1.10.5"},
							},
						},
					},
				},
			},
			{
				Tool:     "node",
				Managers: []inventory.Manager{{Name: "mise", SwitchOnCD: true, Files: []string{"mise.toml", ".tool-versions"}}},
			},
		},
	}

	t.Run("pessimistic ~> 1.10 rejects 1.9.8 on PATH and finds mise 1.10.5", func(t *testing.T) {
		p := Project{
			Requirements: []Requirement{{Tool: "terraform", Want: "~> 1.10", File: "versions.tf", Key: "terraform.required_version"}},
		}
		st := Resolve(p, snap, nil)
		if len(st) != 1 || !st[0].Checked {
			t.Fatalf("status = %+v", st)
		}
		if st[0].Terminal == nil || st[0].Terminal.Version != "1.9.8" {
			t.Fatalf("Terminal = %+v, want 1.9.8", st[0].Terminal)
		}
		if st[0].Checks[0].Met {
			t.Errorf("Met = true, want false for 1.9.8 against ~> 1.10")
		}
		if st[0].Checks[0].Installed == nil || st[0].Checks[0].Installed.Version != "1.10.5" {
			t.Errorf("Installed = %+v, want 1.10.5", st[0].Checks[0].Installed)
		}
	})

	t.Run("pessimistic ~> 1.10.2 accepts 1.10.5 and rejects 1.11.0", func(t *testing.T) {
		match, err := matcher("terraform", "~> 1.10.2", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !match(inventory.Installation{Version: "1.10.5"}) {
			t.Error("expected 1.10.5 to match ~> 1.10.2")
		}
		if match(inventory.Installation{Version: "1.10.1"}) {
			t.Error("expected 1.10.1 not to match ~> 1.10.2")
		}
		if match(inventory.Installation{Version: "1.11.0"}) {
			t.Error("expected 1.11.0 not to match ~> 1.10.2")
		}
		match2, err := matcher("terraform", "~> 1.10", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !match2(inventory.Installation{Version: "1.11.0"}) {
			t.Error("expected 1.11.0 to match ~> 1.10")
		}
		if match2(inventory.Installation{Version: "2.0.0"}) {
			t.Error("expected 2.0.0 not to match ~> 1.10")
		}
	})

	t.Run("mise switches on .terraform-version to installed 1.10.5", func(t *testing.T) {
		p := Project{
			Requirements: []Requirement{{Tool: "terraform", Want: "1.10.5", File: ".terraform-version"}},
		}
		st := Resolve(p, snap, nil)
		if len(st) != 1 || st[0].Terminal == nil || st[0].Terminal.Version != "1.10.5" || !st[0].Checks[0].Met {
			t.Fatalf("status = %+v, want terminal 1.10.5 met", st)
		}
	})

	t.Run("tofu >= 1.8 met by brew 1.8.5", func(t *testing.T) {
		p := Project{
			Requirements: []Requirement{{Tool: "tofu", Want: ">= 1.8", File: "versions.tofu"}},
		}
		st := Resolve(p, snap, nil)
		if len(st) != 1 || !st[0].Checked || !st[0].Checks[0].Met {
			t.Fatalf("status = %+v, want met", st)
		}
	})

	t.Run("missing terraform reports Checked=true and Unmet", func(t *testing.T) {
		empty := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "cli", Cli: &inventory.CliState{}}}}
		p := Project{
			Requirements: []Requirement{{Tool: "terraform", Want: "~> 1.10", File: "versions.tf"}},
		}
		st := Resolve(p, empty, nil)
		if len(st) != 1 || !st[0].Checked {
			t.Fatalf("status = %+v", st)
		}
		if _, unmet := st[0].Unmet(); !unmet {
			t.Error("expected Unmet() to be true when terraform is not installed")
		}
	})
}
