package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const KubeMaxReplicas = math.MaxInt32 // replicas is an int32 in the Kubernetes API.

type deploymentScale struct{ from, to int }

func (s deploymentScale) command(d KubeDraft) []string {
	return []string{"scale", "deployment.apps/" + d.Ref.Name, "--namespace=" + d.Ref.Namespace,
		"--current-replicas=" + strconv.Itoa(s.from), "--replicas=" + strconv.Itoa(s.to), "--resource-version=" + d.Version}
}

// PlanKubeScale checks the Deployment currently on the server, then previews
// the scale subresource update with the same preconditions as the real write.
func PlanKubeScale(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource, from, to int) (Plan, error) {
	if ref.Kind != "Deployment" || ref.path() == "" {
		return Plan{}, errors.New(locales.KubeResourceTarget())
	}
	if from < 0 || from > KubeMaxReplicas || to < 0 || to > KubeMaxReplicas || to == from {
		return Plan{}, errors.New(locales.KubeScaleInvalid())
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
	if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil || scaleOf(obj).desired != from {
		return Plan{}, errors.New(locales.KubeResourceChanged())
	}
	change := &KubeChange{
		draft: KubeDraft{Ref: ref, Version: obj.version(), config: hash},
		scale: &deploymentScale{from, to},
	}
	command := change.command()
	if _, err := k.run(ctx, nil, append(slices.Clone(command), "--dry-run=server")...); err != nil {
		return Plan{}, err
	}
	p := Plan{
		Title: locales.KubeScaleTitle(ref.Name), Why: locales.KubeScalePreview(from, to), Action: locales.KubeResourceApply(),
		Env: slices.Clone(env), Steps: []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, command...)}},
	}
	effect := locales.KubeScaleAdds()
	switch {
	case to == 0:
		effect = locales.KubeScaleStops()
	case to < from:
		effect = locales.KubeScaleRemoves()
	}
	p.Notes = []Note{{Title: locales.KubeReplicas(), Text: effect}}
	names, err := scaleAutoscalers(ctx, k, ref)
	switch {
	case err != nil:
		p.Notes = append(p.Notes, Note{Title: locales.KubeScaleAutoscaler(), Text: locales.KubeScaleAutoscalerUnknown()})
	case len(names) > 0:
		p.Notes = append(p.Notes, Note{Title: locales.KubeScaleAutoscaler(), Text: locales.KubeScaleAutoscalerManages(strings.Join(names, ", "))})
	}
	return p, nil
}

func scaleAutoscalers(ctx context.Context, k resourceClient, ref KubeResource) ([]string, error) {
	data, err := k.run(ctx, nil, "get", "horizontalpodautoscalers.autoscaling", "--namespace="+ref.Namespace, "-o", "json")
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []struct {
			Metadata struct{ Name string }
			Spec     struct {
				ScaleTargetRef kube.ScaleTarget
			}
		}
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("read autoscalers: %w", err)
	}
	var names []string
	for _, h := range list.Items {
		if h.Spec.ScaleTargetRef.TargetsDeployment(ref.Name) {
			names = append(names, h.Metadata.Name)
		}
	}
	slices.Sort(names)
	return names, nil
}

type scaleStatus struct {
	desired, actual, ready         int
	generation, observedGeneration int64
}

func scaleOf(obj resourceJSON) scaleStatus {
	spec, _ := obj["spec"].(map[string]any)
	status, _ := obj["status"].(map[string]any)
	number := func(m map[string]any, key string) int64 {
		n, _ := m[key].(json.Number)
		value, _ := n.Int64()
		return value
	}
	return scaleStatus{
		desired: int(number(spec, "replicas")), actual: int(number(status, "replicas")), ready: int(number(status, "readyReplicas")),
		generation: number(obj.metadata(), "generation"), observedGeneration: number(status, "observedGeneration"),
	}
}

func (m *KubeChange) verifyScale(ctx context.Context, k resourceClient, obj resourceJSON, writeErr, readErr error) (string, error) {
	ref, to := m.draft.Ref, m.scale.to
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var pending string
	for {
		if readErr != nil {
			if ctx.Err() != nil && pending != "" {
				return "", errors.New(pending)
			}
			return "", fmt.Errorf("%s: %w", locales.KubeResourceUnverified(), errors.Join(writeErr, readErr))
		}
		if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil {
			return "", errors.New(locales.KubeResourceChanged())
		}
		status := scaleOf(obj)
		if status.desired != to {
			if writeErr != nil {
				return "", fmt.Errorf("%s: %w", locales.KubeResourceUnverified(), writeErr)
			}
			return "", errors.New(locales.KubeScaleChanged(to, status.desired))
		}
		if status.observedGeneration >= status.generation && status.actual == to && status.ready == to {
			return locales.KubeScaleVerified(to), nil
		}
		pending = locales.KubeScalePending(to, status.ready, status.actual)
		select {
		case <-ctx.Done():
			return "", errors.New(pending)
		case <-ticker.C:
			obj, readErr = k.read(ctx, ref)
		}
	}
}
