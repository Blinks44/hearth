package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDockerHealthSnapshotKeepsOnlyRegistryNames(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	writeFile(t, filepath.Join(dir, "config.json"), `{
		"credsStore":"desktop",
		"credHelpers":{"file-store.example":"","ghcr.io":"gone","registry.example:5000":"working"},
		"auths":{
			"https://index.docker.io/v1/":{"auth":"test-secret-auth"},
			"ghcr.io":{"identitytoken":"test-secret-identity"},
			"registry.example:5000":{"registrytoken":"test-secret-registry"},
			"https://test-secret-user:test-secret-password@example.com/path?token=test-secret-query":{}
		}
	}`)
	exe := filepath.Join(bin, "docker-credential-working"+dockerSuffix)
	writeFile(t, exe, "")
	if err := os.Chmod(exe, 0o755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"PATH=" + bin, "DOCKER_CONFIG=" + dir, "PATHEXT=.EXE"}
	var runner run.Fake
	st, err := detectDocker(t.Context(), &runner, env, env, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := st.Docker
	want := []inventory.DockerRegistry{{Name: "docker.io", MissingHelper: true}, {Name: "file-store.example"}, {Name: "ghcr.io", MissingHelper: true}, {Name: "registry.example:5000"}}
	if !slices.Equal(d.Registries, want) || len(d.CredentialIssues) != 2 ||
		d.CredentialIssues[0].Name != "desktop" || d.CredentialIssues[1].Registry != "ghcr.io" {
		t.Fatalf("registries = %+v, credential issues = %+v", d.Registries, d.CredentialIssues)
	}
	data, err := json.Marshal(st)
	if err != nil || strings.Contains(string(data), "test-secret") {
		t.Fatalf("private Docker values escaped to the snapshot or marshal failed: %v", err)
	}
	if len(runner.Calls()) != 0 {
		t.Fatal("the scan ran a credential helper")
	}
}

func TestDockerDefaultNamedPipe(t *testing.T) {
	const endpoint = "npipe:////./pipe/docker_engine"
	env := sysenv.Env{"DOCKER_CONTEXT=elsewhere"}
	for _, tc := range []struct {
		name, endpoint, status, want string
		res                          run.Result
	}{
		{name: "same", endpoint: endpoint, status: "available"},
		{name: "selected default stopped", endpoint: endpoint, status: "unavailable", want: "unavailable"},
		{name: "different engine", endpoint: "npipe:////./pipe/other", status: "available", want: "different", res: run.Result{Stdout: []byte("29.1.3")}},
		{name: "default stopped", endpoint: "npipe:////./pipe/other", status: "available", want: "unavailable", res: run.Result{ExitCode: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r run.Fake
			r.Set(tc.res, nil, "docker", "--host", endpoint, "version", "--format", "{{.Server.Version}}")
			got := dockerSocketHealth(t.Context(), &r, env, "docker", endpoint, inventory.DockerContext{Endpoint: tc.endpoint, Status: tc.status})
			if got != tc.want {
				t.Errorf("socket issue = %q, want %q", got, tc.want)
			}
			if len(r.Calls()) != 0 && sysenv.Env(r.Calls()[0].Env).Get("DOCKER_CONTEXT") != "" {
				t.Error("probe used the terminal's context override")
			}
		})
	}
}
