package ops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
		Changes: []string{locales.KubeContextSelected(from + " → " + to)},
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
