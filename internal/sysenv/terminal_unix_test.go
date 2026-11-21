//go:build !windows

package sysenv

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

const marker = "MARK"

const script = "printf %s MARK; /usr/bin/env -0; printf %s MARK"

// envOut is what env -0 prints for env, between markers.
func envOut(env ...string) string {
	return marker + strings.Join(env, "\x00") + "\x00" + marker
}

func TestTerminal(t *testing.T) {
	tests := []struct {
		name    string
		shell   string
		argv    []string
		res     run.Result
		err     error
		want    Env
		wantErr bool
	}{
		{
			name:  "zsh",
			shell: "/bin/zsh",
			argv:  []string{"/bin/zsh", "-i", "-l", "-c", script},
			res:   run.Result{Stdout: []byte(envOut("PATH=/opt/homebrew/bin:/usr/bin", "HOME=/Users/me"))},
			want:  Env{"PATH=/opt/homebrew/bin:/usr/bin", "HOME=/Users/me"},
		},
		{
			name:  "rc files print before and after the markers",
			shell: "/bin/bash",
			argv:  []string{"/bin/bash", "-i", "-l", "-c", script},
			res: run.Result{Stdout: []byte("Now using node v22\n" + envOut("PATH=/a") + "\nbye\n" + marker + "junk" + marker),
				Stderr: []byte("bash: no job control in this shell")},
			want: Env{"PATH=/a"},
		},
		{
			name:  "fish greeting",
			shell: "/opt/homebrew/bin/fish",
			argv:  []string{"/opt/homebrew/bin/fish", "-i", "-l", "-c", script},
			res:   run.Result{Stdout: []byte("Welcome to fish, the friendly interactive shell\n" + envOut("PATH=/b", "fish_greeting="))},
			want:  Env{"PATH=/b", "fish_greeting="},
		},
		{
			name:  "csh takes -ic",
			shell: "/bin/tcsh",
			argv:  []string{"/bin/tcsh", "-ic", script},
			res:   run.Result{Stdout: []byte(envOut("PATH=/c"))},
			want:  Env{"PATH=/c"},
		},
		{
			name:  "values with newlines and our flag dropped",
			shell: "/bin/zsh",
			argv:  []string{"/bin/zsh", "-i", "-l", "-c", script},
			res:   run.Result{Stdout: []byte(envOut("PATH=/d", "PS1=a\nb", resolvingFlag+"=1"))},
			want:  Env{"PATH=/d", "PS1=a\nb"},
		},
		{
			name:  "non-zero exit still gives the environment",
			shell: "/bin/zsh",
			argv:  []string{"/bin/zsh", "-i", "-l", "-c", script},
			res:   run.Result{Stdout: []byte(envOut("PATH=/e")), ExitCode: 1},
			want:  Env{"PATH=/e"},
		},
		{
			name:    "no markers",
			shell:   "/bin/zsh",
			argv:    []string{"/bin/zsh", "-i", "-l", "-c", script},
			res:     run.Result{Stdout: []byte("zsh: command not found: printf"), ExitCode: 127},
			wantErr: true,
		},
		{
			name:    "one marker",
			shell:   "/bin/zsh",
			argv:    []string{"/bin/zsh", "-i", "-l", "-c", script},
			res:     run.Result{Stdout: []byte(marker + "PATH=/f\x00")},
			wantErr: true,
		},
		{
			name:    "timeout",
			shell:   "/bin/zsh",
			argv:    []string{"/bin/zsh", "-i", "-l", "-c", script},
			err:     context.DeadlineExceeded,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &run.Fake{}
			r.Set(tt.res, tt.err, tt.argv...)
			got, err := terminal(t.Context(), r, Env{"SHELL=" + tt.shell, "PATH=/usr/bin"}, marker)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Errorf("err = %v, want it to wrap %v", err, tt.err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("env = %q, want %q", got, tt.want)
			}
			calls := r.Calls()
			if len(calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(calls))
			}
			if shellEnv := Env(calls[0].Env); shellEnv.Get(resolvingFlag) != "1" || shellEnv.Get("PATH") != loginPath() {
				t.Errorf("shell env = %q, want baseline PATH %q with %s=1", calls[0].Env, loginPath(), resolvingFlag)
			}
			if calls[0].Timeout != shellTimeout {
				t.Errorf("timeout = %v, want %v", calls[0].Timeout, shellTimeout)
			}
		})
	}
}

func TestLoginEnvResetsPathAndMiseSession(t *testing.T) {
	gui := Env{
		"HOME=/Users/me",
		"SHELL=/bin/zsh",
		"LANG=en_US.UTF-8",
		"PATH=/Users/me/.orbstack/bin:/opt/homebrew/sbin:/opt/homebrew/Cellar/go/1.27.4/libexec/bin:/usr/bin",
		"MISE_SHELL=zsh",
		"__MISE_DIFF=abc",
		"__MISE_ORIG_PATH=/Users/me/.orbstack/bin:/usr/bin",
		"__MISE_SESSION=xyz",
	}
	got := loginEnv(gui)
	if got.Get("PATH") != loginPath() {
		t.Fatalf("PATH = %q, want %q", got.Get("PATH"), loginPath())
	}
	for _, k := range []string{"MISE_SHELL", "__MISE_DIFF", "__MISE_ORIG_PATH", "__MISE_SESSION"} {
		if v := got.Get(k); v != "" {
			t.Errorf("%s = %q, want cleared", k, v)
		}
	}
	for k, want := range map[string]string{"HOME": "/Users/me", "SHELL": "/bin/zsh", "LANG": "en_US.UTF-8"} {
		if v := got.Get(k); v != want {
			t.Errorf("%s = %q, want %q", k, v, want)
		}
	}
}

func TestLoginShellFallback(t *testing.T) {
	// No $SHELL and the user database does not answer.
	if got := loginShell(t.Context(), &run.Fake{}, Env{"PATH=/usr/bin"}); got != "/bin/sh" {
		t.Errorf("loginShell = %q, want /bin/sh", got)
	}
}
