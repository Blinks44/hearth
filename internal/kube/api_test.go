package kube

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// fakeAPI serves aggregated discovery of pods, nodes, ReplicaSets and
// Deployments, or only the older discovery when legacy, and lists by
// path; lists' items come without their kind, as the API sends them.
func fakeAPI(t *testing.T, lists map[string]string, legacy bool) *httptest.Server {
	t.Helper()
	discovery := map[string]string{
		"/api": `{"kind":"APIGroupDiscoveryList","items":[{"metadata":{"name":""},"versions":[{"version":"v1","resources":[
			{"resource":"pods","scope":"Namespaced","responseKind":{"kind":"Pod"}},
			{"resource":"nodes","scope":"Cluster","responseKind":{"kind":"Node"}}]}]}]}`,
		"/apis": `{"kind":"APIGroupDiscoveryList","items":[{"metadata":{"name":"apps"},"versions":[{"version":"v1","resources":[
			{"resource":"replicasets","scope":"Namespaced","responseKind":{"kind":"ReplicaSet"}},
			{"resource":"deployments","scope":"Namespaced","responseKind":{"kind":"Deployment"}}]}]}]}`,
	}
	if legacy {
		discovery["/api"] = `{"kind":"APIVersions","versions":["v1"]}`
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if body, ok := discovery[r.URL.Path]; ok {
			_, _ = fmt.Fprint(w, body)
			return
		}
		body, ok := lists[r.URL.RequestURI()]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"kind":"Status","reason":"NotFound","message":"%s not found"}`, r.URL.Path)
			return
		}
		if code, rest, found := strings.Cut(body, " "); found && strings.HasPrefix(code, "4") {
			w.WriteHeader(http.StatusForbidden)
			body = rest
		}
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// kubeconfig writes a kubeconfig whose context kind-dev signs in to the
// server with user (client-go sends credentials over TLS only), and returns the environment that names it.
func kubeconfig(t *testing.T, srv *httptest.Server, user string) sysenv.Env {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	ca := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	data := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: dev
  cluster: {server: %q, certificate-authority-data: %s}
users:
- name: dev
  user: %s
contexts:
- name: kind-dev
  context: {cluster: dev, user: dev, namespace: shop}
`, srv.URL, ca, user)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return sysenv.Env{"KUBECONFIG=" + path}
}

// shopLists splits testdata/shop.json into the API's list per kind.
func shopLists(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("testdata/shop.json")
	if err != nil {
		t.Fatal(err)
	}
	var shop struct{ Items []map[string]any }
	if err := json.Unmarshal(data, &shop); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"Pod":        "/api/v1/namespaces/shop/pods",
		"ReplicaSet": "/apis/apps/v1/namespaces/shop/replicasets",
		"Deployment": "/apis/apps/v1/namespaces/shop/deployments",
	}
	byKind := map[string][]map[string]any{}
	for _, it := range shop.Items {
		kind := it["kind"].(string)
		delete(it, "kind")
		byKind[kind] = append(byKind[kind], it)
	}
	lists := map[string]string{}
	for kind, path := range paths {
		body, err := json.Marshal(map[string]any{"items": byKind[kind]})
		if err != nil {
			t.Fatal(err)
		}
		lists[path] = string(body)
	}
	return lists
}

func TestAPIReadsAsKubectl(t *testing.T) {
	data, err := os.ReadFile("testdata/shop.json")
	if err != nil {
		t.Fatal(err)
	}
	var f run.Fake
	f.Stdout(string(data), getArgv("--namespace=shop")...)
	want, err := Read(context.Background(), &f, nil, target)
	if err != nil {
		t.Fatal(err)
	}

	srv := fakeAPI(t, shopLists(t), false)
	env := kubeconfig(t, srv, "{token: secret}")
	var none run.Fake
	tg := target
	tg.API = NewAPI()
	for range 2 { // the second read uses the kept connection
		got, err := Read(context.Background(), &none, env, tg)
		if err != nil {
			t.Fatal(err)
		}
		got.Taken = want.Taken
		if !reflect.DeepEqual(got, want) {
			t.Errorf("through the API:\n%+v\nthrough kubectl:\n%+v", got, want)
		}
	}
	if calls := none.Calls(); len(calls) != 0 {
		t.Errorf("kubectl ran: %v", calls)
	}
}

func TestAPIFailures(t *testing.T) {
	lists := shopLists(t)
	lists["/api/v1/namespaces/shop/pods"] = `403 {"kind":"Status","reason":"Forbidden","message":"pods is forbidden: User \"dev\" cannot list resource \"pods\" in API group \"\" in the namespace \"shop\""}`
	srv := fakeAPI(t, lists, false)
	tests := []struct {
		name, user string
		args       []string
		want       detect.KubeAccessState
		detail     string
	}{
		{"forbidden", "{token: secret}", []string{"pods,replicasets,deployments", "--namespace=shop"}, detect.KubeForbidden,
			`Error from server (Forbidden): pods is forbidden: User "dev" cannot list resource "pods" in API group "" in the namespace "shop"`},
		{"unauthorized", "{token: wrong}", []string{"nodes"}, detect.KubeNeedsLogin,
			"error: You must be logged in to the server (Unauthorized)"},
		{"unknown kind", "{token: secret}", []string{"widgets.example.com"}, detect.KubeUnknown,
			`error: the server doesn't have a resource type "widgets.example.com"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := target
			tg.API = NewAPI()
			var list struct{ Items []json.RawMessage }
			err := get(context.Background(), &run.Fake{}, kubeconfig(t, srv, tt.user), tg, tt.args, &list)
			var kerr *Error
			if !errors.As(err, &kerr) {
				t.Fatalf("err = %v, want *Error", err)
			}
			if kerr.Access.State != tt.want || kerr.Access.Detail != tt.detail {
				t.Errorf("access = %+v, want state %d, %q", kerr.Access, tt.want, tt.detail)
			}
		})
	}
}

// What only kubectl shows or reads is left to it: a sign-in program
// that fails, with its own words, and an API without aggregated
// discovery.
func TestAPILeavesToKubectl(t *testing.T) {
	tests := []struct {
		name   string
		legacy bool
		user   string
	}{
		{"failed sign-in", false, `{exec: {apiVersion: client.authentication.k8s.io/v1, command: hearth-no-such-sign-in, interactiveMode: Never}}`},
		{"older API", true, "{token: secret}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeAPI(t, shopLists(t), tt.legacy)
			var f run.Fake
			f.Stdout(`{"items": []}`, getArgv("--namespace=shop")...)
			tg := target
			tg.API = NewAPI()
			if _, err := Read(context.Background(), &f, kubeconfig(t, srv, tt.user), tg); err != nil {
				t.Fatal(err)
			}
			if len(f.Calls()) != 1 {
				t.Errorf("kubectl calls = %d, want 1", len(f.Calls()))
			}
		})
	}
}

func TestParseGet(t *testing.T) {
	tests := []struct {
		args []string
		ok   bool
	}{
		{[]string{"pods,replicasets,deployments", "--all-namespaces", "--output=json"}, true},
		{[]string{"pod/web", "--namespace=shop"}, true},
		{[]string{"endpoints", "--namespace=shop", "--field-selector=metadata.name=web"}, true},
		{[]string{"--raw", "/apis/metrics.k8s.io/v1beta1/pods"}, true},
		{[]string{"pods", "--output=yaml"}, false},
		{[]string{"pod/"}, false},
		{[]string{"pods,"}, false},
	}
	for _, tt := range tests {
		if _, ok := parseGet(tt.args); ok != tt.ok {
			t.Errorf("parseGet(%q) = %t, want %t", tt.args, ok, tt.ok)
		}
	}
}
