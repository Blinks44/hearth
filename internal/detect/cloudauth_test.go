package detect

import (
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestCheckCloudAuth(t *testing.T) {
	fail := func(stderr string) run.Result { return run.Result{ExitCode: 1, Stderr: []byte(stderr)} }

	t.Run("aws verified, expired, missing, and network error", func(t *testing.T) {
		f := &run.Fake{}
		f.Stdout(`{"UserId":"AROAEXAMPLE:sergey","Account":"482100007730","Arn":"arn:aws:sts::482100007730:assumed-role/Developer/sergey"}`,
			"aws", "sts", "get-caller-identity", "--output", "json", "--no-cli-pager")
		got := CheckCloudAuth(t.Context(), f, "aws", "aws", sysenv.Env{"AWS_PROFILE=staging"})
		if got.State != CloudAuthVerified || got.Identity != "Developer/sergey" || got.Account != "4821…7730" {
			t.Fatalf("verified aws = %+v", got)
		}

		for _, tc := range []struct {
			name   string
			stderr string
			want   CloudAuthState
		}{
			{"sso expired", "Error loading SSO Token: The SSO session associated with this profile has expired or is otherwise invalid.", CloudAuthExpired},
			{"missing creds", "Unable to locate credentials. You can configure credentials by running \"aws configure\".", CloudAuthNotLoggedIn},
			{"network", "Could not connect to the endpoint URL: \"https://sts.eu-central-1.amazonaws.com/\"", CloudAuthNetwork},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ff := &run.Fake{}
				ff.Set(fail(tc.stderr), nil, "aws", "sts", "get-caller-identity", "--output", "json", "--no-cli-pager")
				res := CheckCloudAuth(t.Context(), ff, "aws", "aws", nil)
				if res.State != tc.want {
					t.Errorf("state = %v, want %v (%+v)", res.State, tc.want, res)
				}
			})
		}
	})

	t.Run("gcloud verified, permission denied still verified, expired, empty, and network", func(t *testing.T) {
		f := &run.Fake{}
		f.Stdout(`[{"account":"dev@example.com","status":"ACTIVE"}]`, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=json")
		f.Stdout("shop-prod\n", "gcloud", "projects", "list", "--limit=1", "--format=value(projectId)")
		got := CheckCloudAuth(t.Context(), f, "gcloud", "gcloud", nil)
		if got.State != CloudAuthVerified || got.Identity != "dev@example.com" {
			t.Fatalf("verified gcloud = %+v", got)
		}

		// 403 on projects list still confirms the active account is signed in.
		f403 := &run.Fake{}
		f403.Stdout(`[{"account":"dev@example.com","status":"ACTIVE"}]`, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=json")
		f403.Set(fail("ERROR: (gcloud.projects.list) PERMISSION_DENIED: Caller does not have permission"), nil, "gcloud", "projects", "list", "--limit=1", "--format=value(projectId)")
		got = CheckCloudAuth(t.Context(), f403, "gcloud", "gcloud", nil)
		if got.State != CloudAuthVerified || got.Identity != "dev@example.com" {
			t.Fatalf("403 gcloud = %+v", got)
		}

		// Expired refresh token.
		fExp := &run.Fake{}
		fExp.Stdout(`[{"account":"dev@example.com","status":"ACTIVE"}]`, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=json")
		fExp.Set(fail("ERROR: (gcloud.projects.list) Reauthentication failed. cannot prompt during non-interactive execution."), nil, "gcloud", "projects", "list", "--limit=1", "--format=value(projectId)")
		got = CheckCloudAuth(t.Context(), fExp, "gcloud", "gcloud", nil)
		if got.State != CloudAuthExpired {
			t.Fatalf("expired gcloud = %+v", got)
		}

		// No active account.
		fNone := &run.Fake{}
		fNone.Stdout(`[]`, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=json")
		got = CheckCloudAuth(t.Context(), fNone, "gcloud", "gcloud", nil)
		if got.State != CloudAuthNotLoggedIn {
			t.Fatalf("not logged in gcloud = %+v", got)
		}

		// Network error.
		fNet := &run.Fake{}
		fNet.Stdout(`[{"account":"dev@example.com","status":"ACTIVE"}]`, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=json")
		fNet.Set(fail("ERROR: Failed to establish a new connection: [Errno -3] Temporary failure in name resolution"), nil, "gcloud", "projects", "list", "--limit=1", "--format=value(projectId)")
		got = CheckCloudAuth(t.Context(), fNet, "gcloud", "gcloud", nil)
		if got.State != CloudAuthNetwork {
			t.Fatalf("network gcloud = %+v", got)
		}
	})

	t.Run("az verified, service principal fallback, not logged in, and expired", func(t *testing.T) {
		f := &run.Fake{}
		f.Stdout(`{"upn":"alice@contoso.com","displayName":"Alice"}`, "az", "ad", "signed-in-user", "show", "--query", "{upn:userPrincipalName,displayName:displayName}", "--output", "json")
		got := CheckCloudAuth(t.Context(), f, "az", "az", nil)
		if got.State != CloudAuthVerified || got.Identity != "alice@contoso.com" {
			t.Fatalf("verified az = %+v", got)
		}

		// Service principal fallback.
		fSP := &run.Fake{}
		fSP.Set(fail("ERROR: Authorization_RequestDenied: Insufficient privileges"), nil, "az", "ad", "signed-in-user", "show", "--query", "{upn:userPrincipalName,displayName:displayName}", "--output", "json")
		fSP.Stdout(`{"user":"sp-deploy-app","id":"12345678-90ab-cdef-1234-567890abcdef"}`, "az", "account", "show", "--query", "{user:user.name,id:id}", "--output", "json")
		got = CheckCloudAuth(t.Context(), fSP, "az", "az", nil)
		if got.State != CloudAuthVerified || got.Identity != "sp-deploy-app" || got.Account != "1234…cdef" {
			t.Fatalf("sp az = %+v", got)
		}

		// Not logged in.
		fLogin := &run.Fake{}
		fLogin.Set(fail("ERROR: Please run 'az login' to setup account."), nil, "az", "ad", "signed-in-user", "show", "--query", "{upn:userPrincipalName,displayName:displayName}", "--output", "json")
		got = CheckCloudAuth(t.Context(), fLogin, "az", "az", nil)
		if got.State != CloudAuthNotLoggedIn {
			t.Fatalf("az not logged in = %+v", got)
		}

		// Token redaction in detail.
		fTok := &run.Fake{}
		fTok.Set(fail("ERROR: Bearer ya29.a0secretTokenValue expired"), nil, "az", "ad", "signed-in-user", "show", "--query", "{upn:userPrincipalName,displayName:displayName}", "--output", "json")
		got = CheckCloudAuth(t.Context(), fTok, "az", "az", nil)
		if strings.Contains(got.Detail, "ya29.a0secretTokenValue") {
			t.Fatalf("token leaked into detail: %q", got.Detail)
		}
	})
}
