package kube

import (
	"fmt"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestResolveForwardService(t *testing.T) {
	const pods = `{"items":[
		{"metadata":{"name":"old","uid":"old-uid","deletionTimestamp":"2026-11-19T10:00:00Z"},
		 "spec":{"containers":[{"ports":[{"name":"http","containerPort":9000}]}]},
		 "status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"starting","uid":"new-uid"},
		 "spec":{"containers":[{"ports":[{"name":"http","containerPort":8000}]}]},
		 "status":{"phase":"Running"}},
		{"metadata":{"name":"ready","uid":"ready-uid"},
		 "spec":{"containers":[{"ports":[{"name":"http","containerPort":8080,"protocol":"TCP"}]}]},
		 "status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}
	]}`
	for _, tc := range []struct {
		name, clusterIP, target string
		port                    int
	}{
		{"named target", "10.96.1.2", `"http"`, 8080},
		{"numeric target", "10.96.1.2", "3000", 3000},
		{"headless uses service port", "None", `"http"`, 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var runner run.Fake
			svc := fmt.Sprintf(`{"metadata":{"uid":"svc-uid"},"spec":{"clusterIP":%q,
				"selector":{"tier":"frontend","app":"web"},"ports":[
				{"port":80,"protocol":"UDP","targetPort":53},{"port":80,"protocol":"TCP","targetPort":%s}]}}`, tc.clusterIP, tc.target)
			runner.Stdout(svc, "kubectl", "--context=kind-dev", "get", "service/web", "--namespace=shop", "--output=json", "--request-timeout=15s")
			runner.Stdout(pods, "kubectl", "--context=kind-dev", "get", "pods", "--namespace=shop", "--selector=app=web,tier=frontend",
				"--output=json", "--request-timeout=15s")
			got, err := ResolveForwardPod(t.Context(), &runner, nil, Target{Kubectl: "kubectl", Context: "kind-dev"},
				Ref{Namespace: "shop", UID: "svc-uid"}, "service/web", 80)
			want := ForwardPod{Name: "ready", UID: "ready-uid", Port: tc.port}
			if err != nil || got != want {
				t.Fatalf("ResolveForwardPod = %+v, %v, want %+v", got, err, want)
			}
		})
	}
}

func TestForwardPodEnded(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		ended          bool
	}{
		{"unknown", `{"metadata":{"uid":"pod-uid"},"status":{"phase":"Unknown"}}`, false},
		{"succeeded", `{"metadata":{"uid":"pod-uid"},"status":{"phase":"Succeeded"}}`, true},
		{"replaced", `{"metadata":{"uid":"other"},"status":{"phase":"Running"}}`, true},
		{"deleting", `{"metadata":{"uid":"pod-uid","deletionTimestamp":"2026-11-19T10:00:00Z"},"status":{"phase":"Running"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var runner run.Fake
			runner.Stdout(tc.response, "kubectl", "--context=kind-dev", "get", "pod/web", "--namespace=shop", "--output=json", "--request-timeout=15s")
			if got := ForwardPodEnded(t.Context(), &runner, nil, Target{Kubectl: "kubectl", Context: "kind-dev"},
				"shop", ForwardPod{Name: "web", UID: "pod-uid"}); got != tc.ended {
				t.Errorf("ForwardPodEnded = %v, want %v", got, tc.ended)
			}
		})
	}
}
