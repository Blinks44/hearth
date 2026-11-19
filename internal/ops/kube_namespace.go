package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const namespaceCollection = "/api/v1/namespaces?fieldValidation=Strict"

func PlanKubeNamespaceCreate(ctx context.Context, r run.Runner, env sysenv.Env, cli, nameContext, name string) (Plan, error) {
	if cli == "" || nameContext == "" || !KubeNamespaceValid(name) {
		return Plan{}, errors.New(locales.KubeNamespaceInvalid())
	}
	k, hash, closeConfig, err := (resourceClient{cli: cli, context: nameContext, env: env, runner: r}).pin(ctx, [32]byte{})
	if err != nil {
		return Plan{}, err
	}
	defer closeConfig()
	input, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]string{"name": name}})
	if _, err := k.run(ctx, input, "create", "--raw", namespaceCollection+"&dryRun=All", "-f", "-"); err != nil {
		return Plan{}, err
	}
	ref := KubeResource{Kubectl: cli, Context: nameContext, Kind: "Namespace", Name: name}
	change := &KubeChange{draft: KubeDraft{Ref: ref, config: hash}, input: input, createNamespace: true}
	return Plan{Title: locales.KubeNamespaceCreateTitle(name), Why: locales.KubeNamespaceCreateWhy(), Action: locales.KubeNamespaceCreate(),
		Env: slices.Clone(env), Steps: []Step{{Kube: change, Argv: append([]string{cli, "--context=" + nameContext}, change.command()...)}}}, nil
}

func (m *KubeChange) applyNamespaceCreate(ctx context.Context, k resourceClient, checking func()) (string, error) {
	data, err := k.run(ctx, m.input, m.command()...)
	if err != nil {
		return "", fmt.Errorf("%s: %w", locales.KubeResourceUnverified(), err)
	}
	created, err := decodeResource(data)
	if err != nil {
		return "", err
	}
	ref := m.draft.Ref
	ref.UID, _ = created.metadata()["uid"].(string)
	if ref.UID == "" || !created.matches(ref) {
		return "", errors.New(locales.KubeResourceIdentity())
	}
	checking()
	check, cancel := context.WithTimeout(context.WithoutCancel(ctx), 22*time.Second)
	defer cancel()
	current, err := k.read(check, ref)
	if err != nil {
		return "", fmt.Errorf("%s: %w", locales.KubeResourceUnverified(), err)
	}
	if !current.matches(ref) || current.metadata()["deletionTimestamp"] != nil {
		return "", errors.New(locales.KubeResourceChanged())
	}
	return locales.KubeNamespaceCreated(), nil
}

func (k resourceClient) namespaceMembers(ctx context.Context, name string) ([]kube.NamespaceMember, error) {
	members, err := kube.ReadNamespaceInventory(ctx, k.runner, k.env, kube.Target{Kubectl: k.cli, Context: k.context}, k.config, name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", locales.KubeNamespaceInventoryFailed(), err)
	}
	return members, nil
}

func namespaceInventoryNote(members []kube.NamespaceMember) Note {
	if len(members) == 0 {
		return Note{Title: locales.KubeNamespaceContents(), Text: locales.KubeNamespaceEmpty()}
	}
	var lines []string
	for _, member := range members {
		lines = append(lines, member.Kind+" "+member.Name)
	}
	return Note{Title: locales.KubeNamespaceContents(), Text: strings.Join(lines, "\n")}
}
