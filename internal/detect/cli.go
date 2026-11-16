package detect

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// cliVersionArgs ask each command its version without going online:
// kubectl's --client keeps it off the cluster, CHECKPOINT_DISABLE
// terraform off its update check. gcloud's version is read from its SDK.
var cliVersionArgs = map[string][]string{
	"kubectl":   {"version", "--client"},
	"helm":      {"version", "--short"},
	"terraform": {"version"},
	"tofu":      {"version"},
	"aws":       {"--version"},
	"az":        {"version", "--output", "json"},
}

// cliCommands are the command-line tools the CLI page lists, in its order.
var cliCommands = []string{"kubectl", "helm", "aws", "gcloud", "az", "terraform", "tofu"}

// Cli finds the command-line tools of Kubernetes, the clouds and
// infrastructure, and the context each points to now, from their settings
// files: credentials and keys are never read.
func Cli(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	cli := &inventory.CliState{}
	env := append(slices.Clip(term), "CHECKPOINT_DISABLE=1", "AZURE_CORE_COLLECT_TELEMETRY=no")
	for _, cmd := range cliCommands {
		f := newFinder(cmd, term)
		f.lookPath(cmd, term, gui)
		for i := range f.ins {
			in := &f.ins[i]
			if cmd == "gcloud" {
				// <sdk>/bin/gcloud, or the snap's folder; running it takes
				// seconds of Python.
				sdk := filepath.Dir(filepath.Dir(in.RealPath))
				if in.Source.Manager == "snap" {
					sdk = in.Source.Root
				}
				data, _ := os.ReadFile(filepath.Join(sdk, "VERSION"))
				in.Version = strings.TrimSpace(string(data))
				continue
			}
			// Client Version: v1.31.4; v3.16.3+gcfd0749; Terraform v1.9.8;
			// aws-cli/2.22.1 Python/3.12.6; {"azure-cli": "2.67.0", ...}
			if res, err := r.Run(ctx, run.Cmd{Args: append([]string{in.Path}, cliVersionArgs[cmd]...), Env: env}); err == nil && res.ExitCode == 0 {
				in.Version = gitVersion.FindString(string(res.Stdout))
			}
		}
		cli.Tools = append(cli.Tools, f.ins...)
	}
	home := sources.ManagerDirs(term).Home
	if home != "" {
		cli.HelmRepos = helmRepos(term, home)
		cli.KubeContexts, _ = kubeContexts(term, home)
		for _, c := range []func(sysenv.Env, string) *inventory.CliContext{kubeContext, awsProfile, gcloudProject, azSubscription} {
			if cc := c(term, home); cc != nil {
				cli.Contexts = append(cli.Contexts, *cc)
			}
		}
	}
	return inventory.ToolState{Tool: "cli", Cli: cli}, ctx.Err()
}

// helmRepos counts the chart repositories in helm's repositories.yaml.
func helmRepos(env sysenv.Env, home string) int {
	path := env.Get("HELM_REPOSITORY_CONFIG")
	if path == "" {
		path = filepath.Join(or(env.Get("HELM_CONFIG_HOME"), helmConfigDir(env, home)), "repositories.yaml")
	}
	var repos struct {
		Repositories []struct{} `yaml:"repositories"`
	}
	data, _ := os.ReadFile(path) // none added
	_ = yaml.Unmarshal(data, &repos)
	return len(repos.Repositories)
}

// KubeConfigFiles returns kubectl's configured files in merge order. Empty
// entries are ignored and an empty KUBECONFIG uses the default config.
func KubeConfigFiles(env sysenv.Env, home string) []string {
	files := slices.DeleteFunc(filepath.SplitList(env.Get("KUBECONFIG")), func(p string) bool { return p == "" })
	if len(files) == 0 {
		return []string{filepath.Join(home, ".kube", "config")}
	}
	return files
}

// kubeContexts follows kubectl's first-definition-wins merge rule. Typed
// decoding retains only context metadata, never user credentials or exec hooks.
func kubeContexts(env sysenv.Env, home string) ([]inventory.KubeContext, *inventory.CliContext) {
	var contexts []inventory.KubeContext
	var current *inventory.CliContext
	seen := map[string]bool{}
	for _, file := range KubeConfigFiles(env, home) {
		var config struct {
			Current  string `yaml:"current-context"`
			Contexts []struct {
				Name    string `yaml:"name"`
				Context struct {
					Cluster   string `yaml:"cluster"`
					Namespace string `yaml:"namespace"`
				} `yaml:"context"`
			} `yaml:"contexts"`
		}
		data, err := os.ReadFile(file)
		if err != nil || yaml.Unmarshal(data, &config) != nil {
			continue
		}
		if current == nil && config.Current != "" {
			current = &inventory.CliContext{Tool: "kubectl", Name: config.Current, File: file}
		}
		for _, cc := range config.Contexts {
			if cc.Name == "" || seen[cc.Name] {
				continue
			}
			seen[cc.Name] = true
			contexts = append(contexts, inventory.KubeContext{Name: cc.Name, Cluster: cc.Context.Cluster, Namespace: cc.Context.Namespace, File: file})
		}
	}
	slices.SortFunc(contexts, func(a, b inventory.KubeContext) int { return strings.Compare(a.Name, b.Name) })
	return contexts, current
}

func kubeContext(env sysenv.Env, home string) *inventory.CliContext {
	_, current := kubeContexts(env, home)
	return current
}

// awsProfile is the AWS CLI's profile, AWS_PROFILE or default, and its
// region, from ~/.aws/config; ~/.aws/credentials is never read.
func awsProfile(env sysenv.Env, home string) *inventory.CliContext {
	file := or(env.Get("AWS_CONFIG_FILE"), filepath.Join(home, ".aws", "config"))
	profile := or(env.Get("AWS_PROFILE"), or(env.Get("AWS_DEFAULT_PROFILE"), "default"))
	section := "profile " + profile
	if profile == "default" {
		section = "default"
	}
	values, ok := iniSection(file, section)
	if !ok && env.Get("AWS_PROFILE") == "" {
		return nil
	}
	region := or(env.Get("AWS_REGION"), or(env.Get("AWS_DEFAULT_REGION"), values["region"]))
	return &inventory.CliContext{Tool: "aws", Name: profile, Region: region, File: file}
}

// gcloudProject is the project of gcloud's active configuration.
func gcloudProject(env sysenv.Env, home string) *inventory.CliContext {
	dir := or(env.Get("CLOUDSDK_CONFIG"), gcloudConfigDir(env, home))
	if !exists(dir) {
		return nil
	}
	active := env.Get("CLOUDSDK_ACTIVE_CONFIG_NAME")
	if active == "" {
		data, _ := os.ReadFile(filepath.Join(dir, "active_config"))
		active = or(strings.TrimSpace(string(data)), "default")
	}
	file := filepath.Join(dir, "configurations", "config_"+active)
	core, _ := iniSection(file, "core")
	return &inventory.CliContext{Tool: "gcloud", Name: or(env.Get("CLOUDSDK_CORE_PROJECT"), core["project"]), File: file}
}

// azSubscription is Azure CLI's default subscription by name, from
// azureProfile.json; its tokens are in other files.
func azSubscription(env sysenv.Env, home string) *inventory.CliContext {
	file := filepath.Join(or(env.Get("AZURE_CONFIG_DIR"), filepath.Join(home, ".azure")), "azureProfile.json")
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var profile struct {
		Subscriptions []struct {
			Name      string `json:"name"`
			IsDefault bool   `json:"isDefault"`
		} `json:"subscriptions"`
	}
	_ = json.Unmarshal([]byte(strings.TrimPrefix(string(data), "\ufeff")), &profile) // az writes a BOM
	cc := &inventory.CliContext{Tool: "az", File: file}
	for _, s := range profile.Subscriptions {
		if s.IsDefault {
			cc.Name = s.Name
		}
	}
	return cc
}

// iniSection reads the keys of one [section] of an INI file, as AWS's and
// gcloud's settings are; ok reports that the section is there.
func iniSection(path, section string) (values map[string]string, ok bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close() //nolint:errcheck // read only
	values = map[string]string{}
	in := false
	s := bufio.NewScanner(file)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if name, isSection := strings.CutPrefix(line, "["); isSection {
			in = strings.TrimSpace(strings.TrimSuffix(name, "]")) == section
			ok = ok || in
			continue
		}
		if key, value, found := strings.Cut(line, "="); in && found {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return values, ok
}
