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
	change, err := previewKubeToggle(ctx, r, env, ref, "deployment.apps", "paused", pause)
	if err != nil {
		return Plan{}, err
	}
	title, why, action := locales.KubeResumeTitle(ref.Name), locales.KubeResumePreview(), locales.KubeResumeAction()
	if pause {
		title, why, action = locales.KubePauseTitle(ref.Name), locales.KubePausePreview(), locales.KubePauseAction()
	}
	return Plan{
		Title: title, Why: why, Action: action, Env: slices.Clone(env),
		Steps: []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, change.command()...)}},
	}, nil
}

func previewKubeToggle(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource, resource, field string, on bool) (*KubeChange, error) {
	k, hash, closeConfig, err := (resourceClient{cli: ref.Kubectl, context: ref.Context, env: env, runner: r}).pin(ctx, [32]byte{})
	if err != nil {
		return nil, err
	}
	defer closeConfig()
	obj, err := k.read(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil || (resourceSpec(obj)[field] == true) == on {
		return nil, errors.New(locales.KubeResourceChanged())
	}
	version := obj.version()
	command := resourceSpecPatch(ref, resource, version, map[string]any{field: on})
	data, err := k.run(ctx, nil, append(slices.Clone(command), "--dry-run=server")...)
	if err != nil {
		return nil, err
	}
	preview, err := decodeResource(data)
	if err != nil || !preview.matches(ref) || (resourceSpec(preview)[field] == true) != on {
		return nil, errors.New(locales.KubeResourceUnverified())
	}
	base, want := cleanResource(obj), cleanResource(preview)
	newSpec := map[string]any{}
	if value, ok := resourceSpec(want)[field]; ok {
		newSpec[field] = value
	}
	delete(resourceSpec(base), field)
	delete(resourceSpec(want), field)
	if !reflect.DeepEqual(base, want) {
		return nil, errors.New(locales.KubeResourceUnverified())
	}
	// Kubernetes can omit a false field. Check that the field has the
	// shape returned by the dry run, independently of controller updates.
	return &KubeChange{
		draft: KubeDraft{Ref: ref, Version: version, config: hash}, patch: command,
		base: resourceJSON{"spec": map[string]any{field: !on}},
		want: resourceJSON{"spec": newSpec},
	}, nil
}
