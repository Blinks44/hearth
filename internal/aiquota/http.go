package aiquota

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 10 * time.Second
	maxHTTPBodyBytes   = 2 << 20   // 2 MiB response cap
	maxConfigFileBytes = 256 << 10 // 256 KiB local config cap
	minRateLimitWait   = 5 * time.Minute
)

var (
	errUnauthorized = errors.New("unauthorized")
	errRateLimited  = errors.New("rate limited")
)

type rateLimitError struct {
	retryAfter time.Time
}

func (e *rateLimitError) Error() string { return "rate limited" }
func (e *rateLimitError) Unwrap() error { return errRateLimited }

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: defaultHTTPTimeout,
		// Never forward Authorization or Cookie headers across host redirects.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
				req.Header.Del("Authorization")
				req.Header.Del("Cookie")
			}
			return nil
		},
	}
}

// doJSON performs an HTTP request with a bounded body read and maps 401/403
// and 429 responses into typed errors without ever including headers or tokens.
func doJSON(ctx context.Context, client *http.Client, req *http.Request) ([]byte, error) {
	if client == nil {
		client = newHTTPClient()
	}
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, errUnauthorized
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, &rateLimitError{retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxHTTPBodyBytes {
		return nil, errors.New("response body too large")
	}
	return body, nil
}

func parseRetryAfter(header string, now time.Time) time.Time {
	header = strings.TrimSpace(header)
	minAt := now.Add(minRateLimitWait)
	if header == "" {
		return minAt
	}
	if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
		if t := now.Add(time.Duration(secs) * time.Second); t.After(minAt) {
			return t
		}
		return minAt
	}
	if t, err := http.ParseTime(header); err == nil && t.After(minAt) {
		return t
	}
	return minAt
}

// readCappedFile reads up to limit bytes from path, refusing files larger than
// limit so malformed or unexpected huge files never exhaust memory.
func readCappedFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, os.ErrInvalid
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return data, nil
}

// parseJWTClaims decodes the payload of a JWT without verifying the signature
// (only used locally to inspect exp / sub claims of already-stored user tokens).
func parseJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil
	}
	return claims
}

func jwtExpired(claims map[string]any, now time.Time) bool {
	if claims == nil {
		return false
	}
	if exp, ok := claims["exp"].(float64); ok && exp > 0 {
		return now.Unix() >= int64(exp)
	}
	return false
}

func parseISO8601(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseAnyTime parses ISO8601 strings as well as Unix seconds or milliseconds.
func parseAnyTime(v any) time.Time {
	switch x := v.(type) {
	case string:
		if t := parseISO8601(x); !t.IsZero() {
			return t
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil && f > 0 {
			return unixFloatToTime(f)
		}
	case float64:
		if x > 0 {
			return unixFloatToTime(x)
		}
	case int64:
		if x > 0 {
			return unixFloatToTime(float64(x))
		}
	case json.Number:
		if f, err := x.Float64(); err == nil && f > 0 {
			return unixFloatToTime(f)
		}
	}
	return time.Time{}
}

func unixFloatToTime(f float64) time.Time {
	if f > 1e12 {
		return time.UnixMilli(int64(f)).UTC()
	}
	return time.Unix(int64(f), 0).UTC()
}

func parseAnyFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// readFileTail reads up to limit bytes from the end of path, dropping any
// partial leading line when reading past offset 0.
func readFileTail(path string, limit int64) (string, error) {
	if limit <= 0 {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Size() == 0 {
		return "", nil
	}
	end := info.Size()
	start := max(end-limit, 0)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(f, end-start))
	if err != nil {
		return "", err
	}
	if start > 0 {
		idx := strings.IndexByte(string(data), '\n')
		if idx < 0 {
			return "", nil
		}
		data = data[idx+1:]
	}
	return string(data), nil
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
