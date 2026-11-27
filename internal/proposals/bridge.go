// Package proposals carries requests to the desktop, never executable plans.
package proposals

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrNoWindow = errors.New("open the Hearth desktop window before proposing a fix")

// Request selects an existing issue; commands, edits and confirmation are absent.
type Request struct {
	IssueID string `json:"issue_id" jsonschema:"ID returned by get_issues. The desktop rechecks this issue; no command or plan can be supplied."`
}

func (r Request) Valid() bool {
	if len(r.IssueID) != 64 {
		return false
	}
	for _, c := range r.IssueID {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

type endpoint struct{ Address, Token string }
type reply struct {
	Error string `json:"error,omitempty"`
}

// Listen publishes an authenticated loopback endpoint in the user's config dir.
// Closing the window cancels handlers, closes sockets and removes its own record.
func Listen(dir string, handle func(context.Context, Request) error) (func(), error) {
	return ListenWithEvents(dir, handle, nil)
}

// ListenWithEvents publishes the authenticated loopback endpoint for both
// proposal requests (/propose) and strict-schema agent lifecycle events (/agent-event).
func ListenWithEvents(dir string, handle func(context.Context, Request) error, onEvent func(AgentEvent)) (func(), error) {
	if dir == "" {
		return nil, errors.New("proposal config directory is unavailable")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for proposals: %w", err)
	}
	ep := endpoint{ln.Addr().String(), rand.Text()}
	ctx, cancel := context.WithCancel(context.Background())
	server := &http.Server{ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+ep.Token)) != 1 {
			http.Error(w, "refused", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/propose":
			var q Request
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
			d.DisallowUnknownFields()
			if err := d.Decode(&q); err != nil || !q.Valid() || d.Decode(new(any)) != io.EOF {
				http.Error(w, "invalid issue ID", http.StatusBadRequest)
				return
			}
			callCtx, stop := context.WithTimeout(r.Context(), 10*time.Second)
			defer stop()
			result := reply{}
			if handle != nil {
				if err := handle(callCtx, q); err != nil {
					result.Error = err.Error()
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
		case "/agent-event":
			if onEvent == nil {
				http.Error(w, "unavailable", http.StatusNotFound)
				return
			}
			var ev AgentEvent
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
			d.DisallowUnknownFields()
			if err := d.Decode(&ev); err != nil || !ev.Valid() || d.Decode(new(any)) != io.EOF {
				http.Error(w, "invalid agent event", http.StatusBadRequest)
				return
			}
			if ev.Timestamp == 0 {
				ev.Timestamp = time.Now().Unix()
			}
			onEvent(ev)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(reply{})
		default:
			http.Error(w, "refused", http.StatusForbidden)
		}
	})
	path := filepath.Join(dir, "mcp-bridge.json")
	data, _ := json.Marshal(ep)
	// A new inode avoids retaining permissive modes from a previous record.
	tmp, err := os.CreateTemp(dir, ".mcp-bridge-*")
	if err == nil {
		_, err = tmp.Write(data)
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmp.Name(), path)
		}
		_ = os.Remove(tmp.Name())
	}
	if err != nil {
		cancel()
		_ = ln.Close()
		return nil, fmt.Errorf("publish proposal endpoint: %w", err)
	}
	go func() { _ = server.Serve(ln) }()
	return func() {
		cancel()
		_ = server.Close()
		if current, err := os.ReadFile(path); err == nil && string(current) == string(data) {
			_ = os.Remove(path)
		}
	}, nil
}

func postBridge(ctx context.Context, dir, route string, timeout time.Duration, payload any) (*http.Response, error) {
	if dir == "" {
		return nil, ErrNoWindow
	}
	data, err := os.ReadFile(filepath.Join(dir, "mcp-bridge.json"))
	if err != nil {
		return nil, ErrNoWindow
	}
	var ep endpoint
	if json.Unmarshal(data, &ep) != nil || ep.Token == "" {
		return nil, ErrNoWindow
	}
	host, port, err := net.SplitHostPort(ep.Address)
	if err != nil || host != "127.0.0.1" || port == "" {
		return nil, ErrNoWindow
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ep.Address+route, strings.NewReader(string(body)))
	if err != nil {
		return nil, ErrNoWindow
	}
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, ErrNoWindow
	}
	return res, nil
}

// Send never opens a window or starts an operation. Only the desktop can plan.
func Send(ctx context.Context, dir string, q Request) error {
	if !q.Valid() {
		return errors.New("invalid issue ID; call get_issues again")
	}
	res, err := postBridge(ctx, dir, "/propose", 12*time.Second, q)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	var out reply
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 2048)).Decode(&out) != nil {
		return errors.New("hearth refused the proposal; retry in its desktop window")
	}
	if out.Error != "" {
		return errors.New(out.Error)
	}
	return nil
}
