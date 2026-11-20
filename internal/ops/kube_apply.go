package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type KubeManifestSource struct {
	Path, Context, Namespace string
}

type KubeManifestPreview struct {
	KubeManifestSource
	Kustomize              bool
	Created, Unchanged     []string
	Changed                []KubeManifestDiff
	inputHash, previewHash [32]byte
}

type KubeManifestDiff struct {
	Resource string
	Lines    []DiffLine
}

type manifestID struct {
	Version, Kind, Namespace, Name string
}

func (id manifestID) label() string {
	return (KubeResource{Kind: id.Kind, Namespace: id.Namespace, Name: id.Name}).label()
}

func (id manifestID) rollout() bool {
	return id.Version == "apps/v1" && (id.Kind == "Deployment" || id.Kind == "DaemonSet" || id.Kind == "StatefulSet")
}

type manifestSnapshot struct {
	input          []byte
	ids            []manifestID
	source         map[manifestID]resourceJSON
	live, admitted map[manifestID]resourceJSON
	versions       map[manifestID]string
}

func PlanKubeManifests(ctx context.Context, r run.Runner, env sysenv.Env, cli, title string, source KubeManifestSource) (Plan, error) {
	if !filepath.IsAbs(source.Path) || cli == "" || source.Context == "" || !KubeNamespaceValid(source.Namespace) {
		return Plan{}, errors.New(locales.KubeApplySourceInvalid())
	}
	env = env.KubectlDiff()
	k, config, closeConfig, err := (resourceClient{cli: cli, context: source.Context, env: env, runner: r}).pin(ctx, [32]byte{})
	if err != nil {
		return Plan{}, err
	}
	defer closeConfig()
	p := &KubeManifestPreview{KubeManifestSource: source}
	snapshot, kustomize, err := p.snapshot(ctx, k)
	if err != nil {
		return Plan{}, err
	}
	p.Kustomize = kustomize
	p.inputHash, p.previewHash = sha256.Sum256(snapshot.input), snapshot.fingerprint()
	diffLines := 0
	for _, id := range snapshot.ids {
		before, after := snapshot.live[id], snapshot.admitted[id]
		switch {
		case before == nil:
			p.Created = append(p.Created, id.label())
		case reflect.DeepEqual(before, after):
			p.Unchanged = append(p.Unchanged, id.label())
		default:
			change := KubeManifestDiff{Resource: id.label()}
			// Secret values must not enter the dialog, an operation journal or a retained plan.
			if id.Kind != "Secret" {
				a, _ := json.MarshalIndent(before, "", "  ")
				b, _ := json.MarshalIndent(after, "", "  ")
				if !kubeManifestFits(string(a)) || !kubeManifestFits(string(b)) {
					return Plan{}, errors.New(locales.KubeResourceTooLarge())
				}
				change.Lines = Diff(string(a), string(b))
			}
			diffLines += len(change.Lines)
			if diffLines > kubeMaxDiffLines {
				return Plan{}, errors.New(locales.KubeResourceTooLarge())
			}
			p.Changed = append(p.Changed, change)
		}
	}
	change := &KubeChange{draft: KubeDraft{Ref: KubeResource{Kubectl: cli, Context: source.Context}, config: config}, Manifests: p}
	shown := append([]string{cli, "--context=" + source.Context}, p.command("apply")...)
	shown[len(shown)-1] = source.Path
	if kustomize {
		shown[len(shown)-2] = "-k"
	}
	return Plan{
		Title: locales.KubeApplyTitle(title), Why: locales.KubeApplyReviewed(source.Context, source.Namespace),
		Action: locales.KubeApplyConfirm(p.Changes()), Env: env,
		Steps: []Step{{Kube: change, Argv: shown}},
	}, nil
}

func (p *KubeManifestPreview) Changes() int { return len(p.Created) + len(p.Changed) }

func (p *KubeManifestPreview) command(verb string) []string {
	return []string{verb, "--namespace=" + p.Namespace, "--server-side", "--field-manager=hearth", "-f", "-"}
}

func (p *KubeManifestPreview) snapshot(ctx context.Context, k resourceClient) (manifestSnapshot, bool, error) {
	var s manifestSnapshot
	sourceFlag, err := manifestSourceFlag(p.Path)
	if err != nil {
		return s, false, err
	}
	rendered, err := k.run(ctx, nil, "create", "--namespace="+p.Namespace, "--dry-run=client", "--validate=false", sourceFlag, p.Path, "-o", "json")
	if err != nil {
		return s, false, manifestError("create --dry-run=client", err)
	}
	objects, ids, err := manifestObjects(rendered)
	if err != nil || len(ids) == 0 {
		return s, false, errors.New(locales.KubeApplySourceInvalid())
	}
	list := make([]resourceJSON, 0, len(ids))
	for _, id := range ids {
		if id.Namespace != "" && id.Namespace != p.Namespace {
			return s, false, errors.New(locales.KubeApplySourceInvalid())
		}
		list = append(list, objects[id])
	}
	s.input = manifestList(list)
	s.ids, s.source = ids, objects
	// Get first, so an object created or replaced during the dry run cannot
	// be mistaken for the one the user reviewed.
	s.live, err = p.read(ctx, k, s.input)
	if err != nil {
		return s, false, err
	}
	data, err := k.run(ctx, s.input, append(p.command("apply"), "--dry-run=server", "-o", "json")...)
	if err != nil {
		return s, false, manifestError("apply --dry-run=server", err)
	}
	s.admitted, _, err = manifestObjects(data)
	if err != nil || len(s.admitted) != len(ids) {
		return s, false, errors.New(locales.KubeResourceManifest())
	}
	if len(sysenv.LookPathAll("diff", k.env)) > 0 {
		diff, err := k.call(ctx, 45*time.Second, "30s", s.input, p.command("diff")...)
		if err != nil {
			return s, false, manifestError("diff", err)
		}
		if diff.ExitCode != 0 && (diff.ExitCode != 1 || len(diff.Stdout) == 0) {
			return s, false, manifestError("diff", &resourceCommandError{"diff", string(diff.Stderr)})
		}
	}
	s.versions = make(map[manifestID]string, len(s.live))
	for _, id := range ids {
		after := s.admitted[id]
		before := s.live[id]
		if after == nil || before != nil && (before.metadata()["uid"] != after.metadata()["uid"] || before.metadata()["deletionTimestamp"] != nil) {
			return s, false, errors.New(locales.KubeApplyStale())
		}
		if before != nil {
			s.versions[id] = before.version()
		}
		normalizeManifest(before, before != nil)
		normalizeManifest(after, before != nil)
	}
	return s, sourceFlag == "-k", nil
}

func manifestList(objects []resourceJSON) []byte {
	data, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "List", "items": objects})
	return data
}

func manifestSourceFlag(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", locales.KubeApplySourceInvalid(), err)
	}
	if info.IsDir() {
		for _, name := range []string{"kustomization.yaml", "kustomization.yml", "Kustomization"} {
			if st, err := os.Stat(filepath.Join(path, name)); err == nil && st.Mode().IsRegular() {
				return "-k", nil
			}
		}
		if _, err := os.Stat(filepath.Join(path, "Chart.yaml")); !errors.Is(err, os.ErrNotExist) {
			return "", errors.New(locales.KubeApplySourceInvalid())
		}
		return "-f", nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json":
		if info.Mode().IsRegular() {
			return "-f", nil
		}
	}
	return "", errors.New(locales.KubeApplySourceInvalid())
}

// kubectl get returns a single object or List. Create and apply may print
// consecutive JSON objects for a multi-document source.
func manifestObjects(data []byte) (map[manifestID]resourceJSON, []manifestID, error) {
	result := make(map[manifestID]resourceJSON)
	var ids []manifestID
	add := func(obj resourceJSON) bool {
		id := manifestID{}
		id.Version, _ = obj["apiVersion"].(string)
		id.Kind, _ = obj["kind"].(string)
		id.Namespace, _ = obj.metadata()["namespace"].(string)
		id.Name, _ = obj.metadata()["name"].(string)
		if id.Version == "" || id.Kind == "" || !kubeResourceName.MatchString(id.Name) ||
			id.Namespace != "" && !KubeNamespaceValid(id.Namespace) || result[id] != nil {
			return false
		}
		result[id] = obj
		ids = append(ids, id)
		return true
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	invalid := errors.New(locales.KubeResourceManifest())
	for {
		var obj resourceJSON
		switch err := d.Decode(&obj); {
		case errors.Is(err, io.EOF):
			return result, ids, nil
		case err != nil:
			return result, ids, invalid
		}
		if obj["apiVersion"] == "v1" && obj["kind"] == "List" {
			items, _ := obj["items"].([]any)
			for _, item := range items {
				o, _ := item.(map[string]any)
				if !add(o) {
					return result, ids, invalid
				}
			}
		} else if !add(obj) {
			return result, ids, invalid
		}
	}
}

func normalizeManifest(obj resourceJSON, exists bool) {
	cleanResource(obj)
	delete(obj.metadata(), "selfLink")
	annotations, _ := obj.metadata()["annotations"].(map[string]any)
	delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	if len(annotations) == 0 {
		delete(obj.metadata(), "annotations")
	}
	if exists {
		return
	}
	delete(obj.metadata(), "uid")
	// These allocations from a dry run can differ from the actual create.
	if obj["kind"] == "Service" && obj["apiVersion"] == "v1" {
		spec := resourceSpec(obj)
		for _, k := range []string{"clusterIP", "clusterIPs", "healthCheckNodePort"} {
			delete(spec, k)
		}
		ports, _ := spec["ports"].([]any)
		for _, value := range ports {
			port, _ := value.(map[string]any)
			delete(port, "nodePort")
		}
	}
}

func (s manifestSnapshot) fingerprint() [32]byte {
	objects := make([][2]resourceJSON, 0, len(s.ids))
	for _, id := range s.ids {
		objects = append(objects, [2]resourceJSON{s.live[id], s.admitted[id]})
	}
	data, _ := json.Marshal(objects)
	return sha256.Sum256(data)
}

func (p *KubeManifestPreview) read(ctx context.Context, k resourceClient, input []byte) (map[manifestID]resourceJSON, error) {
	data, err := k.run(ctx, input, "get", "--namespace="+p.Namespace, "--ignore-not-found", "-f", "-", "-o", "json")
	if err != nil {
		return nil, manifestError("get", err)
	}
	objects, _, err := manifestObjects(data)
	return objects, err
}

func (p *KubeManifestPreview) apply(ctx context.Context, k resourceClient, checking func()) (string, error) {
	if p.Changes() == 0 {
		return "", errors.New(locales.KubeResourceNoChanges())
	}
	s, _, err := p.snapshot(ctx, k)
	if err != nil {
		return "", err
	}
	if p.inputHash != sha256.Sum256(s.input) || p.previewHash != s.fingerprint() {
		return "", errors.New(locales.KubeApplyStale())
	}
	var writes []resourceJSON
	for _, id := range s.ids {
		if reflect.DeepEqual(s.live[id], s.admitted[id]) {
			continue
		}
		obj := s.source[id]
		if version := s.versions[id]; version != "" {
			obj.metadata()["resourceVersion"] = version
		}
		writes = append(writes, obj)
	}
	res, writeErr := k.call(ctx, 45*time.Second, "30s", manifestList(writes), append(p.command("apply"), "-o", "json")...)
	written, _, parseErr := manifestObjects(res.Stdout)
	checking()
	check, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
	defer cancel()
	current, err := p.read(check, k, s.input)
	if err != nil {
		return "", fmt.Errorf("%s: %w", locales.KubeResourceUnverified(), err)
	}
	var verified, pending, rollouts []string
	for _, id := range s.ids {
		obj := current[id]
		if !manifestApplied(id, s, written[id], obj) {
			pending = append(pending, id.label())
			continue
		}
		verified = append(verified, id.label())
		before, after := resourceSpec(s.live[id]), resourceSpec(s.admitted[id])
		if id.rollout() && after["paused"] != true && !reflect.DeepEqual(before, after) {
			rollouts = append(rollouts, strings.ToLower(id.Kind)+"/"+id.Name)
		}
	}
	if writeErr != nil || parseErr != nil || res.ExitCode != 0 || len(pending) > 0 {
		return "", errors.New(locales.KubeApplyPartial(len(verified), strings.Join(pending, ", ")))
	}
	if len(rollouts) > 0 {
		if _, err := k.runFor(ctx, 5*time.Minute+5*time.Second, "0", nil, append([]string{
			"rollout", "status", "--namespace=" + p.Namespace, "--timeout=5m",
		}, rollouts...)...); err != nil {
			return "", fmt.Errorf("%s: %w", locales.KubeApplyVerified(len(verified)), manifestError("rollout status", err))
		}
		current, err := p.read(ctx, k, s.input)
		if err != nil {
			return "", err
		}
		for _, id := range s.ids {
			if !manifestApplied(id, s, written[id], current[id]) {
				return "", errors.New(locales.KubeApplyStale())
			}
		}
	}
	return locales.KubeApplyVerified(len(verified)), nil
}

func manifestApplied(id manifestID, s manifestSnapshot, written, current resourceJSON) bool {
	before, want := s.live[id], s.admitted[id]
	if current == nil || current.metadata()["deletionTimestamp"] != nil {
		return false
	}
	observedUID := current.metadata()["uid"]
	normalizeManifest(current, before != nil)
	if reflect.DeepEqual(before, want) {
		return reflect.DeepEqual(want, current)
	}
	uid := written.metadata()["uid"]
	if uid == nil || uid != observedUID || (before != nil && uid != before.metadata()["uid"]) {
		return false
	}
	// For creations normalizeManifest removes the UID from the comparison,
	// but the read must still be of the object returned by apply.
	if before == nil {
		return manifestContains(want, current)
	}
	return approvedChanges(before, want, current)
}

// New objects can acquire extra fields from controllers between apply and get.
func manifestContains(want, got map[string]any) bool {
	for key, value := range want {
		actual, exists := got[key]
		wantMap, wantOK := value.(map[string]any)
		gotMap, gotOK := actual.(map[string]any)
		if wantOK && gotOK {
			if !manifestContains(wantMap, gotMap) {
				return false
			}
		} else if !exists || !reflect.DeepEqual(value, actual) {
			return false
		}
	}
	return true
}

// kubectl can put manifests and fields from a rejected Secret in stderr.
func manifestError(command string, err error) error {
	detail := err.Error()
	switch {
	case strings.Contains(detail, "(Forbidden)"):
		return fmt.Errorf("kubectl %s: %s", command, locales.KubeApplyForbidden())
	case strings.Contains(detail, "conflict"):
		return fmt.Errorf("kubectl %s: %s", command, locales.KubeApplyConflict())
	default:
		return errors.New(locales.KubeApplyCommandFailed(command))
	}
}
