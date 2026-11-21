package ops

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// cloudSafeValue matches safe identifiers for AWS profiles/regions, gcloud
// configurations/projects, and Azure subscription IDs/names: no control
// characters, no leading dash (flag injection), and no shell metacharacters.
var cloudSafeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/+= -]*$`)

func validCloudValue(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && !strings.ContainsAny(s, "\x00\r\n'\"`$\\;|&<>") && cloudSafeID.MatchString(s)
}

func validAWSValue(s string) bool {
	return validCloudValue(s) && !strings.ContainsAny(s, " \t")
}

// AWSProfilePlan switches the default AWS_PROFILE for new terminals in
// Hearth's managed shell block (Unix) or user environment (Windows), without
// reading or editing AWS credentials. When region is non-empty and cli is
// set, it also runs `aws configure set region`.
func AWSProfilePlan(cli, from, to, region string, env sysenv.Env, r run.Runner) (Plan, error) {
	to = strings.TrimSpace(to)
	region = strings.TrimSpace(region)
	if !validAWSValue(to) || (region != "" && !validAWSValue(region)) {
		return Plan{}, errors.New(locales.CloudAwsInvalid())
	}
	steps, notes, err := awsProfileSteps(to, env)
	if err != nil {
		return Plan{}, err
	}
	changes := []string{locales.CloudAwsSwitchChange(cmp.Or(from, "default") + " → " + to)}
	if region != "" && cli != "" {
		steps = append(steps, Step{
			Argv:    []string{cli, "configure", "set", "region", region, "--profile", to},
			Timeout: time.Minute,
			Env:     []string{"AWS_PAGER="},
		})
		changes = append(changes, locales.CloudAwsRegionChange(to, region))
	}
	if env.Get("AWS_ACCESS_KEY_ID") != "" {
		notes = append(notes, Note{
			Title: locales.CloudAwsEnvCredsNoteTitle(),
			Text:  locales.CloudAwsEnvCredsNoteText(),
			Tag:   "AWS_ACCESS_KEY_ID",
		})
	}
	p := Plan{
		Title:           locales.CloudAwsSwitchTitle(),
		Why:             locales.CloudAwsSwitchWhy(),
		Env:             append([]string{}, env...),
		Steps:           steps,
		Notes:           notes,
		Changes:         changes,
		RestartTerminal: true,
	}
	if region != "" && cli != "" && r != nil {
		verifyEnv := withEnvVar(env, "AWS_PAGER", "")
		p.Verify = func(ctx context.Context) (string, error) {
			res, err := r.Run(ctx, run.Cmd{
				Args: []string{cli, "configure", "get", "region", "--profile", to},
				Env:  verifyEnv,
			})
			if err != nil {
				return "", fmt.Errorf("check aws region: %w", err)
			}
			if res.ExitCode != 0 || strings.TrimSpace(string(res.Stdout)) != region {
				return "", errors.New(locales.CloudAwsRegionMismatch(to, region))
			}
			return locales.CloudAwsRegionSet(to, region), nil
		}
	}
	return p, nil
}

// GcloudSwitchPlan locally switches the active gcloud configuration and/or
// default project using gcloud's own non-interactive config commands.
func GcloudSwitchPlan(cli, fromConfig, toConfig, fromProject, toProject string, env sysenv.Env, r run.Runner) (Plan, error) {
	toConfig = strings.TrimSpace(toConfig)
	toProject = strings.TrimSpace(toProject)
	changeConfig := toConfig != "" && toConfig != fromConfig
	changeProject := toProject != "" && toProject != fromProject
	if cli == "" || (!changeConfig && !changeProject) {
		return Plan{}, errors.New(locales.CloudGcloudInvalid())
	}
	if (changeConfig && !validAWSValue(toConfig)) || (changeProject && !validAWSValue(toProject)) {
		return Plan{}, errors.New(locales.CloudGcloudInvalid())
	}

	var steps []Step
	var changes []string
	if changeConfig {
		steps = append(steps, Step{
			Argv:    []string{cli, "config", "configurations", "activate", toConfig},
			Timeout: time.Minute,
		})
		changes = append(changes, locales.CloudGcloudConfigChange(cmp.Or(fromConfig, "—")+" → "+toConfig))
	}
	if changeProject {
		steps = append(steps, Step{
			Argv:    []string{cli, "config", "set", "project", toProject},
			Timeout: time.Minute,
		})
		changes = append(changes, locales.CloudGcloudProjectChange(cmp.Or(fromProject, "—")+" → "+toProject))
	}

	var notes []Note
	if env.Get("CLOUDSDK_ACTIVE_CONFIG_NAME") != "" {
		notes = append(notes, Note{
			Title: locales.CloudGcloudEnvOverrideTitle(),
			Text:  locales.CloudGcloudEnvOverrideText("CLOUDSDK_ACTIVE_CONFIG_NAME"),
			Tag:   "CLOUDSDK_ACTIVE_CONFIG_NAME",
		})
	}
	if env.Get("CLOUDSDK_CORE_PROJECT") != "" {
		notes = append(notes, Note{
			Title: locales.CloudGcloudEnvOverrideTitle(),
			Text:  locales.CloudGcloudEnvOverrideText("CLOUDSDK_CORE_PROJECT"),
			Tag:   "CLOUDSDK_CORE_PROJECT",
		})
	}

	runEnv := withEnvVar(env, "CLOUDSDK_CORE_DISABLE_PROMPTS", "1")
	p := Plan{
		Title:   locales.CloudGcloudSwitchTitle(),
		Why:     locales.CloudGcloudSwitchWhy(),
		Env:     append([]string{}, runEnv...),
		Steps:   steps,
		Notes:   notes,
		Changes: changes,
	}
	if r != nil {
		verifyEnv := withoutEnvVars(runEnv, "CLOUDSDK_ACTIVE_CONFIG_NAME", "CLOUDSDK_CORE_PROJECT")
		p.Verify = func(ctx context.Context) (string, error) {
			if changeConfig {
				res, err := r.Run(ctx, run.Cmd{
					Args: []string{cli, "config", "configurations", "list", "--filter=is_active:true", "--format=value(name)"},
					Env:  verifyEnv,
				})
				if err != nil {
					return "", fmt.Errorf("check gcloud configuration: %w", err)
				}
				if res.ExitCode != 0 || strings.TrimSpace(string(res.Stdout)) != toConfig {
					return "", errors.New(locales.CloudGcloudMismatch())
				}
			}
			if changeProject {
				res, err := r.Run(ctx, run.Cmd{
					Args: []string{cli, "config", "get-value", "core/project"},
					Env:  verifyEnv,
				})
				if err != nil {
					return "", fmt.Errorf("check gcloud project: %w", err)
				}
				if res.ExitCode != 0 || strings.TrimSpace(string(res.Stdout)) != toProject {
					return "", errors.New(locales.CloudGcloudMismatch())
				}
			}
			target := strings.Join(slices.DeleteFunc([]string{toConfig, toProject}, func(s string) bool { return s == "" }), " · ")
			return locales.CloudGcloudSet(target), nil
		}
	}
	return p, nil
}

// AzSubscriptionPlan locally switches the active Azure CLI subscription using
// `az account set --subscription`.
func AzSubscriptionPlan(cli, from, toID, toName string, env sysenv.Env, r run.Runner) (Plan, error) {
	toID = strings.TrimSpace(toID)
	toName = strings.TrimSpace(toName)
	target := cmp.Or(toID, toName)
	label := cmp.Or(toName, toID)
	if cli == "" || !validCloudValue(target) {
		return Plan{}, errors.New(locales.CloudAzInvalid())
	}
	runEnv := withEnvVar(env, "AZURE_CORE_COLLECT_TELEMETRY", "no")
	p := Plan{
		Title:   locales.CloudAzSwitchTitle(),
		Why:     locales.CloudAzSwitchWhy(),
		Env:     append([]string{}, runEnv...),
		Steps:   []Step{{Argv: []string{cli, "account", "set", "--subscription", target}, Timeout: time.Minute}},
		Changes: []string{locales.CloudAzSubChange(cmp.Or(from, "—") + " → " + label)},
	}
	if r != nil {
		query := "name"
		want := toName
		if toID != "" {
			query = "id"
			want = toID
		}
		p.Verify = func(ctx context.Context) (string, error) {
			res, err := r.Run(ctx, run.Cmd{
				Args: []string{cli, "account", "show", "--query", query, "--output", "tsv"},
				Env:  runEnv,
			})
			if err != nil {
				return "", fmt.Errorf("check az subscription: %w", err)
			}
			if res.ExitCode != 0 || strings.TrimSpace(string(res.Stdout)) != want {
				return "", errors.New(locales.CloudAzMismatch(label))
			}
			return locales.CloudAzSet(label), nil
		}
	}
	return p, nil
}

// CloudLoginPlan builds the native browser sign-in plan for a cloud CLI.
// Hearth never reads or stores passwords or tokens; the CLI handles the
// browser flow and writes tokens to its own config directory.
func CloudLoginPlan(tool, cli, profile string, hasSSO bool, env sysenv.Env, r run.Runner) (Plan, error) {
	if cli == "" {
		return Plan{}, errors.New(locales.CloudAwsInvalid())
	}
	var (
		provider string
		argv     []string
		runEnv   = append(sysenv.Env(nil), env...)
	)
	switch tool {
	case "gcloud":
		provider = "Google Cloud"
		argv = []string{cli, "auth", "login"}
		runEnv = withEnvVar(runEnv, "CLOUDSDK_CORE_DISABLE_PROMPTS", "1")
	case "az":
		provider = "Azure"
		argv = []string{cli, "login", "--output", "none"}
		runEnv = withEnvVar(runEnv, "AZURE_CORE_COLLECT_TELEMETRY", "no")
	case "aws":
		provider = "AWS"
		if !hasSSO {
			return Plan{}, errors.New(locales.CloudLoginAwsNoSso(cmp.Or(profile, "default")))
		}
		argv = []string{cli, "sso", "login"}
		if profile != "" && profile != "default" {
			if !validAWSValue(profile) {
				return Plan{}, errors.New(locales.CloudAwsInvalid())
			}
			argv = append(argv, "--profile", profile)
		}
		runEnv = withEnvVar(runEnv, "AWS_PAGER", "")
	default:
		return Plan{}, errors.New(locales.CloudAwsInvalid())
	}

	p := Plan{
		Title:  locales.CloudLoginTitle(provider),
		Why:    locales.CloudLoginBanner(),
		Action: locales.CloudLoginOpenBrowser(),
		Env:    append([]string{}, runEnv...),
		Steps:  []Step{{Argv: argv, Timeout: 5 * time.Minute}},
	}
	if r != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			auth := detect.CheckCloudAuth(ctx, r, tool, cli, env)
			if auth.State != detect.CloudAuthVerified {
				return "", errors.New(locales.CloudLoginVerifyFailed())
			}
			return locales.CloudLoginVerified(cmp.Or(auth.Identity, provider)), nil
		}
	}
	return p, nil
}

// InfraVersionPlan installs a Terraform or OpenTofu version matching want
// via mise (installing mise first if needed). It never touches project files
// or runs init, plan, or apply.
func InfraVersionPlan(tool, title, want string, setup Setup, env sysenv.Env, detectFn Detect) (Plan, error) {
	pkg := tool
	if tool == "tofu" {
		pkg = "opentofu"
	}
	spec := MiseSpecForConstraint(want)
	verify := CommandFound(title, tool, detectFn)

	if setup.Have["mise"] {
		return Plan{
			Title:           locales.CloudInfraInstallTitle(title, spec),
			Why:             locales.CloudInfraInstallWhy(title, spec),
			Action:          locales.Install(),
			Env:             append([]string{}, env...),
			Steps:           []Step{{Argv: []string{"mise", "use", "--global", pkg + "@" + spec}}},
			Changes:         []string{"+ " + title + " " + spec + " · mise"},
			Verify:          verify,
			RestartTerminal: true,
		}, nil
	}

	s := setup
	s.Tools = []string{tool}
	s.Have = maps.Clone(setup.Have)
	if s.Have == nil {
		s.Have = map[string]bool{}
	}
	// Ensure SetupPlan includes the tool even if another manager already
	// installed an incompatible copy.
	s.Have[tool] = false
	p, _, err := SetupPlan(s, env)
	if err != nil {
		return Plan{}, err
	}
	wantArg := pkg + "@" + spec
	defaultArg := pkg + "@latest"
	for i := range p.Steps {
		for j, a := range p.Steps[i].Argv {
			if a == defaultArg {
				p.Steps[i].Argv[j] = wantArg
			}
		}
	}
	p.Title = locales.CloudInfraInstallTitle(title, spec)
	p.Why = locales.CloudInfraInstallWhy(title, spec)
	p.Action = locales.Install()
	p.Verify = verify
	return p, nil
}

var semverTokenRe = regexp.MustCompile(`\d+(?:\.\d+){0,2}`)

// MiseSpecForConstraint turns a Terraform/OpenTofu constraint (such as
// "~> 1.10", "~> 1.10.2", ">= 1.8", "= 1.9.8", "1.10.5", or "") into a
// version selector suitable for `mise use --global <pkg>@<spec>`.
func MiseSpecForConstraint(want string) string {
	want = strings.TrimSpace(want)
	switch strings.ToLower(want) {
	case "", "*", "latest":
		return "latest"
	}
	clauses := strings.Split(want, ",")
	first := strings.TrimSpace(clauses[0])
	switch {
	case strings.HasPrefix(first, "~>"):
		v := semverTokenRe.FindString(first)
		if v == "" {
			return "latest"
		}
		parts := strings.Split(v, ".")
		if len(parts) >= 2 {
			// For ~> 1.10 or ~> 1.10.2, selecting minor 1.10 picks the
			// newest 1.10.x patch, which satisfies both.
			return parts[0] + "." + parts[1]
		}
		return parts[0]
	case strings.HasPrefix(first, "=") && !strings.HasPrefix(first, "=="):
		if v := semverTokenRe.FindString(first); v != "" {
			return v
		}
	case strings.HasPrefix(first, ">="), strings.HasPrefix(first, "^"):
		if len(clauses) == 1 {
			return "latest"
		}
		// Upper bound present (e.g. ">= 1.8, < 1.10"): use the lower bound's
		// minor line so we stay within the range.
		if v := semverTokenRe.FindString(first); v != "" {
			parts := strings.Split(v, ".")
			if len(parts) >= 2 {
				return parts[0] + "." + parts[1]
			}
			return v
		}
	default:
		if v := semverTokenRe.FindString(first); v != "" {
			return v
		}
	}
	return "latest"
}

func withEnvVar(env sysenv.Env, key, val string) sysenv.Env {
	prefix := key + "="
	out := make(sysenv.Env, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			if !replaced {
				out = append(out, prefix+val)
				replaced = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, prefix+val)
	}
	return out
}

func withoutEnvVars(env sysenv.Env, keys ...string) sysenv.Env {
	out := make(sysenv.Env, 0, len(env))
	for _, kv := range env {
		drop := false
		for _, k := range keys {
			if strings.HasPrefix(kv, k+"=") {
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
