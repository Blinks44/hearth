package versions

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type jsTransport func(*http.Request) (*http.Response, error)

func (f jsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Modern Yarn is published through its own index, not the Classic npm
// package. The same cache must serve all three releases when offline.
func TestFetchJSManagers(t *testing.T) {
	calls := 0
	c := &Cache{Dir: t.TempDir(), Client: &http.Client{Transport: jsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := ""
		switch r.URL.String() {
		case "https://registry.npmjs.org/npm/latest":
			body = `{"version":"11.0.0"}`
		case "https://registry.npmjs.org/pnpm/latest":
			body = `{"version":"9.15.4"}`
		case "https://repo.yarnpkg.com/tags":
			body = `{"latest":{"stable":"4.6.0"}}`
		default:
			t.Errorf("unexpected request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	for range 2 {
		got, err := FetchJSManagers(t.Context(), c)
		if err != nil {
			t.Fatal(err)
		}
		if got.Npm.Version != "11.0.0" || got.Pnpm.Version != "9.15.4" || got.Yarn.Version != "4.6.0" {
			t.Fatalf("releases: %+v", got)
		}
	}
	if calls != 3 {
		t.Fatalf("requests=%d, want 3", calls)
	}
}
