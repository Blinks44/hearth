package versions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The cache asks the server at most once a day, then with the ETag, and
// keeps showing its copy while the server is out of reach.
func TestCache(t *testing.T) {
	requests := 0
	var gotETag string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotETag = r.Header.Get("If-None-Match")
		if gotETag == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("list"))
	}))
	defer srv.Close()

	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	c := &Cache{Dir: t.TempDir(), Now: func() time.Time { return now }}
	get := func() (string, time.Time) {
		t.Helper()
		body, fetched, err := c.Get(context.Background(), "list.json", srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		return string(body), fetched
	}

	if body, fetched := get(); body != "list" || !fetched.Equal(now) || requests != 1 {
		t.Fatalf("first: %q at %v, %d requests", body, fetched, requests)
	}
	first := now
	now = now.Add(23 * time.Hour)
	if body, fetched := get(); body != "list" || !fetched.Equal(first) || requests != 1 {
		t.Errorf("within a day: %q at %v, %d requests", body, fetched, requests)
	}
	now = now.Add(2 * time.Hour)
	if body, fetched := get(); body != "list" || !fetched.Equal(now) || requests != 2 || gotETag != `"v1"` {
		t.Errorf("a day on: %q at %v, %d requests, ETag %q", body, fetched, requests, gotETag)
	}

	// Offline: the copy, from when it was last fetched.
	checked := now
	srv.Close()
	now = now.Add(48 * time.Hour)
	if body, fetched := get(); body != "list" || !fetched.Equal(checked) {
		t.Errorf("offline: %q at %v", body, fetched)
	}
	empty := &Cache{Dir: t.TempDir()}
	if _, _, err := empty.Get(context.Background(), "list.json", srv.URL); err == nil {
		t.Error("offline without a copy: no error")
	}
}

// A server error is no list: without a copy it is an error.
func TestCacheServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := &Cache{Dir: t.TempDir()}
	if body, _, err := c.Get(context.Background(), "list.json", srv.URL); err == nil {
		t.Errorf("got %q without error", body)
	}
}

func TestOfflineCacheAndClear(t *testing.T) {
	c := &Cache{Dir: filepath.Join(t.TempDir(), "versions")}
	c.SetOffline(true)
	if _, _, err := c.Get(context.Background(), "node.json", "http://invalid.invalid"); !errors.Is(err, ErrOffline) {
		t.Fatalf("missing offline cache: %v", err)
	}
	when := time.Now().Add(-72 * time.Hour)
	if err := c.write(filepath.Join(c.Dir, "node.json"), []byte("saved"), meta{Fetched: when}); err != nil {
		t.Fatal(err)
	}
	body, _, err := c.Get(context.Background(), "node.json", "http://invalid.invalid")
	if err != nil || string(body) != "saved" {
		t.Fatalf("old offline cache: %q %v", body, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.Get(ctx, "node.json", "http://invalid.invalid"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled cache read: %v", err)
	}
	sibling := filepath.Join(filepath.Dir(c.Dir), "settings.json")
	if err := os.WriteFile(sibling, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "node.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("clear removed sibling: %v", err)
	}
}

// Canceling a download before clearing must not recreate a cleared file.
func TestClearCanceledDownload(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := &Cache{Dir: filepath.Join(t.TempDir(), "versions")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := c.Get(ctx, "list.json", srv.URL); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download did not stop")
	}
	if _, err := os.Stat(c.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache recreated after clear: %v", err)
	}
}
