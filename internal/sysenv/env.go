// Package sysenv tells what environment Hearth works with: the terminal's,
// which the user's shell sets up, and Hearth's own as a GUI app, which is
// often poorer. It also finds commands on a PATH and describes the OS.
package sysenv

import (
	"os"
	"slices"
	"strings"
)

// Env is an environment as KEY=value entries, the form os.Environ returns
// and run.Cmd takes. A later entry for a key wins.
type Env []string

// GUI returns Hearth's own environment: what apps started from the Dock,
// Explorer or a desktop menu get.
func GUI() Env { return Env(os.Environ()) }

// Get returns the value of key, or "" when it is not set. Keys are
// case-insensitive on Windows.
func (e Env) Get(key string) string {
	for _, kv := range slices.Backward(e) {
		if k, v, ok := strings.Cut(kv, "="); ok && sameKey(k, key) {
			return v
		}
	}
	return ""
}

// With returns a copy of e with key set to value. Keys are matched
// case-insensitively on Windows.
func (e Env) With(key, value string) Env {
	return append(e.Without(key), key+"="+value)
}

// Without returns a copy of e without any of keys. Keys are matched
// case-insensitively on Windows.
func (e Env) Without(keys ...string) Env {
	out := make(Env, 0, len(e)+1)
	for _, kv := range e {
		k, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, want := range keys {
			if sameKey(k, want) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// with returns a copy of e with key set to value.
func (e Env) with(key, value string) Env { return e.With(key, value) }

// without returns a copy of e without key.
func (e Env) without(key string) Env { return e.Without(key) }

// HelmContext keeps the terminal environment but takes cluster and login
// settings from the explicitly selected kubeconfig context.
func (e Env) HelmContext() Env {
	out := make(Env, 0, len(e))
	for _, kv := range e {
		key, _, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "HELM_KUBE") {
			out = append(out, kv)
		}
	}
	return out
}

// KubectlDiff selects kubectl's built-in diff invocation over a custom tool.
func (e Env) KubectlDiff() Env { return e.without("KUBECTL_EXTERNAL_DIFF") }
