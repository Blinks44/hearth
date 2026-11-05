package versions

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQueryCache(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	c := &Cache{Dir: t.TempDir(), Now: func() time.Time { return now }}
	calls := 0
	fetch := func(context.Context) ([]byte, error) { calls++; return []byte("validated result"), nil }
	c.SetOffline(true)
	if _, _, err := c.Query(t.Context(), "packages.json", fetch); !errors.Is(err, ErrOffline) || calls != 0 {
		t.Fatalf("%v, calls %d", err, calls)
	}
	c.SetOffline(false)
	data, first, err := c.Query(t.Context(), "packages.json", fetch)
	if err != nil || string(data) != "validated result" || !first.Equal(now) || calls != 1 {
		t.Fatalf("%q %v %v calls %d", data, first, err, calls)
	}
	if _, _, err := c.Query(t.Context(), "packages.json", fetch); err != nil || calls != 1 {
		t.Fatalf("%v, calls %d", err, calls)
	}
	now = now.Add(25 * time.Hour)
	broken := func(context.Context) ([]byte, error) { return nil, errors.New("offline") }
	data, at, err := c.Query(t.Context(), "packages.json", broken)
	if err == nil || string(data) != "validated result" || !at.Equal(first) {
		t.Fatalf("%q %v %v", data, at, err)
	}
	c.SetOffline(true)
	data, at, err = c.Query(t.Context(), "packages.json", fetch)
	if err != nil || calls != 1 || string(data) != "validated result" || !at.Equal(first) {
		t.Fatalf("%q %v %v", data, at, err)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Query(t.Context(), "packages.json", fetch); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
}

func TestQueryCancellation(t *testing.T) {
	t.Parallel()
	c := &Cache{Dir: t.TempDir()}
	ctx, cancel := context.WithCancel(t.Context())
	_, _, err := c.Query(ctx, "cancelled.json", func(context.Context) ([]byte, error) {
		cancel()
		return []byte("must not be saved"), nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c.SetOffline(true)
	if _, _, err := c.Query(t.Context(), "cancelled.json", nil); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	c.SetOffline(false)
	_, _, err = c.Query(t.Context(), "disabled.json", func(context.Context) ([]byte, error) {
		c.SetOffline(true)
		return []byte("must not be saved"), nil
	})
	if !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, _, err := c.Query(t.Context(), "disabled.json", nil); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
}
