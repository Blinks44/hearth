package versions

import (
	"context"
	"net/http"
	"net/http/httptest"
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
