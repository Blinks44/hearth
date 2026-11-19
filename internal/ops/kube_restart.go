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

func deploymentSpec(obj resourceJSON) map[string]any {
	spec, _ := obj["spec"].(map[string]any)
	return spec
}

func restartTime(obj resourceJSON) any {
	template, _ := deploymentSpec(obj)["template"].(map[string]any)
	meta, _ := template["metadata"].(map[string]any)
	annotations, _ := meta["annotations"].(map[string]any)
	return annotations[restartAnnotation]
}

func deploymentPatch(ref KubeResource, version string, spec map[string]any) []string {
	patch, _ := json.Marshal(map[string]any{
		"metadata": map[string]string{"uid": ref.UID, "resourceVersion": version},
		"spec":     spec,
	})
	return []string{"patch", "deployment.apps/" + ref.Name, "--namespace=" + ref.Namespace,
		"--type=merge", "--patch=" + string(patch), "-o", "json"}
}

// PlanKubeRestart previews the same template annotation as rollout restart.
// kubectl patch supports server dry-run and can enforce the reviewed version.
func PlanKubeRestart(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource) (Plan, error) {
	if ref.Kind != "Deployment" || ref.path() == "" {
		return Plan{}, errors.New(locales.KubeResourceTarget())
	}
	k, hash, closeConfig, err := (resourceClient{cli: ref.Kubectl, context: ref.Context, env: env, runner: r}).pin(ctx, [32]byte{})
	if err != nil {
		return Plan{}, err
	}
	defer closeConfig()
	obj, err := k.read(ctx, ref)
	if err != nil {
		return Plan{}, err
	}
	if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil {
		return Plan{}, errors.New(locales.KubeResourceChanged())
	}
	spec := deploymentSpec(obj)
	if spec["paused"] == true {
		return Plan{}, errors.New(locales.KubeRestartPaused())
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	command := deploymentPatch(ref, obj.version(), map[string]any{
		"template": map[string]any{
			"metadata": map[string]any{"annotations": map[string]string{restartAnnotation: stamp}},
		},
	})
	data, err := k.run(ctx, nil, append(slices.Clone(command), "--dry-run=server")...)
	if err != nil {
		return Plan{}, err
	}
	preview, err := decodeResource(data)
	if err != nil || !preview.matches(ref) || deploymentSpec(preview)["paused"] == true || restartTime(preview) != stamp {
		return Plan{}, errors.New(locales.KubeResourceUnverified())
	}
	if restartTime(obj) == stamp {
		return Plan{}, errors.New(locales.KubeResourceNoChanges())
	}
	change := &KubeChange{
		draft: KubeDraft{Ref: ref, Version: obj.version(), config: hash}, want: preview, rollout: true, patch: command, restart: true,
	}
	p := Plan{
		Title: locales.KubeRestartTitle(ref.Name), Why: locales.KubeRestartPreview(), Action: locales.KubeRestartAction(),
		Env: slices.Clone(env), Steps: []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, command...)}},
	}
	strategy, _ := spec["strategy"].(map[string]any)
	switch {
	case scaleOf(obj).desired == 0:
		p.Notes = []Note{{Title: locales.KubeReplicas(), Text: locales.KubeRestartZero()}}
	case strategy["type"] == "Recreate":
		p.Notes = []Note{{Title: locales.KubeResourceRollout(), Text: locales.KubeRestartRecreate()}}
	case scaleOf(obj).desired == 1:
		p.Notes = []Note{{Title: locales.KubeReplicas(), Text: locales.KubeRestartSingle()}}
	}
	return p, nil
}
