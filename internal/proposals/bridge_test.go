package proposals

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBridge(t *testing.T) {
	dir := t.TempDir()
	q := Request{strings.Repeat("a", 64)}
	if err := Send(t.Context(), dir, q); !errors.Is(err, ErrNoWindow) {
		t.Fatal(err)
	}
	var calls atomic.Int32
	stop, err := Listen(dir, func(_ context.Context, got Request) error {
		if got != q {
			t.Error(got)
		}
		calls.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	data, err := os.ReadFile(filepath.Join(dir, "mcp-bridge.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ep endpoint
	if err = json.Unmarshal(data, &ep); err != nil {
		t.Fatal(err)
	}
	// Local browser requests and callers without the file's token are refused.
	res, err := http.Post("http://"+ep.Address+"/propose", "application/json", strings.NewReader(`{"issue_id":"`+q.IssueID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden || calls.Load() != 0 {
		t.Fatal("unauthenticated request reached window")
	}
	if err = Send(t.Context(), dir, q); err != nil || calls.Load() != 1 {
		t.Fatalf("send: %v calls %d", err, calls.Load())
	}
	for _, body := range []string{`{"issue_id":"bad"}`, `{"issue_id":"` + q.IssueID + `","command":"rm"}`} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+ep.Address+"/propose", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+ep.Token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest || calls.Load() != 1 {
			t.Fatal("invalid request reached window")
		}
	}
	stop()
	if err = Send(t.Context(), dir, q); !errors.Is(err, ErrNoWindow) {
		t.Fatal(err)
	}
}

func TestBridgeRefusalAndCancellation(t *testing.T) {
	dir := t.TempDir()
	q := Request{strings.Repeat("a", 64)}
	entered := make(chan struct{})
	stop, err := Listen(dir, func(ctx context.Context, _ Request) error {
		close(entered)
		<-ctx.Done()
		return errors.New("window closed")
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- Send(t.Context(), dir, q) }()
	<-entered
	stop()
	if err := <-done; err == nil {
		t.Fatal("closed window accepted request")
	}
}
