package detect

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// CloudAuthState is the outcome of an explicit on-demand cloud sign-in check.
type CloudAuthState int

const (
	// CloudAuthUnknown means the provider failed for an unrecognized reason;
	// Detail holds the clipped error line.
	CloudAuthUnknown CloudAuthState = iota
	// CloudAuthVerified means the provider confirmed the active identity.
	CloudAuthVerified
	// CloudAuthNotLoggedIn means no credentials or active sign-in were found.
	CloudAuthNotLoggedIn
	// CloudAuthExpired means the session, SSO token, or credentials expired
	// or were rejected by the provider.
	CloudAuthExpired
	// CloudAuthNetwork means the provider endpoint could not be reached.
	CloudAuthNetwork
)

// CloudAuth is the provider-confirmed sign-in state for one cloud CLI.
// It lives only in memory while the window is open and is never written to
// snapshots or reports.
type CloudAuth struct {
	Tool  string
	State CloudAuthState
	// Identity is the principal confirmed by the provider, separate from the
	// local profile/configuration name (for example "Developer/sergey" or
	// "user@example.com").
	Identity string
	// Account is a masked account or subscription identifier for display
	// (for example "4821…7730"); never a secret or token.
	Account string
	// Detail is a short, sanitized explanation when State is not Verified.
	Detail  string
	Checked time.Time
	Took    time.Duration
}

// CloudAuthTimeout bounds an on-demand cloud identity check.
const CloudAuthTimeout = 20 * time.Second

// CheckCloudAuth asks a cloud CLI's provider who the current terminal
// environment is signed in as, using read-only commands that never print
// tokens or modify local settings. The scan never calls it.
func CheckCloudAuth(ctx context.Context, r run.Runner, tool, bin string, env sysenv.Env) CloudAuth {
	start := time.Now()
	var res CloudAuth
	switch tool {
	case "aws":
		res = checkAWSAuth(ctx, r, bin, env)
	case "gcloud":
		res = checkGcloudAuth(ctx, r, bin, env)
	case "az":
		res = checkAzAuth(ctx, r, bin, env)
	default:
		res = CloudAuth{Tool: tool, State: CloudAuthUnknown}
	}
	res.Tool = tool
	res.Checked = time.Now()
	res.Took = time.Since(start)
	return res
}

func checkAWSAuth(ctx context.Context, r run.Runner, bin string, env sysenv.Env) CloudAuth {
	runEnv := append(slices.Clip(env), "AWS_PAGER=")
	out, err := r.Run(ctx, run.Cmd{
		Args:    []string{bin, "sts", "get-caller-identity", "--output", "json", "--no-cli-pager"},
		Env:     runEnv,
		Timeout: CloudAuthTimeout,
	})
	if err != nil {
		return classifyCloudError(err.Error())
	}
	if out.ExitCode != 0 {
		return classifyCloudError(firstNonEmpty(string(out.Stderr), string(out.Stdout)))
	}
	var resp struct {
		Account string `json:"Account"`
		Arn     string `json:"Arn"`
		UserID  string `json:"UserId"`
	}
	if err := json.Unmarshal(out.Stdout, &resp); err != nil || (resp.Arn == "" && resp.Account == "") {
		return CloudAuth{State: CloudAuthUnknown, Detail: "unexpected aws sts output"}
	}
	return CloudAuth{
		State:    CloudAuthVerified,
		Identity: awsPrincipal(resp.Arn, resp.UserID),
		Account:  MaskID(resp.Account),
	}
}

// awsPrincipal extracts a readable identity from an AWS ARN without the
// account number prefix:
//
//	arn:aws:sts::123456789012:assumed-role/Developer/sergey -> Developer/sergey
//	arn:aws:iam::123456789012:user/alice -> user/alice
//	arn:aws:iam::123456789012:root -> root
func awsPrincipal(arn, fallback string) string {
	if arn == "" {
		return fallback
	}
	parts := strings.SplitN(arn, ":", 6)
	res := arn
	if len(parts) == 6 {
		res = parts[5]
	}
	if rest, ok := strings.CutPrefix(res, "assumed-role/"); ok {
		return rest
	}
	return res
}

func checkGcloudAuth(ctx context.Context, r run.Runner, bin string, env sysenv.Env) CloudAuth {
	runEnv := append(slices.Clip(env), "CLOUDSDK_CORE_DISABLE_PROMPTS=1")
	out, err := r.Run(ctx, run.Cmd{
		Args:    []string{bin, "auth", "list", "--filter=status:ACTIVE", "--format=json"},
		Env:     runEnv,
		Timeout: CloudAuthTimeout,
	})
	if err != nil {
		return classifyCloudError(err.Error())
	}
	if out.ExitCode != 0 {
		return classifyCloudError(firstNonEmpty(string(out.Stderr), string(out.Stdout)))
	}
	var accounts []struct {
		Account string `json:"account"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(out.Stdout, &accounts); err != nil {
		return CloudAuth{State: CloudAuthUnknown, Detail: "unexpected gcloud auth list output"}
	}
	active := ""
	for _, a := range accounts {
		if a.Account != "" {
			active = a.Account
			break
		}
	}
	if active == "" {
		return CloudAuth{State: CloudAuthNotLoggedIn}
	}
	// Verify that the active credentials are accepted by Google Cloud without
	// requesting or printing any access token.
	verifyOut, err := r.Run(ctx, run.Cmd{
		Args:    []string{bin, "projects", "list", "--limit=1", "--format=value(projectId)"},
		Env:     runEnv,
		Timeout: CloudAuthTimeout,
	})
	if err != nil {
		return classifyCloudError(err.Error())
	}
	if verifyOut.ExitCode == 0 {
		return CloudAuth{State: CloudAuthVerified, Identity: active}
	}
	errText := firstNonEmpty(string(verifyOut.Stderr), string(verifyOut.Stdout))
	lower := strings.ToLower(errText)
	// A 403 / PERMISSION_DENIED proves the provider accepted the identity even
	// though this user cannot list projects.
	if strings.Contains(lower, "permission_denied") || strings.Contains(lower, "does not have permission") || strings.Contains(lower, "403") {
		return CloudAuth{State: CloudAuthVerified, Identity: active}
	}
	return classifyCloudError(errText)
}

func checkAzAuth(ctx context.Context, r run.Runner, bin string, env sysenv.Env) CloudAuth {
	runEnv := append(slices.Clip(env), "AZURE_CORE_COLLECT_TELEMETRY=no")
	out, err := r.Run(ctx, run.Cmd{
		Args:    []string{bin, "ad", "signed-in-user", "show", "--query", "{upn:userPrincipalName,displayName:displayName}", "--output", "json"},
		Env:     runEnv,
		Timeout: CloudAuthTimeout,
	})
	if err != nil {
		return classifyCloudError(err.Error())
	}
	if out.ExitCode == 0 {
		var user struct {
			UPN         string `json:"upn"`
			DisplayName string `json:"displayName"`
		}
		if json.Unmarshal(out.Stdout, &user) == nil && (user.UPN != "" || user.DisplayName != "") {
			identity := or(user.UPN, user.DisplayName)
			return CloudAuth{State: CloudAuthVerified, Identity: identity}
		}
		return CloudAuth{State: CloudAuthUnknown, Detail: "unexpected az output"}
	}
	errText := firstNonEmpty(string(out.Stderr), string(out.Stdout))
	classified := classifyCloudError(errText)
	if classified.State == CloudAuthExpired || classified.State == CloudAuthNotLoggedIn {
		return classified
	}
	lower := strings.ToLower(errText)
	// Service principals and restricted directories cannot query /me; fall back
	// to account metadata only if the error is a directory/permission refusal
	// rather than expired or missing login.
	if strings.Contains(lower, "authorization_requestdenied") || strings.Contains(lower, "service principal") || strings.Contains(lower, "request_resourcenotfound") {
		accOut, accErr := r.Run(ctx, run.Cmd{
			Args:    []string{bin, "account", "show", "--query", "{user:user.name,id:id}", "--output", "json"},
			Env:     runEnv,
			Timeout: CloudAuthTimeout,
		})
		if accErr == nil && accOut.ExitCode == 0 {
			var acc struct {
				User string `json:"user"`
				ID   string `json:"id"`
			}
			if json.Unmarshal(accOut.Stdout, &acc) == nil && acc.User != "" {
				return CloudAuth{State: CloudAuthVerified, Identity: acc.User, Account: MaskID(acc.ID)}
			}
		}
	}
	return classified
}

// ListGcloudProjects returns up to 50 project IDs visible to the active gcloud
// account. Called only when the user opens the project chooser, never by scan.
func ListGcloudProjects(ctx context.Context, r run.Runner, bin string, env sysenv.Env) ([]string, error) {
	runEnv := append(slices.Clip(env), "CLOUDSDK_CORE_DISABLE_PROMPTS=1")
	out, err := r.Run(ctx, run.Cmd{
		Args:    []string{bin, "projects", "list", "--limit=50", "--format=value(projectId)"},
		Env:     runEnv,
		Timeout: CloudAuthTimeout,
	})
	if err != nil {
		return nil, err
	}
	if out.ExitCode != 0 {
		return nil, &cloudError{msg: sanitizeCloudLine(firstNonEmpty(string(out.Stderr), string(out.Stdout)))}
	}
	var ids []string
	for line := range strings.SplitSeq(string(out.Stdout), "\n") {
		if id := strings.TrimSpace(line); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

type cloudError struct{ msg string }

func (e *cloudError) Error() string { return e.msg }

// MaskID shortens an account or subscription ID to its first and last 4
// characters ("4821…7730") for display.
func MaskID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 8 {
		return id
	}
	return id[:4] + "…" + id[len(id)-4:]
}

var cloudErrorRules = []struct {
	state CloudAuthState
	words []string
}{
	{CloudAuthExpired, []string{
		"expired", "reauthentication failed", "re-authenticate", "reauthenticate",
		"refresherror", "invalid_grant", "invalidclienttokenid", "signaturedoesnotmatch",
		"unrecognizedclientexception", "aadsts", "interactionrequired",
		"sso session", "token has expired", "aws sso login", "unauthorized",
	}},
	{CloudAuthNotLoggedIn, []string{
		"unable to locate credentials", "nocredentialproviders", "no credentials",
		"please run 'az login'", "please run \"az login\"", "az login",
		"gcloud auth login", "not logged in", "no credentialed accounts",
		"you do not currently have an active account", "could not be found",
		"no subscription found",
	}},
	{CloudAuthNetwork, []string{
		"could not connect to the endpoint", "connect timeout", "connection refused",
		"no such host", "name or service not known", "nodename nor servname",
		"temporary failure in name resolution", "nameresolutionerror",
		"failed to establish a new connection", "network is unreachable",
		"no route to host", "connectionerror", "deadline exceeded", "i/o timeout",
		"tls", "ssl", "proxyerror", "timed out",
	}},
}

func classifyCloudError(raw string) CloudAuth {
	lines := nonEmptyLines(raw)
	if len(lines) == 0 {
		return CloudAuth{State: CloudAuthUnknown}
	}
	for _, rule := range cloudErrorRules {
		for _, line := range lines {
			lower := strings.ToLower(line)
			if slices.ContainsFunc(rule.words, func(w string) bool { return strings.Contains(lower, w) }) {
				return CloudAuth{State: rule.state, Detail: sanitizeCloudLine(line)}
			}
		}
	}
	return CloudAuth{State: CloudAuthUnknown, Detail: sanitizeCloudLine(lines[0])}
}

func nonEmptyLines(raw string) []string {
	var lines []string
	for l := range strings.SplitSeq(raw, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

var secretTokenRe = regexp.MustCompile(`(?i)(ya29\.[A-Za-z0-9._-]+|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]*|AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16}|bearer\s+\S+)`)

func sanitizeCloudLine(line string) string {
	line = strings.TrimSpace(line)
	line = secretTokenRe.ReplaceAllString(line, "[redacted]")
	return clip(line)
}
