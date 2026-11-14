package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// MCPDef is a server as its file declares it, with the values of its
// variables and headers, which the snapshot does not keep.
type MCPDef struct {
	Transport, Command, URL string
	Args                    []string
	Env, Headers            map[string]string
	// EnvVars, EnvHeaders and BearerVar are Codex's: variables passed on
	// by name, headers set from variables, and the variable of a bearer
	// token.
	EnvVars    []string
	EnvHeaders map[string]string
	BearerVar  string
}

// ReadMCPDef reads s's declaration from its file.
func ReadMCPDef(s inventory.MCPServer) (MCPDef, error) {
	data, err := os.ReadFile(s.File)
	if err != nil {
		return MCPDef{}, fmt.Errorf("read %s: %w", s.File, err)
	}
	def := MCPDef{Transport: s.Transport}
	if filepath.Ext(s.File) == ".toml" {
		var cfg struct {
			MCPServers map[string]map[string]any `toml:"mcp_servers"`
		}
		if _, err := toml.Decode(string(data), &cfg); err != nil {
			return def, fmt.Errorf("read %s: %w", s.File, err)
		}
		c, ok := cfg.MCPServers[s.Name]
		if !ok {
			return def, fmt.Errorf("read %s: no server %s", s.File, s.Name)
		}
		def.Command, _ = c["command"].(string)
		def.URL, _ = c["url"].(string)
		def.Args = stringList(c["args"])
		def.Env, def.Headers = stringMap(c["env"]), stringMap(c["http_headers"])
		def.EnvVars, def.EnvHeaders = stringList(c["env_vars"]), stringMap(c["env_http_headers"])
		def.BearerVar, _ = c["bearer_token_env_var"].(string)
		return def, nil
	}
	var file map[string]json.RawMessage
	if err := json.Unmarshal(data, &file); err != nil {
		return def, fmt.Errorf("read %s: %w", s.File, err)
	}
	key := "mcpServers"
	if s.Agent == "opencode" {
		key = "mcp"
	}
	servers := file[key]
	if s.Agent == "claude" && filepath.Base(s.File) == ".claude.json" && s.Project != "" {
		var projects map[string]struct {
			MCPServers json.RawMessage `json:"mcpServers"`
		}
		_ = json.Unmarshal(file["projects"], &projects)
		servers = projects[s.Project].MCPServers
	}
	var byName map[string]struct {
		Command     json.RawMessage   `json:"command"`
		Args        []string          `json:"args"`
		Env         map[string]string `json:"env"`
		Environment map[string]string `json:"environment"`
		Headers     map[string]string `json:"headers"`
		URL         string            `json:"url"`
		HTTPURL     string            `json:"httpUrl"`
		ServerURL   string            `json:"serverUrl"`
	}
	if err := json.Unmarshal(servers, &byName); err != nil {
		return def, fmt.Errorf("read %s: %w", s.File, err)
	}
	j, ok := byName[s.Name]
	if !ok {
		return def, fmt.Errorf("read %s: no server %s", s.File, s.Name)
	}
	var cmd []string
	if json.Unmarshal(j.Command, &def.Command) != nil && json.Unmarshal(j.Command, &cmd) == nil && len(cmd) > 0 {
		def.Command, def.Args = cmd[0], cmd[1:]
	}
	def.Args = append(def.Args, j.Args...)
	def.URL = first(j.HTTPURL, j.URL, j.ServerURL)
	def.Env, def.Headers = j.Env, j.Headers
	if j.Environment != nil {
		def.Env = j.Environment
	}
	return def, nil
}

func stringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}

func stringList(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, a := range list {
		out = append(out, fmt.Sprint(a))
	}
	return out
}

// MCPCheckTimeout is how long a check waits for a server: a first start
// through npx or uvx downloads the package.
const MCPCheckTimeout = time.Minute

// MCPCheck is what a server answered when Hearth started or reached it
// and asked for its tools.
type MCPCheck struct {
	// Server and Version are the name and version it gave.
	Server, Version string
	Tools           []MCPTool
	// Took is how long it took to answer both.
	Took time.Duration
}

// MCPTool is one of a server's tools.
type MCPTool struct {
	Name string
	// Description is the first line of its description.
	Description string
}

// MCPFailure is why a check failed.
type MCPFailure int

const (
	// MCPFailed is any other reason: Detail says it.
	MCPFailed MCPFailure = iota
	// MCPNotFound: the command is not there.
	MCPNotFound
	// MCPExited: the process ended before it answered.
	MCPExited
	// MCPTimedOut: no answer within the time.
	MCPTimedOut
	// MCPUnreachable: nothing answers at the URL.
	MCPUnreachable
	// MCPDenied: the URL refused the credentials it got, or their lack
	// (HTTP 401 or 403).
	MCPDenied
)

// MCPError is a failed check: Detail is what the server or the system
// said, the last lines of its stderr when it wrote some, with the
// declared secrets hidden.
type MCPError struct {
	Kind   MCPFailure
	Detail string
}

func (e *MCPError) Error() string { return e.Detail }

// CheckMCP starts s as its agent would, or connects to its URL, makes the
// MCP handshake and lists its tools, then stops it. A stdio server runs
// in s's project, or the home folder, with term, the terminal's
// environment, and its declared variables; ${VAR} and ${VAR:-default} in
// the declaration take term's values, as the agents expand them. It
// starts a process: call it in the background, on a button only. A
// failure is an *MCPError.
func CheckMCP(ctx context.Context, s inventory.MCPServer, term sysenv.Env) (MCPCheck, error) {
	def, err := ReadMCPDef(s)
	if err != nil {
		return MCPCheck{}, &MCPError{Kind: MCPFailed, Detail: err.Error()}
	}
	expand := func(v string) string { return expandVars(v, term) }
	var secrets []string
	for _, v := range def.Env {
		secrets = append(secrets, expand(v))
	}
	for _, v := range def.Headers {
		secrets = append(secrets, expand(v))
	}

	var t mcpsdk.Transport
	var stderr tail
	var cmd *exec.Cmd
	var status statusRecorder
	switch def.Transport {
	case "stdio":
		program, err := findProgram(expand(def.Command), s, term)
		if err != nil {
			return MCPCheck{}, &MCPError{Kind: MCPNotFound, Detail: err.Error()}
		}
		// A later entry wins.
		env := slices.Clone(term)
		for k, v := range def.Env {
			env = append(env, k+"="+expand(v))
		}
		args := []string{program}
		for _, a := range def.Args {
			args = append(args, expand(a))
		}
		dir := s.Project
		if dir == "" {
			dir = homeDir(term)
		}
		cmd = run.Command(ctx, run.Cmd{Args: args, Env: env, Dir: dir})
		cmd.Stderr = &stderr
		t = &mcpsdk.CommandTransport{Command: cmd, TerminateDuration: 2 * time.Second}
	default:
		headers := map[string]string{}
		for k, v := range def.Headers {
			headers[k] = expand(v)
		}
		for k, name := range def.EnvHeaders {
			headers[k] = term.Get(name)
		}
		if def.BearerVar != "" {
			headers["Authorization"] = "Bearer " + term.Get(def.BearerVar)
		}
		maps.DeleteFunc(headers, func(_, v string) bool { return v == "" })
		status.headers, status.base = headers, http.DefaultTransport
		client := &http.Client{Transport: &status}
		if def.Transport == "sse" {
			t = &mcpsdk.SSEClientTransport{Endpoint: expand(def.URL), HTTPClient: client}
		} else {
			t = &mcpsdk.StreamableClientTransport{Endpoint: expand(def.URL), HTTPClient: client, MaxRetries: -1, DisableStandaloneSSE: true}
		}
	}

	start := time.Now()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "hearth", Version: "1"}, nil)
	session, err := client.Connect(ctx, t, nil)
	if err != nil {
		return MCPCheck{}, failure(ctx, err, cmd, &stderr, status.code(), secrets)
	}
	var out MCPCheck
	if info := session.InitializeResult().ServerInfo; info != nil {
		out.Server, out.Version = first(info.Title, info.Name), info.Version
	}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			_ = session.Close()
			return MCPCheck{}, failure(ctx, err, cmd, &stderr, status.code(), secrets)
		}
		desc, _, _ := strings.Cut(strings.TrimSpace(tool.Description), "\n")
		out.Tools = append(out.Tools, MCPTool{Name: tool.Name, Description: strings.TrimSpace(desc)})
	}
	out.Took = time.Since(start)
	_ = session.Close()
	return out, nil
}

// findProgram is the program of a stdio server: its command at its path,
// against s's project when relative, or by name on term's PATH.
func findProgram(command string, s inventory.MCPServer, term sysenv.Env) (string, error) {
	switch {
	case command == "":
		return "", errors.New("no command")
	case filepath.IsAbs(command):
	case strings.ContainsAny(command, `/\`):
		command = filepath.Join(first(s.Project, homeDir(term)), command)
	default:
		found := sysenv.LookPathAll(command, term)
		if len(found) == 0 {
			return "", fmt.Errorf("%s: %w", command, exec.ErrNotFound)
		}
		return found[0], nil
	}
	if _, err := os.Stat(command); err != nil {
		return "", err
	}
	return command, nil
}

// varRef is ${NAME} or ${NAME:-default} in a declared value, Cursor's
// ${env:NAME} or OpenCode's {env:NAME}.
var varRef = regexp.MustCompile(`\$\{(?:env:)?([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}|\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandVars puts env's values for the variables v refers to.
func expandVars(v string, env sysenv.Env) string {
	return varRef.ReplaceAllStringFunc(v, func(ref string) string {
		m := varRef.FindStringSubmatch(ref)
		if val := env.Get(m[1] + m[3]); val != "" {
			return val
		}
		return m[2]
	})
}

// failure says why a check failed.
func failure(ctx context.Context, err error, cmd *exec.Cmd, stderr *tail, status int, secrets []string) *MCPError {
	e := &MCPError{Kind: MCPFailed, Detail: err.Error()}
	var dns *net.DNSError
	var op *net.OpError
	switch {
	case ctx.Err() != nil:
		e.Kind = MCPTimedOut
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		e.Kind = MCPNotFound
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		e.Kind = MCPDenied
	case cmd == nil && (errors.As(err, &dns) || errors.As(err, &op)):
		e.Kind = MCPUnreachable
	case cmd != nil && (errors.Is(err, mcpsdk.ErrConnectionClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)):
		e.Kind = MCPExited
	}
	if text := stderr.String(); text != "" {
		e.Detail = text
	}
	for _, s := range secrets {
		if len(s) >= 4 {
			e.Detail = strings.ReplaceAll(e.Detail, s, inventory.Hidden)
		}
	}
	return e
}

// tail keeps the last lines a process writes, up to tailMax bytes.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

const tailMax = 4 << 10

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - tailMax; over > 0 {
		t.buf = t.buf[over:]
		if i := bytes.IndexByte(t.buf, '\n'); i >= 0 {
			t.buf = t.buf[i+1:]
		}
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

// statusRecorder adds the declared headers to a server's requests and
// keeps the last status it refused one with.
type statusRecorder struct {
	headers map[string]string
	base    http.RoundTripper
	mu      sync.Mutex
	refused int
}

func (r *statusRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	resp, err := r.base.RoundTrip(req)
	if err == nil && resp.StatusCode >= 400 {
		r.mu.Lock()
		r.refused = resp.StatusCode
		r.mu.Unlock()
	}
	return resp, err
}

func (r *statusRecorder) code() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.refused
}
