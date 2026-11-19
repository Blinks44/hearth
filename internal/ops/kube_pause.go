package ops

import (
	"context"
	"errors"
	"reflect"
	"slices"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// PlanKubePause sets the same field as rollout pause/resume with API
// preconditions and a server-side preview.
func PlanKubePause(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource, pause bool) (Plan, error) {
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
	if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil || (deploymentSpec(obj)["paused"] == true) == pause {
		return Plan{}, errors.New(locales.KubeResourceChanged())
	}
	version := obj.version()
	command := deploymentPatch(ref, version, map[string]any{"paused": pause})
	data, err := k.run(ctx, nil, append(slices.Clone(command), "--dry-run=server")...)
	if err != nil {
		return Plan{}, err
	}
	preview, err := decodeResource(data)
	if err != nil || !preview.matches(ref) || (deploymentSpec(preview)["paused"] == true) != pause {
		return Plan{}, errors.New(locales.KubeResourceUnverified())
	}
	base, want := cleanResource(obj), cleanResource(preview)
	pausedSpec := map[string]any{}
	if paused, ok := deploymentSpec(want)["paused"]; ok {
		pausedSpec["paused"] = paused
	}
	delete(deploymentSpec(base), "paused")
	delete(deploymentSpec(want), "paused")
	if !reflect.DeepEqual(base, want) {
		return Plan{}, errors.New(locales.KubeResourceUnverified())
	}
	// Kubernetes omits spec.paused when false. Check that the field has the
	// shape returned by the dry run, independently of controller updates.
	change := &KubeChange{
		draft: KubeDraft{Ref: ref, Version: version, config: hash}, patch: command,
		base: resourceJSON{"spec": map[string]any{"paused": !pause}},
		want: resourceJSON{"spec": pausedSpec},
	}
	p := Plan{
		Title: locales.KubeResumeTitle(ref.Name), Why: locales.KubeResumePreview(), Action: locales.KubeResumeAction(),
		Env: slices.Clone(env), Steps: []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, command...)}},
	}
	if pause {
		p.Title, p.Why, p.Action = locales.KubePauseTitle(ref.Name), locales.KubePausePreview(), locales.KubePauseAction()
	}
	return p, nil
}
