package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	neturl "net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

// envRef is a value taken from the environment, as the agents write one:
// ${NAME}, $NAME, ${env:NAME}, {env:NAME} or %NAME%.
var envRef = regexp.MustCompile(`\$\{[^}]+\}|\$[A-Za-z_][A-Za-z0-9_]*|\{env:[^}]+\}|%[A-Za-z_][A-Za-z0-9_]*%`)

// keyName is a key's name secretName misses, as OPENAI_KEY.
var keyName = regexp.MustCompile(`(?i)(^|[_-])key$`)

// AuthScheme splits a header's value into its scheme, as "Bearer ", and
// the credential after it; scheme is "" for a value without one.
func AuthScheme(value string) (scheme, credential string) {
	for _, s := range []string{"Bearer ", "Basic ", "token "} {
		if len(value) > len(s) && strings.EqualFold(value[:len(s)], s) {
			return value[:len(s)], value[len(s):]
		}
	}
	return "", value
}

// PlainSecret reports that value, of the variable name, is a secret
// written out, as plainSecret does.
func PlainSecret(name, value string) bool { return plainSecret(name, value) }

// plainSecret reports that value, of the variable or header name, is a
// secret written out: a token by its prefix, or under a secret's name a
// word of 6 characters or more that is no path, URL or switch. A value
// taken from the environment is none.
func plainSecret(name, value string) bool {
	if envRef.MatchString(value) {
		return false
	}
	_, v := AuthScheme(strings.TrimSpace(value))
	switch {
	case secretValue.MatchString(v):
		return true
	case !secretName.MatchString(name) && !keyName.MatchString(name), len(v) < 6, strings.ContainsAny(v, " \t"), strings.Contains(v, "://"),
		strings.HasPrefix(v, "/"), strings.HasPrefix(v, "~"), v == "true", v == "false":
		return false
	}
	return true
}

// plainNames are the keys of m, sorted, whose string values are secrets
// written out.
func plainNames[V any](m map[string]V) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		var v string
		switch raw := any(m[k]).(type) {
		case string:
			v = raw
		case json.RawMessage:
			if json.Unmarshal(raw, &v) != nil {
				continue
			}
		default:
			continue
		}
		if plainSecret(k, v) {
			out = append(out, k)
		}
	}
	return out
}

// plainArgs reports a secret written out in args or url, those of s
// before masked hid them: what masked hid, but of a URL only a password
// and query values under a secret's name.
func plainArgs(s inventory.MCPServer, args []string, url string) bool {
	for i, a := range args {
		switch {
		case i >= len(s.Args) || s.Args[i] == a || envRef.MatchString(a):
		case strings.Contains(a, "://"):
			if urlSecret(a) {
				return true
			}
		default:
			return true
		}
	}
	return url != "" && urlSecret(url)
}

// urlSecret reports a password or a secret's query value written out in
// raw.
func urlSecret(raw string) bool {
	u, err := neturl.Parse(raw)
	if err != nil {
		return false
	}
	if p, ok := u.User.Password(); ok && !envRef.MatchString(p) {
		return true
	}
	for k, vs := range u.Query() {
		for _, v := range vs {
			if plainSecret(k, v) {
				return true
			}
		}
	}
	return false
}

// SettingsSecrets are the variables the "env" of the JSON settings file at
// path sets to secrets written out; none when the file is not there.
func SettingsSecrets(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Env map[string]json.RawMessage `json:"env"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return plainNames(cfg.Env), nil
}
