package ops

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// KubeContextPlan changes only kubectl's local current-context using its
// native config command. It never contacts a cluster or runs auth plugins.
func KubeContextPlan(cli, from, to string, env sysenv.Env, r run.Runner) (Plan, error) {
	if cli == "" || to == "" || strings.ContainsAny(to, "\x00\r\n") {
		return Plan{}, errors.New(locales.KubeContextInvalid())
	}
	p := Plan{
		Title: locales.KubeContextTitle(to), Why: locales.KubeContextWhy(),
		Env:     append([]string{}, env...),
		Steps:   []Step{{Argv: []string{cli, "config", "use-context", "--", to}, Timeout: time.Minute}},
		Changes: []string{locales.KubeContextSelected(cmp.Or(from, "—") + " → " + to)},
	}
	p.Verify = func(ctx context.Context) (string, error) {
		res, err := r.Run(ctx, run.Cmd{Args: []string{cli, "config", "current-context"}, Env: env})
		if err != nil {
			return "", fmt.Errorf("check kubectl context: %w", err)
		}
		if res.ExitCode != 0 || strings.TrimSpace(string(res.Stdout)) != to {
			return "", errors.New(locales.KubeContextMismatch(to))
		}
		return locales.KubeContextSelected(to), nil
	}
	return p, nil
}

// kubeNamespace is a valid namespace name: an RFC 1123 label.
var kubeNamespace = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// KubeNamespaceValid reports that name can be a namespace's.
func KubeNamespaceValid(name string) bool { return kubeNamespace.MatchString(name) }

// KubeNamespacePlan sets the namespace kubectl uses in the context name, in
// the kubeconfig file that defines it, with kubectl's own config command.
// Like KubeContextPlan it never contacts a cluster: the namespace need not
// exist there yet.
func KubeNamespacePlan(cli, name, from, to, file string, env sysenv.Env, r run.Runner) (Plan, error) {
	if cli == "" || name == "" || strings.ContainsAny(name, "\x00\r\n") {
		return Plan{}, errors.New(locales.KubeContextInvalid())
	}
	if !KubeNamespaceValid(to) {
		return Plan{}, errors.New(locales.KubeNamespaceInvalid())
	}
	if from == "" {
		from = "default"
	}
	p := Plan{
		Title: locales.KubeNamespaceTitle(to, name), Why: locales.KubeNamespaceWhy(),
		Env:     append([]string{}, env...),
		Steps:   []Step{{Argv: []string{cli, "config", "set-context", "--namespace=" + to, "--", name}, Timeout: time.Minute}},
		Changes: []string{locales.KubeNamespaceChanged(name, from+" → "+to, file)},
	}
	p.Verify = func(ctx context.Context) (string, error) {
		res, err := r.Run(ctx, run.Cmd{Args: []string{cli, "config", "view", "--minify", "--context=" + name, "-o", "jsonpath={.contexts[0].context.namespace}"}, Env: env})
		if err != nil {
			return "", fmt.Errorf("check kubectl namespace: %w", err)
		}
		if res.ExitCode != 0 || strings.TrimSpace(string(res.Stdout)) != to {
			return "", errors.New(locales.KubeNamespaceMismatch(name, to))
		}
		return locales.KubeNamespaceSet(name, to), nil
	}
	return p, nil
}

// kindName is a cluster name kind takes: its nodes' container and host
// names start with it, so it stays a short DNS label.
var kindName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,30}[a-z0-9])?$`)

// KindNameValid reports that name can be a kind cluster's.
func KindNameValid(name string) bool { return kindName.MatchString(name) }

// kindTimeout bounds creating a cluster: the first one downloads the node
// image.
const kindTimeout = 20 * time.Minute

// KindCreatePlan creates a local kind cluster of a control-plane node
// and workers more, in the Docker engine kind finds. kind makes its
// context kubectl's current one; when kubectl and from, the current
// context before, are not "", a step after puts from back. A create that
// fails or is stopped deletes what it made: the plan refuses a name a
// cluster has already, so the delete only takes the new one.
func KindCreatePlan(kind, kubectl, name string, workers int, from string, env sysenv.Env, r run.Runner) (Plan, error) {
	if kind == "" || !KindNameValid(name) {
		return Plan{}, errors.New(locales.KindNameInvalid())
	}
	create := []string{kind, "create", "cluster", "--name", name, "--wait", "3m"}
	var config string
	if workers > 0 {
		config = filepath.Join(os.TempDir(), "hearth-kind-"+name+".yaml")
		create = append(create, "--config", config)
	}
	p := Plan{
		Title: locales.KindCreateTitle(name), Why: locales.KindCreateWhy(),
		Env:    append([]string{}, env...),
		Action: locales.KindCreateAction(),
		Steps: []Step{{
			Argv: create, Timeout: kindTimeout, UndoAlways: true,
			Undo: &Step{Argv: []string{kind, "delete", "cluster", "--name", name}, Timeout: 5 * time.Minute,
				// kind exits at its first failed node while it still makes
				// the others: delete them once they come.
				Again: func(ctx context.Context) bool {
					res, err := r.Run(ctx, run.Cmd{Args: []string{kind, "get", "nodes", "--name", name}, Env: env, Timeout: time.Minute})
					return err != nil || res.ExitCode != 0 || len(strings.Fields(string(res.Stdout))) > 0
				}},
		}},
		Notes:   []Note{{Title: locales.KindImageNote(), Text: locales.KindImageNoteText()}},
		Changes: []string{locales.KindCreatedChange(name, 1+workers), locales.KindContextAdded("kind-" + name)},
	}
	if config != "" {
		p.Notes = append(p.Notes, Note{Title: locales.KindConfigNote(config), Text: kindConfig(workers)})
	}
	restore := kubectl != "" && from != ""
	if restore {
		p.Steps = append(p.Steps, Step{Argv: []string{kubectl, "config", "use-context", "--", from}, Timeout: time.Minute, Note: locales.KindContextBack(from)})
		p.Changes = append(p.Changes, locales.KindContextKept(from))
	} else {
		p.Changes = append(p.Changes, locales.KindContextNow("kind-"+name))
	}
	p.Before = func(ctx context.Context) error {
		names, err := kindClusters(ctx, r, kind, env)
		if err != nil {
			return err
		}
		if slices.Contains(names, name) {
			return errors.New(locales.KindExists(name))
		}
		if config == "" {
			return nil
		}
		if err := os.WriteFile(config, []byte(kindConfig(workers)), 0o600); err != nil {
			return fmt.Errorf("write kind config: %w", err)
		}
		return nil
	}
	p.Verify = func(ctx context.Context) (string, error) {
		names, err := kindClusters(ctx, r, kind, env)
		if err != nil {
			return "", err
		}
		if !slices.Contains(names, name) {
			return "", errors.New(locales.KindMissing(name))
		}
		if restore {
			res, err := r.Run(ctx, run.Cmd{Args: []string{kubectl, "config", "current-context"}, Env: env})
			if err != nil {
				return "", fmt.Errorf("check kubectl context: %w", err)
			}
			if strings.TrimSpace(string(res.Stdout)) != from {
				return "", errors.New(locales.KubeContextMismatch(from))
			}
		}
		return locales.KindCreated(name), nil
	}
	return p, nil
}

// kindConfig is kind's cluster config of a control-plane node and
// workers more.
func kindConfig(workers int) string {
	var b strings.Builder
	b.WriteString("kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n")
	for range workers {
		b.WriteString("- role: worker\n")
	}
	return b.String()
}

// KindDeletePlan deletes a kind cluster: its node containers with all in
// them, and its context from kubeconfig. current is kubectl's current
// context, which kind leaves unset when it was the cluster's; projects
// are the Hearth projects linked to its context.
func KindDeletePlan(kind, name, current string, projects []string, env sysenv.Env, r run.Runner) (Plan, error) {
	if kind == "" || name == "" || strings.ContainsAny(name, "\x00\r\n") {
		return Plan{}, errors.New(locales.KindNameInvalid())
	}
	p := Plan{
		Title: locales.KindDeleteTitle(name), Why: locales.KindDeleteWhy(),
		Env:     append([]string{}, env...),
		Action:  locales.KindDeleteAction(),
		Steps:   []Step{{Argv: []string{kind, "delete", "cluster", "--name", name}, Timeout: 5 * time.Minute}},
		Notes:   []Note{{Title: locales.KindDataLost(), Text: locales.KindDataLostText(name)}},
		Changes: []string{locales.KindDeleted(name), locales.KindContextRemoved("kind-" + name)},
	}
	if current == "kind-"+name {
		p.Notes = append(p.Notes, Note{Title: locales.KindCurrentNote(current), Text: locales.KindCurrentNoteText()})
		p.Changes = append(p.Changes, locales.KindNoCurrent())
	}
	if len(projects) > 0 {
		p.Notes = append(p.Notes, Note{Title: locales.KindLinkedNote(strings.Join(projects, ", ")), Text: locales.KindLinkedNoteText()})
	}
	p.Verify = func(ctx context.Context) (string, error) {
		names, err := kindClusters(ctx, r, kind, env)
		if err != nil {
			return "", err
		}
		if slices.Contains(names, name) {
			return "", errors.New(locales.KindStillThere(name))
		}
		return locales.KindDeleted(name), nil
	}
	return p, nil
}

// kindClusters are the names kind get clusters lists.
func kindClusters(ctx context.Context, r run.Runner, kind string, env sysenv.Env) ([]string, error) {
	res, err := r.Run(ctx, run.Cmd{Args: []string{kind, "get", "clusters"}, Env: env, Timeout: time.Minute})
	if err != nil {
		return nil, fmt.Errorf("list kind clusters: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("kind get clusters: %s", strings.TrimSpace(string(res.Stderr)))
	}
	return strings.Fields(string(res.Stdout)), nil
}
