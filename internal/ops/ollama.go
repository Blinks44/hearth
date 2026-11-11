package ops

import (
	"context"
	"errors"
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
