package detect

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// KubeAccessState is what a check of a context found.
type KubeAccessState int

const (
	// KubeUnknown: kubectl failed for a reason Hearth does not know; the
	// check's Detail is kubectl's own words.
	KubeUnknown KubeAccessState = iota
	// KubeReachable: the API answered and the user may list pods in the
	// context's namespace.
	KubeReachable
	// KubeUnreachable: nothing answered at the API's address in time.
	KubeUnreachable
	// KubeNoAuthProgram: the program kubectl runs to sign in is not on
	// the terminal's PATH.
	KubeNoAuthProgram
	// KubeNeedsLogin: the API or the sign-in program refused the
	// credentials: they are missing or expired.
	KubeNeedsLogin
	// KubeForbidden: signed in, without the right to list pods there.
	KubeForbidden
	// KubeAuthFailed: the sign-in program ran and gave no credentials,
	// saying nothing of signing in; Detail is its words.
	KubeAuthFailed
)

// KubeAccess is the result of a check of a context's cluster.
type KubeAccess struct {
	State KubeAccessState
	// Version is the API's version once it answered.
	Version string
	// Detail is the line that says why, kubectl's or the sign-in
	// program's; the program's name for KubeNoAuthProgram.
	Detail string
	Took   time.Duration
}

// KubeAccessTimeout bounds a check: a sign-in program may take a while.
const KubeAccessTimeout = 45 * time.Second

// CheckKubeAccess asks the context's cluster, through kubectl with the
// terminal's environment, whether it answers and lets the user list pods
// in the context's namespace. It runs the context's sign-in program, as
// any kubectl command would; it reads and changes nothing in the cluster.
// The scan never calls it.
func CheckKubeAccess(ctx context.Context, r run.Runner, kubectl string, k inventory.KubeContext, env sysenv.Env) KubeAccess {
	start := time.Now()
	res := checkKube(ctx, r, kubectl, k, env)
	res.Took = time.Since(start)
	return res
}

func checkKube(ctx context.Context, r run.Runner, kubectl string, k inventory.KubeContext, env sysenv.Env) KubeAccess {
	// A program kubectl would not find is known before asking.
	if k.Auth != "" && filepath.Base(k.Auth) == k.Auth && len(sysenv.LookPathAll(k.Auth, env)) == 0 {
		return KubeAccess{State: KubeNoAuthProgram, Detail: k.Auth}
	}
	kctx := "--context=" + k.Name
	out, err := r.Run(ctx, run.Cmd{Args: []string{kubectl, kctx, "version", "--output=json", "--request-timeout=15s"}, Env: env, Timeout: KubeAccessTimeout})
	if err != nil {
		return KubeAccess{Detail: err.Error()}
	}
	var version struct {
		Server struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	_ = json.Unmarshal(out.Stdout, &version) // none without an answer
	if out.ExitCode != 0 || version.Server.GitVersion == "" {
		return kubeFailure(string(out.Stderr))
	}
	acc := KubeAccess{Version: version.Server.GitVersion}
	namespace := k.Namespace
	if namespace == "" {
		namespace = "default"
	}
	out, err = r.Run(ctx, run.Cmd{Args: []string{kubectl, kctx, "auth", "can-i", "list", "pods", "--namespace=" + namespace, "--request-timeout=15s"}, Env: env, Timeout: KubeAccessTimeout})
	if err != nil {
		acc.Detail = err.Error()
		return acc
	}
	switch answer := strings.TrimSpace(string(out.Stdout)); {
	case answer == "yes":
		acc.State = KubeReachable
	case strings.HasPrefix(answer, "no"):
		acc.State, acc.Detail = KubeForbidden, "kubectl auth can-i list pods -n "+namespace+": "+answer
	default:
		f := kubeFailure(string(out.Stderr))
		acc.State, acc.Detail = f.State, f.Detail
	}
	return acc
}

// missingExec is client-go's word for a sign-in program it did not find.
var missingExec = regexp.MustCompile(`exec: executable \S+ not found`)

// kubeRules sort kubectl's failures by its words, in this order: the
// sign-in program's come first, as kubectl wraps them in "Unable to
// connect to the server", which alone means the address did not answer.
var kubeRules = []struct {
	state KubeAccessState
	words []string
}{
	{KubeNoAuthProgram, []string{"executable file not found", "exec: executable"}},
	{KubeNeedsLogin, []string{"unauthorized", "must be logged in", "expired", "re-authenticate", "reauthenticate", "login"}},
	{KubeAuthFailed, []string{"getting credentials"}},
	{KubeForbidden, []string{"forbidden"}},
	{KubeUnreachable, []string{"connection refused", "no such host", "i/o timeout", "network is unreachable", "no route to host", "deadline exceeded", "client.timeout", "timeout awaiting", "was refused", "dial tcp"}},
}

// kubeFailure tells why kubectl failed from what it said, and no more:
// what it does not recognize stays unknown. Detail is the line that says
// why; for a sign-in program that failed, its own words before kubectl's.
func kubeFailure(stderr string) KubeAccess {
	var lines []string
	for l := range strings.SplitSeq(stderr, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return KubeAccess{}
	}
	for _, rule := range kubeRules {
		for _, line := range lines {
			lower := strings.ToLower(line)
			if !slices.ContainsFunc(rule.words, func(w string) bool { return strings.Contains(lower, w) }) {
				continue
			}
			if rule.state == KubeNoAuthProgram && !strings.Contains(lower, "executable file not found") && !missingExec.MatchString(lower) {
				continue
			}
			if rule.state == KubeAuthFailed && len(lines) > 1 && line == lines[len(lines)-1] {
				// The program's own words come before kubectl's.
				line = lines[0]
			}
			return KubeAccess{State: rule.state, Detail: clip(line)}
		}
	}
	return KubeAccess{Detail: clip(lines[len(lines)-1])}
}

// clip keeps a line at most 300 characters long.
func clip(line string) string {
	if r := []rune(line); len(r) > 300 {
		line = string(r[:300]) + "…"
	}
	return line
}
