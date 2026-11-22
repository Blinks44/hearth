package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// InstructionInstall creates a new file or file symlink, or migrates an
// existing native instruction file to the shared ~/.agents/AGENTS.md with a
// backup. Observed includes the source and override so a stale plan fails
// before writing.
type InstructionInstall struct {
	Agent               string
	Observed            inventory.GlobalInstructions
	Env                 sysenv.Env
	Mode, Text          string
	SharedCreatedInPlan bool
}

// Preview is the exact text a new file will expose, even for a symlink.
func (in InstructionInstall) Preview() FileEdit {
	path := in.Observed.Native.Path
	if in.Mode == "create-shared" {
		path = in.Observed.Shared.Path
	}
	return FileEdit{Path: path, After: in.Text, New: true}
}

// SharedInstructionsRequest describes creating ~/.agents/AGENTS.md (either from
// template text or migrated from MigrateFrom's native file) and connecting
// selected agents via file symlinks in a single confirmed plan.
type SharedInstructionsRequest struct {
	Text            string
	MigrateFrom     string
	MigrateFromName string
	LinkAgents      []string
}

func validateInstructionText(text string) error {
	if strings.TrimSpace(text) == "" || len(text) > 32*1024 || strings.Count(text, "\n") > 500 || !utf8.ValidString(text) {
		return errors.New(locales.GlobalInstructionsTextLimit())
	}
	return nil
}

// SharedInstructionsPlan builds a plan that creates ~/.agents/AGENTS.md,
// optionally backs up and replaces MigrateFrom's native file with a symlink,
// and creates symlinks for any other selected agents. Building writes nothing.
func SharedInstructionsPlan(req SharedInstructionsRequest, env sysenv.Env) (Plan, error) {
	if err := validateInstructionText(req.Text); err != nil {
		return Plan{}, err
	}
	envCopy := append(sysenv.Env(nil), env...)
	anchorID := req.MigrateFrom
	if anchorID == "" && len(req.LinkAgents) > 0 {
		anchorID = req.LinkAgents[0]
	}
	if anchorID == "" {
		anchorID = "codex"
	}
	anchorObs, ok := ai.AgentInstructions(anchorID, envCopy)
	if !ok || anchorObs.Shared.Exists || anchorObs.Shared.Err != "" || !filepath.IsAbs(anchorObs.Shared.Path) {
		return Plan{}, errors.New(locales.GlobalInstructionsChanged())
	}

	steps := []Step{{Instructions: &InstructionInstall{
		Agent:    anchorID,
		Observed: anchorObs,
		Env:      envCopy,
		Mode:     "create-shared",
		Text:     req.Text,
	}}}
	changes := []string{anchorObs.Shared.Path}
	var linkLines []string
	var linkedIDs []string
	var notes []Note

	if req.MigrateFrom != "" {
		migObs, ok := ai.AgentInstructions(req.MigrateFrom, envCopy)
		if !ok || migObs.Shared.Exists || migObs.Shared.Err != "" ||
			!migObs.Native.Exists || migObs.Native.Link || migObs.Native.Err != "" ||
			migObs.Override.Exists || !filepath.IsAbs(migObs.Native.Path) {
			return Plan{}, errors.New(locales.GlobalInstructionsChanged())
		}
		steps = append(steps, Step{Instructions: &InstructionInstall{
			Agent:               req.MigrateFrom,
			Observed:            migObs,
			Env:                 envCopy,
			Mode:                "migrate-link",
			Text:                req.Text,
			SharedCreatedInPlan: true,
		}})
		changes = append(changes, migObs.Native.Path)
		linkLines = append(linkLines, migObs.Native.Path+" → "+migObs.Shared.Path)
		linkedIDs = append(linkedIDs, req.MigrateFrom)
		backupHint := migObs.Native.Path + ".hearth-" + time.Now().Format("2006-01-02") + "….bak"
		notes = append(notes, Note{Title: locales.OpBackup(), Text: locales.OpBackupText(backupHint), Tag: locales.OpTagBackup()})
	}

	seen := map[string]bool{}
	if req.MigrateFrom != "" {
		seen[req.MigrateFrom] = true
	}
	for _, id := range req.LinkAgents {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		obs, ok := ai.AgentInstructions(id, envCopy)
		if !ok || obs.Shared.Exists || obs.Shared.Err != "" ||
			obs.Native.Exists || obs.Native.Err != "" ||
			obs.Override.Exists || !filepath.IsAbs(obs.Native.Path) {
			return Plan{}, errors.New(locales.GlobalInstructionsChanged())
		}
		steps = append(steps, Step{Instructions: &InstructionInstall{
			Agent:               id,
			Observed:            obs,
			Env:                 envCopy,
			Mode:                "link",
			Text:                req.Text,
			SharedCreatedInPlan: true,
		}})
		changes = append(changes, obs.Native.Path)
		linkLines = append(linkLines, obs.Native.Path+" → "+obs.Shared.Path)
		linkedIDs = append(linkedIDs, id)
	}

	if len(linkLines) > 0 {
		notes = append([]Note{{Title: locales.SharedInstructionsLinksNote(), Text: strings.Join(linkLines, "\n")}}, notes...)
	}
	notes = append(notes, Note{Title: locales.GlobalInstructionsProtection(), Text: locales.SharedInstructionsProtectionText()})

	why := locales.SharedInstructionsCreateWhy()
	action := locales.SharedInstructionsCreateOnlyAction()
	if len(linkedIDs) > 0 {
		action = locales.SharedInstructionsAction()
	}
	if req.MigrateFrom != "" {
		name := req.MigrateFromName
		if name == "" {
			name = req.MigrateFrom
		}
		why = locales.SharedInstructionsMigrateWhy(name, filepath.Base(anchorObs.Native.Path))
		action = locales.SharedInstructionsMigrateAction()
	}

	p := Plan{
		Title:   locales.SharedInstructionsPlanTitle(),
		Why:     why,
		Action:  action,
		Steps:   steps,
		Changes: changes,
		Notes:   notes,
	}
	verifyIDs := slices.Clone(linkedIDs)
	p.Verify = func(context.Context) (string, error) {
		sharedText, err := ai.InstructionText(anchorObs.Shared.Path)
		if err != nil || sharedText != req.Text {
			return "", ErrChanged
		}
		for _, id := range verifyIDs {
			got, ok := ai.AgentInstructions(id, envCopy)
			if !ok || got.State != "linked" || got.Native.Err != "" {
				return "", ErrChanged
			}
			textNow, err := ai.InstructionText(got.Native.Path)
			if err != nil || textNow != req.Text {
				return "", ErrChanged
			}
		}
		return locales.SharedInstructionsReady(), nil
	}
	return p, nil
}

// CodexInstructionPlan reads again in the background. Merely building it writes nothing.
func CodexInstructionPlan(env sysenv.Env, mode, text string) (Plan, error) {
	return GlobalInstructionPlan("codex", "Codex", env, mode, text)
}

// GlobalInstructionPlan builds a plan to create, link, or copy global
// instructions for agentID (displayed as agentName). Merely building it writes
// nothing.
func GlobalInstructionPlan(agentID, agentName string, env sysenv.Env, mode, text string) (Plan, error) {
	observed, ok := ai.AgentInstructions(agentID, env)
	if !ok {
		return Plan{}, errors.New(locales.GlobalInstructionsChanged())
	}
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
		if again, ok := ai.AgentInstructions(agentID, env); !ok || again != observed {
			return Plan{}, ErrChanged
		}
	default:
		return Plan{}, errors.New("unknown instruction action")
	}
	if err := validateInstructionText(text); err != nil {
		return Plan{}, err
	}
	if !filepath.IsAbs(observed.Native.Path) {
		return Plan{}, errors.New(locales.GlobalInstructionsChanged())
	}
	if agentName == "" {
		agentName = agentID
	}
	in := InstructionInstall{Agent: agentID, Observed: observed, Env: append(sysenv.Env(nil), env...), Mode: mode, Text: text}
	why, action := locales.GlobalInstructionsCreateWhy(filepath.Base(observed.Native.Path), agentName), locales.GlobalInstructionsCreate()
	if mode == "link" {
		why, action = locales.GlobalInstructionsLinkWhy(agentName), locales.GlobalInstructionsConnect()
	}
	if mode == "copy" {
		why, action = locales.GlobalInstructionsCopyWhy(agentName), locales.GlobalInstructionsCopy()
	}
	p := Plan{Title: locales.GlobalInstructionsTitle(), Why: why, Action: action, Steps: []Step{{Instructions: &in}}, Changes: []string{observed.Native.Path}}
	if mode == "link" {
		p.Notes = []Note{{Title: locales.GlobalInstructionsConnect(), Text: observed.Native.Path + " → " + observed.Shared.Path}}
	}
	p.Notes = append(p.Notes, Note{Title: locales.GlobalInstructionsProtection(), Text: locales.GlobalInstructionsProtectionText(agentName)})
	p.Verify = func(context.Context) (string, error) {
		got, ok := ai.AgentInstructions(agentID, env)
		if !ok || got.Shared != observed.Shared || got.Override != observed.Override {
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
	agentID := in.Agent
	if agentID == "" {
		agentID = "codex"
	}
	got, ok := ai.AgentInstructions(agentID, in.Env)
	if !ok {
		return nil, ErrChanged
	}

	switch in.Mode {
	case "create-shared":
		if got.Shared != in.Observed.Shared || got.Shared.Exists || got.Shared.Err != "" {
			return nil, ErrChanged
		}
		path := in.Observed.Shared.Path
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("make shared instruction directory: %w", err)
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil || filepath.Join(parent, filepath.Base(path)) != in.Observed.Shared.Target {
			return nil, ErrChanged
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, fmt.Errorf("create shared instructions: %w", err)
		}
		_, err = f.WriteString(in.Text)
		err = errors.Join(err, f.Close())
		if err != nil {
			return nil, fmt.Errorf("write shared instructions: %w", errors.Join(err, os.Remove(path)))
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("stat created shared instructions: %w", err)
		}
		return func() error {
			now, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !os.SameFile(info, now) {
				return ErrChanged
			}
			text, err := ai.InstructionText(path)
			if err != nil || text != in.Text {
				return ErrChanged
			}
			return os.Remove(path)
		}, nil

	case "migrate-link":
		if got.Native != in.Observed.Native || got.Override != in.Observed.Override || !got.Shared.Exists || got.Shared.Err != "" {
			return nil, ErrChanged
		}
		path := in.Observed.Native.Path
		origInfo, err := os.Lstat(path)
		if err != nil || origInfo.Mode()&os.ModeSymlink != 0 {
			return nil, ErrChanged
		}
		origData, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read instructions before migration: %w", err)
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil || filepath.Join(parent, filepath.Base(path)) != in.Observed.Native.Target {
			return nil, ErrChanged
		}
		mode := origInfo.Mode().Perm()
		backup := path + ".hearth-" + time.Now().Format("2006-01-02-150405") + ".bak"
		if err := os.WriteFile(backup, origData, mode); err != nil {
			return nil, fmt.Errorf("back up instructions: %w", err)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove original instructions: %w", err)
		}
		if err := os.Symlink(in.Observed.Shared.Path, path); err != nil {
			restoreErr := writeFile(path, origData, mode)
			if restoreErr == nil {
				_ = os.Remove(backup)
			}
			return nil, fmt.Errorf("link migrated instructions: %w", errors.Join(err, restoreErr))
		}
		linkInfo, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("stat migrated instructions link: %w", err)
		}
		return func() error {
			now, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !os.SameFile(linkInfo, now) {
				return ErrChanged
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			return writeFile(path, origData, mode)
		}, nil
	}

	if in.SharedCreatedInPlan {
		if got.Native != in.Observed.Native || got.Override != in.Observed.Override || !got.Shared.Exists || got.Shared.Err != "" {
			return nil, ErrChanged
		}
	} else if got != in.Observed {
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
