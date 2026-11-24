package kube

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

func TestReadCustomResources(t *testing.T) {
	var fake run.Fake
	definitions := []string{"kubectl", "--context=dev", "get", "customresourcedefinitions.apiextensions.k8s.io", "--output=json", "--request-timeout=15s"}
	fixture := func(file string) string {
		t.Helper()
		data, err := os.ReadFile("testdata/custom/" + file + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	fake.Stdout(fixture("definitions"), definitions...)
	target := Target{Kubectl: "kubectl", Context: "dev", Namespaces: []string{"shop", "tools"}, CustomKind: "certificates.hearth.example", Events: Ref{Namespace: "shop", UID: "cert1"}}
	for _, namespace := range target.Namespaces {
		data := `{"items":[]}`
		if namespace == "shop" {
			data = fixture("objects")
		}
		fake.Stdout(data, "kubectl", "--context=dev", "get", "certificates.v1.hearth.example", "--namespace="+namespace, "--output=json", "--request-timeout=15s")
	}
	fake.Stdout(`{"items":[]}`, "kubectl", "--context=dev", "get", "events", "--namespace=shop", "--field-selector=involvedObject.uid=cert1", "--output=json", "--request-timeout=15s")
	cluster, err := Read(t.Context(), &fake, nil, target)
	if err != nil || cluster.Resources == nil || len(cluster.Resources.Items) != 2 || cluster.EventsOf != target.Events {
		t.Fatalf("cluster = %+v, %v", cluster, err)
	}
	cert := cluster.Resources.Items[0]
	want := []string{"False", "9007199254740995", "false", "", ""}
	if cert.UID != "cert1" || !slices.Equal(cert.Custom.Cells, want) || len(cert.Custom.Conditions) != 1 || cert.Custom.Conditions[0].Reason != "Failed" {
		t.Fatalf("certificate = %+v, custom details = %+v", cert, cert.Custom)
	}
	if got := cluster.Resources.Items[1].Custom.Conditions; len(got) != 0 {
		t.Fatalf("nonstandard status yielded conditions: %+v", got)
	}
	args := []string{"kubectl", "--context=dev", "get", "certificates.v1.hearth.example", "--namespace=shop", "--output=yaml", "--request-timeout=15s", "--", "tls-shop"}
	manifest := "kind: Certificate\nmetadata:\n  uid: cert1\n"
	fake.Stdout(manifest, args...)
	if got, err := ReadResourceYAML(t.Context(), &fake, nil, target, cert); err != nil || got != manifest {
		t.Fatalf("YAML = %q, %v", got, err)
	}
	fake.Stdout("kind: Certificate\nmetadata:\n  uid: replaced\n", args...)
	if _, err := ReadResourceYAML(t.Context(), &fake, nil, target, cert); err == nil {
		t.Fatal("accepted YAML for a replacement object")
	}

	target.CustomKind, target.Events = "issuers.hearth.example", Ref{}
	fake.Stdout(`{"items":[{"metadata":{"name":"issuer","uid":"one"}}]}`, "kubectl", "--context=dev", "get", "issuers.v2.hearth.example", "--output=json", "--request-timeout=15s")
	cluster, err = Read(t.Context(), &fake, nil, target)
	if err != nil || cluster.Resources == nil || len(cluster.Resources.Items) != 1 || cluster.Resources.Custom.Namespaced() {
		t.Fatalf("cluster-scoped resources = %+v, %v", cluster, err)
	}

	for _, args := range [][]string{
		{"kubectl", "--context=dev", "get", "issuers.v2.hearth.example", "--output=json", "--request-timeout=15s"},
		definitions,
	} {
		fake.Set(run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden): resource is forbidden")}, nil, args...)
		cluster, err = Read(t.Context(), &fake, nil, target)
		kerr, ok := errors.AsType[*Error](err)
		if !ok || kerr.Access.State != detect.KubeForbidden || cluster.Resources != nil {
			t.Fatalf("expected explicit denial, got %+v, %v", cluster, err)
		}
	}
}

func TestProbeCustomPresence(t *testing.T) {
	t.Parallel()
	var (
		mu       sync.Mutex
		requests []string
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Accept") != metadataListAccept {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		if r.URL.Query().Get("limit") != "1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/hearth.example/v1/namespaces/digital/certificates":
			_, _ = fmt.Fprint(w, `{"items":[{"metadata":{"name":"tls-app"}}]}`)
		case "/apis/flink.apache.org/v1beta1/namespaces/digital/flinkbluegreendeployments":
			_, _ = fmt.Fprint(w, `{"items":[]}`)
		case "/apis/cluster.example/v1/clusterthings":
			_, _ = fmt.Fprint(w, `{"items":[{"metadata":{"name":"global-one"}}]}`)
		case "/apis/broken.example/v1/namespaces/digital/brokens":
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"kind":"Status","reason":"Forbidden","message":"forbidden"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	env := kubeconfig(t, srv, "{token: secret}")
	certDef := CustomDefinition{Group: "hearth.example", Scope: "Namespaced", Versions: []CustomVersion{{Name: "v1", Served: true, Storage: true}}}
	certDef.Names.Plural = "certificates"
	emptyDef := CustomDefinition{Group: "flink.apache.org", Scope: "Namespaced", Versions: []CustomVersion{{Name: "v1beta1", Served: true, Storage: true}}}
	emptyDef.Names.Plural = "flinkbluegreendeployments"
	clusterDef := CustomDefinition{Group: "cluster.example", Scope: "Cluster", Versions: []CustomVersion{{Name: "v1", Served: true, Storage: true}}}
	clusterDef.Names.Plural = "clusterthings"
	brokenDef := CustomDefinition{Group: "broken.example", Scope: "Namespaced", Versions: []CustomVersion{{Name: "v1", Served: true, Storage: true}}}
	brokenDef.Names.Plural = "brokens"

	target := Target{API: NewAPI(), Context: "kind-dev", Namespaces: []string{"digital"}}
	got := ProbeCustomPresence(t.Context(), env, target, []CustomDefinition{certDef, emptyDef, clusterDef, brokenDef})
	if len(got) != 3 {
		t.Fatalf("got = %#v, want 3 entries (broken must fail open)", got)
	}
	if !got[certDef.Name()] {
		t.Errorf("certificates presence = false, want true")
	}
	if has, ok := got[emptyDef.Name()]; !ok || has {
		t.Errorf("flinkbluegreendeployments = (%t, %t), want (false, true)", has, ok)
	}
	if !got[clusterDef.Name()] {
		t.Errorf("clusterthings presence = false, want true")
	}
	if _, ok := got[brokenDef.Name()]; ok {
		t.Errorf("broken CRD should be omitted (fail open), got %#v", got)
	}
	if len(requests) != 4 {
		t.Errorf("requests = %v, want 4", requests)
	}

	// Nil API fails open cleanly.
	if nilGot := ProbeCustomPresence(t.Context(), env, Target{Context: "kind-dev"}, []CustomDefinition{certDef}); nilGot != nil {
		t.Errorf("nil API returned %#v, want nil", nilGot)
	}
}
