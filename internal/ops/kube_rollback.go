package ops

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strconv"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"

// PlanKubeRollback restores a specific ReplicaSet template and its annotations
// like rollout undo, using a versioned replace with the manifest on stdin.
func PlanKubeRollback(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource, name, uid string, revision int) (Plan, error) {
	rs := KubeResource{Kubectl: ref.Kubectl, Context: ref.Context, Kind: "ReplicaSet", Namespace: ref.Namespace, Name: name, UID: uid}
	if ref.Kind != "Deployment" || ref.path() == "" || rs.path() == "" || revision <= 0 {
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
	if resourceSpec(obj)["paused"] == true {
		return Plan{}, errors.New(locales.KubeRollbackPaused())
	}
	currentAnnotations, _ := obj.metadata()["annotations"].(map[string]any)
	if currentAnnotations[deploymentRevisionAnnotation] == strconv.Itoa(revision) {
		return Plan{}, errors.New(locales.KubeResourceChanged())
	}
	previous, err := k.read(ctx, rs)
	if err != nil {
		return Plan{}, err
	}
	if !previous.matches(rs) || previous.metadata()["deletionTimestamp"] != nil || !rollbackRevisionMatches(previous, ref, revision) {
		return Plan{}, errors.New(locales.KubeResourceChanged())
	}
	template, _ := resourceSpec(previous)["template"].(map[string]any)
	currentTemplate := resourceSpec(obj)["template"]
	if len(template) == 0 || currentTemplate == nil {
		return Plan{}, errors.New(locales.KubeResourceUnverified())
	}
	meta, _ := template["metadata"].(map[string]any)
	labels, _ := meta["labels"].(map[string]any)
	delete(labels, "pod-template-hash")
	if len(labels) == 0 {
		delete(meta, "labels")
	}
	if reflect.DeepEqual(currentTemplate, template) {
		return Plan{}, errors.New(locales.KubeResourceNoChanges())
	}

	next := maps.Clone(obj)
	next["metadata"] = maps.Clone(obj.metadata())
	next["spec"] = maps.Clone(resourceSpec(obj))
	resourceSpec(next)["template"] = template
	annotations := rollbackAnnotations(obj, previous)
	if len(annotations) == 0 {
		delete(next.metadata(), "annotations")
	} else {
		next.metadata()["annotations"] = annotations
	}
	change := &KubeChange{
		draft:    KubeDraft{Ref: ref, Version: obj.version(), config: hash},
		rollback: &KubeDraft{Ref: rs, Version: previous.version()},
		input:    next.manifest(), rollout: true,
	}
	data, err := k.run(ctx, change.input, "replace", "--raw", ref.path()+"?fieldValidation=Strict&dryRun=All", "-f", "-")
	if err != nil {
		return Plan{}, err
	}
	preview, err := decodeResource(data)
	if err != nil || !preview.matches(ref) || !reflect.DeepEqual(cleanResource(preview), cleanResource(next)) {
		return Plan{}, errors.New(locales.KubeResourceUnverified())
	}
	change.base, change.want = cleanResource(obj), preview
	before, after := rollbackPreview(change.base), rollbackPreview(change.want)
	if !kubeManifestFits(before) || !kubeManifestFits(after) {
		return Plan{}, errors.New(locales.KubeResourceTooLarge())
	}
	change.Diff = Diff(before, after)
	return Plan{
		Title: locales.KubeRollbackTitle(ref.Name, revision), Why: locales.KubeRollbackPreview(), Action: locales.KubeRollbackAction(),
		Env: slices.Clone(env), Steps: []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, change.command()...)}},
	}, nil
}

func rollbackRevisionMatches(rs resourceJSON, ref KubeResource, revision int) bool {
	annotations, _ := rs.metadata()["annotations"].(map[string]any)
	if annotations[deploymentRevisionAnnotation] != strconv.Itoa(revision) {
		return false
	}
	owners, _ := rs.metadata()["ownerReferences"].([]any)
	return slices.ContainsFunc(owners, func(value any) bool {
		owner, _ := value.(map[string]any)
		return owner["controller"] == true && owner["kind"] == "Deployment" && owner["name"] == ref.Name && owner["uid"] == ref.UID
	})
}

func rollbackAnnotations(obj, rs resourceJSON) map[string]any {
	// kubectl keeps these Deployment annotations through rollout undo.
	protected := []string{
		"kubectl.kubernetes.io/last-applied-configuration", deploymentRevisionAnnotation,
		"deployment.kubernetes.io/revision-history", "deployment.kubernetes.io/desired-replicas",
		"deployment.kubernetes.io/max-replicas", "deprecated.deployment.rollback.to",
		"scheduling.k8s.io/workload-name", "scheduling.k8s.io/group-template-name",
		"scheduling.k8s.io/parent-compositepodgroup",
	}
	current, _ := obj.metadata()["annotations"].(map[string]any)
	saved, _ := rs.metadata()["annotations"].(map[string]any)
	result := make(map[string]any)
	for name, value := range saved {
		if !slices.Contains(protected, name) {
			result[name] = value
		}
	}
	for _, name := range protected {
		if value, ok := current[name]; ok {
			result[name] = value
		}
	}
	return result
}

func rollbackPreview(obj resourceJSON) string {
	data, _ := json.MarshalIndent(map[string]any{
		"annotations": obj.metadata()["annotations"],
		"template":    resourceSpec(obj)["template"],
	}, "", "  ")
	return string(data)
}
