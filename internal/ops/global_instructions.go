package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// InstructionInstall creates a new file or file symlink, never replacing one.
// Observed includes the source and override so a stale plan fails before writing.
type InstructionInstall struct {
	Observed   inventory.GlobalInstructions
	Env        sysenv.Env
	Mode, Text string
}

// Preview is the exact text a new file will expose, even for a symlink.
func (in InstructionInstall) Preview() FileEdit {
	return FileEdit{Path: in.Observed.Native.Path, After: in.Text, New: true}
}

// CodexInstructionPlan reads again in the background. Merely building it writes nothing.
func CodexInstructionPlan(env sysenv.Env, mode, text string) (Plan, error) {
	observed := ai.CodexInstructions(env)
	switch mode {
	case "create":
		if observed.State != "missing" {
			return Plan{}, errors.New(locales.GlobalInstructionsChanged())
		}
	case "link", "copy":
		if observed.State != "shared" {
			return Plan{}, errors.New(locales.GlobalInstructionsChanged())
		}
		var err error
		text, err = ai.InstructionText(observed.Shared.Path)
		if err != nil {
			return Plan{}, fmt.Errorf("read shared instructions: %w", err)
		}
		if ai.CodexInstructions(env) != observed {
			return Plan{}, ErrChanged
		}
	default:
		return Plan{}, errors.New("unknown instruction action")
	}
	if strings.TrimSpace(text) == "" || len(text) > 32*1024 || strings.Count(text, "\n") > 500 || !utf8.ValidString(text) {
		return Plan{}, errors.New(locales.GlobalInstructionsTextLimit())
	}
	if !filepath.IsAbs(observed.Native.Path) {
		return Plan{}, errors.New(locales.GlobalInstructionsChanged())
	}
	in := InstructionInstall{Observed: observed, Env: append(sysenv.Env(nil), env...), Mode: mode, Text: text}
	why, action := locales.GlobalInstructionsCreateWhy(), locales.GlobalInstructionsCreate()
	if mode == "link" {
		why, action = locales.GlobalInstructionsLinkWhy(), locales.GlobalInstructionsConnect()
	}
	if mode == "copy" {
		why, action = locales.GlobalInstructionsCopyWhy(), locales.GlobalInstructionsCopy()
	}
	p := Plan{Title: locales.GlobalInstructionsTitle(), Why: why, Action: action, Steps: []Step{{Instructions: &in}}, Changes: []string{observed.Native.Path}}
	if mode == "link" {
		p.Notes = []Note{{Title: locales.GlobalInstructionsConnect(), Text: observed.Native.Path + " → " + observed.Shared.Path}}
	}
	p.Notes = append(p.Notes, Note{Title: locales.GlobalInstructionsProtection(), Text: locales.GlobalInstructionsProtectionText()})
	p.Verify = func(context.Context) (string, error) {
		got := ai.CodexInstructions(env)
		if got.Shared != observed.Shared || got.Override != observed.Override {
			return "", ErrChanged
		}
		if got.Active != observed.Native.Path || got.Native.Sum == "" || got.Native.Err != "" {
			return "", ErrChanged
		}
		if mode == "link" && got.State != "linked" {
			return "", ErrChanged
		}
		textNow, err := ai.InstructionText(got.Native.Path)
		if err != nil || textNow != text {
			return "", ErrChanged
		}
		return locales.GlobalInstructionsReady(), nil
	}
	return p, nil
}

func applyInstructionInstall(ctx context.Context, in InstructionInstall) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ai.CodexInstructions(in.Env) != in.Observed {
		return nil, ErrChanged
	}
	path := in.Observed.Native.Path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("make instruction directory: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil || filepath.Join(parent, filepath.Base(path)) != in.Observed.Native.Target {
		return nil, ErrChanged
	}
	// Both operations fail atomically if any file or dangling link appeared.
	if in.Mode == "link" {
		if err := os.Symlink(in.Observed.Shared.Path, path); err != nil {
			return nil, fmt.Errorf("link instructions: %w", err)
		}
	} else {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, fmt.Errorf("create instructions: %w", err)
		}
		_, err = f.WriteString(in.Text)
		err = errors.Join(err, f.Close())
		if err != nil {
			return nil, fmt.Errorf("write instructions: %w", errors.Join(err, os.Remove(path)))
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("stat created instructions: %w", err)
	}
	undo := func() error {
		now, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !os.SameFile(info, now) {
			return ErrChanged
		}
		if in.Mode != "link" {
			text, err := ai.InstructionText(path)
			if err != nil || text != in.Text {
				return ErrChanged
			}
		}
		return os.Remove(path)
	}
	return undo, nil
}
