package ops

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func instructionEnv(t *testing.T) (sysenv.Env, string, string) {
	t.Helper()
	home := t.TempDir()
	env := sysenv.Env{"HOME=" + home, "CODEX_HOME=" + filepath.Join(home, "custom-codex")}
	return env, filepath.Join(home, ".agents/AGENTS.md"), filepath.Join(home, "custom-codex/AGENTS.md")
}
func instructionWrite(t *testing.T, p, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCodexInstructionPlan(t *testing.T) {
	for _, mode := range []string{"create", "copy", "link"} {
		t.Run(mode, func(t *testing.T) {
			env, shared, native := instructionEnv(t)
			const text = "Explain changes.\n"
			if mode != "create" {
				instructionWrite(t, shared, text)
			}
			p, err := CodexInstructionPlan(env, mode, text)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Lstat(native); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("building a plan changed files")
			}
			in := p.Steps[0].Instructions
			if in == nil || in.Preview().After != text {
				t.Fatal("preview differs from installed text")
			}
			st := Execute(t.Context(), &run.Fake{}, p, io.Discard, func(State) {})
			if mode == "link" && st.InstructionCopyAvailable {
				if _, err = os.Lstat(native); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed link created a copy")
				}
				cp, err := CodexInstructionPlan(env, "copy", "")
				if err != nil {
					t.Fatal(err)
				}
				st = Execute(t.Context(), &run.Fake{}, cp, io.Discard, func(State) {})
			}
			if st.Phase != Succeeded {
				t.Fatalf("execution: %+v", st)
			}
			data, err := os.ReadFile(native)
			if err != nil || string(data) != text {
				t.Fatalf("installed %q: %v", data, err)
			}
			if _, err = CodexInstructionPlan(env, mode, text); err == nil {
				t.Fatal("would replace an existing file")
			}
		})
	}
}

func TestCodexInstructionPlanStale(t *testing.T) {
	for _, tc := range []struct{ name, target, text string }{
		{"source changed", "shared", "changed"}, {"destination appeared", "native", "keep me"}, {"empty destination appeared", "native", ""}, {"override appeared", "override", "override"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, shared, native := instructionEnv(t)
			instructionWrite(t, shared, "Source")
			p, err := CodexInstructionPlan(env, "copy", "")
			if err != nil {
				t.Fatal(err)
			}
			target := map[string]string{"shared": shared, "native": native, "override": filepath.Join(filepath.Dir(native), "AGENTS.override.md")}[tc.target]
			instructionWrite(t, target, tc.text)
			st := Execute(t.Context(), &run.Fake{}, p, io.Discard, func(State) {})
			if st.Phase != Failed {
				t.Fatalf("stale plan ran: %+v", st)
			}
			data, _ := os.ReadFile(target)
			if string(data) != tc.text {
				t.Fatal("modified existing file")
			}
			if tc.target != "native" {
				if _, err = os.Lstat(native); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("wrote before detecting stale input")
				}
			}
		})
	}
}

func TestCodexInstructionPlanRefusesExisting(t *testing.T) {
	for _, tc := range []struct{ name, path, text string }{
		{"empty native", "AGENTS.md", ""}, {"native", "AGENTS.md", "keep"}, {"override", "AGENTS.override.md", "keep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, shared, native := instructionEnv(t)
			instructionWrite(t, shared, "Common")
			instructionWrite(t, filepath.Join(filepath.Dir(native), tc.path), tc.text)
			for _, mode := range []string{"create", "link", "copy"} {
				if _, err := CodexInstructionPlan(env, mode, "New"); err == nil {
					t.Fatalf("%s replaced existing instructions", mode)
				}
			}
		})
	}
}

func TestInstructionInstallRollback(t *testing.T) {
	env, shared, native := instructionEnv(t)
	instructionWrite(t, shared, "Common")
	p, err := CodexInstructionPlan(env, "copy", "")
	if err != nil {
		t.Fatal(err)
	}
	p.Steps = append(p.Steps, Step{Argv: []string{"fail"}})
	fake := &run.Fake{}
	fake.Set(run.Result{ExitCode: 1}, nil, "fail")
	st := Execute(t.Context(), fake, p, io.Discard, func(State) {})
	if st.Phase != Failed || len(st.Restored) != 1 {
		t.Fatalf("rollback: %+v", st)
	}
	if _, err = os.Lstat(native); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new file survived rollback")
	}
	text, _ := os.ReadFile(shared)
	if string(text) != "Common" {
		t.Fatal("rollback changed source")
	}
}

func TestInstructionInstallKeepsChangedFileOnRollback(t *testing.T) {
	env, _, native := instructionEnv(t)
	p, err := CodexInstructionPlan(env, "create", "New")
	if err != nil {
		t.Fatal(err)
	}
	undo, err := applyInstructionInstall(t.Context(), *p.Steps[0].Instructions)
	if err != nil {
		t.Fatal(err)
	}
	instructionWrite(t, native, "User edit")
	if err = undo(); !errors.Is(err, ErrChanged) {
		t.Fatalf("removed a user edit: %v", err)
	}
	if got := ai.CodexInstructions(env); got.State != "native" {
		t.Fatal(got)
	}
}

func TestInstructionInstallRejectsBrokenLink(t *testing.T) {
	env, shared, native := instructionEnv(t)
	instructionWrite(t, shared, "Common")
	instructionWrite(t, native, "temporary")
	if err := os.Remove(native); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), native); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := CodexInstructionPlan(env, "copy", ""); err == nil {
		t.Fatal("replaced broken link")
	}
}

func TestInstructionInstallCancelled(t *testing.T) {
	env, _, native := instructionEnv(t)
	p, err := CodexInstructionPlan(env, "create", "Rules")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = applyInstructionInstall(ctx, *p.Steps[0].Instructions); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = os.Lstat(native); !os.IsNotExist(err) {
		t.Fatal("cancelled installation wrote a file")
	}
}

func TestInstructionInstallParentChanged(t *testing.T) {
	env, shared, native := instructionEnv(t)
	instructionWrite(t, shared, "Source")
	p, err := CodexInstructionPlan(env, "copy", "")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Dir(native)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err = applyInstructionInstall(t.Context(), *p.Steps[0].Instructions); !errors.Is(err, ErrChanged) {
		t.Fatalf("followed a new directory link: %v", err)
	}
	if _, err = os.Lstat(filepath.Join(outside, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("wrote through a new parent link")
	}
}

func TestGlobalInstructionPlanOtherAgents(t *testing.T) {
	for _, tc := range []struct {
		id, name, rel string
	}{
		{"antigravity", "Antigravity CLI", ".gemini/config/AGENTS.md"},
		{"claude", "Claude Code", ".claude/CLAUDE.md"},
		{"gemini", "Gemini CLI", ".gemini/GEMINI.md"},
		{"opencode", "OpenCode", ".config/opencode/AGENTS.md"},
		{"grok", "Grok Build", ".grok/AGENTS.md"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			home := t.TempDir()
			env := sysenv.Env{"HOME=" + home}
			shared := filepath.Join(home, ".agents", "AGENTS.md")
			native := filepath.Join(home, filepath.FromSlash(tc.rel))
			const text = "Global rules\n"
			instructionWrite(t, shared, text)

			p, err := GlobalInstructionPlan(tc.id, tc.name, env, "link", "")
			if err != nil {
				t.Fatal(err)
			}
			st := Execute(t.Context(), &run.Fake{}, p, io.Discard, func(State) {})
			if st.InstructionCopyAvailable {
				cp, err := GlobalInstructionPlan(tc.id, tc.name, env, "copy", "")
				if err != nil {
					t.Fatal(err)
				}
				st = Execute(t.Context(), &run.Fake{}, cp, io.Discard, func(State) {})
			}
			if st.Phase != Succeeded {
				t.Fatalf("execution failed: %+v", st)
			}
			data, err := os.ReadFile(native)
			if err != nil || string(data) != text {
				t.Fatalf("installed %q (%v)", data, err)
			}
			got, ok := ai.AgentInstructions(tc.id, env)
			if !ok || got.Active != native {
				t.Fatalf("post-install instructions: %+v (%v)", got, ok)
			}
		})
	}

	t.Run("antigravity stale alternate path aborts write", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		instructionWrite(t, filepath.Join(home, ".agents/AGENTS.md"), "Common\n")
		p, err := GlobalInstructionPlan("antigravity", "Antigravity CLI", env, "copy", "")
		if err != nil {
			t.Fatal(err)
		}
		// User creates an alternate global instruction file before confirming.
		instructionWrite(t, filepath.Join(home, ".gemini/GEMINI.md"), "Manual\n")
		st := Execute(t.Context(), &run.Fake{}, p, io.Discard, func(State) {})
		if st.Phase != Failed {
			t.Fatalf("stale antigravity plan ran: %+v", st)
		}
		if _, err := os.Lstat(filepath.Join(home, ".gemini/config/AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("wrote canonical file after alternate file appeared")
		}
	})
}

func canCreateFileSymlink(t *testing.T) bool {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("ok\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := os.Symlink(target, filepath.Join(dir, "link.txt"))
	if err == nil {
		return true
	}
	if instructionLinkDenied(err) {
		return false
	}
	t.Fatalf("probe symlink failed unexpectedly: %v", err)
	return false
}

func TestSharedInstructionsPlanCreateAndLink(t *testing.T) {
	t.Run("creates shared file without symlinks", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		const text = "# Personal rules\n\n- Keep diffs small.\n"

		p, err := SharedInstructionsPlan(SharedInstructionsRequest{Text: text}, env)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Steps) != 1 {
			t.Fatalf("steps = %d, want 1", len(p.Steps))
		}
		shared := filepath.Join(home, ".agents", "AGENTS.md")
		if _, err := os.Lstat(shared); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("building plan created shared file")
		}
		st := Execute(t.Context(), &run.Fake{}, p, io.Discard, func(State) {})
		if st.Phase != Succeeded {
			t.Fatalf("execution failed: %+v", st)
		}
		got, err := os.ReadFile(shared)
		if err != nil || string(got) != text {
			t.Fatalf("shared = %q (%v)", got, err)
		}
		if _, err := SharedInstructionsPlan(SharedInstructionsRequest{Text: text}, env); err == nil {
			t.Fatal("accepted SharedInstructionsPlan when shared file already exists")
		}
	})

	t.Run("creates shared file and links agents", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		const text = "# Personal rules\n\n- Keep diffs small.\n"

		p, err := SharedInstructionsPlan(SharedInstructionsRequest{
			Text:       text,
			LinkAgents: []string{"claude", "codex"},
		}, env)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Steps) != 3 {
			t.Fatalf("steps = %d, want 3", len(p.Steps))
		}
		shared := filepath.Join(home, ".agents", "AGENTS.md")
		if _, err := os.Lstat(shared); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("building plan created shared file")
		}

		st := Execute(t.Context(), &run.Fake{}, p, io.Discard, func(State) {})
		if !canCreateFileSymlink(t) {
			if st.Phase != Failed {
				t.Fatalf("expected failure when symlinks are denied, got %+v", st)
			}
			if _, err := os.Lstat(shared); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("shared AGENTS.md survived rollback on symlink denial: %v", err)
			}
			t.Skip("symlinks unavailable on this platform/account")
		}
		if st.Phase != Succeeded {
			t.Fatalf("execution failed: %+v", st)
		}
		got, err := os.ReadFile(shared)
		if err != nil || string(got) != text {
			t.Fatalf("shared = %q (%v)", got, err)
		}
		for _, tc := range []struct{ id, rel string }{
			{"claude", ".claude/CLAUDE.md"},
			{"codex", ".codex/AGENTS.md"},
		} {
			native := filepath.Join(home, filepath.FromSlash(tc.rel))
			info, err := os.Lstat(native)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("%s native %s is not symlink: %v", tc.id, native, err)
			}
			in, ok := ai.AgentInstructions(tc.id, env)
			if !ok || in.State != "linked" {
				t.Fatalf("%s state = %+v (%v), want linked", tc.id, in, ok)
			}
		}
		// Refuses if ~/.agents/AGENTS.md already exists.
		if _, err := SharedInstructionsPlan(SharedInstructionsRequest{Text: text}, env); err == nil {
			t.Fatal("accepted SharedInstructionsPlan when shared file already exists")
		}
	})
}

func TestSharedInstructionsPlanMigrateAndRollback(t *testing.T) {
	t.Run("migrates with backup and links another agent", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		claudeNative := filepath.Join(home, ".claude", "CLAUDE.md")
		const orig = "# My Claude rules\n\n- Always write tests.\n"
		instructionWrite(t, claudeNative, orig)

		p, err := SharedInstructionsPlan(SharedInstructionsRequest{
			Text:            orig,
			MigrateFrom:     "claude",
			MigrateFromName: "Claude Code",
			LinkAgents:      []string{"codex"},
		}, env)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Steps) != 3 || p.Steps[0].Instructions.Mode != "create-shared" || p.Steps[1].Instructions.Mode != "migrate-link" || p.Steps[2].Instructions.Mode != "link" {
			t.Fatalf("unexpected steps: %+v", p.Steps)
		}
		shared := filepath.Join(home, ".agents", "AGENTS.md")
		st := Execute(t.Context(), &run.Fake{}, p, io.Discard, func(State) {})
		if !canCreateFileSymlink(t) {
			if st.Phase != Failed {
				t.Fatalf("expected failure when symlinks are denied, got %+v", st)
			}
			if _, err := os.Lstat(shared); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("shared AGENTS.md survived rollback on symlink denial: %v", err)
			}
			if got, err := os.ReadFile(claudeNative); err != nil || string(got) != orig {
				t.Fatalf("CLAUDE.md not restored after symlink denial: %q (%v)", got, err)
			}
			t.Skip("symlinks unavailable on this platform/account")
		}
		if st.Phase != Succeeded {
			t.Fatalf("migration failed: %+v", st)
		}
		if got, err := os.ReadFile(shared); err != nil || string(got) != orig {
			t.Fatalf("shared = %q (%v)", got, err)
		}
		backups, err := filepath.Glob(claudeNative + ".hearth-*.bak")
		if err != nil || len(backups) != 1 {
			t.Fatalf("expected 1 backup of CLAUDE.md, got %v (%v)", backups, err)
		}
		if b, err := os.ReadFile(backups[0]); err != nil || string(b) != orig {
			t.Fatalf("backup content = %q (%v)", b, err)
		}
		info, err := os.Lstat(claudeNative)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("CLAUDE.md should be symlink after migration: %v", err)
		}
		if in, ok := ai.AgentInstructions("codex", env); !ok || in.State != "linked" {
			t.Fatalf("codex not linked: %+v (%v)", in, ok)
		}
	})

	t.Run("rolls back migration and shared file when later step fails", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		claudeNative := filepath.Join(home, ".claude", "CLAUDE.md")
		const orig = "# Original rules\n"
		instructionWrite(t, claudeNative, orig)

		p, err := SharedInstructionsPlan(SharedInstructionsRequest{
			Text:            orig,
			MigrateFrom:     "claude",
			MigrateFromName: "Claude Code",
		}, env)
		if err != nil {
			t.Fatal(err)
		}
		p.Steps = append(p.Steps, Step{Argv: []string{"fail"}})
		fake := &run.Fake{}
		fake.Set(run.Result{ExitCode: 1}, nil, "fail")

		st := Execute(t.Context(), fake, p, io.Discard, func(State) {})
		if st.Phase != Failed {
			t.Fatalf("expected failure, got %+v", st)
		}
		if _, err := os.Lstat(filepath.Join(home, ".agents", "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("shared AGENTS.md survived rollback: %v", err)
		}
		info, err := os.Lstat(claudeNative)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("CLAUDE.md was not restored as regular file: %v", err)
		}
		if got, err := os.ReadFile(claudeNative); err != nil || string(got) != orig {
			t.Fatalf("restored CLAUDE.md content = %q (%v)", got, err)
		}
	})

	t.Run("refuses to overwrite another agent's existing file", func(t *testing.T) {
		home := t.TempDir()
		env := sysenv.Env{"HOME=" + home}
		instructionWrite(t, filepath.Join(home, ".codex", "AGENTS.md"), "Keep codex\n")
		if _, err := SharedInstructionsPlan(SharedInstructionsRequest{
			Text:       "New shared\n",
			LinkAgents: []string{"codex"},
		}, env); err == nil {
			t.Fatal("accepted linking over an existing agent file")
		}
	})
}
