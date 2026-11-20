package kube

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// requestTimeout bounds one API request, as kubectl's --request-timeout.
const requestTimeout = 15 * time.Second

// API reads clusters through client-go's sign-in and transport. It keeps
// each context's sign-in, connection and the kinds the API serves between
// reads, where every kubectl call
// starts, signs in and connects anew. The kubeconfig and its sign-in
// program are the terminal's, as for kubectl.
type API struct {
	mu    sync.Mutex
	conns map[string]*conn // by context
}

// NewAPI returns an API with no connections yet.
func NewAPI() *API { return &API{conns: map[string]*conn{}} }

type conn struct {
	// config fingerprints the kubeconfig and environment it was made of.
	config    [sha256.Size]byte
	base      *url.URL
	client    *http.Client
	namespace string

	kindsMu sync.Mutex
	kinds   []apiKind // in discovery's order; nil until read
}

// apiKind is a kind the API serves, under one of its versions.
type apiKind struct {
	group, version, resource, kind string
	namespaced                     bool
}

func (k apiKind) apiVersion() string {
	return schema.GroupVersion{Group: k.group, Version: k.version}.String()
}

// errUseKubectl means the API does not read this request and kubectl
// should: arguments it does not know, a kubeconfig it cannot use, an API
// older than aggregated discovery (Kubernetes 1.27), or a sign-in that
// failed, whose program's own words only kubectl shows.
var errUseKubectl = errors.New("read through kubectl")

// get reads what `kubectl get args --output=json` would print into v.
func (a *API) get(ctx context.Context, env sysenv.Env, t Target, args []string, v any) error {
	req, ok := parseGet(args)
	if !ok {
		return errUseKubectl
	}
	c, err := a.conn(env, t.Context)
	if err != nil {
		return errUseKubectl
	}
	// A sign-in program does not stop with the request's context: the
	// read is left to finish on its own when the caller is gone or the
	// time kubectl would have had is up.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := c.read(ctx, req)
		done <- result{data, err}
	}()
	var res result
	select {
	case res = <-done:
	case <-ctx.Done():
		return &Error{Access: detect.KubeFailure(ctx.Err().Error())}
	}
	if res.err != nil {
		if errors.Is(res.err, errUseKubectl) || strings.Contains(res.err.Error(), "getting credentials") {
			return errUseKubectl
		}
		if failed, ok := errors.AsType[*Error](res.err); ok {
			return failed
		}
		return &Error{Access: detect.KubeFailure(res.err.Error())}
	}
	if err := json.Unmarshal(res.data, v); err != nil {
		return fmt.Errorf("kubectl get %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// getRequest is a kubectl get the API reads.
type getRequest struct {
	raw       string   // a path read as it is
	resources []string // or these kinds
	name      string   // the one object of resources[0]
	namespace string
	all       bool
	query     url.Values
}

// parseGet understands the kubectl get arguments this package uses.
func parseGet(args []string) (getRequest, bool) {
	var req getRequest
	req.query = url.Values{}
	if len(args) == 2 && args[0] == "--raw" { // metrics read it without --output
		req.raw = args[1]
		return req, strings.HasPrefix(req.raw, "/")
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return req, false
	}
	kinds, name, single := strings.Cut(args[0], "/")
	if single && (name == "" || strings.Contains(kinds, ",")) {
		return req, false
	}
	req.resources, req.name = strings.Split(kinds, ","), name
	for _, arg := range args[1:] {
		switch key, value, _ := strings.Cut(arg, "="); key {
		case "--namespace":
			req.namespace = value
		case "--all-namespaces":
			req.all = true
		case "--selector":
			req.query.Set("labelSelector", value)
		case "--field-selector":
			req.query.Set("fieldSelector", value)
		case "--output":
			if value != "json" {
				return req, false
			}
		default:
			return req, false
		}
	}
	return req, !slices.Contains(req.resources, "")
}

// conn returns the context's connection, made anew when its kubeconfig
// or the terminal's environment changed.
func (a *API) conn(env sysenv.Env, context string) (*conn, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	files := detect.KubeConfigFiles(env, home)
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%q\x00%q\x00", context, []string(env))
	for _, f := range files {
		data, _ := os.ReadFile(f) // kubectl skips a file that is missing
		_, _ = fmt.Fprintf(h, "%q\x00%d\x00", f, len(data))
		h.Write(data)
	}
	var config [sha256.Size]byte
	h.Sum(config[:0])

	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.conns[context]; c != nil && c.config == config {
		return c, nil
	}
	c, err := connect(env, files, context)
	if err != nil {
		return nil, err
	}
	c.config = config
	a.conns[context] = c
	return c, nil
}

func connect(env sysenv.Env, files []string, context string) (*conn, error) {
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{Precedence: files},
		&clientcmd.ConfigOverrides{CurrentContext: context})
	cfg, err := loader.ClientConfig()
	if err != nil {
		return nil, err
	}
	namespace, _, err := loader.Namespace()
	if err != nil {
		return nil, err
	}
	if p := cfg.ExecProvider; p != nil {
		// client-go runs the program with Hearth's own PATH and
		// environment; kubectl in the terminal would run it with the
		// terminal's.
		if filepath.Base(p.Command) == p.Command {
			if found := sysenv.LookPathAll(p.Command, env); len(found) > 0 {
				p.Command = found[0]
			}
		}
		vars := make([]clientcmdapi.ExecEnvVar, 0, len(env)+len(p.Env))
		for _, kv := range env {
			if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
				vars = append(vars, clientcmdapi.ExecEnvVar{Name: k, Value: v})
			}
		}
		p.Env = append(vars, p.Env...) // the kubeconfig's own come last and win
	}
	cfg.Timeout = requestTimeout
	client, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}
	base, _, err := rest.DefaultServerUrlFor(cfg)
	if err != nil {
		return nil, err
	}
	return &conn{base: base, client: client, namespace: cmp.Or(namespace, "default")}, nil
}

// read answers the request as kubectl get --output=json would: a list
// of every kind's items, each with its kind and apiVersion, or the one
// object.
func (c *conn) read(ctx context.Context, req getRequest) ([]byte, error) {
	if req.raw != "" {
		return c.fetch(ctx, req.raw, nil, "")
	}
	kinds := make([]apiKind, len(req.resources))
	for i, resource := range req.resources {
		k, err := c.kindOf(ctx, resource)
		if err != nil {
			return nil, err
		}
		kinds[i] = k
	}
	if req.name != "" {
		return c.fetch(ctx, c.path(kinds[0], req)+"/"+url.PathEscape(req.name), nil, "")
	}
	lists := make([][]json.RawMessage, len(kinds))
	errs := make([]error, len(kinds))
	var wg sync.WaitGroup
	for i, k := range kinds {
		wg.Go(func() {
			data, err := c.fetch(ctx, c.path(k, req), req.query, "")
			if err != nil {
				errs[i] = err
				return
			}
			var list struct {
				Items []json.RawMessage `json:"items"`
			}
			if err := json.Unmarshal(data, &list); err != nil {
				errs[i] = fmt.Errorf("kubectl get %s: %w", k.resource, err)
				return
			}
			for j, item := range list.Items {
				list.Items[j] = withKind(item, k.kind, k.apiVersion())
			}
			lists[i] = list.Items
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString(`{"apiVersion":"v1","kind":"List","items":[`)
	n := 0
	for _, items := range lists {
		for _, item := range items {
			if n > 0 {
				out.WriteByte(',')
			}
			out.Write(item)
			n++
		}
	}
	out.WriteString("]}")
	return out.Bytes(), nil
}

// kindOf finds a kind as kubectl does: resource, resource.group or
// resource.version.group; without a group, the core group's comes first.
func (c *conn) kindOf(ctx context.Context, resource string) (apiKind, error) {
	full, partial := schema.ParseResourceArg(resource)
	for _, fresh := range []bool{false, true} { // a kind added since discovery
		kinds, err := c.served(ctx, fresh)
		if err != nil {
			return apiKind{}, err
		}
		for _, k := range kinds {
			if full != nil && k.resource == full.Resource && k.group == full.Group && k.version == full.Version {
				return k, nil
			}
		}
		for _, k := range kinds {
			if k.resource == partial.Resource && (partial.Group == "" || k.group == partial.Group) {
				return k, nil
			}
		}
	}
	return apiKind{}, &Error{Access: detect.KubeFailure(fmt.Sprintf("error: the server doesn't have a resource type %q", resource))}
}

// discoveryAccept asks for aggregated discovery: every group's versions
// and kinds in one answer.
const discoveryAccept = "application/json;g=apidiscovery.k8s.io;v=v2;as=APIGroupDiscoveryList"

// served is the kinds the API serves, as discovery read them once, or
// anew when fresh: the core group's, then the others', each group's
// preferred version first.
func (c *conn) served(ctx context.Context, fresh bool) ([]apiKind, error) {
	c.kindsMu.Lock()
	defer c.kindsMu.Unlock()
	if c.kinds != nil && !fresh {
		return c.kinds, nil
	}
	var kinds []apiKind
	for _, path := range []string{"/api", "/apis"} {
		data, err := c.fetch(ctx, path, nil, discoveryAccept)
		if err != nil {
			return nil, err
		}
		var list struct {
			Kind  string `json:"kind"`
			Items []struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Versions []struct {
					Version   string `json:"version"`
					Resources []struct {
						Resource     string `json:"resource"`
						Scope        string `json:"scope"`
						ResponseKind struct {
							Kind string `json:"kind"`
						} `json:"responseKind"`
					} `json:"resources"`
				} `json:"versions"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &list); err != nil || list.Kind != "APIGroupDiscoveryList" {
			return nil, errUseKubectl // an API older than aggregated discovery
		}
		for _, g := range list.Items {
			for _, v := range g.Versions {
				for _, r := range v.Resources {
					kinds = append(kinds, apiKind{group: g.Metadata.Name, version: v.Version, resource: r.Resource,
						kind: r.ResponseKind.Kind, namespaced: r.Scope == "Namespaced"})
				}
			}
		}
	}
	c.kinds = kinds
	return kinds, nil
}

// path is the URL path of a kind's objects in the request's namespace.
func (c *conn) path(k apiKind, req getRequest) string {
	p := "/apis/" + k.apiVersion()
	if k.group == "" {
		p = "/api/" + k.version
	}
	if k.namespaced && !req.all {
		p += "/namespaces/" + url.PathEscape(cmp.Or(req.namespace, c.namespace))
	}
	return p + "/" + k.resource
}

// fetch GETs a path of the API server, as JSON unless accept says else.
func (c *conn) fetch(ctx context.Context, path string, query url.Values, accept string) ([]byte, error) {
	u := *c.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", cmp.Or(accept, "application/json"))
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	const limit = 64 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		var status struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &status) // a proxy may answer without a Status
		message := cmp.Or(status.Message, strings.TrimSpace(string(data)))
		return nil, &Error{Access: detect.KubeFailure(statusLine(resp.StatusCode, status.Reason, message))}
	}
	if len(data) > limit {
		return nil, fmt.Errorf("kubectl get %s: the answer is too large", path)
	}
	return data, nil
}

// statusLine says what the API refused in kubectl's words, which the
// access check's rules and the callers know.
func statusLine(code int, reason, message string) string {
	if code == http.StatusUnauthorized {
		return "error: You must be logged in to the server (Unauthorized)"
	}
	return fmt.Sprintf("Error from server (%s): %s", cmp.Or(reason, http.StatusText(code)), cmp.Or(message, http.StatusText(code)))
}

// withKind puts the kind and apiVersion, which a list's items lack, in
// front of an item, as kubectl does.
func withKind(item json.RawMessage, kind, apiVersion string) json.RawMessage {
	rest, ok := bytes.CutPrefix(bytes.TrimSpace(item), []byte("{"))
	if !ok {
		return item
	}
	head := fmt.Sprintf(`{"kind":%q,"apiVersion":%q`, kind, apiVersion)
	if rest = bytes.TrimSpace(rest); !bytes.HasPrefix(rest, []byte("}")) {
		head += ","
	}
	return append([]byte(head), rest...)
}
