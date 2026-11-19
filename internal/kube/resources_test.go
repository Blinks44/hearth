package kube

import (
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestReadResources(t *testing.T) {
	target := Target{Kubectl: "kubectl", Context: "dev", Namespaces: []string{"shop", "tools"}}
	table := []struct {
		kind  ResourceKind
		input string
		check func(Resource) bool
	}{
		{StatefulSets, `"spec":{"replicas":3,"serviceName":"db","podManagementPolicy":"OrderedReady"},"status":{"readyReplicas":1,"updatedReplicas":2}`,
			func(r Resource) bool {
				return r.Desired == 3 && r.Ready == 1 && r.Updated == 2 && r.Service == "db" && r.PodOrder == "OrderedReady"
			}},
		{DaemonSets, `"status":{"desiredNumberScheduled":3,"numberReady":2,"updatedNumberScheduled":3}`,
			func(r Resource) bool { return r.Desired == 3 && r.Ready == 2 && r.Updated == 3 }},
		{ReplicaSets, `"spec":{"replicas":1},"status":{"readyReplicas":0}`,
			func(r Resource) bool { return r.Desired == 1 && r.Ready == 0 }},
		{Ingresses, `"spec":{"ingressClassName":"nginx","tls":[{"hosts":["shop.local"],"secretName":"shop-tls"}],"rules":[{"host":"shop.local","http":{"paths":[{"path":"/api","backend":{"service":{"name":"api","port":{"number":8080}}}}]}}]},"status":{"loadBalancer":{"ingress":[{"ip":"192.0.2.4"}]}}`,
			func(r Resource) bool {
				return r.IngressClass == "nginx" && slices.Equal(r.Addresses, []string{"192.0.2.4"}) && slices.Equal(r.TLSSecrets, []string{"shop-tls"}) &&
					len(r.Routes) == 1 && r.Routes[0].URL() == "https://shop.local/api" && r.Routes[0].Service == "api" && r.Routes[0].Port == "8080"
			}},
		{Endpoints, `"subsets":[{"addresses":[{"ip":"10.0.0.1"}],"notReadyAddresses":[{"ip":"10.0.0.2"}],"ports":[{"port":8080,"protocol":"TCP"}]}]`,
			func(r Resource) bool {
				return r.Ready == 1 && r.Desired == 2 && slices.Equal(r.Addresses, []string{"10.0.0.1"}) && len(r.Ports) == 1 && r.Ports[0].Port == 8080
			}},
		{NetworkPolicies, `"spec":{"podSelector":{"matchLabels":{"app":"api"}},"policyTypes":["Ingress","Egress"],"ingress":[{}]}`,
			func(r Resource) bool {
				return r.Selector["app"] == "api" && slices.Equal(r.PolicyTypes, []string{"Ingress", "Egress"}) && r.IngressRules == 1 && r.EgressRules == 0
			}},
		{ConfigMaps, `"data":{"WEB":"test"},"binaryData":{"ASSET":"AA=="},"immutable":true`,
			func(r Resource) bool { return slices.Equal(r.Keys, []string{"ASSET", "WEB"}) && r.Immutable }},
	}
	for _, test := range table {
		t.Run(string(test.kind), func(t *testing.T) {
			var fake run.Fake
			prefix := `{"items":[{"metadata":{"name":"demo","namespace":"shop","uid":"first"},`
			fake.Stdout(prefix+test.input+`}]}`, "kubectl", "--context=dev", "get", string(test.kind), "--namespace=shop", "--output=json", "--request-timeout=15s")
			fake.Stdout(`{"items":[]}`, "kubectl", "--context=dev", "get", string(test.kind), "--namespace=tools", "--output=json", "--request-timeout=15s")
			got, err := ReadResources(t.Context(), &fake, nil, target, test.kind)
			if err != nil || len(got) != 1 || got[0].UID != "first" || got[0].Kind != test.kind || !test.check(got[0]) {
				t.Fatalf("resources = %+v, %v", got, err)
			}
		})
	}
}

func TestReadResourceYAML(t *testing.T) {
	var fake run.Fake
	target := Target{Kubectl: "kubectl", Context: "dev"}
	resource := Resource{Kind: ConfigMaps, Namespace: "shop", Name: "settings", UID: "first"}
	args := []string{"kubectl", "--context=dev", "get", "configmaps", "--namespace=shop", "--output=yaml", "--request-timeout=15s", "--", "settings"}
	const manifest = "kind: ConfigMap\nmetadata:\n  uid: first\n"
	fake.Stdout(manifest, args...)
	if got, err := ReadResourceYAML(t.Context(), &fake, nil, target, resource); err != nil || got != manifest {
		t.Fatalf("YAML = %q, %v", got, err)
	}
	fake.Stdout("kind: ConfigMap\nmetadata:\n  uid: replacement\n", args...)
	if got, err := ReadResourceYAML(t.Context(), &fake, nil, target, resource); err == nil || got != "" {
		t.Fatalf("replacement YAML = %q, %v", got, err)
	}
}

func TestReadResourceClusterKeepsListOnEventDenial(t *testing.T) {
	var fake run.Fake
	target := Target{Kubectl: "kubectl", Context: "dev", Resource: StatefulSets, Events: Ref{Namespace: "shop", UID: "first"}}
	fake.Stdout(`{"items":[{"metadata":{"name":"db","namespace":"shop","uid":"first"}}]}`,
		"kubectl", "--context=dev", "get", "statefulsets.apps", "--all-namespaces", "--output=json", "--request-timeout=15s")
	fake.Set(run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden): events is forbidden")}, nil,
		"kubectl", "--context=dev", "get", "events", "--namespace=shop", "--field-selector=involvedObject.uid=first", "--output=json", "--request-timeout=15s")
	got, err := Read(t.Context(), &fake, nil, target)
	if err != nil || got.Resources == nil || len(got.Resources.Items) != 1 || got.Resources.EventsErr == nil || got.EventsOf != target.Events {
		t.Fatalf("cluster = %+v, %v", got, err)
	}
}

func TestIngressURL(t *testing.T) {
	for _, host := range []string{"", "*.shop.local", "user@shop.local", "shop.local/other", "https://shop.local"} {
		if address := (IngressRoute{Host: host}).URL(); address != "" {
			t.Errorf("unexpected URL for %q: %s", host, address)
		}
	}
	if got := (IngressRoute{Host: "shop.local", Path: "/x?y#z"}).URL(); got != "http://shop.local/x%3Fy%23z" {
		t.Errorf("encoded path = %q", got)
	}
	if got := (IngressRoute{Host: "2001:db8::1"}).URL(); got != "http://[2001:db8::1]/" {
		t.Errorf("IPv6 address = %q", got)
	}
}
