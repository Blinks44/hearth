package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// secretVar is a variable a secret moves into.
type secretVar struct{ Name, Value string }

// refFormats are how the agents' JSON files take a value from the
// environment: in env, headers and the rest alike.
var refFormats = map[string]string{
	"claude":   "${%s}",
	"gemini":   "${%s}",
	"cursor":   "${env:%s}",
	"opencode": "{env:%s}",
}

// SecretMovable reports that Hearth moves the secrets s's file writes out
// in its env and headers into variables: for an agent whose file takes
// values from the environment, Codex's by name. Claude Code's server of a
// project kept in ~/.claude.json is left, as MCPRemovable says.
func SecretMovable(s inventory.MCPServer) bool {
	if len(s.PlainEnv) == 0 && len(s.PlainHeaders) == 0 || !MCPRemovable(s) {
		return false
	}
	_, ok := refFormats[s.Agent]
	return ok || s.Agent == "codex"
}

// varName is the variable a header of server's moves into, as
// LINEAR_TOKEN for Authorization or LINEAR_X_API_KEY.
func varName(server, header string) string {
	name := server + "_" + header
	if strings.EqualFold(header, "Authorization") {
		name = server + "_TOKEN"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '_'
	}, name)
}

// MCPSecretPlan returns the plan that moves the secrets s's file writes out
// in its env and headers into the user's environment variables, the file
// then taking them from there; those in its arguments and URL stay. It
// reads the disk: call it in the background. env is the terminal's, which
// terminal checks the result in; names gives the agents' names.
func MCPSecretPlan(s inventory.MCPServer, env sysenv.Env, terminal Terminal, names func(string) string) (Plan, error) {
	if !SecretMovable(s) {
		return Plan{}, errors.New(locales.SecretNoRef(names(s.Agent)))
	}
	def, err := ai.ReadMCPDef(s)
	if err != nil {
		return Plan{}, err
	}
	// A variable a secret of the env moves into is named as it is there.
	var vars []secretVar
	header := map[string]string{} // header → its variable
	for _, n := range s.PlainEnv {
		vars = append(vars, secretVar{n, def.Env[n]})
	}
	for _, h := range s.PlainHeaders {
		// The scheme stays in the file; Codex's bearer_token_env_var is the
		// token alone, and its other headers take whole values.
		v := def.Headers[h]
		if scheme, cred := ai.AuthScheme(v); s.Agent != "codex" || bearer(h, scheme) {
			v = cred
		}
		header[h] = varName(s.Name, h)
		vars = append(vars, secretVar{header[h], v})
	}
	exports, where, err := exportSteps(vars, env)
	if err != nil {
		return Plan{}, err
	}
	hide := make([]string, 0, len(vars))
	for _, v := range vars {
		hide = append(hide, v.Value)
	}
	for _, v := range def.Headers {
		hide = append(hide, v)
	}

	var edit []Step
	format := formatOf(s.Agent, s.File)
	switch format {
	case mcpTOML:
		text, _, err := readText(s.File)
		if err != nil {
			return Plan{}, err
		}
		after, err := codexRefs(text, s, header)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", s.File, err)
		}
		edit = []Step{{Edit: &FileEdit{Path: s.File, Before: text, After: after, Hide: hide, DropCopy: true}}}
	case mcpClaude:
		// claude mcp changes ~/.claude.json: the server goes and comes back
		// with the variables in place of the values, as it was otherwise.
		original, err := claudeServer(s)
		if err != nil {
			return Plan{}, err
		}
		raw, err := jsonRefs(original, nil, s, header)
		if err != nil {
			return Plan{}, err
		}
		compact, err := compactJSON(raw)
		if err != nil {
			return Plan{}, err
		}
		// If the server does not come back, as add-json fails or the check
		// does not pass, the one that was comes back as it was.
		before, err := compactJSON(original)
		if err != nil {
			return Plan{}, err
		}
		masked := FileEdit{Hide: hide}.Masked
		remove := []string{"claude", "mcp", "remove", "--scope", "user", s.Name}
		restore := []string{"claude", "mcp", "add-json", "--scope", "user", s.Name, before}
		edit = []Step{
			{Argv: remove, Undo: &Step{Argv: restore, Runs: append(slices.Clone(restore[:6]), masked(before)), Note: s.File}},
			{Argv: []string{"claude", "mcp", "add-json", "--scope", "user", s.Name, compact}, Undo: &Step{Argv: remove}},
		}
	default:
		text, _, err := readText(s.File)
		if err != nil {
			return Plan{}, err
		}
		root := "mcpServers"
		if format == mcpOpenCode {
			root = "mcp"
		}
		after, err := jsonRefs(text, []string{root, s.Name}, s, header)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", s.File, err)
		}
		edit = []Step{{Edit: &FileEdit{Path: s.File, Before: text, After: after, Hide: hide, DropCopy: true}}}
	}

	title := names(s.Agent)
	p := Plan{
		Title: locales.SecretMcpTitle(s.Name),
		Why:   locales.SecretMcpWhy(tildeIn(s.File, env), where, title),
		Steps: append(exports, edit...),
	}
	for _, v := range vars {
		p.Changes = append(p.Changes, locales.SecretChange(v.Name, where))
	}
	p.Verify = func(ctx context.Context) (string, error) {
		if err := checkVars(ctx, terminal, vars); err != nil {
			return "", err
		}
		found, ok := findServer(readServers(env, s.Project), s.Agent, s.Name, s.Project)
		if !ok {
			return "", errors.New(locales.McpCheckMissing(s.Name, title))
		}
		if len(found.PlainEnv) > 0 || len(found.PlainHeaders) > 0 {
			return "", errors.New(locales.SecretStill(tildeIn(s.File, env)))
		}
		return locales.SecretMoved(varNames(vars)), nil
	}
	return p, nil
}

// SettingsSecretPlan returns the plan that moves the secrets the "env" of
// a's settings file writes out into the user's environment variables of
// the same names, which the agent started from the terminal gets. It
// reads the disk: call it in the background. env is the terminal's, which
// terminal checks the result in; names gives the agents' names.
func SettingsSecretPlan(a inventory.Agent, env sysenv.Env, terminal Terminal, names func(string) string) (Plan, error) {
	text, _, err := readText(a.Settings)
	if err != nil {
		return Plan{}, err
	}
	var cfg struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal([]byte(text), &cfg); err != nil {
		return Plan{}, fmt.Errorf("%s: %w", a.Settings, err)
	}
	var vars []secretVar
	after := text
	for _, n := range a.PlainEnv {
		vars = append(vars, secretVar{n, cfg.Env[n]})
		if after, err = JSONDelete(after, []string{"env", n}); err != nil {
			return Plan{}, fmt.Errorf("%s: %w", a.Settings, err)
		}
	}
	// An "env" left empty goes too.
	if len(vars) == len(cfg.Env) {
		if after, err = JSONDelete(after, []string{"env"}); err != nil {
			return Plan{}, fmt.Errorf("%s: %w", a.Settings, err)
		}
	}
	exports, where, err := exportSteps(vars, env)
	if err != nil {
		return Plan{}, err
	}
	hide := make([]string, 0, len(vars))
	for _, v := range vars {
		hide = append(hide, v.Value)
	}
	title := names(a.ID)
	p := Plan{
		Title: locales.SecretSettingsTitle(title),
		Why:   locales.SecretSettingsWhy(tildeIn(a.Settings, env), where, title),
		Steps: append(exports, Step{Edit: &FileEdit{Path: a.Settings, Before: text, After: after, Hide: hide, DropCopy: true}}),
	}
	for _, v := range vars {
		p.Changes = append(p.Changes, locales.SecretChange(v.Name, where))
	}
	p.Verify = func(ctx context.Context) (string, error) {
		if err := checkVars(ctx, terminal, vars); err != nil {
			return "", err
		}
		if left, err := ai.SettingsSecrets(a.Settings); err != nil || len(left) > 0 {
			return "", errors.New(locales.SecretStill(tildeIn(a.Settings, env)))
		}
		return locales.SecretMoved(varNames(vars)), nil
	}
	return p, nil
}

// exportSteps are the steps that set vars for the user, with where they
// go, as ~/.zshrc; none for a variable the terminal has at that value
// already. One it has at another value is an error: the agent would get
// that one.
func exportSteps(vars []secretVar, env sysenv.Env) ([]Step, string, error) {
	var set []secretVar
	for _, v := range vars {
		switch have := env.Get(v.Name); {
		case have == v.Value:
		case have != "":
			return nil, "", errors.New(locales.SecretTaken(v.Name))
		case slices.ContainsFunc(set, func(o secretVar) bool { return o.Name == v.Name }):
			return nil, "", errors.New(locales.SecretTaken(v.Name))
		default:
			set = append(set, v)
		}
	}
	return userVars(set, env)
}

// checkVars checks that a new terminal has vars at their values.
func checkVars(ctx context.Context, terminal Terminal, vars []secretVar) error {
	term, err := terminal(ctx)
	if err != nil {
		return err
	}
	for _, v := range vars {
		if term.Get(v.Name) != v.Value {
			return errors.New(locales.SecretNotInTerminal(v.Name))
		}
	}
	return nil
}

// varNames are the names of vars, joined.
func varNames(vars []secretVar) string {
	var out []string
	for _, v := range vars {
		out = append(out, v.Name)
	}
	return strings.Join(out, ", ")
}

// tildeIn is path with the home of env as ~.
func tildeIn(path string, env sysenv.Env) string {
	home := env.Get("HOME")
	if home == "" {
		home = env.Get("USERPROFILE")
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok && home != "" {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// jsonRefs is text, a JSON file, with the secrets of s at path, the
// server's object, taken from the variables: its env's of the same names,
// its headers' from header's, the scheme, as "Bearer ", kept.
func jsonRefs(text string, path []string, s inventory.MCPServer, header map[string]string) (string, error) {
	ref := func(name string) string { return fmt.Sprintf(refFormats[s.Agent], name) }
	envKey := "env"
	if s.Agent == "opencode" {
		envKey = "environment"
	}
	var obj struct {
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(inner(text, path)), &obj); err != nil {
		return "", err
	}
	set := func(key []string, value string) error {
		raw, _ := json.Marshal(value)
		var err error
		text, err = JSONSet(text, append(slices.Clone(path), key...), raw)
		return err
	}
	for _, n := range s.PlainEnv {
		if err := set([]string{envKey, n}, ref(n)); err != nil {
			return "", err
		}
	}
	for _, h := range s.PlainHeaders {
		scheme, _ := ai.AuthScheme(obj.Headers[h])
		if err := set([]string{"headers", h}, scheme+ref(header[h])); err != nil {
			return "", err
		}
	}
	return text, nil
}

// bearer reports a bearer token in the header h, its value's scheme
// given.
func bearer(h, scheme string) bool {
	return strings.EqualFold(h, "Authorization") && strings.EqualFold(scheme, "Bearer ")
}

// inner is the JSON value at path in text; text for none.
func inner(text string, path []string) string {
	raw := json.RawMessage(text)
	for _, key := range path {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) != nil {
			return "{}"
		}
		raw = m[key]
	}
	return string(raw)
}

// compactJSON is text, JSON, on one line, as claude mcp add-json takes it.
func compactJSON(text string) (string, error) {
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(text)); err != nil {
		return "", err
	}
	return b.String(), nil
}

// claudeServer is the JSON of the user's server s in ~/.claude.json.
func claudeServer(s inventory.MCPServer) (string, error) {
	text, _, err := readText(s.File)
	if err != nil {
		return "", err
	}
	raw := inner(text, []string{"mcpServers", s.Name})
	if raw == "" || raw == "{}" {
		return "", fmt.Errorf("read %s: no server %s", s.File, s.Name)
	}
	return raw, nil
}

// codexRefs is text, a Codex config, with the secrets of s taken from
// variables: its env's passed by name in env_vars, its Authorization's
// bearer token from bearer_token_env_var, other headers from
// env_http_headers, by header's variables. The server's table is written
// anew, its comments lost; the rest of the text stays as it is.
func codexRefs(text string, s inventory.MCPServer, header map[string]string) (string, error) {
	var cfg struct {
		MCPServers map[string]map[string]any `toml:"mcp_servers"`
	}
	if _, err := toml.Decode(text, &cfg); err != nil {
		return "", err
	}
	server, ok := cfg.MCPServers[s.Name]
	if !ok {
		return "", fmt.Errorf("no server %s", s.Name)
	}
	if e, ok := server["env"].(map[string]any); ok {
		vars, _ := server["env_vars"].([]any)
		for _, n := range s.PlainEnv {
			delete(e, n)
			if !slices.Contains(vars, any(n)) {
				vars = append(vars, n)
			}
		}
		server["env_vars"] = vars
		if len(e) == 0 {
			delete(server, "env")
		}
	}
	if hs, ok := server["http_headers"].(map[string]any); ok {
		envHeaders, _ := server["env_http_headers"].(map[string]any)
		for _, h := range s.PlainHeaders {
			v, _ := hs[h].(string)
			delete(hs, h)
			if scheme, _ := ai.AuthScheme(v); bearer(h, scheme) {
				server["bearer_token_env_var"] = header[h]
				continue
			}
			if envHeaders == nil {
				envHeaders = map[string]any{}
			}
			envHeaders[h] = header[h]
		}
		if envHeaders != nil {
			server["env_http_headers"] = envHeaders
		}
		if len(hs) == 0 {
			delete(server, "http_headers")
		}
	}
	var b strings.Builder
	enc := toml.NewEncoder(&b)
	enc.Indent = ""
	if err := enc.Encode(map[string]any{"mcp_servers": map[string]any{s.Name: server}}); err != nil {
		return "", err
	}
	// A blank line before each table in it, as people write them.
	lines := splitLines(strings.TrimPrefix(b.String(), "[mcp_servers]\n"))
	for i := len(lines) - 1; i > 0; i-- {
		if tomlTable(lines[i]) != "" && lines[i-1] != "" {
			lines = slices.Insert(lines, i, "")
		}
	}
	after, ok := replaceTOMLTable(text, serverTable(s.Name), strings.TrimRight(joinLines(lines), "\n"))
	if !ok {
		return "", fmt.Errorf("no table [%s]", serverTable(s.Name))
	}
	return after, nil
}
