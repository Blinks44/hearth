package ops

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

const namespaceObject = `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"demo","uid":"uid","resourceVersion":"7"}}`

func TestKubeNamespaceCreate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, actual string
		want         Phase
		interrupted  bool
	}{
		{name: "verified", actual: namespaceObject, want: Succeeded},
		{name: "replacement", actual: strings.Replace(namespaceObject, `"uid":"uid"`, `"uid":"other"`, 1), want: NotVerified},
		{name: "lost acknowledgement", actual: namespaceObject, want: NotVerified, interrupted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls []string
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				calls = append(calls, cmd.Args[len(cmd.Args)-1])
				if slices.Contains(cmd.Args, "config") {
					return run.Result{Stdout: []byte("config")}, nil
				}
				if slices.Contains(cmd.Args, "create") {
					input, _ := io.ReadAll(cmd.Stdin)
					obj, err := decodeResource(input)
					if err != nil || obj["kind"] != "Namespace" || obj.metadata()["name"] != "demo" {
						t.Fatalf("unexpected namespace input %s, %v", input, err)
					}
					if !slices.Contains(cmd.Args, namespaceCollection+"&dryRun=All") && !slices.Contains(cmd.Args, namespaceCollection) {
						t.Fatalf("unexpected create: %v", cmd.Args)
					}
					calls[len(calls)-1] = cmd.Args[len(cmd.Args)-3]
					if tc.interrupted && slices.Contains(cmd.Args, namespaceCollection) {
						return run.Result{}, context.Canceled
					}
					return run.Result{Stdout: []byte(namespaceObject)}, nil
				}
				return run.Result{Stdout: []byte(tc.actual)}, nil
			})
			p, err := PlanKubeNamespaceCreate(t.Context(), r, nil, "kubectl", "staging", "demo")
			if err != nil || p.Steps[0].Kube.ConfirmName() != "" ||
				!slices.Equal(calls, []string{"json", namespaceCollection + "&dryRun=All"}) {
				t.Fatalf("preview: calls %v, err %v", calls, err)
			}
			state := Execute(t.Context(), r, p, io.Discard, func(State) {})
			want := []string{"json", namespaceCollection + "&dryRun=All", "json", namespaceCollection, "/api/v1/namespaces/demo"}
			if state.Phase != tc.want || !slices.Equal(calls, want) {
				t.Fatalf("create: phase %v, calls %v", state.Phase, calls)
			}
		})
	}
}

func TestKubeNamespaceDeletionInventory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, changed string
		forbidden     bool
		want          Phase
		writes        int
	}{
		{name: "verified pending deletion", want: Succeeded, writes: 1},
		{name: "new object after plan", changed: "ConfigMap\tnew\tnew-uid\t\n", want: NotVerified},
		{name: "list forbidden", forbidden: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ref := KubeResource{Kubectl: "kubectl", Context: "staging", Kind: "Namespace", Name: "demo", UID: "uid"}
			applying, writes := false, 0
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				switch {
				case slices.Contains(cmd.Args, "config"):
					return run.Result{Stdout: []byte("config")}, nil
				case slices.Contains(cmd.Args, "api-resources"):
					return run.Result{Stdout: []byte("configmaps\nsecrets\n")}, nil
				case slices.Contains(cmd.Args, "configmaps,secrets"):
					if tc.forbidden {
						return run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden): cannot list secrets")}, nil
					}
					members := "Secret\tcredentials\tsecret-uid\t\n"
					if applying {
						members += tc.changed
					}
					return run.Result{Stdout: []byte(members)}, nil
				case slices.Contains(cmd.Args, "delete"):
					var options struct {
						Preconditions map[string]string
						DryRun        []string
					}
					if err := json.NewDecoder(cmd.Stdin).Decode(&options); err != nil ||
						options.Preconditions["uid"] != ref.UID || options.Preconditions["resourceVersion"] != "7" ||
						(!applying) != slices.Equal(options.DryRun, []string{"All"}) {
						t.Fatalf("delete options: %+v, %v", options, err)
					}
					if applying {
						writes++
					}
					return run.Result{}, nil
				case slices.Contains(cmd.Args, "/api/v1/namespaces/demo"):
					obj := namespaceObject
					if writes > 0 {
						obj = strings.Replace(obj, `"resourceVersion":"7"`, `"resourceVersion":"8","deletionTimestamp":"now"`, 1)
					}
					return run.Result{Stdout: []byte(obj)}, nil
				}
				t.Fatalf("unexpected command %v", cmd.Args)
				return run.Result{}, nil
			})
			d, err := LoadKubeDraft(t.Context(), r, nil, ref, false)
			if err != nil {
				t.Fatal(err)
			}
			p, err := PlanKubeChange(t.Context(), r, nil, d, "", true)
			if tc.forbidden {
				if err == nil || !strings.Contains(err.Error(), locales.KubeNamespaceInventoryFailed()) {
					t.Fatalf("preview should fail for incomplete listing: %v", err)
				}
				return
			}
			if err != nil || p.Steps[0].Kube.ConfirmName() != "demo" || writes != 0 || p.Notes[0].Text != "Secret credentials" {
				t.Fatalf("preview: %+v, writes %d, err %v", p, writes, err)
			}
			applying = true
			state := Execute(t.Context(), r, p, io.Discard, func(State) {})
			if state.Phase != tc.want || writes != tc.writes {
				t.Fatalf("delete: %+v, writes %d", state, writes)
			}
		})
	}
}
