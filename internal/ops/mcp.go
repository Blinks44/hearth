package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// mcpFormat is how a file declares MCP servers.
type mcpFormat int

const (
	mcpJSON     mcpFormat = iota // "mcpServers" in a JSON file
	mcpClaude                    // ~/.claude.json, which claude mcp changes
	mcpTOML                      // [mcp_servers.<name>], as Codex's config.toml
	mcpOpenCode                  // "mcp" in opencode.json
)

// formatOf is the format of an agent's file of MCP servers.
func formatOf(agent, path string) mcpFormat {
	switch {
	case filepath.Ext(path) == ".toml":
		return mcpTOML
	case agent == "opencode":
		return mcpOpenCode
	case agent == "claude" && filepath.Base(path) == ".claude.json":
		return mcpClaude
	}
	return mcpJSON
}

// readDef reads s's declaration from its file, for a copy: variables
// Codex passes by name have no counterpart the other agents write the
// same way.
func readDef(s inventory.MCPServer) (ai.MCPDef, error) {
	def, err := ai.ReadMCPDef(s)
	if err == nil && (len(def.EnvVars) > 0 || len(def.EnvHeaders) > 0 || def.BearerVar != "") {
		return def, errors.New(locales.McpCannotCopy(s.Name))
	}
	return def, err
}

// object writes a JSON object with its members in the order given,
// leaving out the empty ones.
func object(pairs ...any) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i := 0; i+1 < len(pairs); i += 2 {
		v := pairs[i+1]
		switch v := v.(type) {
		case string:
			if v == "" {
				continue
			}
		case []string:
			if len(v) == 0 {
				continue
			}
		case map[string]string:
			if len(v) == 0 {
				continue
			}
		}
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(pairs[i])
		val, _ := json.Marshal(v)
		b.Write(k)
		b.WriteByte(':')
		b.Write(val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// jsonDef is def as agent's JSON file declares a server: the keys differ
// a little between the agents.
func jsonDef(def ai.MCPDef, agent string, format mcpFormat) json.RawMessage {
	return jsonDefWithDisabled(def, agent, format, false)
}

func jsonDefWithDisabled(def ai.MCPDef, agent string, format mcpFormat, disabled bool) json.RawMessage {
	if format == mcpOpenCode {
		if def.Transport == "stdio" {
			if disabled {
				return object("type", "local", "command", append([]string{def.Command}, def.Args...), "environment", def.Env, "enabled", false)
			}
			return object("type", "local", "command", append([]string{def.Command}, def.Args...), "environment", def.Env)
		}
		if disabled {
			return object("type", "remote", "url", def.URL, "headers", def.Headers, "enabled", false)
		}
		return object("type", "remote", "url", def.URL, "headers", def.Headers)
	}
	var pairs []any
	switch {
	case def.Transport == "stdio" && agent == "copilot":
		pairs = []any{"type", "local", "command", def.Command, "args", def.Args, "env", def.Env, "tools", []string{"*"}}
	case def.Transport == "stdio":
		pairs = []any{"command", def.Command, "args", def.Args, "env", def.Env}
	case agent == "copilot":
		pairs = []any{"type", def.Transport, "url", def.URL, "headers", def.Headers, "tools", []string{"*"}}
	case agent == "gemini" && def.Transport == "http":
		pairs = []any{"httpUrl", def.URL, "headers", def.Headers}
	case agent == "devin" || agent == "antigravity":
		pairs = []any{"serverUrl", def.URL, "headers", def.Headers}
	default:
		pairs = []any{"type", def.Transport, "url", def.URL, "headers", def.Headers}
	}
	if disabled {
		pairs = append(pairs, "disabled", true)
	}
	return object(pairs...)
}

// bareKey is a TOML key written without quotes.
var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// serverTable is the table of the server name in a Codex config.
func serverTable(name string) string {
	if bareKey.MatchString(name) {
		return "mcp_servers." + name
	}
	return "mcp_servers." + strconv.Quote(name)
}

// tomlDef is def as a Codex config declares a server, its table and those
// in it, without the [mcp_servers] line.
func tomlDef(name string, def ai.MCPDef) (string, error) {
	return tomlDefWithDisabled(name, def, false)
}

func tomlDefWithDisabled(name string, def ai.MCPDef, disabled bool) (string, error) {
	server := map[string]any{}
	if def.Command != "" {
		server["command"] = def.Command
	}
	if len(def.Args) > 0 {
		server["args"] = def.Args
	}
	if len(def.Env) > 0 {
		server["env"] = def.Env
	}
	if len(def.EnvVars) > 0 {
		server["env_vars"] = def.EnvVars
	}
	if def.URL != "" {
		server["url"] = def.URL
	}
	if len(def.Headers) > 0 {
		server["http_headers"] = def.Headers
	}
	if len(def.EnvHeaders) > 0 {
		server["env_http_headers"] = def.EnvHeaders
	}
	if def.BearerVar != "" {
		server["bearer_token_env_var"] = def.BearerVar
	}
	if disabled {
		server["enabled"] = false
	}
	var b strings.Builder
	enc := toml.NewEncoder(&b)
	enc.Indent = ""
	if err := enc.Encode(map[string]any{"mcp_servers": map[string]any{name: server}}); err != nil {
		return "", err
	}
	return strings.TrimPrefix(b.String(), "[mcp_servers]\n"), nil
}

var managedJSONKeys = []string{
	"type", "command", "args", "env", "environment",
	"url", "httpUrl", "serverUrl", "headers", "disabled", "enabled",
}

func readJSONObjectAt(doc string, section, name string) (json.RawMessage, bool) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &root); err != nil {
		return nil, false
	}
	secRaw, ok := root[section]
	if !ok {
		return nil, false
	}
	var sec map[string]json.RawMessage
	if err := json.Unmarshal(secRaw, &sec); err != nil {
		return nil, false
	}
	entry, ok := sec[name]
	return entry, ok
}

func mergeJSONObject(existing, updated json.RawMessage, dropKeys []string) json.RawMessage {
	var base map[string]json.RawMessage
	if err := json.Unmarshal(existing, &base); err != nil || base == nil {
		return updated
	}
	var next map[string]json.RawMessage
	if err := json.Unmarshal(updated, &next); err != nil || next == nil {
		return updated
	}
	for _, k := range dropKeys {
		delete(base, k)
	}
	maps.Copy(base, next)
	out, err := json.Marshal(base)
	if err != nil {
		return updated
	}
	return out
}

func mergeTOMLServer(doc, name string, def ai.MCPDef, disabled bool) (string, error) {
	var parsed struct {
		MCPServers map[string]map[string]any `toml:"mcp_servers"`
	}
	server := map[string]any{}
	if _, err := toml.Decode(doc, &parsed); err == nil {
		if existing, ok := parsed.MCPServers[name]; ok {
			maps.Copy(server, existing)
		}
	}
	for _, k := range []string{
		"command", "args", "env", "env_vars",
		"url", "http_headers", "env_http_headers", "bearer_token_env_var", "enabled",
	} {
		delete(server, k)
	}
	if def.Command != "" {
		server["command"] = def.Command
	}
	if len(def.Args) > 0 {
		server["args"] = def.Args
	}
	if len(def.Env) > 0 {
		server["env"] = def.Env
	}
	if len(def.EnvVars) > 0 {
		server["env_vars"] = def.EnvVars
	}
	if def.URL != "" {
		server["url"] = def.URL
	}
	if len(def.Headers) > 0 {
		server["http_headers"] = def.Headers
	}
	if len(def.EnvHeaders) > 0 {
		server["env_http_headers"] = def.EnvHeaders
	}
	if def.BearerVar != "" {
		server["bearer_token_env_var"] = def.BearerVar
	}
	if disabled {
		server["enabled"] = false
	}
	var b strings.Builder
	enc := toml.NewEncoder(&b)
	enc.Indent = ""
	if err := enc.Encode(map[string]any{"mcp_servers": map[string]any{name: server}}); err != nil {
		return "", err
	}
	return strings.TrimPrefix(b.String(), "[mcp_servers]\n"), nil
}

// removeTOMLTable is text without the table and those in it.
func removeTOMLTable(text, table string) string {
	after, _ := replaceTOMLTable(text, table, "")
	return after
}

// replaceTOMLTable is text with body, a table's lines, in place of the
// table and those in it; body "" removes them. ok is false and text as it
// is when there is no such table.
func replaceTOMLTable(text, table, body string) (string, bool) {
	lines := splitLines(text)
	start := slices.IndexFunc(lines, func(l string) bool { return tomlTable(l) == table })
	if start < 0 {
		return text, false
	}
	end := start + 1
	for end < len(lines) {
		t := tomlTable(lines[end])
		if t != "" && !strings.HasPrefix(t, table+".") {
			break
		}
		end++
	}
	if body != "" {
		// The blank line before the next table stays.
		for end > start+1 && lines[end-1] == "" && end < len(lines) {
			end--
		}
		return joinLines(slices.Replace(lines, start, end, splitLines(body)...)), true
	}
	lines = slices.Delete(lines, start, end)
	// No two blank lines where the table was.
	if start > 0 && lines[start-1] == "" && (start == len(lines) || lines[start] == "") {
		lines = slices.Delete(lines, start-1, start)
	}
	return joinLines(lines), true
}

// secrets are the values in def that s, as the snapshot keeps it, hides:
// the variables', the headers', and the arguments and URL it masked.
func secrets(def ai.MCPDef, s inventory.MCPServer) []string {
	var raw []string
	for _, v := range def.Env {
		raw = append(raw, v)
	}
	for _, v := range def.Headers {
		raw = append(raw, v)
	}
	for i, a := range def.Args {
		if i < len(s.Args) && s.Args[i] != a {
			raw = append(raw, a)
		}
	}
	if def.URL != s.URL {
		raw = append(raw, def.URL)
	}
	var out []string
	add := func(v string) {
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	for _, v := range raw {
		if v == "" {
			continue
		}
		add(v)
		if b, err := json.Marshal(v); err == nil && len(b) >= 2 {
			add(string(b[1 : len(b)-1]))
		}
		if q := strconv.Quote(v); len(q) >= 2 {
			add(q[1 : len(q)-1])
		}
	}
	return out
}

// hideMCPFile includes Antigravity's OAuth client secrets, which can also
// appear in the unchanged context of a diff for a different server.
func hideMCPFile(agent, before string, hide []string) []string {
	if agent != "antigravity" {
		return hide
	}
	var cfg struct {
		Servers map[string]struct {
			OAuth struct {
				ClientSecret string `json:"clientSecret"`
			} `json:"oauth"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal([]byte(before), &cfg) == nil {
		for _, server := range cfg.Servers {
			if value := server.OAuth.ClientSecret; value != "" {
				raw, _ := json.Marshal(value)
				hide = append(hide, string(raw[1:len(raw)-1]), value)
			}
		}
	}
	return hide
}

// readText is the file at path, "" with new true when it is not there.
func readText(path string) (text string, isNew bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", true, nil
	}
	return string(data), false, err
}

// readServers reads again the servers of project, the user's when it is
// "", as a scan would.
func readServers(env sysenv.Env, project string) []inventory.MCPServer {
	if project != "" {
		servers, _ := ai.ProjectMCP(project, env)
		return servers
	}
	servers, _ := ai.MCP(env)
	return servers
}

// findServer is the server name of agent in servers, of project.
func findServer(servers []inventory.MCPServer, agent, name, project string) (inventory.MCPServer, bool) {
	i := slices.IndexFunc(servers, func(s inventory.MCPServer) bool {
		return s.Agent == agent && s.Name == name && s.Project == project
	})
	if i < 0 {
		return inventory.MCPServer{}, false
	}
	return servers[i], true
}

// MCPConnectPlan returns the plan that declares s, a server of another
// agent's, for the agent id too, in the same scope: in its file, or for
// Claude Code's own through claude mcp. The values of its variables and
// headers go along and show hidden. It reads the disk: call it in the
// background. names gives the agents' names.
func MCPConnectPlan(s inventory.MCPServer, id string, env sysenv.Env, names func(string) string) (Plan, error) {
	def, err := readDef(s)
	if err != nil {
		return Plan{}, err
	}
	return mcpAddPlan(s, def, id, env, names)
}

// HearthMCPPlan connects this installed binary using the existing MCP operation.
// The server is read-only; the config edit still requires desktop confirmation.
func HearthMCPPlan(executable, id string, env sysenv.Env, names func(string) string) (Plan, error) {
	if !filepath.IsAbs(executable) {
		return Plan{}, errors.New("hearth executable must have an absolute path")
	}
	info, err := os.Stat(executable)
	if err != nil {
		return Plan{}, fmt.Errorf("locate Hearth executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Plan{}, errors.New("hearth executable is not a regular file")
	}
	p, err := mcpAddPlan(inventory.MCPServer{Name: "hearth", Agent: "hearth", Transport: "stdio"}, ai.MCPDef{Transport: "stdio", Command: executable, Args: []string{"mcp"}}, id, env, names)
	if err == nil {
		p.Why = locales.HearthMCPDescription()
	}
	return p, err
}

func mcpAddPlan(s inventory.MCPServer, def ai.MCPDef, id string, env sysenv.Env, names func(string) string) (Plan, error) {
	if id == "antigravity" && def.Transport == "sse" {
		return Plan{}, errors.New(locales.AntigravityMcpSse())
	}
	title := names(id)
	path := ai.MCPFile(id, env, s.Project)
	if path == "" {
		return Plan{}, errors.New(locales.McpNoFile(title))
	}
	if _, ok := findServer(readServers(env, s.Project), id, s.Name, s.Project); ok {
		return Plan{}, errors.New(locales.McpAlready(s.Name, title))
	}
	hide := secrets(def, s)
	p := Plan{
		Title:   locales.McpConnectTitle(s.Name, title),
		Why:     locales.McpConnectWhy(names(s.Agent)),
		Changes: []string{"+ " + s.Name + " · " + title},
	}
	format := formatOf(id, path)
	switch format {
	case mcpClaude:
		raw := object("type", def.Transport, "command", def.Command, "args", def.Args, "env", def.Env, "url", def.URL, "headers", def.Headers)
		edit := FileEdit{Hide: hide}
		p.Steps = []Step{{
			Argv: []string{"claude", "mcp", "add-json", "--scope", "user", s.Name, string(raw)},
			Runs: []string{"claude", "mcp", "add-json", "--scope", "user", s.Name, edit.Masked(string(raw))},
		}}
	case mcpTOML:
		text, isNew, err := readText(path)
		if err != nil {
			return Plan{}, err
		}
		table, err := tomlDef(s.Name, def)
		if err != nil {
			return Plan{}, err
		}
		after := strings.TrimRight(text, "\n")
		if after != "" {
			after += "\n\n"
		}
		p.Steps = []Step{{Edit: &FileEdit{Path: path, Before: text, After: after + table, New: isNew, Hide: hide}}}
	default:
		before, isNew, err := readText(path)
		if err != nil {
			return Plan{}, err
		}
		text := before
		if strings.TrimSpace(text) == "" {
			text = "{}\n"
		}
		key := "mcpServers"
		if format == mcpOpenCode {
			key = "mcp"
		}
		after, err := JSONSet(text, []string{key, s.Name}, jsonDef(def, id, format))
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", path, err)
		}
		p.Steps = []Step{{Edit: &FileEdit{Path: path, Before: before, After: after, New: isNew, Hide: hideMCPFile(id, before, hide)}}}
	}
	p.Verify = func(context.Context) (string, error) {
		if _, ok := findServer(readServers(env, s.Project), id, s.Name, s.Project); !ok {
			return "", errors.New(locales.McpCheckMissing(s.Name, title))
		}
		return locales.McpCheckDeclared(s.Name, title), nil
	}
	return p, nil
}

// MCPRemovable reports that Hearth takes s away from its agent: all but
// a project's server Claude Code keeps in ~/.claude.json, which claude
// mcp removes only from the project's folder.
func MCPRemovable(s inventory.MCPServer) bool {
	return formatOf(s.Agent, s.File) != mcpClaude || s.Project == ""
}

// MCPRemovePlan returns the plan that takes s away from its agent: from
// its file, or for Claude Code's own through claude mcp. It reads the
// disk: call it in the background. names gives the agents' names.
func MCPRemovePlan(s inventory.MCPServer, env sysenv.Env, names func(string) string) (Plan, error) {
	if !MCPRemovable(s) {
		return Plan{}, errors.New(locales.McpClaudeLocal(s.Name))
	}
	title := names(s.Agent)
	p := Plan{
		Title:   locales.McpRemoveTitle(s.Name, title),
		Why:     locales.McpRemoveWhy(),
		Changes: []string{"− " + s.Name + " · " + title},
	}
	format := formatOf(s.Agent, s.File)
	switch format {
	case mcpClaude:
		p.Steps = []Step{{Argv: []string{"claude", "mcp", "remove", "--scope", "user", s.Name}}}
	default:
		text, _, err := readText(s.File)
		if err != nil {
			return Plan{}, err
		}
		var after string
		switch format {
		case mcpTOML:
			after = removeTOMLTable(text, serverTable(s.Name))
		case mcpOpenCode:
			after, err = JSONDelete(text, []string{"mcp", s.Name})
		default:
			after, err = JSONDelete(text, []string{"mcpServers", s.Name})
		}
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", s.File, err)
		}
		def, _ := readDef(s)
		p.Steps = []Step{{Edit: &FileEdit{Path: s.File, Before: text, After: after, Hide: hideMCPFile(s.Agent, text, secrets(def, s))}}}
	}
	p.Verify = func(context.Context) (string, error) {
		if _, ok := findServer(readServers(env, s.Project), s.Agent, s.Name, s.Project); ok {
			return "", errors.New(locales.McpCheckStill(s.Name, title))
		}
		return locales.McpCheckRemoved(s.Name, title), nil
	}
	return p, nil
}

// MCPToggles reports that s's agent turns a server off by a setting of
// its own, which Hearth then sets.
func MCPToggles(s inventory.MCPServer) bool {
	f := formatOf(s.Agent, s.File)
	return f == mcpTOML || f == mcpOpenCode || s.Agent == "antigravity"
}

// MCPEnablePlan returns the plan that turns s on, or off, in its file.
// It reads the disk: call it in the background. names gives the agents'
// names.
func MCPEnablePlan(s inventory.MCPServer, on bool, env sysenv.Env, names func(string) string) (Plan, error) {
	if !MCPToggles(s) {
		return Plan{}, errors.New(locales.McpNoToggle(names(s.Agent)))
	}
	text, _, err := readText(s.File)
	if err != nil {
		return Plan{}, err
	}
	var after string
	switch {
	case formatOf(s.Agent, s.File) == mcpTOML:
		after, err = setTOMLRaw(text, serverTable(s.Name), "enabled", strconv.FormatBool(on))
	case s.Agent == "antigravity":
		after, err = JSONSet(text, []string{"mcpServers", s.Name, "disabled"}, json.RawMessage(strconv.FormatBool(!on)))
	default:
		after, err = JSONSet(text, []string{"mcp", s.Name, "enabled"}, json.RawMessage(strconv.FormatBool(on)))
	}
	if err != nil {
		return Plan{}, fmt.Errorf("%s: %w", s.File, err)
	}
	title := names(s.Agent)
	def, _ := readDef(s)
	p := Plan{
		Title: locales.McpOffTitle(s.Name, title),
		Why:   locales.McpToggleWhy(),
		Steps: []Step{{Edit: &FileEdit{Path: s.File, Before: text, After: after, Hide: hideMCPFile(s.Agent, text, secrets(def, s))}}},
	}
	if on {
		p.Title = locales.McpOnTitle(s.Name, title)
	}
	p.Changes = []string{p.Title}
	p.Verify = func(context.Context) (string, error) {
		found, ok := findServer(readServers(env, s.Project), s.Agent, s.Name, s.Project)
		if !ok || found.Disabled == on {
			return "", errors.New(locales.McpCheckToggle(s.Name, title))
		}
		if on {
			return locales.McpCheckOn(s.Name, title), nil
		}
		return locales.McpCheckOff(s.Name, title), nil
	}
	return p, nil
}

func validateMCPDef(name string, def ai.MCPDef) (string, ai.MCPDef, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsAny(name, "\r\n\t/\\") {
		return "", def, errors.New(locales.McpFormBadName())
	}
	def.Transport = strings.ToLower(strings.TrimSpace(def.Transport))
	if def.Transport == "" {
		def.Transport = "stdio"
	}
	switch def.Transport {
	case "stdio":
		def.Command = strings.TrimSpace(def.Command)
		if def.Command == "" {
			return "", def, errors.New(locales.McpFormBadCommand())
		}
		def.URL = ""
		def.Headers = nil
	case "http", "sse":
		def.URL = strings.TrimSpace(def.URL)
		u, err := url.Parse(def.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", def, errors.New(locales.McpFormBadURL())
		}
		def.Command = ""
		def.Args = nil
		def.Env = nil
	default:
		return "", def, errors.New(locales.McpFormBadTransport())
	}
	return name, def, nil
}

func defSecrets(def ai.MCPDef) []string {
	s := ai.MaskMCP(inventory.MCPServer{
		Transport: def.Transport,
		Command:   def.Command,
		Args:      def.Args,
		URL:       def.URL,
	})
	return secrets(def, s)
}

// MCPAddPlan returns a plan that declares a new MCP server for agent in user
// or project scope. Secret values are masked in diffs and command previews.
func MCPAddPlan(name, agent, project string, def ai.MCPDef, env sysenv.Env, names func(string) string) (Plan, error) {
	name, def, err := validateMCPDef(name, def)
	if err != nil {
		return Plan{}, err
	}
	s := ai.MaskMCP(inventory.MCPServer{
		Name:      name,
		Agent:     agent,
		Project:   project,
		Transport: def.Transport,
		Command:   def.Command,
		Args:      def.Args,
		URL:       def.URL,
	})
	p, err := mcpAddPlan(s, def, agent, env, names)
	if err != nil {
		return Plan{}, err
	}
	title := names(agent)
	p.Title = locales.McpAddTitle(name, title)
	p.Why = locales.McpAddWhy(title)
	return p, nil
}

// MCPEditable reports that Hearth can edit s in place: all declarations except
// Claude Code's per-project entries inside ~/.claude.json.
func MCPEditable(s inventory.MCPServer) bool {
	return MCPRemovable(s)
}

// MCPEditPlan returns a plan that updates s's declaration to def in its config
// file (or via claude mcp for ~/.claude.json), preserving its disabled state
// and masking all secret values.
func MCPEditPlan(s inventory.MCPServer, def ai.MCPDef, env sysenv.Env, names func(string) string) (Plan, error) {
	if !MCPEditable(s) {
		return Plan{}, errors.New(locales.McpClaudeLocal(s.Name))
	}
	_, def, err := validateMCPDef(s.Name, def)
	if err != nil {
		return Plan{}, err
	}
	if s.Agent == "antigravity" && def.Transport == "sse" {
		return Plan{}, errors.New(locales.AntigravityMcpSse())
	}
	oldDef, _ := ai.ReadMCPDef(s)
	hide := append(defSecrets(def), secrets(oldDef, s)...)
	title := names(s.Agent)
	p := Plan{
		Title:   locales.McpEditTitle(s.Name, title),
		Why:     locales.McpEditWhy(title),
		Changes: []string{"~ " + s.Name + " · " + title},
	}
	format := formatOf(s.Agent, s.File)
	switch format {
	case mcpClaude:
		original, err := claudeServer(s)
		if err != nil {
			return Plan{}, err
		}
		before, err := compactJSON(original)
		if err != nil {
			return Plan{}, err
		}
		raw := mergeJSONObject(json.RawMessage(original), object("type", def.Transport, "command", def.Command, "args", def.Args, "env", def.Env, "url", def.URL, "headers", def.Headers), managedJSONKeys)
		after := string(raw)
		masked := FileEdit{Hide: hide}.Masked
		remove := []string{"claude", "mcp", "remove", "--scope", "user", s.Name}
		restore := []string{"claude", "mcp", "add-json", "--scope", "user", s.Name, before}
		add := []string{"claude", "mcp", "add-json", "--scope", "user", s.Name, after}
		p.Steps = []Step{
			{Argv: remove, Undo: &Step{Argv: restore, Runs: append(slices.Clone(restore[:6]), masked(before)), Note: s.File}},
			{Argv: add, Runs: append(slices.Clone(add[:6]), masked(after)), Undo: &Step{Argv: remove}},
		}
	case mcpTOML:
		text, _, err := readText(s.File)
		if err != nil {
			return Plan{}, err
		}
		if def.Transport == "stdio" && len(def.EnvVars) == 0 {
			def.EnvVars = oldDef.EnvVars
		}
		if def.Transport != "stdio" {
			if len(def.EnvHeaders) == 0 {
				def.EnvHeaders = oldDef.EnvHeaders
			}
			if def.BearerVar == "" {
				def.BearerVar = oldDef.BearerVar
			}
		}
		table, err := mergeTOMLServer(text, s.Name, def, s.Disabled)
		if err != nil {
			return Plan{}, err
		}
		after, ok := replaceTOMLTable(text, serverTable(s.Name), table)
		if !ok {
			return Plan{}, errors.New(locales.McpCheckMissing(s.Name, title))
		}
		p.Steps = []Step{{Edit: &FileEdit{Path: s.File, Before: text, After: after, Hide: hide}}}
	default:
		before, _, err := readText(s.File)
		if err != nil {
			return Plan{}, err
		}
		key := "mcpServers"
		if format == mcpOpenCode {
			key = "mcp"
		}
		nextRaw := jsonDefWithDisabled(def, s.Agent, format, s.Disabled)
		if existing, ok := readJSONObjectAt(before, key, s.Name); ok {
			nextRaw = mergeJSONObject(existing, nextRaw, managedJSONKeys)
		}
		after, err := JSONSet(before, []string{key, s.Name}, nextRaw)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", s.File, err)
		}
		p.Steps = []Step{{Edit: &FileEdit{Path: s.File, Before: before, After: after, Hide: hideMCPFile(s.Agent, before, hide)}}}
	}
	wantURL := ai.MaskMCP(inventory.MCPServer{URL: def.URL}).URL
	wantTransport := def.Transport
	if wantTransport == "sse" && (format == mcpTOML || format == mcpOpenCode || s.Agent == "devin") {
		wantTransport = "http"
	}
	p.Verify = func(context.Context) (string, error) {
		found, ok := findServer(readServers(env, s.Project), s.Agent, s.Name, s.Project)
		if !ok || found.Transport != wantTransport || (def.Transport == "stdio" && found.Command != def.Command) || (def.Transport != "stdio" && found.URL != wantURL) {
			return "", errors.New(locales.McpCheckMissing(s.Name, title))
		}
		return locales.McpCheckUpdated(s.Name, title), nil
	}
	return p, nil
}
