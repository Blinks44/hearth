package detect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Docker finds Docker CLI copies and installed engines, reads contexts,
// and probes only the selected local endpoint. It never starts an engine,
// changes a context, invokes credential helpers or contacts a remote host.
func Docker(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	engines, candidates := dockerLocations(term, sources.ManagerDirs(term).Home)
	return detectDocker(ctx, r, term, gui, engines, candidates)
}

func detectDocker(ctx context.Context, r run.Runner, term, gui sysenv.Env, engines []inventory.DockerEngine, candidates []string) (inventory.ToolState, error) {
	f := newFinder("docker", term)
	f.lookPath("docker", term, gui)
	home := sources.ManagerDirs(term).Home
	for _, p := range candidates {
		if exists(p) {
			f.add(p)
		}
	}
	d := &inventory.DockerState{Engines: engines}
	st := inventory.ToolState{Tool: "docker", Installations: f.ins, Docker: d}
	for i := range st.Installations {
		in := &st.Installations[i]
		if in.Source.Shim {
			continue
		}
		for _, engine := range engines {
			if engine.Path != "" && within(in.RealPath, engine.Path) {
				in.Source = inventory.Source{Kind: inventory.KindInstaller, Manager: engine.Name, Root: engine.Path}
			}
		}
		v := versionOf(ctx, r, term, in.Path)
		if fields := strings.Fields(v); len(fields) >= 3 && fields[0] == "Docker" && fields[1] == "version" {
			in.Version = strings.TrimSuffix(fields[2], ",")
		}
	}
	config := term.Get("DOCKER_CONFIG")
	if config == "" && home != "" {
		config = filepath.Join(home, ".docker")
	}
	d.Contexts, d.Selected, d.Err = dockerContextFiles(config)
	active := inventory.TerminalCopy(st.Installations)
	if active >= 0 && !st.Installations[active].Source.Shim {
		exe := st.Installations[active].Path
		res, err := r.Run(ctx, run.Cmd{Args: []string{exe, "context", "ls", "--format", "{{json .}}"}, Env: term})
		if err == nil && res.ExitCode == 0 && !res.Truncated {
			if contexts, err := dockerContextList(res.Stdout); err == nil && len(contexts) > 0 {
				d.Contexts, d.Err = contexts, ""
			}
		}
	}
	dockerSelect(d, term)
	d.Plugins = dockerPlugins(config)
	if config != "" {
		d.PluginDir = filepath.Join(config, "cli-plugins")
	}
	if active >= 0 && !st.Installations[active].Source.Shim && d.Err == "" {
		for i := range d.Contexts {
			c := &d.Contexts[i]
			if c.Name != d.Selected || !c.Local {
				continue
			}
			exe := st.Installations[active].Path
			res, err := r.Run(ctx, run.Cmd{Args: []string{exe, "--host", c.Endpoint, "version", "--format", "{{json .}}"}, Env: append(slices.Clone(term), "DOCKER_CONTEXT="), Timeout: 3 * time.Second})
			var version struct{ Server *struct{ Version string } }
			c.Status = "unavailable"
			if err == nil && res.ExitCode == 0 && !res.Truncated && json.Unmarshal(res.Stdout, &version) == nil && version.Server != nil && version.Server.Version != "" {
				c.Status, c.Version = "available", version.Server.Version
			}
		}
	}
	return st, ctx.Err()
}

func dockerContextList(data []byte) ([]inventory.DockerContext, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	var out []inventory.DockerContext
	for {
		var row struct{ Name, DockerEndpoint string }
		err := dec.Decode(&row)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read Docker contexts: %w", err)
		}
		if row.Name != "" {
			out = append(out, dockerContext(row.Name, row.DockerEndpoint))
		}
	}
}

func dockerContextFiles(config string) ([]inventory.DockerContext, string, string) {
	out := []inventory.DockerContext{dockerContext("default", dockerDefaultEndpoint)}
	selected, problem := "default", ""
	if config == "" {
		return out, selected, problem
	}
	var cfg struct{ CurrentContext string }
	data, err := os.ReadFile(filepath.Join(config, "config.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) || err == nil && json.Unmarshal(data, &cfg) != nil {
		problem = "config.json"
	} else if cfg.CurrentContext != "" {
		selected = cfg.CurrentContext
	}
	files, _ := filepath.Glob(filepath.Join(config, "contexts", "meta", "*", "meta.json"))
	for _, p := range files {
		var meta struct {
			Name      string
			Endpoints map[string]struct{ Host string }
		}
		if json.Unmarshal([]byte(readFile(p)), &meta) != nil {
			problem = "meta.json"
			continue
		}
		if meta.Name != "" && meta.Name != "default" {
			out = append(out, dockerContext(meta.Name, meta.Endpoints["docker"].Host))
		}
	}
	return out, selected, problem
}

func dockerSelect(d *inventory.DockerState, env sysenv.Env) {
	// The CLI's order: DOCKER_HOST wins over DOCKER_CONTEXT, which wins
	// over config.json's currentContext.
	if host := env.Get("DOCKER_HOST"); host != "" {
		d.Selected, d.Override = "default", "DOCKER_HOST"
		found := false
		for i := range d.Contexts {
			if d.Contexts[i].Name == "default" {
				d.Contexts[i], found = dockerContext("default", host), true
			}
		}
		if !found {
			d.Contexts = append(d.Contexts, dockerContext("default", host))
		}
	} else if name := env.Get("DOCKER_CONTEXT"); name != "" {
		d.Selected, d.Override = name, "DOCKER_CONTEXT"
	}
	if !slices.ContainsFunc(d.Contexts, func(c inventory.DockerContext) bool { return c.Name == d.Selected }) {
		d.Contexts = append(d.Contexts, inventory.DockerContext{Name: d.Selected})
	}
}

func dockerContext(name, endpoint string) inventory.DockerContext {
	local := strings.HasPrefix(endpoint, "unix:///") || strings.HasPrefix(endpoint, "npipe:////./pipe/")
	c := inventory.DockerContext{Name: name, Endpoint: endpoint, Local: local}
	if !local && endpoint != "" {
		scheme, _, _ := strings.Cut(endpoint, ":")
		if scheme != "ssh" && scheme != "tcp" && scheme != "http" && scheme != "https" {
			scheme = "remote"
		}
		c.Endpoint = scheme + "://…"
	}
	lower := strings.ToLower(name + " " + c.Endpoint)
	switch {
	case strings.Contains(lower, "orbstack"):
		c.Engine = "OrbStack"
	case strings.Contains(lower, "colima"):
		c.Engine = "Colima"
	case strings.Contains(lower, "rancher") || strings.Contains(lower, "/.rd/"):
		c.Engine = "Rancher Desktop"
	case strings.Contains(lower, "podman"):
		c.Engine = "Podman"
	case strings.Contains(lower, "desktop-linux") || strings.Contains(lower, "/.docker/run/") || strings.Contains(lower, "dockerdesktop"):
		c.Engine = "Docker Desktop"
	}
	return c
}

func dockerPlugins(config string) []inventory.Package {
	if config == "" {
		return nil
	}
	var out []inventory.Package
	for _, name := range []string{"compose", "buildx"} {
		if exists(filepath.Join(config, "cli-plugins", "docker-"+name+dockerSuffix)) {
			out = append(out, inventory.Package{Name: name, Commands: []string{"docker " + name}})
		}
	}
	return out
}

func dockerCommand(env sysenv.Env, name string) string {
	if paths := sysenv.LookPathAll(name, env); len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func dockerApp(candidates ...string) string {
	for _, p := range candidates {
		if exists(p) {
			return p
		}
	}
	return ""
}
