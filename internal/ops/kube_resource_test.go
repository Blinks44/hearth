package ops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

type resourceRunner func(context.Context, run.Cmd) (run.Result, error)

func (f resourceRunner) Run(ctx context.Context, cmd run.Cmd) (run.Result, error) {
	return f(ctx, cmd)
}

func kubeTestObject(uid, rv, image string) string {
	return `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"demo","namespace":"work","uid":"` +
		uid + `","resourceVersion":"` + rv + `"},"spec":{"image":"` + image + `"}}`
}

func TestKubeResourceEdit(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "staging", Kind: "Deployment", Namespace: "work", Name: "demo", UID: "uid"}
	base, edited, written := kubeTestObject("uid", "4", "old"), kubeTestObject("uid", "4", "new"), kubeTestObject("uid", "5", "new")
	for _, tc := range []struct {
		name, current, candidate, result, stderr string
		configChanged, timeout                   bool
		wantWrites                               int
		want                                     Phase
	}{
		{name: "verified", current: base, candidate: edited, result: written, wantWrites: 1, want: Succeeded},
		{name: "timed out after commit", current: base, candidate: edited, result: written, timeout: true, wantWrites: 1, want: Succeeded},
		{name: "timed out before commit", current: base, candidate: edited, result: base, timeout: true, wantWrites: 1, want: NotVerified},
		{name: "conflicting version", current: kubeTestObject("uid", "5", "old"), candidate: edited, want: NotVerified},
		{name: "replaced", current: kubeTestObject("other", "4", "old"), candidate: edited, want: NotVerified},
		{name: "context remapped", current: base, candidate: edited, configChanged: true, want: NotVerified},
		{name: "forbidden", current: base, candidate: edited, result: base, stderr: "Error from server (Forbidden): update denied", wantWrites: 1, want: NotVerified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			phase, writes, reads := "load", 0, 0
			r := resourceRunner(func(ctx context.Context, cmd run.Cmd) (run.Result, error) {
				if !slices.Contains(cmd.Args, "--context=staging") {
					t.Fatalf("wrong context: %v", cmd.Args)
				}
				ok := func(s string) (run.Result, error) { return run.Result{Stdout: []byte(s)}, nil }
				if slices.Contains(cmd.Args, "config") {
					if phase == "execute" && tc.configChanged {
						return ok(`{"server":"new"}`)
					}
					return ok(`{"server":"old"}`)
				}
				if !slices.ContainsFunc(cmd.Args, func(arg string) bool {
					return strings.HasPrefix(arg, "--kubeconfig=") && filepath.Base(arg) == "config"
				}) {
					t.Fatalf("command does not use pinned kubeconfig: %v", cmd.Args)
				}
				switch {
				case slices.Contains(cmd.Args, "get"):
					if !slices.Contains(cmd.Args, ref.path()) {
						t.Fatalf("get: %v", cmd.Args)
					}
					if phase != "execute" {
						return ok(base)
					}
					reads++
					if writes == 0 {
						return ok(tc.current)
					}
					if ctx.Err() != nil {
						t.Fatalf("read after write has a canceled context: %v", ctx.Err())
					}
					return ok(tc.result)
				case slices.Contains(cmd.Args, "create"):
					if !slices.Contains(cmd.Args, "--dry-run=client") {
						t.Fatal(cmd.Args)
					}
					if slices.Contains(cmd.Args, "yaml") {
						b, _ := io.ReadAll(cmd.Stdin)
						return ok(string(b))
					}
					return ok(tc.candidate)
				case slices.Contains(cmd.Args, "replace"):
					b, _ := io.ReadAll(cmd.Stdin)
					obj, err := decodeResource(b)
					if err != nil || !obj.matches(ref) || obj.version() != "4" {
						t.Fatalf("wrong edit preconditions: %s, %v", b, err)
					}
					if phase == "plan" {
						if !slices.Contains(cmd.Args, ref.path()+"?fieldValidation=Strict&dryRun=All") {
							t.Fatal(cmd.Args)
						}
						return ok(edited)
					}
					writes++
					if tc.timeout {
						return run.Result{}, context.DeadlineExceeded
					}
					if tc.stderr != "" {
						return run.Result{ExitCode: 1, Stderr: []byte(tc.stderr)}, nil
					}
					return ok(written)
				}
				t.Fatalf("unexpected command %v", cmd.Args)
				return run.Result{}, nil
			})
			d, err := LoadKubeDraft(t.Context(), r, nil, ref, true)
			if err != nil {
				t.Fatal(err)
			}
			phase = "plan"
			p, err := PlanKubeChange(t.Context(), r, nil, d, "draft", false)
			if err != nil || len(p.Steps[0].Kube.Diff) == 0 || writes != 0 {
				t.Fatalf("preview: plan=%+v writes=%d err=%v", p, writes, err)
			}
			phase = "execute"
			result := Execute(t.Context(), r, p, io.Discard, func(State) {})
			if result.Phase != tc.want || writes != tc.wantWrites || writes > 0 && reads != 2 {
				t.Fatalf("result=%+v writes=%d reads=%d", result, writes, reads)
			}
		})
	}
}

func TestKubeResourceDelete(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "staging", Kind: "Deployment", Namespace: "work", Name: "demo", UID: "uid"}
	base := kubeTestObject("uid", "4", "old")
	for _, tc := range []struct {
		name, after, readErr, want string
		writeErr                   error
	}{
		{name: "absence confirmed", readErr: "Error from server (NotFound): deployment gone", want: locales.KubeResourceDeleted()},
		{name: "recreated", after: kubeTestObject("other", "5", "old"), want: locales.KubeResourceDeleted()},
		{name: "accepted", after: base, want: locales.KubeResourceDeleteAccepted()},
		{name: "pending finalizers", after: strings.Replace(base, `"resourceVersion":"4"`, `"resourceVersion":"5","deletionTimestamp":"now"`, 1), want: locales.KubeResourceDeleting()},
		{name: "ambiguous", readErr: "Error from server (Forbidden): get denied", writeErr: context.DeadlineExceeded, want: locales.KubeResourceUnverified()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			writes := 0
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				switch {
				case slices.Contains(cmd.Args, "config"):
					return run.Result{Stdout: []byte("pinned")}, nil
				case slices.Contains(cmd.Args, "get"):
					if writes == 0 {
						return run.Result{Stdout: []byte(base)}, nil
					}
					if tc.readErr != "" {
						return run.Result{ExitCode: 1, Stderr: []byte(tc.readErr)}, nil
					}
					return run.Result{Stdout: []byte(tc.after)}, nil
				case slices.Contains(cmd.Args, "delete"):
					if !slices.Contains(cmd.Args, ref.path()) || slices.Contains(cmd.Args, "--force") {
						t.Fatalf("unsafe deletion arguments: %v", cmd.Args)
					}
					var opts struct {
						PropagationPolicy string
						Preconditions     struct{ UID, ResourceVersion string }
						DryRun            []string
					}
					if err := json.NewDecoder(cmd.Stdin).Decode(&opts); err != nil {
						t.Fatal(err)
					}
					if opts.Preconditions.UID != ref.UID || opts.Preconditions.ResourceVersion != "4" || opts.PropagationPolicy != "Background" {
						t.Fatalf("unsafe deletion options: %+v", opts)
					}
					if slices.Equal(opts.DryRun, []string{"All"}) {
						return run.Result{}, nil
					}
					writes++
					return run.Result{}, tc.writeErr
				}
				return run.Result{}, errors.New("unexpected command")
			})
			d, err := LoadKubeDraft(t.Context(), r, nil, ref, false)
			if err != nil {
				t.Fatal(err)
			}
			p, err := PlanKubeChange(t.Context(), r, nil, d, "", true)
			if err != nil || writes != 0 || !p.Steps[0].Kube.DeletesResource() {
				t.Fatalf("delete preview: %+v writes=%d err=%v", p, writes, err)
			}
			result := Execute(t.Context(), r, p, io.Discard, func(State) {})
			if writes != 1 || !strings.HasPrefix(result.Verified+result.Err, tc.want) {
				t.Fatalf("delete result=%+v writes=%d", result, writes)
			}
		})
	}
}

func TestKubeResourceRejectsSecret(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "staging", Kind: "Secret", Namespace: "work", Name: "demo", UID: "uid"}
	if _, err := LoadKubeDraft(t.Context(), nil, nil, ref, true); err == nil {
		t.Fatal("a Secret reached the resource reader")
	}
}

func TestKubeResourceExtendedKinds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind        string
		apiVersion  string
		wantPath    string
		rolloutArg  string
		baseBody    string
		editedBody  string
		writtenBody string
	}{
		{
			kind: "StatefulSet", apiVersion: "apps/v1",
			wantPath:    "/apis/apps/v1/namespaces/work/statefulsets/demo",
			rolloutArg:  "statefulset/demo",
			baseBody:    `{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"spec":{"template":{"spec":{"containers":[{"name":"app","image":"v1"}]}}}}`,
			editedBody:  `{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"spec":{"template":{"spec":{"containers":[{"name":"app","image":"v2"}]}}}}`,
			writtenBody: `{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"5"},"spec":{"template":{"spec":{"containers":[{"name":"app","image":"v2"}]}}}}`,
		},
		{
			kind: "DaemonSet", apiVersion: "apps/v1",
			wantPath:    "/apis/apps/v1/namespaces/work/daemonsets/demo",
			rolloutArg:  "daemonset/demo",
			baseBody:    `{"apiVersion":"apps/v1","kind":"DaemonSet","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"spec":{"template":{"spec":{"containers":[{"name":"agent","image":"v1"}]}}}}`,
			editedBody:  `{"apiVersion":"apps/v1","kind":"DaemonSet","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"spec":{"template":{"spec":{"containers":[{"name":"agent","image":"v2"}]}}}}`,
			writtenBody: `{"apiVersion":"apps/v1","kind":"DaemonSet","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"5"},"spec":{"template":{"spec":{"containers":[{"name":"agent","image":"v2"}]}}}}`,
		},
		{
			kind: "ConfigMap", apiVersion: "v1",
			wantPath:    "/api/v1/namespaces/work/configmaps/demo",
			baseBody:    `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"data":{"ENV":"dev"}}`,
			editedBody:  `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"data":{"ENV":"prod"}}`,
			writtenBody: `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"5"},"data":{"ENV":"prod"}}`,
		},
		{
			kind: "Ingress", apiVersion: "networking.k8s.io/v1",
			wantPath:    "/apis/networking.k8s.io/v1/namespaces/work/ingresses/demo",
			baseBody:    `{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"spec":{"ingressClassName":"nginx"}}`,
			editedBody:  `{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"spec":{"ingressClassName":"traefik"}}`,
			writtenBody: `{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"5"},"spec":{"ingressClassName":"traefik"}}`,
		},
		{
			kind: "PersistentVolumeClaim", apiVersion: "v1",
			wantPath:    "/api/v1/namespaces/work/persistentvolumeclaims/demo",
			baseBody:    `{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"spec":{"resources":{"requests":{"storage":"1Gi"}}}}`,
			editedBody:  `{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"4"},"spec":{"resources":{"requests":{"storage":"2Gi"}}}}`,
			writtenBody: `{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":"5"},"spec":{"resources":{"requests":{"storage":"2Gi"}}}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			ref := KubeResource{Kubectl: "kubectl", Context: "staging", Kind: tc.kind, Namespace: "work", Name: "demo", UID: "uid"}
			if got := ref.path(); got != tc.wantPath {
				t.Fatalf("path = %q, want %q", got, tc.wantPath)
			}

			// Edit + verify (including rollout status when applicable).
			phase, writes, rollouts := "load", 0, 0
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				ok := func(s string) (run.Result, error) { return run.Result{Stdout: []byte(s)}, nil }
				switch {
				case slices.Contains(cmd.Args, "config"):
					return ok(`{"server":"same"}`)
				case slices.Contains(cmd.Args, "get"):
					if writes == 0 {
						return ok(tc.baseBody)
					}
					return ok(tc.writtenBody)
				case slices.Contains(cmd.Args, "create"):
					if slices.Contains(cmd.Args, "yaml") {
						b, _ := io.ReadAll(cmd.Stdin)
						return ok(string(b))
					}
					return ok(tc.editedBody)
				case slices.Contains(cmd.Args, "replace"):
					if phase == "plan" {
						return ok(tc.editedBody)
					}
					writes++
					return ok(tc.writtenBody)
				case slices.Contains(cmd.Args, "rollout"):
					if tc.rolloutArg == "" || !slices.Contains(cmd.Args, tc.rolloutArg) {
						t.Fatalf("unexpected rollout command: %v", cmd.Args)
					}
					rollouts++
					return ok("")
				}
				t.Fatalf("unexpected command: %v", cmd.Args)
				return run.Result{}, nil
			})
			d, err := LoadKubeDraft(t.Context(), r, nil, ref, true)
			if err != nil {
				t.Fatalf("LoadKubeDraft: %v", err)
			}
			phase = "plan"
			plan, err := PlanKubeChange(t.Context(), r, nil, d, "draft", false)
			if err != nil || len(plan.Steps[0].Kube.Diff) == 0 {
				t.Fatalf("PlanKubeChange edit: plan=%+v err=%v", plan, err)
			}
			phase = "execute"
			res := Execute(t.Context(), r, plan, io.Discard, func(State) {})
			wantRollouts := 0
			if tc.rolloutArg != "" {
				wantRollouts = 1
			}
			if res.Phase != Succeeded || writes != 1 || rollouts != wantRollouts {
				t.Fatalf("edit result=%+v writes=%d rollouts=%d", res, writes, rollouts)
			}

			// Delete + verify absence.
			delWrites := 0
			dr := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				switch {
				case slices.Contains(cmd.Args, "config"):
					return run.Result{Stdout: []byte("pinned")}, nil
				case slices.Contains(cmd.Args, "get"):
					if delWrites == 0 {
						return run.Result{Stdout: []byte(tc.baseBody)}, nil
					}
					return run.Result{ExitCode: 1, Stderr: []byte("Error from server (NotFound): gone")}, nil
				case slices.Contains(cmd.Args, "delete"):
					var opts struct{ DryRun []string }
					if err := json.NewDecoder(cmd.Stdin).Decode(&opts); err != nil {
						t.Fatal(err)
					}
					if slices.Equal(opts.DryRun, []string{"All"}) {
						return run.Result{}, nil
					}
					delWrites++
					return run.Result{}, nil
				}
				return run.Result{}, errors.New("unexpected command")
			})
			dd, err := LoadKubeDraft(t.Context(), dr, nil, ref, false)
			if err != nil {
				t.Fatalf("LoadKubeDraft delete: %v", err)
			}
			dp, err := PlanKubeChange(t.Context(), dr, nil, dd, "", true)
			if err != nil || !dp.Steps[0].Kube.DeletesResource() {
				t.Fatalf("PlanKubeChange delete: plan=%+v err=%v", dp, err)
			}
			dres := Execute(t.Context(), dr, dp, io.Discard, func(State) {})
			if dres.Phase != Succeeded || delWrites != 1 {
				t.Fatalf("delete result=%+v writes=%d", dres, delWrites)
			}
		})
	}
}
