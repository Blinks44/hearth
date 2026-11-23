package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// ollamaPullTimeout ends a download: models take gigabytes.
const ollamaPullTimeout = 4 * time.Hour

// OllamaModelName is name as Ollama keeps it, "llama3.2" being
// "llama3.2:latest"; ok is false for what is no model's name, as empty,
// with spaces or an option.
func OllamaModelName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsFunc(name, func(r rune) bool { return r <= ' ' }) {
		return "", false
	}
	if i := strings.LastIndex(name, "/"); !strings.Contains(name[i+1:], ":") {
		name += ":latest"
	}
	return name, true
}

// OllamaPullPlan returns the plan that downloads the model name through
// ollama pull; ok is false for what is no model's name.
func OllamaPullPlan(name string, detect Detect) (p Plan, ok bool) {
	name, ok = OllamaModelName(name)
	if !ok {
		return Plan{}, false
	}
	p = Plan{
		Title:   locales.OllamaPullTitle(name),
		Why:     locales.OllamaPullWhy(),
		Steps:   []Step{{Argv: []string{"ollama", "pull", name}, Timeout: ollamaPullTimeout}},
		Changes: []string{"+ " + name + " · Ollama"},
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			has, err := ollamaHas(ctx, detect, name)
			if err == nil && !has {
				err = errors.New(locales.OllamaPullMissing(name))
			}
			return locales.OllamaPullFound(name), err
		}
	}
	return p, true
}

// OllamaStopPlan returns the plan that unloads a loaded model from Ollama's
// memory via `ollama stop` without removing its files from disk.
func OllamaStopPlan(m inventory.Model, detect Detect) Plan {
	p := Plan{
		Title:   locales.OllamaUnloadTitle(m.Name),
		Why:     locales.OllamaUnloadWhy(),
		Action:  locales.OllamaUnloadAction(),
		Steps:   []Step{{Argv: []string{"ollama", "stop", m.Name}, Timeout: time.Minute}},
		Changes: []string{m.Name + " · Ollama"},
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			loaded, err := ollamaIsLoaded(ctx, detect, m.Name)
			if err == nil && loaded {
				err = errors.New(locales.OllamaUnloadStill(m.Name))
			}
			return locales.OllamaUnloadDone(m.Name), err
		}
	}
	return p
}

// OllamaRemovePlan returns the plan that removes m, a model Ollama
// downloaded, through ollama rm; askedBy are the MCP servers that name
// it, which lose it.
func OllamaRemovePlan(m inventory.Model, askedBy []string, detect Detect) Plan {
	p := Plan{
		Title:   locales.PkgUninstallTitle(m.Name),
		Why:     locales.OllamaRemoveWhy(),
		Steps:   []Step{{Argv: []string{"ollama", "rm", m.Name}, Timeout: time.Minute}},
		Changes: []string{"− " + m.Name + " · Ollama"},
	}
	for _, s := range askedBy {
		p.Notes = append(p.Notes, Note{Title: locales.OllamaRemoveAsked(s), Text: locales.OllamaRemoveAskedText()})
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			has, err := ollamaHas(ctx, detect, m.Name)
			if err == nil && has {
				err = errors.New(locales.OllamaRemoveLeft(m.Name))
			}
			return locales.OllamaRemoveGone(m.Name), err
		}
	}
	return p
}

// LMStudioRemovePlan returns the plan that removes m, a model folder LM Studio
// downloaded, after checking its path stays strictly inside modelsRoot without
// traversing symlinks.
func LMStudioRemovePlan(modelsRoot string, m inventory.Model, askedBy []string, detect Detect) (Plan, error) {
	root := filepath.Clean(modelsRoot)
	dir := filepath.Clean(m.Path)
	if !lexicalInsideModelsRoot(root, dir) {
		return Plan{}, errors.New(locales.LmStudioRemoveBadPath())
	}
	p := Plan{
		Title:   locales.PkgUninstallTitle(m.Name),
		Why:     locales.LmStudioRemoveWhy(),
		Steps:   []Step{{Remove: dir}},
		Changes: []string{"− " + m.Name + " · LM Studio"},
		Before: func(context.Context) error {
			if !realInsideModelsRoot(root, dir) {
				return errors.New(locales.LmStudioRemoveBadPath())
			}
			return nil
		},
	}
	for _, s := range askedBy {
		p.Notes = append(p.Notes, Note{Title: locales.OllamaRemoveAsked(s), Text: locales.OllamaRemoveAskedText()})
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			has, err := lmStudioHas(ctx, detect, dir)
			if err == nil && has {
				err = errors.New(locales.LmStudioRemoveLeft(m.Name))
			}
			return locales.OllamaRemoveGone(m.Name), err
		}
	}
	return p, nil
}

func lexicalInsideModelsRoot(root, dir string) bool {
	if root == "" || dir == "" {
		return false
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(dir) || dir == filepath.Dir(dir) {
		return false
	}
	return strictSubpath(root, dir)
}

func realInsideModelsRoot(root, dir string) bool {
	if !lexicalInsideModelsRoot(root, dir) {
		return false
	}
	if info, err := os.Lstat(dir); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	return strictSubpath(realRoot, realDir)
}

func strictSubpath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "" || rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ollamaHas finds the models again and reports that Ollama has name.
func ollamaHas(ctx context.Context, detect Detect, name string) (bool, error) {
	st, err := detect(ctx)
	if err != nil {
		return false, err
	}
	if st.Models == nil || st.Models.Ollama == nil {
		return false, nil
	}
	if e := st.Models.Ollama.Err; e != "" {
		return false, errors.New(e)
	}
	return slices.ContainsFunc(st.Models.Ollama.Models, func(m inventory.Model) bool { return strings.EqualFold(m.Name, name) }), nil
}

// ollamaIsLoaded finds the models again and reports whether Ollama still keeps name in memory.
func ollamaIsLoaded(ctx context.Context, detect Detect, name string) (bool, error) {
	st, err := detect(ctx)
	if err != nil {
		return false, err
	}
	if st.Models == nil || st.Models.Ollama == nil {
		return false, nil
	}
	if e := st.Models.Ollama.Err; e != "" {
		return false, errors.New(e)
	}
	return slices.ContainsFunc(st.Models.Ollama.Models, func(m inventory.Model) bool {
		return strings.EqualFold(m.Name, name) && m.Loaded
	}), nil
}

// lmStudioHas finds the models again and reports whether LM Studio still lists dir.
func lmStudioHas(ctx context.Context, detect Detect, dir string) (bool, error) {
	st, err := detect(ctx)
	if err != nil {
		return false, err
	}
	if st.Models == nil || st.Models.LMStudio == nil {
		return false, nil
	}
	if e := st.Models.LMStudio.Err; e != "" {
		return false, errors.New(e)
	}
	return slices.ContainsFunc(st.Models.LMStudio.Models, func(m inventory.Model) bool {
		return filepath.Clean(m.Path) == dir
	}), nil
}

// OllamaServerPlan returns the plan that starts or stops the local Ollama
// server. Homebrew formula installs use brew services; other installs use the
// platform-specific app or service control.
func OllamaServerPlan(action ServiceAction, app inventory.ModelApp, detect Detect) (Plan, bool) {
	port := app.Port
	if port == "" {
		port = "11434"
	}
	var (
		step  Step
		notes []Note
		why   string
		ok    bool
	)
	if app.Source.Manager == "brew" && !strings.Contains(app.Path, ".app/") {
		verb := [...]string{"start", "stop"}[action]
		step = Step{Argv: []string{"brew", "services", verb, "ollama"}, Timeout: 2 * time.Minute}
		why = locales.ServiceWhyBrew()
		if action == ServiceStop {
			notes = []Note{{Title: locales.ServiceBrewLogin(), Text: locales.ServiceBrewLoginText(), Tag: "brew services"}}
		}
		ok = true
	} else {
		step, notes, ok = ollamaServerStep(action, app)
		why = locales.OllamaServerWhy()
	}
	if !ok {
		return Plan{}, false
	}
	p := Plan{
		Why:   why,
		Steps: []Step{step},
		Notes: notes,
	}
	if action == ServiceStart {
		p.Title = locales.OllamaServerStartTitle()
		p.Changes = []string{locales.OllamaServerStartChange()}
	} else {
		p.Title = locales.OllamaServerStopTitle()
		p.Changes = []string{locales.OllamaServerStopChange()}
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			up, err := ollamaListening(ctx, detect, port)
			if err != nil {
				return "", err
			}
			if action == ServiceStart {
				if !up {
					return "", errors.New(locales.OllamaServerNotRunning(port))
				}
				return locales.OllamaServerRunning(port), nil
			}
			if up {
				return "", errors.New(locales.OllamaServerStillRunning(port))
			}
			return locales.OllamaServerStopped(), nil
		}
	}
	return p, true
}

func ollamaListening(ctx context.Context, detect Detect, port string) (bool, error) {
	st, err := detect(ctx)
	if err != nil {
		return false, err
	}
	if st.Databases == nil {
		return false, nil
	}
	return slices.ContainsFunc(st.Databases.Listeners, func(l inventory.DatabaseListener) bool {
		return l.Port == port
	}), nil
}

// OllamaInstallPlan returns a plan to install Ollama via an available package
// manager (Homebrew, winget, or pacman). ok is false when none of them is found.
func OllamaInstallPlan(managers []string, detect Detect) (Plan, bool) {
	var (
		step  Step
		notes []Note
	)
	switch {
	case slices.Contains(managers, "brew"):
		step = Step{Argv: []string{"brew", "install", "ollama"}, Timeout: 10 * time.Minute}
	case slices.Contains(managers, "winget"):
		step = Step{
			Argv:    []string{"winget", "install", "--id", "Ollama.Ollama", "-e", "--accept-package-agreements", "--accept-source-agreements"},
			Timeout: 10 * time.Minute,
		}
	case slices.Contains(managers, "pacman"):
		step = Step{Argv: []string{"pacman", "-S", "--noconfirm", "ollama"}, Admin: true, Timeout: 10 * time.Minute}
		notes = []Note{AdminNote()}
	default:
		return Plan{}, false
	}
	p := Plan{
		Title:   locales.OllamaInstallTitle(),
		Why:     locales.OllamaInstallWhy(),
		Steps:   []Step{step},
		Changes: []string{"+ Ollama"},
		Notes:   notes,
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if st.Models == nil || st.Models.Ollama == nil {
				return "", errors.New(locales.OllamaInstallMissing())
			}
			return locales.OllamaInstallFound(), nil
		}
	}
	return p, true
}
