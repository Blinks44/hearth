package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// mcpFormat is how a file declares MCP servers.
type mcpFormat int

const (
	// mcpServersJSON is JSON with the servers under "mcpServers".
	mcpServersJSON mcpFormat = iota
	// claudeState is ~/.claude.json: the user's servers under
	// "mcpServers", and those of single projects under "projects".
	claudeState
	// codexTOML is Codex's config.toml, the servers in [mcp_servers.<name>];
	// Grok Build's is the same.
	codexTOML
	// openCodeJSON is opencode.json, the servers under "mcp".
	openCodeJSON
)

type mcpFile struct {
	path   string
	format mcpFormat
}

// mcpFiles are the files each agent reads MCP servers from, by
// detection.md §7: the user's, as the agents table writes files, and a
// project's, relative to it.
var mcpFiles = map[string]struct{ user, project []mcpFile }{
	"claude":  {[]mcpFile{{"~/.claude.json", claudeState}}, []mcpFile{{".mcp.json", mcpServersJSON}}},
	"codex":   {[]mcpFile{{"config.toml", codexTOML}}, []mcpFile{{".codex/config.toml", codexTOML}}},
	"copilot": {[]mcpFile{{"mcp-config.json", mcpServersJSON}}, []mcpFile{{".mcp.json", mcpServersJSON}, {".github/mcp.json", mcpServersJSON}}},
	"gemini":  {[]mcpFile{{"settings.json", mcpServersJSON}}, []mcpFile{{".gemini/settings.json", mcpServersJSON}}},
	"antigravity": {[]mcpFile{{"~/.gemini/config/mcp_config.json", mcpServersJSON}}, []mcpFile{
		{".agents/mcp_config.json", mcpServersJSON}, {".agent/mcp_config.json", mcpServersJSON},
		{"_agents/mcp_config.json", mcpServersJSON}, {"_agent/mcp_config.json", mcpServersJSON},
	}},
	"cursor":   {[]mcpFile{{"mcp.json", mcpServersJSON}}, []mcpFile{{".cursor/mcp.json", mcpServersJSON}}},
	"opencode": {[]mcpFile{{"opencode.json", openCodeJSON}}, []mcpFile{{"opencode.json", openCodeJSON}}},
	"devin":    {[]mcpFile{{"mcp_config.json", mcpServersJSON}}, []mcpFile{{".devin/mcp_config.json", mcpServersJSON}}},
	"grok":     {[]mcpFile{{"config.toml", codexTOML}}, []mcpFile{{".grok/config.toml", codexTOML}}},
	"kimi":     {[]mcpFile{{"mcp.json", mcpServersJSON}}, []mcpFile{{".kimi-code/mcp.json", mcpServersJSON}}},
}

// MCPFile is the file the agent id reads MCP servers from first: the
// user's, or, when project is not "", that project's; "" for an agent
// with none, as Aider.
func MCPFile(id string, env sysenv.Env, project string) string {
	files := mcpFiles[id]
	if project != "" {
		if len(files.project) == 0 {
			return ""
		}
		return filepath.Join(project, filepath.FromSlash(files.project[0].path))
	}
	if len(files.user) == 0 {
		return ""
	}
	home := homeDir(env)
	if id == "claude" {
		return claudeStatePath(env)
	}
	for _, a := range agents {
		if a.id == id {
			return resolve(files.user[0].path, dir(a.dir, env, home), home)
		}
	}
	return ""
}

// MCP reads the MCP servers in the user's files of every agent, in the
// agents' order, and checks their commands on the PATH of env, the
// terminal's. errs say which files could not be read; the others count.
func MCP(env sysenv.Env) (servers []inventory.MCPServer, errs []string) {
	home := homeDir(env)
	for _, a := range agents {
		d := dir(a.dir, env, home)
		for _, f := range mcpFiles[a.id].user {
			path := resolve(f.path, d, home)
			if a.id == "claude" && f.format == claudeState {
				path = claudeStatePath(env)
			}
			ss, err := readMCP(path, f.format, a.id)
			if err != nil {
				errs = append(errs, err.Error())
			}
			servers = append(servers, ss...)
		}
	}
	for i := range servers {
		checkCommand(&servers[i], env)
	}
	return servers, errs
}

// ProjectMCP reads the MCP servers in the files of the project in dir.
// Only commands given by a path are checked: one by name is looked for on
// the terminal's PATH, which a project is read without.
func ProjectMCP(dir string) (servers []inventory.MCPServer, errs []error) {
	for _, a := range agents {
		for _, f := range mcpFiles[a.id].project {
			ss, err := readMCP(filepath.Join(dir, filepath.FromSlash(f.path)), f.format, a.id)
			// A file two agents read fails once.
			if err != nil && !slices.ContainsFunc(errs, func(e error) bool { return e.Error() == err.Error() }) {
				errs = append(errs, err)
			}
			for _, s := range ss {
				s.Project = dir
				if c := s.Command; s.Transport == "stdio" && !filepath.IsAbs(c) && strings.ContainsAny(c, `/\`) {
					s.Command = filepath.Join(dir, c)
					checkCommand(&s, nil)
					s.Command = c
				} else {
					checkCommand(&s, nil)
				}
				servers = append(servers, s)
			}
		}
	}
	return servers, errs
}

// readMCP reads the servers in the file at path; none when it is not
// there.
func readMCP(path string, format mcpFormat, agent string) ([]inventory.MCPServer, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var out []inventory.MCPServer
	add := func(servers map[string]json.RawMessage, project string) error {
		for _, name := range slices.Sorted(maps.Keys(servers)) {
			s, err := jsonServer(servers[name])
			if err != nil {
				return fmt.Errorf("server %s: %w", name, err)
			}
			s.Name, s.Agent, s.File, s.Project = name, agent, path, project
			out = append(out, s)
		}
		return nil
	}
	if err == nil {
		switch format {
		case mcpServersJSON, openCodeJSON:
			var cfg struct {
				MCPServers map[string]json.RawMessage `json:"mcpServers"`
				MCP        map[string]json.RawMessage `json:"mcp"`
			}
			if err = json.Unmarshal(data, &cfg); err == nil {
				if format == openCodeJSON {
					err = add(cfg.MCP, "")
				} else {
					err = add(cfg.MCPServers, "")
				}
			}
		case claudeState:
			var cfg struct {
				MCPServers map[string]json.RawMessage `json:"mcpServers"`
				Projects   map[string]struct {
					MCPServers map[string]json.RawMessage `json:"mcpServers"`
				} `json:"projects"`
			}
			if err = json.Unmarshal(data, &cfg); err == nil {
				err = add(cfg.MCPServers, "")
				for _, p := range slices.Sorted(maps.Keys(cfg.Projects)) {
					if err == nil {
						err = add(cfg.Projects[p].MCPServers, p)
					}
				}
			}
		case codexTOML:
			out, err = readCodex(data, agent, path)
		}
	}
	if err != nil {
		return out, fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

// jsonServer is a server as JSON files write one; it takes the fields of
// all the agents, which differ little.
func jsonServer(data json.RawMessage) (inventory.MCPServer, error) {
	var j struct {
		Type string `json:"type"`
		// Command is a string; OpenCode's an array, the arguments in it.
		Command json.RawMessage `json:"command"`
		Args    []string        `json:"args"`
		// The values of Env, OpenCode's Environment and Headers are
		// never kept.
		Env         map[string]json.RawMessage `json:"env"`
		Environment map[string]json.RawMessage `json:"environment"`
		Headers     map[string]json.RawMessage `json:"headers"`
		URL         string                     `json:"url"`
		HTTPURL     string                     `json:"httpUrl"`   // Gemini's streamable HTTP
		ServerURL   string                     `json:"serverUrl"` // Antigravity CLI and Devin
		Disabled    bool                       `json:"disabled"`
		Enabled     *bool                      `json:"enabled"` // OpenCode's
	}
	if err := json.Unmarshal(data, &j); err != nil {
		return inventory.MCPServer{}, err
	}
	s := inventory.MCPServer{Args: j.Args, Disabled: j.Disabled || (j.Enabled != nil && !*j.Enabled)}
	if len(j.Command) > 0 {
		var cmd []string
		if json.Unmarshal(j.Command, &s.Command) != nil {
			if err := json.Unmarshal(j.Command, &cmd); err != nil {
				return s, fmt.Errorf("command: %w", err)
			}
			if len(cmd) > 0 {
				s.Command, s.Args = cmd[0], append(cmd[1:], j.Args...)
			}
		}
	}
	s.URL = first(j.HTTPURL, j.URL, j.ServerURL)
	s.Env = names(j.Env, j.Environment)
	s.Headers = names(j.Headers)
	s.PlainEnv = append(plainNames(j.Env), plainNames(j.Environment)...)
	s.PlainHeaders = plainNames(j.Headers)
	switch strings.ToLower(j.Type) {
	case "sse":
		s.Transport = "sse"
	case "http", "streamable-http", "streamablehttp", "remote":
		s.Transport = "http"
	case "stdio", "local":
		s.Transport = "stdio"
	default:
		s.Transport = "stdio"
		if s.Command == "" && s.URL != "" {
			s.Transport = "http"
		}
	}
	m := masked(s)
	m.PlainArgs = plainArgs(m, s.Args, s.URL)
	return m, nil
}

// readCodex reads the [mcp_servers] of Codex's config.toml.
func readCodex(data []byte, agent, path string) ([]inventory.MCPServer, error) {
	var cfg struct {
		MCPServers map[string]struct {
			Command string         `toml:"command"`
			Args    []string       `toml:"args"`
			Env     map[string]any `toml:"env"`
			// EnvVars are names passed through, or tables with a name.
			EnvVars           []any          `toml:"env_vars"`
			URL               string         `toml:"url"`
			HTTPHeaders       map[string]any `toml:"http_headers"`
			EnvHTTPHeaders    map[string]any `toml:"env_http_headers"`
			BearerTokenEnvVar string         `toml:"bearer_token_env_var"`
			Enabled           *bool          `toml:"enabled"`
		} `toml:"mcp_servers"`
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, err
	}
	var out []inventory.MCPServer
	for _, name := range slices.Sorted(maps.Keys(cfg.MCPServers)) {
		c := cfg.MCPServers[name]
		s := inventory.MCPServer{Name: name, Agent: agent, File: path, Transport: "stdio",
			Command: c.Command, Args: c.Args, URL: c.URL, Disabled: c.Enabled != nil && !*c.Enabled}
		if s.URL != "" {
			s.Transport = "http"
		}
		passed := map[string]any{}
		for _, v := range c.EnvVars {
			switch v := v.(type) {
			case string:
				passed[v] = nil
			case map[string]any:
				if n, ok := v["name"].(string); ok {
					passed[n] = nil
				}
			}
		}
		s.Env = names(c.Env, passed)
		headers := map[string]any{}
		if c.BearerTokenEnvVar != "" {
			headers["Authorization"] = nil
		}
		s.Headers = names(c.HTTPHeaders, c.EnvHTTPHeaders, headers)
		s.PlainEnv, s.PlainHeaders = plainNames(c.Env), plainNames(c.HTTPHeaders)
		m := masked(s)
		m.PlainArgs = plainArgs(m, s.Args, s.URL)
		out = append(out, m)
	}
	return out, nil
}

// names are the keys of the maps, sorted, each once.
func names[V any](ms ...map[string]V) []string {
	var out []string
	for _, m := range ms {
		for k := range m {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	slices.Sort(out)
	return out
}

// first is the first of vs that is not empty.
func first(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// secretName is a name of something secret, as GITHUB_TOKEN, --api-key
// or Authorization; secretValue is a token by its well-known prefix.
var (
	secretName  = regexp.MustCompile(`(?i)token|secret|passw|api[-_]?key|auth|credential|bearer|private[-_]?key`)
	secretValue = regexp.MustCompile(`^(ghp_|gho_|ghs_|github_pat_|sk-|xox[abpr]-|glpat-|AKIA)`)
)

// masked hides in s's arguments and URL what looks secret.
func masked(s inventory.MCPServer) inventory.MCPServer {
	if s.URL != "" {
		s.URL = maskURL(s.URL)
	}
	if len(s.Args) > 0 {
		args := make([]string, len(s.Args))
		for i, a := range s.Args {
			prev := ""
			if i > 0 {
				prev = s.Args[i-1]
			}
			args[i] = maskArg(a, prev)
		}
		s.Args = args
	}
	return s
}

// maskArg hides a after a secret's flag, as --token, a secret token, the
// value of NAME=value or "Name: value" for a secret's name, and a URL's
// password and query values.
func maskArg(a, prev string) string {
	switch {
	case secretValue.MatchString(a):
		return inventory.Hidden
	case strings.HasPrefix(prev, "-") && !strings.Contains(prev, "=") && secretName.MatchString(prev) && !strings.HasPrefix(a, "-"):
		return inventory.Hidden
	case strings.Contains(a, "://"):
		return maskURL(a)
	}
	if k, _, ok := strings.Cut(a, "="); ok && secretName.MatchString(k) {
		return k + "=" + inventory.Hidden
	}
	if k, _, ok := strings.Cut(a, ":"); ok && secretName.MatchString(k) {
		return k + ": " + inventory.Hidden
	}
	return a
}

// maskURL hides the user and password in raw and its query values; all of
// it when it does not parse.
func maskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return inventory.Hidden
	}
	user := u.User != nil
	var query []string
	if u.RawQuery != "" {
		for kv := range strings.SplitSeq(u.RawQuery, "&") {
			if k, _, ok := strings.Cut(kv, "="); ok {
				kv = k + "=" + inventory.Hidden
			}
			query = append(query, kv)
		}
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	s := u.String()
	if user {
		s = strings.Replace(s, "://", "://"+inventory.Hidden+"@", 1)
	}
	if len(query) > 0 {
		s += "?" + strings.Join(query, "&")
	}
	return s
}

// checkCommand looks for the command of a stdio server: at its path, or
// by name on the PATH of env; env nil checks only a path. A command
// written with a variable or ~ is not checked: the agent expands it.
func checkCommand(s *inventory.MCPServer, env sysenv.Env) {
	c := s.Command
	switch {
	case s.Transport != "stdio" || c == "" || strings.ContainsAny(c, "$%~"):
	case filepath.IsAbs(c):
		if _, err := os.Stat(c); err == nil {
			s.CommandPath = c
		} else {
			s.Missing = true
		}
	case strings.ContainsAny(c, `/\`) || env == nil:
	default:
		if found := sysenv.LookPathAll(c, env); len(found) > 0 {
			s.CommandPath = found[0]
		} else {
			s.Missing = true
		}
	}
}

// Claude Code relocates its state file inside an explicitly configured directory.
// Without CLAUDE_CONFIG_DIR the legacy state remains at ~/.claude.json.
func claudeStatePath(env sysenv.Env) string {
	if dir := env.Get("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	if home := homeDir(env); home != "" {
		return filepath.Join(home, ".claude.json")
	}
	return ""
}
