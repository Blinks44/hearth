package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// HelmRelease is the current revision reported by helm list.
type HelmRelease struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Chart       string `json:"chart"`
	AppVersion  string `json:"app_version"`
	Revision    string `json:"revision"`
	Status      string `json:"status"`
	Updated     string `json:"updated"`
	Description string `json:"-"`
}

func (r HelmRelease) Pending() bool { return strings.HasPrefix(r.Status, "pending-") }

func readHelm(ctx context.Context, r run.Runner, env sysenv.Env, t Target, scopes [][]string) ([]HelmRelease, error) {
	env = env.HelmContext()
	releases := []HelmRelease{}
	// Helm 3 needs --all to include pending releases. Helm 4 defaults to
	// all states and newer versions reject the flag.
	useAll := t.HelmVersion == "" || strings.HasPrefix(strings.TrimPrefix(t.HelmVersion, "v"), "3.")
	for _, scope := range scopes {
		args := append([]string{"list", "--max=0", "--output=json"}, scope...)
		var list []HelmRelease
		var err error
		if useAll {
			err = helmJSON(ctx, r, env, t, &list, append(args, "--all")...)
			kerr, ok := errors.AsType[*Error](err)
			useAll = !ok || !strings.Contains(kerr.Access.Detail, "unknown flag: --all")
		}
		if !useAll {
			err = helmJSON(ctx, r, env, t, &list, args...)
		}
		if err != nil {
			return nil, err
		}
		releases = append(releases, list...)
	}

	// Descriptions are optional and must not hold up the release list.
	historyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for i := range releases {
		if historyCtx.Err() != nil {
			break
		}
		release := &releases[i]
		if release.Status != "failed" && !release.Pending() {
			continue
		}
		var history []struct {
			Revision    int    `json:"revision"`
			Status      string `json:"status"`
			Updated     string `json:"updated"`
			Description string `json:"description"`
		}
		err := helmJSON(historyCtx, r, env, t, &history, "history", "--namespace="+release.Namespace,
			"--max=1", "--output=json", "--", release.Name)
		if err == nil && len(history) == 1 && strconv.Itoa(history[0].Revision) == release.Revision && history[0].Status == release.Status {
			release.Description = history[0].Description
		}
	}
	return releases, nil
}

func helmJSON(ctx context.Context, r run.Runner, env sysenv.Env, t Target, dst any, args ...string) error {
	argv := append([]string{t.Helm, "--kube-context=" + t.Context}, args...)
	out, err := r.Run(ctx, run.Cmd{Args: argv, Env: env, Timeout: timeout, MaxOutput: 64 << 20})
	if err != nil {
		return fmt.Errorf("helm %s: %w", args[0], err)
	}
	if out.ExitCode != 0 {
		return &Error{Access: detect.KubeFailure(string(out.Stderr)), command: "helm"}
	}
	if out.Truncated {
		return fmt.Errorf("helm %s: the answer is too large", args[0])
	}
	if err := json.Unmarshal(out.Stdout, dst); err != nil {
		return fmt.Errorf("helm %s: %w", args[0], err)
	}
	return nil
}
