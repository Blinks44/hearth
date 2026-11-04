package sysenv

import (
	"slices"
	"testing"
)

func TestEnvGet(t *testing.T) {
	env := Env{"A=1", "B=", "A=2", "C=x=y", "broken"}
	tests := []struct{ key, want string }{
		{"A", "2"}, // the later entry wins
		{"B", ""},
		{"C", "x=y"},
		{"D", ""},
		{"broken", ""},
	}
	for _, tt := range tests {
		if got := env.Get(tt.key); got != tt.want {
			t.Errorf("Get(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestEnvWith(t *testing.T) {
	env := Env{"A=1", "B=2", "A=3"}
	got := env.with("A", "4")
	if want := (Env{"B=2", "A=4"}); !slices.Equal(got, want) {
		t.Errorf("with = %q, want %q", got, want)
	}
	if want := (Env{"A=1", "B=2", "A=3"}); !slices.Equal(env, want) {
		t.Errorf("with changed the original: %q", env)
	}
}
