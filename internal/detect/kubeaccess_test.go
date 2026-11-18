package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// What kubectl says sorts a check into what is known of it; anything else
// stays unknown, with kubectl's words.
func TestCheckKubeAccess(t *testing.T) {
	bin := t.TempDir()
	// aws.exe for Windows' PATHEXT.
	for _, name := range []string{"aws", "aws.exe"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := sysenv.Env{"PATH=" + bin}
	version := []string{"kubectl", "--context=staging", "version", "--output=json", "--request-timeout=15s"}
	canI := []string{"kubectl", "--context=staging", "auth", "can-i", "list", "pods", "--namespace=shop", "--request-timeout=15s"}
	answered := `{"clientVersion":{"gitVersion":"v1.33.9"},"serverVersion":{"gitVersion":"v1.31.2"}}`
	fail := func(stderr string) run.Result {
		return run.Result{ExitCode: 1, Stdout: []byte(`{"clientVersion":{"gitVersion":"v1.33.9"}}`), Stderr: []byte(stderr)}
	}
	tests := []struct {
		name    string
		auth    string
		version run.Result
		canI    run.Result
		want    KubeAccessState
		detail  string
	}{
		{"answers and allows", "aws", run.Result{Stdout: []byte(answered)}, run.Result{Stdout: []byte("yes\n")}, KubeReachable, ""},
		{"answers, no pods", "aws", run.Result{Stdout: []byte(answered)}, run.Result{ExitCode: 1, Stdout: []byte("no\n")}, KubeForbidden, "kubectl auth can-i list pods -n shop: no"},
		{"program not on PATH", "gke-gcloud-auth-plugin", run.Result{}, run.Result{}, KubeNoAuthProgram, "gke-gcloud-auth-plugin"},
		{"program by path missing", "/opt/gone/aws",
			fail("Unable to connect to the server: getting credentials: exec: executable /opt/gone/aws not found\n\nIt looks like you are trying to use a client-go credential plugin that is not installed."),
			run.Result{}, KubeNoAuthProgram, "Unable to connect to the server: getting credentials: exec: executable /opt/gone/aws not found"},
		{"sso expired", "aws",
			fail("Error when retrieving token from sso: Token has expired and refresh failed\nUnable to connect to the server: getting credentials: exec: executable aws failed with exit code 255"),
			run.Result{}, KubeNeedsLogin, "Error when retrieving token from sso: Token has expired and refresh failed"},
		{"unauthorized", "", fail("error: You must be logged in to the server (Unauthorized)"), run.Result{}, KubeNeedsLogin, "error: You must be logged in to the server (Unauthorized)"},
		{"program fails otherwise", "aws",
			fail("plugin crashed: unexpected EOF\nUnable to connect to the server: getting credentials: exec: executable aws failed with exit code 1"),
			run.Result{}, KubeAuthFailed, "plugin crashed: unexpected EOF"},
		{"refused", "", fail(`The connection to the server 127.0.0.1:6443 was refused - did you specify the right host or port?`), run.Result{}, KubeUnreachable, "The connection to the server 127.0.0.1:6443 was refused - did you specify the right host or port?"},
		{"no host", "", fail(`Unable to connect to the server: dial tcp: lookup api.example.test: no such host`), run.Result{}, KubeUnreachable, "Unable to connect to the server: dial tcp: lookup api.example.test: no such host"},
		{"forbidden version", "", fail(`Error from server (Forbidden): forbidden: User "system:anonymous" cannot get path "/version"`), run.Result{}, KubeForbidden, `Error from server (Forbidden): forbidden: User "system:anonymous" cannot get path "/version"`},
		{"certificate", "", fail("Unable to connect to the server: tls: failed to verify certificate: x509: certificate signed by unknown authority"), run.Result{}, KubeUnknown, "Unable to connect to the server: tls: failed to verify certificate: x509: certificate signed by unknown authority"},
		{"can-i fails", "", run.Result{Stdout: []byte(answered)}, run.Result{ExitCode: 1, Stderr: []byte("error: You must be logged in to the server (Unauthorized)")}, KubeNeedsLogin, "error: You must be logged in to the server (Unauthorized)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &run.Fake{}
			f.Set(tc.version, nil, version...)
			f.Set(tc.canI, nil, canI...)
			k := inventory.KubeContext{Name: "staging", Namespace: "shop", Auth: tc.auth}
			got := CheckKubeAccess(t.Context(), f, "kubectl", k, env)
			if got.State != tc.want || got.Detail != tc.detail {
				t.Errorf("got %v %q, want %v %q", got.State, got.Detail, tc.want, tc.detail)
			}
			if tc.want == KubeReachable && got.Version != "v1.31.2" {
				t.Errorf("version %q", got.Version)
			}
			// A program known missing is not run for.
			if tc.name == "program not on PATH" && len(f.Calls()) != 0 {
				t.Errorf("ran %v", f.Calls())
			}
		})
	}
}
