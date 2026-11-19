package ops

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func PlanKubeCordon(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource, cordon bool) (Plan, error) {
	if ref.Kind != "Node" || ref.path() == "" {
		return Plan{}, errors.New(locales.KubeResourceTarget())
	}
	change, err := previewKubeToggle(ctx, r, env, ref, "node", "unschedulable", cordon)
	if err != nil {
		return Plan{}, err
	}
	title, why, action := locales.KubeNodeUncordonTitle(ref.Name), locales.KubeNodeUncordonWhy(), locales.KubeNodeUncordonAction()
	if cordon {
		title, why, action = locales.KubeNodeCordonTitle(ref.Name), locales.KubeNodeCordonWhy(), locales.KubeNodeCordonAction()
	}
	return Plan{Title: title, Why: why, Action: action, Env: slices.Clone(env),
		Steps: []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, change.command()...)}}}, nil
}

type drainPod struct {
	Namespace, Name, UID, Owner, Skip string
	EmptyDir                          bool
}

func (p drainPod) label() string { return p.Namespace + "/" + p.Name }

func (p drainPod) ref(node KubeResource) KubeResource {
	return KubeResource{Kubectl: node.Kubectl, Context: node.Context, Kind: "Pod", Namespace: p.Namespace, Name: p.Name, UID: p.UID}
}

type nodeDrain struct{ pods []drainPod }

func nodePodsCommand(name string) []string {
	return []string{"get", "pods", "--all-namespaces", "--field-selector=spec.nodeName=" + name, "-o", "json"}
}

func (k resourceClient) nodePods(ctx context.Context, name string) ([]drainPod, error) {
	data, err := k.run(ctx, nil, nodePodsCommand(name)...)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []struct {
			Metadata struct {
				Namespace, Name, UID string
				Annotations          map[string]string
				OwnerReferences      []struct {
					Kind, Name string
					Controller bool
				}
			}
			Spec struct {
				NodeName string
				Volumes  []struct{ EmptyDir *struct{} }
			}
		}
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("read drain pods: %w", err)
	}
	pods := make([]drainPod, 0, len(list.Items))
	for _, item := range list.Items {
		m := item.Metadata
		if item.Spec.NodeName != name || m.UID == "" || !KubeNamespaceValid(m.Namespace) || !kubeResourceName.MatchString(m.Name) {
			return nil, errors.New(locales.KubeResourceUnverified())
		}
		p := drainPod{Namespace: m.Namespace, Name: m.Name, UID: m.UID}
		for _, owner := range m.OwnerReferences {
			if owner.Controller {
				p.Owner = owner.Kind + " " + owner.Name
				if owner.Kind == "DaemonSet" {
					p.Skip = "DaemonSet"
				}
				break
			}
		}
		if _, mirror := m.Annotations["kubernetes.io/config.mirror"]; mirror {
			p.Skip = "Static Pod"
		}
		p.EmptyDir = slices.ContainsFunc(item.Spec.Volumes, func(volume struct{ EmptyDir *struct{} }) bool { return volume.EmptyDir != nil })
		pods = append(pods, p)
	}
	slices.SortFunc(pods, func(a, b drainPod) int { return cmp.Compare(a.label(), b.label()) })
	return pods, nil
}

func evictionCommand(p KubeResource) []string {
	return []string{"create", "--raw", p.path() + "/eviction", "-f", "-"}
}

func (k resourceClient) evict(ctx context.Context, p KubeResource, dry bool) error {
	options := map[string]any{"preconditions": map[string]string{"uid": p.UID}}
	if dry {
		options["dryRun"] = []string{"All"}
	}
	data, _ := json.Marshal(map[string]any{
		"apiVersion": "policy/v1", "kind": "Eviction",
		"metadata":      map[string]string{"name": p.Name, "namespace": p.Namespace},
		"deleteOptions": options,
	})
	_, err := k.run(ctx, data, evictionCommand(p)...)
	return err
}

func evictionWaiting(err error) bool {
	e, ok := errors.AsType[*resourceCommandError](err)
	return ok && strings.HasPrefix(e.detail, "Error from server (TooManyRequests):")
}

func PlanKubeDrain(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource) (Plan, error) {
	if ref.Kind != "Node" || ref.path() == "" {
		return Plan{}, errors.New(locales.KubeResourceTarget())
	}
	client := resourceClient{cli: ref.Kubectl, context: ref.Context, env: env, runner: r}
	k, hash, closeConfig, err := client.pin(ctx, [32]byte{})
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
	pods, err := k.nodePods(ctx, ref.Name)
	if err != nil {
		return Plan{}, err
	}
	var notes []Note
	for _, p := range pods {
		switch {
		case p.Skip != "":
			notes = append(notes, Note{Title: p.label(), Text: locales.KubeNodeDrainSkip(p.Skip)})
		case p.EmptyDir:
			return Plan{}, errors.New(locales.KubeNodeDrainEmptyDir(p.label()))
		case p.Owner == "":
			return Plan{}, errors.New(locales.KubeNodeDrainUnmanaged(p.label()))
		default:
			text := locales.KubeNodeDrainEvict(p.Owner)
			if err := k.evict(ctx, p.ref(ref), true); evictionWaiting(err) {
				text += "\n" + err.Error()
			} else if err != nil {
				return Plan{}, fmt.Errorf("%s: %w", p.label(), err)
			}
			notes = append(notes, Note{Title: p.label(), Text: text})
		}
	}
	change := &KubeChange{
		draft: KubeDraft{Ref: ref, Version: obj.version(), config: hash},
		drain: &nodeDrain{pods: pods},
	}
	if resourceSpec(obj)["unschedulable"] != true {
		change.patch = resourceSpecPatch(ref, "node", obj.version(), map[string]any{"unschedulable": true})
		data, err := k.run(ctx, nil, append(slices.Clone(change.patch), "--dry-run=server")...)
		if err != nil {
			return Plan{}, err
		}
		preview, err := decodeResource(data)
		if err != nil || !preview.matches(ref) || resourceSpec(preview)["unschedulable"] != true {
			return Plan{}, errors.New(locales.KubeResourceUnverified())
		}
	}
	status, _ := obj["status"].(map[string]any)
	conditions, _ := status["conditions"].([]any)
	if !slices.ContainsFunc(conditions, func(raw any) bool {
		c, _ := raw.(map[string]any)
		return c["type"] == "Ready" && c["status"] == "True"
	}) {
		notes = append(notes, Note{Title: locales.KubeNodeNotReady(), Text: locales.KubeNodeDrainNotReady()})
	}
	args := change.patch
	if args == nil {
		args = nodePodsCommand(ref.Name)
		if i := slices.IndexFunc(pods, func(p drainPod) bool { return p.Skip == "" }); i >= 0 {
			args = evictionCommand(pods[i].ref(ref))
		}
	}
	return Plan{Title: locales.KubeNodeDrainTitle(ref.Name), Why: locales.KubeNodeDrainWhy(),
		Action: locales.KubeNodeDrainAction(), Env: slices.Clone(env), Notes: notes,
		Steps: []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, args...)}}}, nil
}

func (m *KubeChange) applyNodeDrain(ctx context.Context, k resourceClient, checking func()) (string, error) {
	ref := m.draft.Ref
	pods, err := k.nodePods(ctx, ref.Name)
	if err != nil {
		return "", err
	}
	if !slices.Equal(pods, m.drain.pods) {
		return "", errors.New(locales.KubeResourceChanged())
	}
	var cordonErr error
	if m.patch != nil {
		_, cordonErr = k.run(ctx, nil, m.patch...)
	}
	checking()
	if err := k.verifyCordon(ctx, ref); err != nil {
		return "", errors.Join(cordonErr, err)
	}

	wait, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	next, err := k.evictNodePods(wait, ref, pods)
	if err != nil {
		return "", fmt.Errorf("%s: %w", locales.KubeNodeDrainIncomplete(next), err)
	}
	return locales.KubeNodeDrainVerified(), nil
}

func (k resourceClient) verifyCordon(ctx context.Context, ref KubeResource) error {
	obj, err := k.read(ctx, ref)
	if err != nil {
		return err
	}
	if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil || resourceSpec(obj)["unschedulable"] != true {
		return errors.New(locales.KubeResourceChanged())
	}
	return nil
}

func (k resourceClient) evictNodePods(ctx context.Context, node KubeResource, approved []drainPod) (string, error) {
	requested := map[string]bool{}
	allowed := map[string]drainPod{}
	for _, pod := range approved {
		allowed[pod.UID] = pod
	}
	var pending string
	for {
		if err := k.verifyCordon(ctx, node); err != nil {
			return pending, err
		}
		pods, err := k.nodePods(ctx, node.Name)
		if err != nil {
			return pending, err
		}
		pending = ""
		for _, pod := range pods {
			if pod.Skip != "" {
				continue
			}
			if allowed[pod.UID] != pod {
				return pod.label(), errors.New(locales.KubeResourceChanged())
			}
			pending = pod.label()
			if !requested[pod.UID] {
				err := k.evict(ctx, pod.ref(node), false)
				switch {
				case err == nil:
					requested[pod.UID] = true
				case evictionWaiting(err):
					pending += " · " + err.Error()
				default:
					return pending, err
				}
			}
		}
		if pending == "" {
			return "", nil
		}
		select {
		case <-ctx.Done():
			return pending, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}
