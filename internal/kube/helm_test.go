package kube

import (
	"errors"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestReadHelm(t *testing.T) {
	target := Target{Helm: "/bin/helm", HelmVersion: "3.16.3", Context: "dev", Namespaces: []string{"shop", "tools"}}
	var f run.Fake
	f.Stdout(`[{"name":"web","namespace":"shop","revision":"12","chart":"web-1.0.3","app_version":"2.4","status":"failed"}]`,
		"/bin/helm", "--kube-context=dev", "list", "--max=0", "--output=json", "--namespace=shop", "--all")
	f.Stdout(`[{"name":"web","namespace":"tools","revision":"2","status":"pending-upgrade"}]`,
		"/bin/helm", "--kube-context=dev", "list", "--max=0", "--output=json", "--namespace=tools", "--all")
	f.Stdout(`[{"revision":12,"status":"failed","description":"The deployment timed out"}]`,
		"/bin/helm", "--kube-context=dev", "history", "--namespace=shop", "--max=1", "--output=json", "--", "web")
	f.Stdout(`[{"revision":2,"status":"deployed","description":"Upgrade complete"}]`,
		"/bin/helm", "--kube-context=dev", "history", "--namespace=tools", "--max=1", "--output=json", "--", "web")
	env := sysenv.Env{"KUBECONFIG=/config", "HELM_KUBEAPISERVER=https://wrong.example", "HELM_KUBETOKEN=other-account", "HELM_DRIVER=configmap"}
	c, err := Read(t.Context(), &f, env, target)
	if err != nil || len(c.HelmReleases) != 2 || c.Taken.IsZero() {
		t.Fatalf("cluster = %+v, error = %v", c, err)
	}
	failed, pending := c.HelmReleases[0], c.HelmReleases[1]
	if failed.Chart != "web-1.0.3" || failed.AppVersion != "2.4" || failed.Revision != "12" ||
		failed.Namespace != "shop" || failed.Status != "failed" || failed.Description != "The deployment timed out" {
		t.Errorf("failed release = %+v", failed)
	}
	if !pending.Pending() || pending.Description != "" {
		t.Errorf("pending release = %+v", pending)
	}
	if calls := f.Calls(); len(calls) != 4 || !slices.Equal(calls[0].Env, []string{"KUBECONFIG=/config", "HELM_DRIVER=configmap"}) {
		t.Errorf("unexpected commands or environment: %+v", calls)
	}
}

func TestReadHelmEmptyOrDenied(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		denied        bool
	}{
		{name: "empty", version: "4.3.2"},
		{name: "unknown version"},
		{name: "forbidden", version: "4.3.2", denied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var f run.Fake
			args := []string{"/bin/helm", "--kube-context=dev", "list", "--max=0", "--output=json", "--all-namespaces"}
			if tc.version == "" {
				f.Set(run.Result{Stderr: []byte("Error: unknown flag: --all"), ExitCode: 1}, nil, append(slices.Clone(args), "--all")...)
			}
			if tc.denied {
				f.Set(run.Result{Stderr: []byte("secrets is forbidden: cannot list resource"), ExitCode: 1}, nil, args...)
			} else {
				f.Stdout("[]", args...)
			}
			c, err := Read(t.Context(), &f, nil, Target{Helm: args[0], HelmVersion: tc.version, Context: "dev"})
			if tc.denied {
				e, ok := errors.AsType[*Error](err)
				if !ok || e.Access.State != detect.KubeForbidden || c.HelmReleases != nil {
					t.Fatalf("cluster = %+v, error = %v", c, err)
				}
			} else if err != nil || c.HelmReleases == nil || len(c.HelmReleases) != 0 {
				t.Fatalf("cluster = %+v, error = %v", c, err)
			}
		})
	}
}
