package ops

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const restartAnnotation = "kubectl.kubernetes.io/restartedAt"

func restartResource(kind string) string {
	switch kind {
	case "Deployment":
		return "deployment.apps"
	case "StatefulSet":
		return "statefulset.apps"
	case "DaemonSet":
		return "daemonset.apps"
	}
	return ""
}

func restartTime(obj resourceJSON) any {
	template, _ := resourceSpec(obj)["template"].(map[string]any)
	meta, _ := template["metadata"].(map[string]any)
	annotations, _ := meta["annotations"].(map[string]any)
	return annotations[restartAnnotation]
}

func daemonDesired(obj resourceJSON) int {
	status, _ := obj["status"].(map[string]any)
	n, _ := status["desiredNumberScheduled"].(json.Number)
	value, _ := n.Int64()
	return int(value)
}

// PlanKubeRestart previews the same template annotation as rollout restart.
// kubectl patch supports server dry-run and can enforce the reviewed version.
func PlanKubeRestart(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource) (Plan, error) {
	if restartResource(ref.Kind) == "" || ref.path() == "" {
		return Plan{}, errors.New(locales.KubeResourceTarget())
	}
	k, hash, closeConfig, err := (resourceClient{cli: ref.Kubectl, context: ref.Context, env: env, runner: r}).pin(ctx, [32]byte{})
	if err != nil {
		return Plan{}, err
	}
	defer closeConfig()
	change, obj, err := k.planRestart(ctx, ref, hash)
	if err != nil {
		return Plan{}, err
	}
	why := locales.KubeRestartPreview()
	if ref.Kind != "Deployment" {
		why = locales.KubeRestartWorkloadPreview(ref.Kind)
	}
	p := Plan{
		Title: locales.KubeRestartTitle(ref.Name), Why: why, Action: locales.KubeRestartAction(),
		Env: slices.Clone(env), Steps: []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, change.command()...)}},
	}
	spec := resourceSpec(obj)
	switch ref.Kind {
	case "Deployment":
		strategy, _ := spec["strategy"].(map[string]any)
		switch {
		case scaleOf(obj).desired == 0:
			p.Notes = []Note{{Title: locales.KubeReplicas(), Text: locales.KubeRestartZero()}}
		case strategy["type"] == "Recreate":
			p.Notes = []Note{{Title: locales.KubeResourceRollout(), Text: locales.KubeRestartRecreate()}}
		case scaleOf(obj).desired == 1:
			p.Notes = []Note{{Title: locales.KubeReplicas(), Text: locales.KubeRestartSingle()}}
		}
	case "StatefulSet":
		switch scaleOf(obj).desired {
		case 0:
			p.Notes = []Note{{Title: locales.KubeReplicas(), Text: locales.KubeRestartWorkloadZero(ref.Kind)}}
		case 1:
			p.Notes = []Note{{Title: locales.KubeReplicas(), Text: locales.KubeRestartSingle()}}
		}
	case "DaemonSet":
		switch daemonDesired(obj) {
		case 0:
			p.Notes = []Note{{Title: locales.KubeReplicas(), Text: locales.KubeRestartWorkloadZero(ref.Kind)}}
		case 1:
			p.Notes = []Note{{Title: locales.KubeReplicas(), Text: locales.KubeRestartSingle()}}
		}
	}
	return p, nil
}

func (k resourceClient) planRestart(ctx context.Context, ref KubeResource, hash [32]byte) (*KubeChange, resourceJSON, error) {
	obj, err := k.read(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil {
		return nil, nil, errors.New(locales.KubeResourceChanged())
	}
	spec := resourceSpec(obj)
	if ref.Kind == "Deployment" && spec["paused"] == true {
		return nil, nil, errors.New(locales.KubeRestartPaused())
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	command := resourceSpecPatch(ref, restartResource(ref.Kind), obj.version(), map[string]any{
		"template": map[string]any{
			"metadata": map[string]any{"annotations": map[string]string{restartAnnotation: stamp}},
		},
	})
	data, err := k.run(ctx, nil, append(slices.Clone(command), "--dry-run=server")...)
	if err != nil {
		return nil, nil, err
	}
	preview, err := decodeResource(data)
	if err != nil || !preview.matches(ref) || (ref.Kind == "Deployment" && resourceSpec(preview)["paused"] == true) || restartTime(preview) != stamp {
		return nil, nil, errors.New(locales.KubeResourceUnverified())
	}
	if restartTime(obj) == stamp {
		return nil, nil, errors.New(locales.KubeResourceNoChanges())
	}
	change := &KubeChange{
		draft: KubeDraft{Ref: ref, Version: obj.version(), config: hash}, want: preview, rollout: true, patch: command, restart: true,
	}
	return change, obj, nil
}
