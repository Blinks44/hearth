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
