package ops

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PlanKubeBulk previews each exact target using one pinned context.
func PlanKubeBulk(ctx context.Context, r run.Runner, env sysenv.Env, refs []KubeResource, restart bool) (Plan, error) {
	if len(refs) == 0 {
		return Plan{}, errors.New(locales.KubeResourceTarget())
	}
	first := refs[0]
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if ref.path() == "" || ref.Context != first.Context || ref.Kubectl != first.Kubectl ||
			ref.Kind != first.Kind || (ref.Kind != "Pod" && ref.Kind != "Deployment") || seen[ref.UID] {
			return Plan{}, errors.New(locales.KubeResourceTarget())
		}
		seen[ref.UID] = true
	}
	k, hash, closeConfig, err := (resourceClient{cli: first.Kubectl, context: first.Context, env: env, runner: r}).pin(ctx, [32]byte{})
	if err != nil {
		return Plan{}, err
	}
	defer closeConfig()
	p := Plan{
		KubeBulk: true, Env: slices.Clone(env), Title: locales.KubeBulkDelete(),
		Why: locales.KubeBulkDeleteWhy(), Action: locales.KubeResourceDeleteAction(),
		Notes: []Note{{Title: locales.KubeResourceCascade(), Text: locales.KubeResourceDeleteData()}},
	}
	if first.Kind == "Pod" {
		p.Notes = append(p.Notes, Note{Title: locales.KubeBulkOwnedPods()})
	}
	if restart {
		p.Title, p.Why, p.Action = locales.KubeBulkRestart(), locales.KubeBulkRestartWhy(), locales.KubeRestartAction()
		p.Notes = nil
		if first.Kind == "Pod" {
			p.Title, p.Why, p.Action = locales.KubeBulkRecreate(), locales.KubeBulkRecreateWhy(), locales.KubeBulkRecreate()
		}
	}
	for _, ref := range refs {
		change, err := k.planBulkTarget(ctx, ref, hash, restart)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", ref.label(), err)
		}
		p.Steps = append(p.Steps, Step{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, change.command()...)})
	}
	return p, nil
}

func (k resourceClient) planBulkTarget(ctx context.Context, ref KubeResource, hash [32]byte, restart bool) (*KubeChange, error) {
	if restart && ref.Kind == "Deployment" {
		change, _, err := k.planRestart(ctx, ref, hash)
		return change, err
	}
	obj, err := k.read(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil {
		return nil, errors.New(locales.KubeResourceChanged())
	}
	if restart {
		status, _ := obj["status"].(map[string]any)
		if status["phase"] == "Succeeded" || status["phase"] == "Failed" {
			return nil, errors.New(locales.KubeBulkUnmanaged())
		}
		switch resourceController(obj)["kind"] {
		case "ReplicaSet", "StatefulSet", "DaemonSet", "ReplicationController":
		default:
			return nil, errors.New(locales.KubeBulkUnmanaged())
		}
	}
	change, err := k.planDelete(ctx, KubeDraft{Ref: ref, Version: obj.version(), config: hash})
	if err != nil {
		return nil, err
	}
	change.batchDelete = true
	return change, nil
}

func (m *KubeChange) UID() string { return m.draft.Ref.UID }

func (m *KubeChange) Context() string { return m.draft.Ref.Context }

func (m *KubeChange) DeletesResource() bool {
	return m.want == nil && m.scale == nil && m.jobName == ""
}

// executeKubeBulk runs targets independently and records each outcome.
func (e *execution) executeKubeBulk(ctx context.Context, r run.Runner, p Plan) State {
	first := p.Steps[0].Kube.draft
	ref := first.Ref
	e.state.StepDetails = make([]string, len(p.Steps))
	k, _, closeConfig, err := (resourceClient{cli: ref.Kubectl, context: ref.Context, env: p.Env, runner: r}).pin(ctx, first.config)
	if err != nil {
		e.end(0, Skipped, NotVerified, -1, err.Error())
		e.emit()
		return e.snapshot()
	}
	defer closeConfig()
	failed := false
	for i, step := range p.Steps {
		if ctx.Err() != nil {
			e.end(i, Skipped, Stopped, -1, "")
			e.emit()
			return e.snapshot()
		}
		e.state.Current, e.state.Phase, e.state.Steps[i] = i, Running, Busy
		e.printf("Kubernetes: %s (UID %s)\n", step.Kube.Resource(), step.Kube.UID())
		e.emit()
		found, err := step.Kube.applyPinned(ctx, k, func() {
			e.state.Phase = Verifying
			e.emit()
		})
		e.state.Steps[i], e.state.StepDetails[i] = Done, found
		if err != nil {
			failed = true
			e.state.Steps[i], e.state.StepDetails[i] = StepFailed, err.Error()
			if ctx.Err() != nil {
				e.state.Steps[i] = StepStopped
			}
		}
		e.printf("%s: %s\n", step.Kube.Resource(), e.state.StepDetails[i])
		e.emit()
	}
	switch {
	case ctx.Err() != nil:
		e.state.Phase = Stopped
	case failed:
		e.state.Phase, e.state.Err = NotVerified, locales.KubeBulkPartial()
	default:
		e.state.Phase = Succeeded
	}
	e.emit()
	return e.snapshot()
}
