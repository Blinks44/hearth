package kube

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// HelmTarget identifies a release in the selected context and namespace.
type HelmTarget struct {
	Context, Helm, Namespace, Name string
}

func (t HelmTarget) Args(args ...string) []string {
	return append([]string{t.Helm, "--kube-context=" + t.Context, "--namespace=" + t.Namespace}, args...)
}

type HelmRevision struct {
	Revision    int    `json:"revision"`
	Updated     string `json:"updated"`
	Status      string `json:"status"`
	Chart       string `json:"chart"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
}

func (v HelmRevision) CanRollback(current HelmRevision) bool {
	return v.Revision > 0 && v.Revision < current.Revision && (v.Status == "deployed" || v.Status == "superseded")
}

var ErrHelmReleaseMissing = errors.New("helm release not found")

func HelmHistory(ctx context.Context, r run.Runner, env sysenv.Env, t HelmTarget, config string) ([]HelmRevision, error) {
	data, err := helmRead(ctx, r, env, t, config, "history", "--output=json", "--", t.Name)
	if err != nil {
		return nil, err
	}
	var history []HelmRevision
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("helm history: %w", err)
	}
	slices.SortFunc(history, func(a, b HelmRevision) int { return cmp.Compare(b.Revision, a.Revision) })
	return history, nil
}

func HelmContent(ctx context.Context, r run.Runner, env sysenv.Env, t HelmTarget, config string, revision int, manifest bool) (string, error) {
	args := []string{"get", "values", "--output=yaml"}
	if manifest {
		args = []string{"get", "manifest"}
	}
	data, err := helmRead(ctx, r, env, t, config, append(args, "--revision="+strconv.Itoa(revision), "--", t.Name)...)
	return string(data), err
}

func helmRead(ctx context.Context, r run.Runner, env sysenv.Env, t HelmTarget, config string, args ...string) ([]byte, error) {
	argv := t.Args()
	if config != "" {
		argv = append(argv, "--kubeconfig="+config)
	}
	out, err := r.Run(ctx, run.Cmd{Args: append(argv, args...), Env: env.HelmContext(), Timeout: 15 * time.Second, MaxOutput: 256 << 10})
	if err != nil {
		return nil, fmt.Errorf("helm %s: %w", args[0], err)
	}
	if out.ExitCode != 0 {
		stderr := strings.TrimSpace(string(out.Stderr))
		if stderr == "Error: release: not found" || stderr == "Error: release: not found: "+t.Name {
			return nil, ErrHelmReleaseMissing
		}
		return nil, &Error{Access: detect.KubeFailure(stderr), command: "helm"}
	}
	if out.Truncated || strings.Count(string(out.Stdout), "\n") > 1500 {
		return nil, errors.New(locales.KubeResourceTooLarge())
	}
	return out.Stdout, nil
}
