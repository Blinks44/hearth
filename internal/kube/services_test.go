package kube

import (
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestReadServices(t *testing.T) {
	const services = `{"items":[
		{"metadata":{"name":"web","namespace":"shop","uid":"svc"},
		 "spec":{"type":"ClusterIP","clusterIP":"10.96.1.2","selector":{"app":"web","tier":""},
		         "ports":[{"port":80,"protocol":"TCP","targetPort":"http"},{"port":53,"protocol":"UDP","targetPort":53}]}},
		{"metadata":{"name":"alias","namespace":"shop"},"spec":{"type":"ExternalName"}},
		{"metadata":{"name":"idle","namespace":"shop"},"spec":{"type":"ClusterIP","selector":{"app":"idle"}}}
	]}`
	const pods = `{"items":[
		{"metadata":{"namespace":"shop","labels":{"app":"web","tier":""}},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"namespace":"shop","labels":{"app":"web","tier":""}},"status":{"conditions":[{"type":"Ready","status":"False"}]}},
		{"metadata":{"namespace":"tools","labels":{"app":"web","tier":""}}},
		{"metadata":{"namespace":"shop","labels":{"app":"web"}}}
	]}`
	for _, tt := range []struct {
		name  string
		pods  run.Result
		known bool
	}{
		{"counts by selector and namespace", run.Result{Stdout: []byte(pods)}, true},
		{"no permission to list pods", run.Result{ExitCode: 1, Stderr: []byte("Forbidden")}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var runner run.Fake
			runner.Stdout(services, "kubectl", "--context=kind-dev", "get", "services", "--all-namespaces", "--output=json", "--request-timeout=15s")
			runner.Set(tt.pods, nil, "kubectl", "--context=kind-dev", "get", "pods", "--all-namespaces", "--output=json", "--request-timeout=15s")
			cl, err := Read(t.Context(), &runner, nil, Target{Kubectl: "kubectl", Context: "kind-dev", Services: true})
			if err != nil || len(cl.Services) != 3 {
				t.Fatalf("Read = %+v, %v", cl, err)
			}
			web, alias, idle := cl.Services[0], cl.Services[1], cl.Services[2]
			if !web.HasSelector || web.PodsKnown != tt.known || web.Ports[0].String() != "80 → http/TCP" || web.Ports[1].String() != "53/UDP" ||
				tt.known && (web.Pods != 2 || web.Ready != 1) {
				t.Errorf("web = %+v", web)
			}
			if !alias.External || alias.HasSelector || alias.PodsKnown || idle.PodsKnown != tt.known || idle.Pods != 0 {
				t.Errorf("alias = %+v, idle = %+v", alias, idle)
			}
		})
	}
}

func TestReadServiceDetailsAndEvents(t *testing.T) {
	const services = `{"items":[
		{"metadata":{"name":"web","namespace":"shop","uid":"svc1","creationTimestamp":"2026-05-01T10:00:00Z","labels":{"tier":"frontend"}},
		 "spec":{"type":"LoadBalancer","clusterIP":"10.96.1.2","externalIPs":["203.0.113.5"],"sessionAffinity":"ClientIP",
		         "externalTrafficPolicy":"Local","internalTrafficPolicy":"Cluster","selector":{"app":"web"},
		         "ports":[{"name":"https","port":443,"nodePort":30443,"protocol":"TCP","targetPort":8443}]},
		 "status":{"loadBalancer":{"ingress":[{"ip":"198.51.100.10"},{"hostname":"lb.example"}]}}}
	]}`
	var runner run.Fake
	runner.Stdout(services, "kubectl", "--context=kind-dev", "get", "services", "--namespace=shop", "--output=json", "--request-timeout=15s")
	runner.Stdout(`{"items":[]}`, "kubectl", "--context=kind-dev", "get", "pods", "--namespace=shop", "--output=json", "--request-timeout=15s")
	runner.Stdout(`{"items":[]}`, "kubectl", "--context=kind-dev", "get", "endpoints", "--namespace=shop", "--field-selector=metadata.name=web", "--output=json", "--request-timeout=15s")
	runner.Stdout(`{"items":[{"type":"Normal","reason":"EnsuredLoadBalancer","message":"Ensured load balancer"}]}`,
		"kubectl", "--context=kind-dev", "get", "events", "--namespace=shop", "--field-selector=involvedObject.uid=svc1", "--output=json", "--request-timeout=15s")

	cl, err := Read(t.Context(), &runner, nil, Target{
		Kubectl: "kubectl", Context: "kind-dev", Namespaces: []string{"shop"}, Services: true,
		Events: Ref{Namespace: "shop", UID: "svc1"},
	})
	if err != nil || len(cl.Services) != 1 {
		t.Fatalf("Read = %+v, %v", cl, err)
	}
	svc := cl.Services[0]
	if svc.Created.IsZero() || svc.SessionAffinity != "ClientIP" || svc.ExternalTrafficPolicy != "Local" ||
		svc.Selector["app"] != "web" || svc.Labels["tier"] != "frontend" ||
		len(svc.LoadBalancer) != 2 || len(svc.ExternalIPs) != 1 ||
		svc.Ports[0].DetailString() != "https: 443 → 8443/TCP (NodePort 30443)" {
		t.Fatalf("unexpected service details: %+v", svc)
	}
	if cl.EventsErr != nil || len(cl.Events) != 1 || cl.Events[0].Reason != "EnsuredLoadBalancer" {
		t.Fatalf("events = %+v, err = %v", cl.Events, cl.EventsErr)
	}
}
