package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// KubeResource identifies a row from the cluster view.
type KubeResource struct {
	Kubectl, Context, Kind, Namespace, Name, UID string
}

type kubeResourceType struct {
	group, version, plural string
}

var kubeResourceTypes = map[string]kubeResourceType{
	"Node": {"", "v1", "nodes"}, "Namespace": {"", "v1", "namespaces"}, "Pod": {"", "v1", "pods"}, "Service": {"", "v1", "services"},
	"Deployment": {"apps", "v1", "deployments"},
	"ReplicaSet": {"apps", "v1", "replicasets"},
	"Job":        {"batch", "v1", "jobs"}, "CronJob": {"batch", "v1", "cronjobs"},
}

var kubeResourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,251}[a-z0-9]$|^[a-z0-9]$`)

func (ref KubeResource) path() string {
	typ, ok := kubeResourceTypes[ref.Kind]
	if !ok || ref.Kubectl == "" || ref.Context == "" || ref.UID == "" ||
		!kubeResourceName.MatchString(ref.Name) {
		return ""
	}
	api := "/api/" + typ.version
	if typ.group != "" {
		api = "/apis/" + typ.group + "/" + typ.version
	}
	if ref.Kind == "Node" || ref.Kind == "Namespace" {
		if ref.Namespace != "" || ref.Kind == "Namespace" && !KubeNamespaceValid(ref.Name) {
			return ""
		}
		return api + "/" + typ.plural + "/" + ref.Name
	}
	if !KubeNamespaceValid(ref.Namespace) {
		return ""
	}
	return api + "/namespaces/" + ref.Namespace + "/" + typ.plural + "/" + ref.Name
}

func (ref KubeResource) label() string {
	if ref.Namespace == "" {
		return ref.Kind + " " + ref.Name
	}
	return ref.Kind + " " + ref.Namespace + "/" + ref.Name
}

type resourceJSON map[string]any

func decodeResource(data []byte) (resourceJSON, error) {
	var obj resourceJSON
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(&obj); err != nil || obj == nil {
		return nil, errors.New(locales.KubeResourceManifest())
	}
	if !json.Valid(data) {
		return nil, errors.New(locales.KubeResourceManifest())
	}
	return obj, nil
}

func (obj resourceJSON) metadata() map[string]any {
	m, _ := obj["metadata"].(map[string]any)
	return m
}

func resourceSpec(obj resourceJSON) map[string]any {
	spec, _ := obj["spec"].(map[string]any)
	return spec
}

func (obj resourceJSON) version() string {
	rv, _ := obj.metadata()["resourceVersion"].(string)
	return rv
}

func (obj resourceJSON) matches(ref KubeResource) bool {
	typ := kubeResourceTypes[ref.Kind]
	apiVersion := typ.version
	if typ.group != "" {
		apiVersion = typ.group + "/" + apiVersion
	}
	m := obj.metadata()
	namespaceMatches := m["namespace"] == ref.Namespace
	if ref.Kind == "Node" || ref.Kind == "Namespace" {
		namespaceMatches = m["namespace"] == nil || m["namespace"] == ""
	}
	return obj["apiVersion"] == apiVersion && obj["kind"] == ref.Kind &&
		m["name"] == ref.Name && namespaceMatches && m["uid"] == ref.UID && obj.version() != ""
}

func (obj resourceJSON) manifest() []byte {
	obj.clean()
	data, _ := json.Marshal(obj)
	return data
}

func (obj resourceJSON) clean() {
	delete(obj, "status")
	for _, field := range []string{"managedFields", "generation", "creationTimestamp", "deletionTimestamp", "deletionGracePeriodSeconds"} {
		delete(obj.metadata(), field)
	}
}

type resourceClient struct {
	cli, context, config string
	env                  sysenv.Env
	runner               run.Runner
}

type resourceCommandError struct{ command, detail string }

func (e *resourceCommandError) Error() string { return "kubectl " + e.command + ": " + e.detail }

func resourceNotFound(err error) bool {
	call, ok := errors.AsType[*resourceCommandError](err)
	return ok && call.command == "get" && strings.HasPrefix(call.detail, "Error from server (NotFound):")
}

func (k resourceClient) run(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	return k.runFor(ctx, 20*time.Second, "15s", input, args...)
}

func (k resourceClient) runFor(ctx context.Context, timeout time.Duration, requestTimeout string, input []byte, args ...string) ([]byte, error) {
	argv := []string{k.cli, "--context=" + k.context, "--request-timeout=" + requestTimeout}
	if k.config != "" {
		argv = append(argv, "--kubeconfig="+k.config)
	}
	res, err := k.runner.Run(ctx, run.Cmd{Args: append(argv, args...), Env: k.env,
		Stdin: bytes.NewReader(input), Timeout: timeout, MaxOutput: 4 << 20})
	if err != nil {
		return nil, fmt.Errorf("kubectl %s: %w", args[0], err)
	}
	if res.Truncated {
		return nil, errors.New(locales.KubeResourceTooLarge())
	}
	if res.ExitCode != 0 {
		return nil, &resourceCommandError{args[0], strings.TrimSpace(string(res.Stderr))}
	}
	return res.Stdout, nil
}

func (k resourceClient) read(ctx context.Context, ref KubeResource) (resourceJSON, error) {
	data, err := k.run(ctx, nil, "get", "--raw", ref.path())
	if err != nil {
		return nil, err
	}
	return decodeResource(data)
}

// Pin resolves the selected context once and uses the same private kubeconfig
// for every command in this phase, including the read after a write.
func (k resourceClient) pin(ctx context.Context, expected [32]byte) (resourceClient, [32]byte, func(), error) {
	config, err := k.run(ctx, nil, "config", "view", "--minify", "--flatten", "--raw", "-o", "json")
	if err != nil {
		return k, [32]byte{}, nil, errors.New(locales.KubeResourceContext())
	}
	hash := sha256.Sum256(config)
	if expected != ([32]byte{}) && expected != hash {
		return k, hash, nil, errors.New(locales.KubeResourceContext())
	}
	dir, err := os.MkdirTemp("", "hearth-kube-")
	if err != nil {
		return k, hash, nil, fmt.Errorf("pin kubeconfig: %w", err)
	}
	closeConfig := func() { _ = os.RemoveAll(dir) }
	k.config = filepath.Join(dir, "config")
	if err := os.WriteFile(k.config, config, 0o600); err != nil {
		closeConfig()
		return k, hash, nil, fmt.Errorf("pin kubeconfig: %w", err)
	}
	return k, hash, closeConfig, nil
}

// KubeDraft is the version of an object read for editing or deletion.
type KubeDraft struct {
	Ref     KubeResource
	YAML    string
	Version string
	config  [32]byte
}

const kubeMaxDraft = 256 << 10
const kubeMaxDiffLines = 1500

func kubeManifestFits(text string) bool {
	return len(text) <= kubeMaxDraft && strings.Count(text, "\n") <= kubeMaxDiffLines
}

func LoadKubeDraft(ctx context.Context, r run.Runner, env sysenv.Env, ref KubeResource, edit bool) (KubeDraft, error) {
	if ref.path() == "" || ref.Kind == "Namespace" && edit {
		return KubeDraft{}, errors.New(locales.KubeResourceTarget())
	}
	k, hash, closeConfig, err := (resourceClient{cli: ref.Kubectl, context: ref.Context, env: env, runner: r}).pin(ctx, [32]byte{})
	if err != nil {
		return KubeDraft{}, err
	}
	defer closeConfig()
	obj, err := k.read(ctx, ref)
	if err != nil {
		return KubeDraft{}, err
	}
	if !obj.matches(ref) || obj.metadata()["deletionTimestamp"] != nil {
		return KubeDraft{}, errors.New(locales.KubeResourceTarget())
	}
	d := KubeDraft{Ref: ref, Version: obj.version(), config: hash}
	if edit {
		data, err := k.convert(ctx, obj.manifest(), "yaml")
		if err != nil {
			return KubeDraft{}, err
		}
		d.YAML = string(data)
		if !kubeManifestFits(d.YAML) {
			return KubeDraft{}, errors.New(locales.KubeResourceTooLarge())
		}
	}
	return d, nil
}

func (k resourceClient) convert(ctx context.Context, data []byte, output string) ([]byte, error) {
	return k.run(ctx, data, "create", "--dry-run=client", "--validate=false", "-f", "-", "-o", output)
}

// KubeChange is one reviewed write, bound to its context and version.
type KubeChange struct {
	draft            KubeDraft
	input            []byte
	base             resourceJSON
	want             resourceJSON
	rollout          bool
	scale            *deploymentScale
	patch            []string
	restart          bool
	batchDelete      bool
	rollback         *KubeDraft
	jobName          string
	createNamespace  bool
	namespaceMembers []kube.NamespaceMember
	drain            *nodeDrain
	Diff             []DiffLine
}

func (m *KubeChange) ConfirmName() string {
	if !m.createNamespace && m.drain == nil && m.want == nil && m.scale == nil && m.jobName == "" {
		return m.draft.Ref.Name
	}
	return ""
}

func (m *KubeChange) Target() string {
	if m.createNamespace {
		return m.draft.Ref.Context
	}
	return m.draft.Ref.Context + " · " + locales.KubeResourceVersion(m.draft.Version)
}

func (m *KubeChange) Resource() string { return m.draft.Ref.label() }

func (m *KubeChange) command() []string {
	if m.createNamespace {
		return []string{"create", "--raw", namespaceCollection, "-f", "-"}
	}
	if m.jobName != "" {
		return []string{"create", "-f", "-", "--namespace=" + m.draft.Ref.Namespace, "-o", "json"}
	}
	if m.scale != nil {
		return m.scale.command(m.draft)
	}
	if m.patch != nil {
		return m.patch
	}
	if m.want == nil {
		return []string{"delete", "--raw", m.draft.Ref.path(), "-f", "-"}
	}
	return []string{"replace", "--raw", m.draft.Ref.path() + "?fieldValidation=Strict", "-f", "-"}
}

func resourceSpecPatch(ref KubeResource, resource, version string, spec map[string]any) []string {
	patch, _ := json.Marshal(map[string]any{
		"metadata": map[string]string{"uid": ref.UID, "resourceVersion": version},
		"spec":     spec,
	})
	args := []string{"patch", resource + "/" + ref.Name}
	if ref.Namespace != "" {
		args = append(args, "--namespace="+ref.Namespace)
	}
	return append(args, "--type=merge", "--patch="+string(patch), "-o", "json")
}

func deleteOptions(ref KubeResource, version string, dry bool) []byte {
	options := map[string]any{
		"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Background",
		"preconditions": map[string]string{"uid": ref.UID, "resourceVersion": version},
	}
	if dry {
		options["dryRun"] = []string{"All"}
	}
	data, _ := json.Marshal(options)
	return data
}

func (d KubeDraft) current(ctx context.Context, k resourceClient) (resourceJSON, error) {
	obj, err := k.read(ctx, d.Ref)
	if err != nil {
		return nil, err
	}
	if !obj.matches(d.Ref) || obj.version() != d.Version || obj.metadata()["deletionTimestamp"] != nil {
		return nil, errors.New(locales.KubeResourceChanged())
	}
	return obj, nil
}

func cleanResource(obj resourceJSON) resourceJSON {
	obj.clean()
	delete(obj.metadata(), "resourceVersion")
	return obj
}

// approvedChanges compares the changed fields recursively. Controllers may
// update unrelated annotations and status between the write and its check.
func approvedChanges(before, want, actual map[string]any) bool {
	keys := maps.Clone(before)
	maps.Copy(keys, want)
	for k := range keys {
		old, had := before[k]
		next, wanted := want[k]
		got, exists := actual[k]
		if had == wanted && reflect.DeepEqual(old, next) {
			continue
		}
		oldMap, oldOK := old.(map[string]any)
		nextMap, nextOK := next.(map[string]any)
		gotMap, gotOK := got.(map[string]any)
		if oldOK && nextOK && gotOK {
			if !approvedChanges(oldMap, nextMap, gotMap) {
				return false
			}
		} else if exists != wanted || !reflect.DeepEqual(got, next) {
			return false
		}
	}
	return true
}

// PlanKubeChange uses a server dry run before offering the operation.
func PlanKubeChange(ctx context.Context, r run.Runner, env sysenv.Env, d KubeDraft, yaml string, remove bool) (Plan, error) {
	ref := d.Ref
	if ref.path() == "" || d.config == ([32]byte{}) || d.Version == "" || ref.Kind == "Namespace" && !remove {
		return Plan{}, errors.New(locales.KubeResourceTarget())
	}
	k, _, closeConfig, err := (resourceClient{cli: ref.Kubectl, context: ref.Context, env: env, runner: r}).pin(ctx, d.config)
	if err != nil {
		return Plan{}, err
	}
	defer closeConfig()
	current, err := d.current(ctx, k)
	if err != nil {
		return Plan{}, err
	}
	change := &KubeChange{draft: d}
	p := Plan{Env: slices.Clone(env)}
	if remove {
		var members []kube.NamespaceMember
		if ref.Kind == "Namespace" {
			members, err = k.namespaceMembers(ctx, ref.Name)
			if err != nil {
				return Plan{}, err
			}
		}
		change, err = k.planDelete(ctx, d)
		if err != nil {
			return Plan{}, err
		}
		p.Title, p.Why, p.Action = locales.KubeResourceDeleteTitle(ref.Name), locales.KubeResourceDeleteWhy(), locales.KubeResourceDeleteAction()
		p.Notes = []Note{{Title: locales.KubeResourceCascade(), Text: locales.KubeResourceDeleteData()}}
		if ref.Kind == "Namespace" {
			change.namespaceMembers = members
			p.Why = locales.KubeNamespaceDeleteWhy()
			p.Notes = []Note{namespaceInventoryNote(members)}
		}
		if ref.Kind == "Pod" && resourceController(current) != nil {
			p.Notes = append(p.Notes, Note{Title: locales.KubeResourceOwnedPod()})
		}
	} else {
		if !kubeManifestFits(yaml) {
			return Plan{}, errors.New(locales.KubeResourceTooLarge())
		}
		data, err := k.convert(ctx, []byte(yaml), "json")
		if err != nil {
			return Plan{}, err
		}
		obj, err := decodeResource(data)
		if err != nil || !obj.matches(ref) || obj.version() != d.Version {
			return Plan{}, errors.New(locales.KubeResourceIdentity())
		}
		if !reflect.DeepEqual(obj.metadata()["finalizers"], current.metadata()["finalizers"]) ||
			!reflect.DeepEqual(obj.metadata()["ownerReferences"], current.metadata()["ownerReferences"]) {
			return Plan{}, errors.New(locales.KubeResourceProtection())
		}
		change.input = obj.manifest()
		data, err = k.run(ctx, change.input, "replace", "--raw", ref.path()+"?fieldValidation=Strict&dryRun=All", "-f", "-")
		if err != nil {
			return Plan{}, err
		}
		preview, err := decodeResource(data)
		if err != nil || !preview.matches(ref) {
			return Plan{}, errors.New(locales.KubeResourceIdentity())
		}
		preview.metadata()["resourceVersion"] = d.Version
		raw := preview.manifest()
		change.base, change.want = cleanResource(current), cleanResource(preview)
		if reflect.DeepEqual(change.base, change.want) {
			return Plan{}, errors.New(locales.KubeResourceNoChanges())
		}
		after, err := k.convert(ctx, raw, "yaml")
		if err != nil {
			return Plan{}, err
		}
		if !kubeManifestFits(string(after)) {
			return Plan{}, errors.New(locales.KubeResourceTooLarge())
		}
		change.Diff = Diff(d.YAML, string(after))
		p.Title, p.Why, p.Action = locales.KubeResourceApplyTitle(ref.Name), locales.KubeResourceDryRun(), locales.KubeResourceApply()
		if ref.Kind == "Deployment" {
			before, _ := change.base["spec"].(map[string]any)
			after, _ := change.want["spec"].(map[string]any)
			if !reflect.DeepEqual(before["template"], after["template"]) {
				change.rollout = after["paused"] != true
				note := locales.KubeResourceRolloutPaused()
				if change.rollout {
					note = locales.KubeResourceRolloutDetails()
				}
				p.Notes = []Note{{Title: locales.KubeResourceRollout(), Text: note}}
			}
		}
	}
	p.Steps = []Step{{Kube: change, Argv: append([]string{ref.Kubectl, "--context=" + ref.Context}, change.command()...)}}
	return p, nil
}

func resourceController(obj resourceJSON) map[string]any {
	owners, _ := obj.metadata()["ownerReferences"].([]any)
	for _, owner := range owners {
		m, _ := owner.(map[string]any)
		if m["controller"] == true {
			return m
		}
	}
	return nil
}

func (k resourceClient) planDelete(ctx context.Context, d KubeDraft) (*KubeChange, error) {
	ref := d.Ref
	change := &KubeChange{draft: d, input: deleteOptions(ref, d.Version, false)}
	if _, err := k.run(ctx, deleteOptions(ref, d.Version, true), change.command()...); err != nil {
		return nil, err
	}
	return change, nil
}

func (m *KubeChange) apply(ctx context.Context, r run.Runner, env sysenv.Env, checking func()) (string, error) {
	ref := m.draft.Ref
	k, _, closeConfig, err := (resourceClient{cli: ref.Kubectl, context: ref.Context, env: env, runner: r}).pin(ctx, m.draft.config)
	if err != nil {
		return "", err
	}
	defer closeConfig()
	return m.applyPinned(ctx, k, checking)
}

func (m *KubeChange) applyPinned(ctx context.Context, k resourceClient, checking func()) (string, error) {
	ref := m.draft.Ref
	if m.createNamespace {
		return m.applyNamespaceCreate(ctx, k, checking)
	}
	if _, err := m.draft.current(ctx, k); err != nil {
		return "", err
	}
	if ref.Kind == "Namespace" {
		members, err := k.namespaceMembers(ctx, ref.Name)
		if err != nil {
			return "", err
		}
		if !slices.Equal(members, m.namespaceMembers) {
			return "", errors.New(locales.KubeNamespaceContentsChanged())
		}
	}
	if m.drain != nil {
		return m.applyNodeDrain(ctx, k, checking)
	}
	if m.jobName != "" {
		return m.startCronJob(ctx, k, checking)
	}
	if m.rollback != nil {
		if _, err := m.rollback.current(ctx, k); err != nil {
			return "", err
		}
	}
	_, writeErr := k.run(ctx, m.input, m.command()...)
	if m.scale != nil {
		checking()
	}
	// A canceled command may already have reached the API server. Re-read with
	// its own deadline before reporting, even if the operation was stopped.
	check, cancel := context.WithTimeout(context.WithoutCancel(ctx), 22*time.Second)
	defer cancel()
	obj, readErr := k.read(check, ref)
	if m.scale != nil {
		return m.verifyScale(ctx, k, obj, writeErr, readErr)
	}
	if m.want == nil && resourceNotFound(readErr) {
		return locales.KubeResourceDeleted(), nil
	}
	if readErr != nil {
		return "", fmt.Errorf("%s: %w", locales.KubeResourceUnverified(), errors.Join(writeErr, readErr))
	}
	if m.want == nil {
		if uid, _ := obj.metadata()["uid"].(string); uid != "" && uid != ref.UID {
			return locales.KubeResourceDeleted(), nil
		}
		if obj.matches(ref) && obj.metadata()["deletionTimestamp"] != nil {
			if m.batchDelete || ref.Kind == "Namespace" {
				return locales.KubeResourceDeleting(), nil
			}
			return "", errors.New(locales.KubeResourceDeleting())
		}
	}
	if m.want != nil && obj.version() != m.draft.Version && m.matchesChange(obj) {
		if m.rollout {
			verified, unverified := locales.KubeResourceRolloutVerified(), locales.KubeResourceRolloutUnverified()
			switch {
			case m.restart:
				checking()
				verified, unverified = locales.KubeRestartVerified(), locales.KubeRestartUnverified()
			case m.rollback != nil:
				checking()
				verified, unverified = locales.KubeRollbackVerified(), locales.KubeResourceRolloutUnverified()
			}
			if err := m.verifyRollout(ctx, k); err != nil {
				return "", fmt.Errorf("%s: %w", unverified, err)
			}
			return verified, nil
		}
		return locales.KubeResourceApplied(), nil
	}
	if writeErr != nil {
		return "", fmt.Errorf("%s: %w", locales.KubeResourceUnverified(), writeErr)
	}
	if m.want == nil && obj.matches(ref) {
		return "", errors.New(locales.KubeResourceDeleteAccepted())
	}
	return "", errors.New(locales.KubeResourceUnverified())
}

func (m *KubeChange) verifyRollout(ctx context.Context, k resourceClient) error {
	ref := m.draft.Ref
	if _, err := k.runFor(ctx, 5*time.Minute+5*time.Second, "0", nil,
		"rollout", "status", "deployment/"+ref.Name, "--namespace="+ref.Namespace, "--timeout=5m"); err != nil {
		return err
	}
	obj, err := k.read(ctx, ref)
	if err != nil {
		return err
	}
	if !m.matchesChange(obj) {
		return errors.New(locales.KubeResourceChanged())
	}
	return nil
}

func (m *KubeChange) matchesChange(obj resourceJSON) bool {
	if !obj.matches(m.draft.Ref) || (m.patch != nil && obj.metadata()["deletionTimestamp"] != nil) {
		return false
	}
	if m.restart {
		spec := resourceSpec(obj)
		return spec["paused"] != true &&
			reflect.DeepEqual(spec["template"], resourceSpec(m.want)["template"])
	}
	if m.rollback != nil &&
		(resourceSpec(obj)["paused"] == true || !reflect.DeepEqual(resourceSpec(obj)["template"], resourceSpec(m.want)["template"])) {
		return false
	}
	return approvedChanges(m.base, m.want, cleanResource(obj))
}
