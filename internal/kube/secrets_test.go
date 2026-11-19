package kube

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	gotemplate "text/template"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

type secretRunner func(run.Cmd) run.Result

func (r secretRunner) Run(_ context.Context, cmd run.Cmd) (run.Result, error) {
	return r(cmd), nil
}

func renderSecretTemplate(t *testing.T, cmd run.Cmd, obj any) run.Result {
	t.Helper()
	const prefix = "--output=go-template="
	i := slices.IndexFunc(cmd.Args, func(arg string) bool { return strings.HasPrefix(arg, prefix) })
	if i < 0 {
		t.Fatal("kubectl was asked for unfiltered Secret data")
	}
	decode := func(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
	tpl, err := gotemplate.New("kubectl").Funcs(gotemplate.FuncMap{"base64decode": decode}).Parse(strings.TrimPrefix(cmd.Args[i], prefix))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := tpl.Execute(&output, obj); err != nil {
		t.Fatal(err)
	}
	return run.Result{Stdout: output.Bytes()}
}

func TestReadSecrets(t *testing.T) {
	t.Parallel()
	fixture := map[string]any{
		"metadata": map[string]any{
			"name": "checkout-db", "namespace": "shop", "uid": "s1", "creationTimestamp": "2026-11-19T12:00:00Z",
			"annotations": map[string]any{"kubectl.kubernetes.io/last-applied-configuration": "fixture-must-not-leak"},
		},
		"type": "Opaque",
		"data": map[string]string{"PASSWORD": "Zml4dHVyZS1vbmx5", "EMPTY": ""},
	}
	other := map[string]any{
		"metadata": map[string]any{
			"name": "managed", "namespace": "shop", "uid": "s2", "creationTimestamp": "2026-11-19T12:00:00Z",
			"labels": map[string]any{"app.kubernetes.io/managed-by": "Helm"},
		},
		"type": "example.test/custom\tα",
	}
	r := secretRunner(func(cmd run.Cmd) run.Result {
		if !slices.Equal(cmd.Args[:5], []string{"kubectl", "--context=kind-dev", "get", "secrets", "--namespace=shop"}) || cmd.Output != nil {
			t.Fatalf("unexpected Secret list command: %v", cmd.Args)
		}
		out := renderSecretTemplate(t, cmd, map[string]any{"items": []any{other, fixture}})
		for _, value := range []string{"fixture-must-not-leak", "Zml4dHVyZS1vbmx5", "fixture-only"} {
			if bytes.Contains(out.Stdout, []byte(value)) {
				t.Fatal("Secret list leaked a value")
			}
		}
		return out
	})
	trg := target
	trg.Secrets = true
	cl, err := Read(t.Context(), r, nil, trg)
	wantKeys := []SecretKey{{"EMPTY", 0}, {"PASSWORD", 12}}
	if err != nil || len(cl.Secrets) != 2 || !slices.Equal(cl.Secrets[0].Keys, wantKeys) ||
		cl.Secrets[0].UID != "s1" || cl.Secrets[0].Created.IsZero() || cl.Secrets[0].Helm ||
		!cl.Secrets[1].Helm || cl.Secrets[1].Type != "example.test/custom\tα" {
		t.Fatalf("secrets = %+v, err = %v", cl.Secrets, err)
	}
}

func TestReadSecretValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, uid, key, want string
		fail                 bool
	}{
		{name: "exact key", uid: "s1", key: "password.txt", want: "fixture"},
		{name: "empty", uid: "s1", key: "empty"},
		{name: "recreated Secret", uid: "old", key: "password.txt", fail: true},
		{name: "removed key", uid: "s1", key: "removed", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := secretRunner(func(cmd run.Cmd) run.Result {
				if !slices.Equal(cmd.Args[:5], []string{"kubectl", "--context=kind-dev", "get", "secret/checkout-db", "--namespace=shop"}) || cmd.Output != nil {
					t.Fatalf("unexpected Secret get command: %v", cmd.Args)
				}
				return renderSecretTemplate(t, cmd, map[string]any{
					"metadata": map[string]string{"uid": "s1"},
					"data":     map[string]string{"password.txt": "Zml4dHVyZQ==", "empty": "", "other": "b3RoZXI="},
				})
			})
			value, err := ReadSecretValue(t.Context(), r, nil, target, Secret{Namespace: "shop", Name: "checkout-db", UID: tc.uid}, tc.key)
			if (err != nil) != tc.fail || string(value) != tc.want {
				t.Fatalf("unexpected result for %s: %v", tc.name, err)
			}
		})
	}
}

func TestSecretFailureOmitsCommandOutput(t *testing.T) {
	t.Parallel()
	stdout, stderr := []byte("fixture"), []byte("Error from server (Forbidden): fixture")
	r := secretRunner(func(run.Cmd) run.Result {
		return run.Result{Stdout: stdout, Stderr: stderr, ExitCode: 1}
	})
	value, err := ReadSecretValue(t.Context(), r, nil, target, Secret{Name: "demo"}, "key")
	kerr, ok := errors.AsType[*Error](err)
	if !ok || kerr.Access.State != detect.KubeForbidden || kerr.Access.Detail != "" || value != nil ||
		!bytes.Equal(stdout, make([]byte, len(stdout))) || !bytes.Equal(stderr, make([]byte, len(stderr))) {
		t.Fatal("failed Secret read retained command output or lost the access state")
	}
}
