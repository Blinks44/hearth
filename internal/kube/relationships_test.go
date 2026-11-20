package kube

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

func TestPodReferences(t *testing.T) {
	var p pod
	if err := json.Unmarshal([]byte(`{
		"metadata":{"name":"web-0","namespace":"shop"},
		"spec":{
			"serviceAccountName":"app",
			"imagePullSecrets":[{"name":"registry"}],
			"volumes":[
				{"name":"data","persistentVolumeClaim":{"claimName":"web-data"}},
				{"name":"cache","ephemeral":{"volumeClaimTemplate":{}}},
				{"name":"config","configMap":{"name":"settings"}},
				{"name":"credentials","secret":{"secretName":"db"}},
				{"name":"mixed","projected":{"sources":[{"configMap":{"name":"settings"}},{"secret":{"name":"proxy"}}]}},
				{"name":"kube-api-access-abcde","projected":{"sources":[{"configMap":{"name":"kube-root-ca.crt"}}]}}
			],
			"initContainers":[{"envFrom":[{"secretRef":{"name":"init"}}]}],
			"containers":[{"env":[{"name":"SIGNING_KEY","valueFrom":{"secretKeyRef":{"name":"signing","key":"private"}}}]}],
			"ephemeralContainers":[{"envFrom":[{"configMapRef":{"name":"diagnostics"}}]}]
		}
	}`), &p); err != nil {
		t.Fatal(err)
	}
	want := []Relationship{
		{Kind: "PersistentVolumeClaim", Namespace: "shop", Name: "web-data", Detail: "data"},
		{Kind: "PersistentVolumeClaim", Namespace: "shop", Name: "web-0-cache", Detail: "cache"},
		{Kind: "ConfigMap", Namespace: "shop", Name: "settings", Detail: "config"},
		{Kind: "Secret", Namespace: "shop", Name: "db", Detail: "credentials"},
		{Kind: "Secret", Namespace: "shop", Name: "proxy", Detail: "mixed"},
		{Kind: "Secret", Namespace: "shop", Name: "signing", Detail: "SIGNING_KEY"},
		{Kind: "Secret", Namespace: "shop", Name: "init"},
		{Kind: "ConfigMap", Namespace: "shop", Name: "diagnostics"},
		{Kind: "Secret", Namespace: "shop", Name: "registry"},
		{Kind: "ServiceAccount", Namespace: "shop", Name: "app"},
	}
	if got := p.references(); !slices.Equal(got, want) {
		t.Errorf("references = %+v, want %+v", got, want)
	}
}

func TestReadServiceRelationships(t *testing.T) {
	for _, tc := range []struct {
		name, selector  string
		pods, endpoints run.Result
		names           []string
		errors          int
	}{
		{"selector and endpoints", `,"selector":{"app":"web"}`,
			run.Result{Stdout: []byte(`{"items":[{"metadata":{"name":"selected","namespace":"shop","uid":"pod-selected","labels":{"app":"web"}}},
			{"metadata":{"name":"other","namespace":"tools","uid":"pod-other","labels":{"app":"web"}}}]}`)},
			run.Result{Stdout: []byte(`{"items":[{"metadata":{"namespace":"shop","name":"web"},"subsets":[{
				"addresses":[{"targetRef":{"kind":"Pod","name":"selected","uid":"pod-selected"}},{"ip":"1.2.3.4"}],
				"notReadyAddresses":[{"targetRef":{"kind":"Pod","name":"backend","uid":"pod-backend","namespace":"shop"}},
					{"targetRef":{"kind":"Pod","name":"other","namespace":"tools"}}]}]}]}`)}, []string{"selected", "backend"}, 0},
		{"forbidden pod list still shows endpoints", `,"selector":{"app":"web"}`,
			run.Result{ExitCode: 1, Stderr: []byte("Forbidden")},
			run.Result{Stdout: []byte(`{"items":[{"metadata":{"namespace":"shop","name":"web"},"subsets":[{"addresses":[{"targetRef":{"kind":"Pod","name":"backend","uid":"pod-backend"}}]}]}]}`)},
			[]string{"backend"}, 1},
		{"selectorless", "", run.Result{}, run.Result{Stdout: []byte(`{"items":[{"metadata":{"namespace":"shop","name":"web"},"subsets":[{"addresses":[{"targetRef":{"kind":"Pod","name":"backend","uid":"pod-backend"}}]}]}]}`)},
			[]string{"backend"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var runner run.Fake
			target := Target{Kubectl: "kubectl", Context: "kind-dev", Services: true, Events: Ref{Namespace: "shop", UID: "svc"}}
			runner.Stdout(`{"items":[{"metadata":{"name":"web","namespace":"shop","uid":"svc"},"spec":{"type":"ClusterIP"`+tc.selector+`}}]}`,
				"kubectl", "--context=kind-dev", "get", "services", "--all-namespaces", "--output=json", "--request-timeout=15s")
			runner.Set(tc.pods, nil, "kubectl", "--context=kind-dev", "get", "pods", "--all-namespaces", "--output=json", "--request-timeout=15s")
			runner.Set(tc.endpoints, nil, "kubectl", "--context=kind-dev", "get", "endpoints", "--namespace=shop", "--field-selector=metadata.name=web", "--output=json", "--request-timeout=15s")
			cl, err := Read(t.Context(), &runner, nil, target)
			if err != nil || cl.Relationships == nil {
				t.Fatalf("read service relationships: %+v, %v", cl, err)
			}
			d := cl.Relationships
			var names []string
			for _, link := range d.Links {
				if link.Kind != "Pod" || link.Namespace != "shop" || link.UID == "" {
					t.Errorf("invalid backend %+v", link)
				}
				names = append(names, link.Name)
			}
			if !slices.Equal(names, tc.names) || len(d.Errors) != tc.errors {
				t.Errorf("links = %v, errors = %v", d.Links, d.Errors)
			}
		})
	}
}

func TestReadDeploymentBudgets(t *testing.T) {
	var runner run.Fake
	add := func(data, kind string, args ...string) {
		argv := append([]string{"kubectl", "--context=kind-dev", "get", kind, "--namespace=shop"}, args...)
		runner.Stdout(data, append(argv, "--output=json", "--request-timeout=15s")...)
	}
	add(`{"items":[{"kind":"Deployment","metadata":{"name":"web","namespace":"shop","uid":"d"},
		"spec":{"replicas":0,"template":{"metadata":{"labels":{"app":"web","role":"primary"}}}}}]}`, "pods,replicasets,deployments")
	add(`{"items":[]}`, "events", "--field-selector=involvedObject.uid=d")
	add(`{"items":[{"metadata":{"namespace":"shop","name":"scale","uid":"h"},
		"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"web"}}}]}`, string(Autoscalers))
	add(`{"items":[
		{"metadata":{"namespace":"shop","name":"all","uid":"p1"},"spec":{"selector":{}}},
		{"metadata":{"namespace":"shop","name":"matching","uid":"p2"},"spec":{"selector":{"matchLabels":{"app":"web"},"matchExpressions":[
			{"key":"role","operator":"In","values":["primary"]},{"key":"tier","operator":"NotIn","values":["frontend"]},
			{"key":"app","operator":"Exists"},{"key":"retired","operator":"DoesNotExist"}]}}},
		{"metadata":{"namespace":"shop","name":"null","uid":"p3"},"spec":{"selector":null}},
		{"metadata":{"namespace":"shop","name":"mismatch","uid":"p4"},"spec":{"selector":{"matchLabels":{"app":"another"}}}},
		{"metadata":{"namespace":"outside","name":"other","uid":"p5"},"spec":{"selector":{}}}
	]}`, string(DisruptionBudgets))
	target := Target{Kubectl: "kubectl", Context: "kind-dev", Namespaces: []string{"shop"}, DeploymentAutoscalers: true, Events: Ref{Namespace: "shop", UID: "d"}}
	cl, err := Read(t.Context(), &runner, nil, target)
	if err != nil || cl.Relationships == nil {
		t.Fatalf("read deployment relationships: %+v, %v", cl, err)
	}
	want := []Relationship{
		{Kind: "HorizontalPodAutoscaler", Namespace: "shop", Name: "scale", UID: "h", Inbound: true},
		{Kind: "PodDisruptionBudget", Namespace: "shop", Name: "all", UID: "p1", Inbound: true},
		{Kind: "PodDisruptionBudget", Namespace: "shop", Name: "matching", UID: "p2", Inbound: true},
	}
	if d := cl.Relationships; len(d.Errors) != 0 || !slices.Equal(d.Links, want) {
		t.Errorf("relationships = %+v, want %v", d, want)
	}

	eventArgs := []string{"kubectl", "--context=kind-dev", "get", "events", "--namespace=shop",
		"--field-selector=involvedObject.uid=d", "--output=json", "--request-timeout=15s"}
	runner.Set(run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden): cannot list events")}, nil, eventArgs...)
	cl, err = Read(t.Context(), &runner, nil, target)
	if err != nil || len(cl.Deployments) != 1 || cl.Relationships == nil {
		t.Fatalf("forbidden events hid deployments or relationships: %+v, %v", cl, err)
	}
	if e, ok := errors.AsType[*Error](cl.EventsErr); !ok || e.Access.State != detect.KubeForbidden || cl.EventsOf != target.Events {
		t.Errorf("events belong to %+v with error %v, want a forbidden read for %+v", cl.EventsOf, cl.EventsErr, target.Events)
	}
	if len(cl.Deployments[0].Autoscalers) != 1 || !slices.Equal(cl.Relationships.Links, want) {
		t.Errorf("forbidden events hid HPA or PDB: %+v", cl.Relationships)
	}
}
