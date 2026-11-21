package detect

import (
	"bytes"
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

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Docker finds Docker CLI copies and installed engines, reads contexts,
// and probes the selected local endpoint and the default socket if they differ.
// It never starts an engine,
// changes a context, invokes credential helpers or contacts a remote host.
func Docker(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	engines, candidates := dockerLocations(term, sources.ManagerDirs(term).Home)
	return detectDocker(ctx, r, term, gui, engines, candidates)
}

// StandaloneDockerCLI finds a PATH copy that can outlive an engine app.
// Resolve symlinks such as the docker link OrbStack puts in /usr/local/bin.
func StandaloneDockerCLI(term sysenv.Env) string {
	home := sources.ManagerDirs(term).Home
	engines, _ := dockerLocations(term, home)
	for _, path := range sysenv.LookPathAll("docker", term) {
		real, err := filepath.EvalSymlinks(path)
		if err != nil || dockerCLIEngine(real, engines).Name != "" ||
			strings.Contains(strings.ToLower(filepath.ToSlash(real)), ".app/") {
			continue
		}
		if home != "" && slices.ContainsFunc([]string{".orbstack", ".rd", ".kiri"}, func(dir string) bool {
			root := filepath.Join(home, dir)
			return within(path, root) || within(real, root)
		}) {
			continue
		}
		return path
	}
	return ""
}

func dockerCLIEngine(real string, engines []inventory.DockerEngine) inventory.DockerEngine {
	for _, engine := range slices.Backward(engines) {
		if engine.Path != "" && within(real, engine.Path) {
			return engine
		}
	}
	return inventory.DockerEngine{}
}

func detectDocker(ctx context.Context, r run.Runner, term, gui sysenv.Env, engines []inventory.DockerEngine, candidates []string) (inventory.ToolState, error) {
	f := newFinder("docker", term)
	f.lookPath("docker", term, gui)
	for _, p := range candidates {
		if exists(p) {
			f.add(p)
		}
	}
	d := &inventory.DockerState{Engines: engines}
	for _, engine := range engines {
		if engine.Name != "Colima" || engine.Path == "" {
			continue
		}
		colima := &inventory.ColimaState{Path: engine.Path}
		dir := term.Get("LIMA_HOME")
		if dir == "" {
			dir = filepath.Join(docker.ColimaConfigDir(term), "_lima")
		}
		// Colima creates its storage directories on the first CLI call.
		// A scan of a fresh installation must leave them alone.
		if _, err := os.Stat(dir); filepath.IsAbs(dir) && !errors.Is(err, os.ErrNotExist) {
			profiles, err := docker.ReadColimaProfiles(ctx, r, term, engine.Path)
			colima.Profiles, colima.Unreadable = profiles, err != nil
			for i := range profiles {
				if profiles[i].Status == "running" && profiles[i].Runtime == "docker" {
					_ = docker.ReadColimaStatus(ctx, r, term, engine.Path, &profiles[i])
				}
			}
		}
		if colima.Brew = colimaBrew(term, engine.Path); colima.Brew != "" && docker.ColimaUsesServiceProfile(term) {
			colima.Autostart = docker.ColimaAutostart(ctx, r, term, colima.Brew)
		}
		d.Colima = colima
	}
	st := inventory.ToolState{Tool: "docker", Installations: f.ins, Docker: d}
	for i := range st.Installations {
		in := &st.Installations[i]
		if in.Source.Shim {
			continue
		}
		if engine := dockerCLIEngine(in.RealPath, engines); engine.Name != "" {
			in.Source = inventory.Source{Kind: inventory.KindInstaller, Manager: engine.Name, Root: engine.Path}
		}
		v := versionOf(ctx, r, term, in.Path)
		if fields := strings.Fields(v); len(fields) >= 3 && fields[0] == "Docker" && fields[1] == "version" {
			in.Version = strings.TrimSuffix(fields[2], ",")
		}
	}
	config := docker.ConfigDir(term)
	d.ConfigDir = config
	cfg, cfgErr := readDockerConfig(config)
	d.Contexts, d.Selected, d.Err = dockerContextFiles(config, cfg.CurrentContext, cfgErr)
	if cfgErr == nil {
		dockerConfigHealth(d, cfg, term)
	}
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
	d.Plugins = dockerPlugins(config, cfg.CLIPluginsExtraDirs)
	if config != "" {
		d.PluginDir = filepath.Join(config, "cli-plugins")
		d.BrokenPlugins = brokenDockerPlugins(d.PluginDir)
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
			c.Denied = bytes.Contains(res.Stderr, []byte("permission denied"))
			c.Joined = c.Denied && dockerJoined(term.Get("USER"))
			if err == nil && res.ExitCode == 0 && !res.Truncated && json.Unmarshal(res.Stdout, &version) == nil && version.Server != nil && version.Server.Version != "" {
				c.Status, c.Version = "available", version.Server.Version
				dockerDatabases(ctx, r, term, exe, c.Endpoint, d)
				readCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
				target := docker.Target{Docker: exe, Context: c.Name, Endpoint: c.Endpoint}
				if engine, err := docker.Read(readCtx, r, term, target); err == nil {
					d.Running, d.Containers = engine.Running, engine.Containers
					d.Health = docker.ReadHealth(readCtx, r, term, target, engine)
				}
				cancel()
			}
			d.DefaultSocketIssue = dockerSocketHealth(ctx, r, term, exe, dockerDefaultEndpoint, *c)
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

func dockerContextFiles(config, currentContext string, configErr error) ([]inventory.DockerContext, string, string) {
	out := []inventory.DockerContext{dockerContext("default", dockerDefaultEndpoint)}
	selected, problem := "default", ""
	if config == "" {
		return out, selected, problem
	}
	if configErr != nil {
		problem = "config.json"
	} else if currentContext != "" {
		selected = currentContext
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
	case strings.Contains(lower, "kiri"):
		c.Engine = "Kiri"
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

func dockerPlugins(config string, extras []string) []inventory.Package {
	if config == "" {
		return nil
	}
	dirs := append([]string{filepath.Join(config, "cli-plugins")}, extras...)
	var out []inventory.Package
	for _, name := range []string{"compose", "buildx"} {
		if slices.ContainsFunc(dirs, func(dir string) bool {
			return filepath.IsAbs(dir) && exists(filepath.Join(dir, "docker-"+name+dockerSuffix))
		}) {
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

func colimaBrew(env sysenv.Env, path string) string {
	brew := dockerCommand(env, "brew")
	if brew == "" {
		return ""
	}
	real, err := filepath.EvalSymlinks(path)
	if err == nil && within(real, filepath.Join(filepath.Dir(filepath.Dir(brew)), "Cellar", "colima")) {
		return brew
	}
	return ""
}
