package ops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

var helmReleaseName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

type helmChange struct {
	target  kube.HelmTarget
	history []kube.HelmRevision
	to      kube.HelmRevision
}

func PlanKubeHelm(ctx context.Context, r run.Runner, env sysenv.Env, kubectl string, target kube.HelmTarget, observed kube.HelmRevision, revision int) (Plan, error) {
	if kubectl == "" || target.Helm == "" || target.Context == "" ||
		!KubeNamespaceValid(target.Namespace) || (len(target.Name) > 53 || !helmReleaseName.MatchString(target.Name)) || observed.Revision < 1 {
		return Plan{}, errors.New(locales.KubeResourceTarget())
	}
	env = env.HelmContext()
	k, config, closeConfig, err := (resourceClient{cli: kubectl, context: target.Context, env: env, runner: r}).pin(ctx, [32]byte{})
	if err != nil {
		return Plan{}, err
	}
	defer closeConfig()
	history, err := kube.HelmHistory(ctx, r, env, target, k.config)
	if err != nil {
		return Plan{}, err
	}
	if len(history) == 0 || history[0] != observed || observed.Status == "uninstalling" ||
		(kube.HelmRelease{Status: observed.Status}).Pending() {
		return Plan{}, errors.New(locales.KubeHelmChanged())
	}

	change := &helmChange{target: target, history: history}
	p := Plan{Env: env, Title: locales.KubeHelmDeleteTitle(target.Name), Action: locales.KubeResourceDelete(),
		Why: locales.KubeHelmDeleteImpact()}
	if revision != 0 {
		i := slices.IndexFunc(history, func(v kube.HelmRevision) bool { return v.Revision == revision })
		if i < 0 || !history[i].CanRollback(observed) {
			return Plan{}, errors.New(locales.KubeHelmChanged())
		}
		change.to = history[i]
		p.Title = locales.KubeRollbackTitle(target.Name, revision)
		p.Why = locales.KubeHelmRollbackImpact(observed.Revision, revision, change.to.Chart)
		p.Action = locales.KubeRollbackAction()
	}
	ref := KubeResource{Kubectl: kubectl, Context: target.Context, Namespace: target.Namespace, Name: target.Name, Kind: "Helm"}
	p.Steps = []Step{{
		Argv: target.Args(change.command()...),
		Kube: &KubeChange{draft: KubeDraft{Ref: ref, Version: strconv.Itoa(observed.Revision), config: config}, helm: change},
	}}
	p.Notes = []Note{{Title: locales.OpCommands(), Text: Command(p.Steps[0].Argv)}}
	return p, nil
}

func (h *helmChange) command() []string {
	if h.to.Revision != 0 {
		return []string{"rollback", "--wait", "--timeout=2m", "--history-max=0", "--", h.target.Name, strconv.Itoa(h.to.Revision)}
	}
	return []string{"uninstall", "--wait", "--timeout=2m", "--cascade=foreground", "--", h.target.Name}
}

func (h *helmChange) apply(ctx context.Context, k resourceClient, checking func()) (string, error) {
	history, err := kube.HelmHistory(ctx, k.runner, k.env, h.target, k.config)
	if err != nil {
		return "", err
	}
	if !slices.Equal(history, h.history) {
		return "", errors.New(locales.KubeHelmChanged())
	}
	argv := append(h.target.Args("--kubeconfig="+k.config), h.command()...)
	res, err := k.runner.Run(ctx, run.Cmd{Args: argv, Env: k.env, Timeout: 5 * time.Minute, MaxOutput: 64 << 10})
	checking()
	// Look up the result even if Helm was interrupted after sending a write.
	verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	after, verifyErr := kube.HelmHistory(verifyCtx, k.runner, k.env, h.target, k.config)
	if err == nil && res.ExitCode == 0 {
		if h.to.Revision == 0 && errors.Is(verifyErr, kube.ErrHelmReleaseMissing) {
			return locales.KubeHelmRemoved(), nil
		}
		if h.to.Revision != 0 && verifyErr == nil && len(after) > 0 {
			v := after[0]
			if v.Revision == history[0].Revision+1 && v.Status == "deployed" && v.Chart == h.to.Chart &&
				v.Description == fmt.Sprintf("Rollback to %d", h.to.Revision) {
				return locales.KubeHelmRestored(v.Revision), nil
			}
		}
	}
	// Helm may print chart or hook contents, so keep command output out of
	// the operation journal. Report the state observed after the write.
	if errors.Is(verifyErr, kube.ErrHelmReleaseMissing) {
		return "", errors.New(locales.KubeHelmCheckMissing())
	}
	if verifyErr == nil && len(after) > 0 {
		return "", errors.New(locales.KubeHelmCheckRevision(after[0].Revision, after[0].Status))
	}
	return "", errors.New(locales.KubeHelmUnverified())
}
