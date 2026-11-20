package ops

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeManifestApply(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                                                                     string
		sourceChanged, clusterChanged, deletedBeforeWrite, partial, dryRunDenied bool
		want                                                                     Phase
	}{
		{name: "apply and read back", want: Succeeded},
		{name: "source edited after preview", sourceChanged: true, want: NotVerified},
		{name: "resource replaced after preview", clusterChanged: true, want: NotVerified},
		{name: "existing target deleted during write", deletedBeforeWrite: true, want: NotVerified},
		{name: "partial server write", partial: true, want: NotVerified},
		{name: "server dry run rejected", dryRunDenied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "kustomization.yaml"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			const secretValue = "hidden-fixture-value"
			secret := func(uid, value string) string {
				return fmt.Sprintf(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"auth","namespace":"shop","uid":%q,"resourceVersion":"14"},"data":{"token":%q}}`, uid, value)
			}
			const service = `{"apiVersion":"v1","kind":"Service","metadata":{"name":"http","namespace":"shop","uid":"svc"},"spec":{"clusterIP":"10.1.0.10"}}`
			const sourceService = `{"apiVersion":"v1","kind":"Service","metadata":{"name":"http","namespace":"shop"},"spec":{}}`
			const config = `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"stable","namespace":"shop","uid":"cfg","resourceVersion":"8"},"data":{"key":"stable"}}`
			liveUID, value := "sec", secretValue
			executing := false
			writes, reads := 0, 0
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				if !slices.Contains(cmd.Args, "--context=kind-shop") || slices.Contains(cmd.Args, "--prune") {
					t.Fatal(cmd.Args)
				}
				ok := func(s string) (run.Result, error) { return run.Result{Stdout: []byte(s)}, nil }
				switch {
				case slices.Contains(cmd.Args, "config"):
					return ok(`{"kind":"Config"}`)
				case slices.Contains(cmd.Args, "create"):
					if !slices.Contains(cmd.Args, "--dry-run=client") || !slices.Contains(cmd.Args, "-k") {
						t.Fatal(cmd.Args)
					}
					return ok(secret("", value) + sourceService + config)
				case slices.Contains(cmd.Args, "get"):
					reads++
					if writes == 0 {
						return ok(secret(liveUID, "old") + config)
					}
					if tc.partial {
						return ok(secret(liveUID, value) + config)
					}
					if tc.deletedBeforeWrite {
						return ok(service + config)
					}
					return ok(secret(liveUID, value) + strings.Replace(service, "10.1.0.10", "10.1.0.11", 1) + config)
				case slices.Contains(cmd.Args, "--dry-run=server"):
					assertManifestApplyPreconditions(t, cmd, liveUID)
					if tc.dryRunDenied {
						return run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden): " + secretValue)}, nil
					}
					return ok(secret(liveUID, value) + service + config)
				case slices.Contains(cmd.Args, "apply"):
					if !executing {
						t.Fatal("preview wrote to cluster")
					}
					assertManifestApplyPreconditions(t, cmd, liveUID)
					writes++
					if tc.deletedBeforeWrite {
						return run.Result{ExitCode: 1, Stdout: []byte(service), Stderr: []byte("object was deleted")}, nil
					}
					if tc.partial {
						return run.Result{ExitCode: 1, Stdout: []byte(secret(liveUID, value)), Stderr: []byte(secretValue)}, nil
					}
					return ok(secret(liveUID, value) + service + config)
				default:
					t.Fatal(cmd.Args)
					return run.Result{}, nil
				}
			})
			p, err := PlanKubeManifests(t.Context(), r, nil, "checkout", "shop", KubeManifestSource{Path: dir, Context: "kind-shop", Namespace: "shop"})
			if tc.dryRunDenied {
				if err == nil || strings.Contains(err.Error(), secretValue) || writes != 0 {
					t.Fatalf("invalid error for rejected preview: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			view := p.Steps[0].Kube.Manifests
			if !view.Kustomize || len(view.Created) != 1 || len(view.Changed) != 1 || len(view.Changed[0].Lines) != 0 || len(view.Unchanged) != 1 {
				t.Fatalf("wrong preview: %+v", view)
			}
			if tc.sourceChanged {
				value = "new-content"
			}
			if tc.clusterChanged {
				liveUID = "different-object"
			}
			executing = true
			var log strings.Builder
			result := Execute(t.Context(), r, p, &log, func(State) {})
			wantWrites, wantReads := 1, 3
			if tc.sourceChanged || tc.clusterChanged {
				wantWrites, wantReads = 0, 2
			}
			if result.Phase != tc.want || writes != wantWrites || reads != wantReads {
				t.Fatalf("state=%+v writes=%d reads=%d", result, writes, reads)
			}
			if tc.partial && !strings.Contains(result.Err, "Service shop/http") {
				t.Fatalf("missing unverified resource: %s", result.Err)
			}
			if tc.deletedBeforeWrite && !strings.Contains(result.Err, "Secret shop/auth") {
				t.Fatalf("missing deleted resource: %s", result.Err)
			}
			if (tc.sourceChanged || tc.clusterChanged) && result.Err != locales.KubeApplyStale() {
				t.Fatalf("wrong stale plan result: %s", result.Err)
			}
			if strings.Contains(log.String()+result.Err, secretValue) {
				t.Fatal("Secret leaked to the operation log or result")
			}
		})
	}
}

func assertManifestApplyPreconditions(t *testing.T, cmd run.Cmd, uid string) {
	t.Helper()
	data, err := io.ReadAll(cmd.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	objects, _, err := manifestObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	secret := objects[manifestID{"v1", "Secret", "shop", "auth"}]
	if secret.metadata()["uid"] != uid || secret.version() != "14" {
		t.Fatal("the existing target is missing its server UID or resourceVersion")
	}
	service := objects[manifestID{"v1", "Service", "shop", "http"}]
	if service == nil || service.metadata()["uid"] != nil || service.version() != "" {
		t.Fatal("the new resource should have no UID or resourceVersion yet")
	}
}
