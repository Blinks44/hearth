package detect

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type dockerConfig struct {
	CurrentContext      string
	CLIPluginsExtraDirs []string
	CredsStore          string
	CredHelpers         map[string]string
	Auths               map[string]json.RawMessage
}

func readDockerConfig(dir string) (dockerConfig, error) {
	var cfg dockerConfig
	if dir == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(data, &cfg)
}

func dockerConfigHealth(d *inventory.DockerState, cfg dockerConfig, env sysenv.Env) {
	replacement := docker.CredentialReplacement(env)
	report := func(helper inventory.DockerCredentialIssue) bool {
		if helper.Name == "" || docker.CredentialHelperAvailable(env, helper.Name) {
			return false
		}
		if !docker.CredentialHelperName(helper.Name) {
			helper.Name = ""
		} else {
			helper.Replacement = replacement
		}
		d.CredentialIssues = append(d.CredentialIssues, helper)
		return true
	}
	registries := map[string]struct{}{}
	missingStore := report(inventory.DockerCredentialIssue{Name: cfg.CredsStore})
	for key := range cfg.Auths {
		if name := docker.RegistryName(key); name != "" {
			registries[name] = struct{}{}
		}
	}
	for key, helper := range cfg.CredHelpers {
		if name := docker.RegistryName(key); name != "" {
			report(inventory.DockerCredentialIssue{Registry: name, Key: key, Name: helper})
			registries[name] = struct{}{}
		}
	}
	for name := range registries {
		key := name
		if name == docker.HubRegistry {
			key = "https://index.docker.io/v1/"
		}
		missing := missingStore
		if helper, ok := cfg.CredHelpers[key]; ok {
			// Even an empty override takes precedence over credsStore.
			missing = helper != "" && !docker.CredentialHelperAvailable(env, helper)
		}
		d.Registries = append(d.Registries, inventory.DockerRegistry{Name: name, MissingHelper: missing})
	}
	slices.SortFunc(d.Registries, func(a, b inventory.DockerRegistry) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(d.CredentialIssues, func(a, b inventory.DockerCredentialIssue) int { return strings.Compare(a.Key, b.Key) })
}

func brokenDockerPlugins(dir string) []string {
	if dir == "" {
		return nil
	}
	files, _ := os.ReadDir(dir)
	var broken []string
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "docker-") || file.Type()&os.ModeSymlink == 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, file.Name())); errors.Is(err, os.ErrNotExist) {
			broken = append(broken, file.Name())
		}
	}
	return broken
}

func dockerSocketHealth(ctx context.Context, r run.Runner, env sysenv.Env, exe, defaultEndpoint string, selected inventory.DockerContext) string {
	defaultPath, unix := strings.CutPrefix(defaultEndpoint, "unix://")
	selectedPath, alsoUnix := strings.CutPrefix(selected.Endpoint, "unix://")
	same := selected.Endpoint == defaultEndpoint
	if !same && unix && alsoUnix {
		real, err := filepath.EvalSymlinks(defaultPath)
		if errors.Is(err, os.ErrNotExist) && selected.Status == "available" {
			return "missing"
		}
		other, errOther := filepath.EvalSymlinks(selectedPath)
		if err != nil || errOther != nil && !errors.Is(errOther, os.ErrNotExist) {
			return ""
		}
		same = real == other
		info, err := os.Stat(real)
		if !same && (err != nil || info.Mode()&os.ModeSocket == 0) {
			return ""
		}
	}
	if same {
		if selected.Status == "unavailable" && !selected.Denied {
			return "unavailable"
		}
		return ""
	}
	if unix != alsoUnix || selected.Status == "" {
		return ""
	}
	target := docker.Target{Docker: exe, Endpoint: defaultEndpoint}
	cmd := target.Command(env, "version", "--format", "{{.Server.Version}}")
	cmd.Timeout = time.Second
	res, err := r.Run(ctx, cmd)
	if err != nil || res.ExitCode != 0 || res.Truncated || strings.TrimSpace(string(res.Stdout)) == "" {
		return "unavailable"
	}
	return "different"
}
