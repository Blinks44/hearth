package kube

import (
	"errors"
	"os"
	"slices"
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
