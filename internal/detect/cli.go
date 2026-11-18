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
	"kind":      {"version"},
	"terraform": {"version"},
	"tofu":      {"version"},
	"aws":       {"--version"},
	"az":        {"version", "--output", "json"},
}

// cliCommands are the command-line tools the CLI page lists, in its order.
var cliCommands = []string{"kubectl", "helm", "kind", "aws", "gcloud", "az", "terraform", "tofu"}

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
		kc := readKubeConfig(term, home)
		cli.KubeContexts, cli.KubeFiles, cli.KubeconfigSet = kc.contexts, kc.files, strings.Trim(term.Get("KUBECONFIG"), string(os.PathListSeparator)) != ""
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
// and repeated entries are ignored, as kubectl does, and an empty
// KUBECONFIG uses the default config.
func KubeConfigFiles(env sysenv.Env, home string) []string {
	var files []string
	for _, p := range filepath.SplitList(env.Get("KUBECONFIG")) {
		if p != "" && !slices.Contains(files, p) {
			files = append(files, p)
		}
	}
	if len(files) == 0 {
		return []string{filepath.Join(home, ".kube", "config")}
	}
	return files
}

// kubeConfig is what Hearth reads of kubeconfig files: the files in merge
// order, their contexts by file and name, and the current context.
type kubeConfig struct {
	files    []inventory.KubeFile
	contexts []inventory.KubeContext
	current  *inventory.CliContext
}

// kubeContextUser is a context and the user it names, while the files are
// read.
type kubeContextUser struct {
	inventory.KubeContext
	user string
}

// readKubeConfig follows kubectl's first-definition-wins merge rule: a
// context of a name an earlier file has is kept, marked Shadowed, and a
// user's definition comes from the first file that has it. Typed decoding
// retains only context metadata and the command of a user's exec hook,
// never credentials, its arguments or its environment.
func readKubeConfig(env sysenv.Env, home string) kubeConfig {
	var kc kubeConfig
	seen := map[string]bool{}
	// users are the auth programs by user, the first definition's.
	users := map[string]string{}
	var contextUsers []string // by kc.contexts' index
	for _, file := range KubeConfigFiles(env, home) {
		var config struct {
			Current  string `yaml:"current-context"`
			Contexts []struct {
				Name    string `yaml:"name"`
				Context struct {
					Cluster   string `yaml:"cluster"`
					Namespace string `yaml:"namespace"`
					User      string `yaml:"user"`
				} `yaml:"context"`
			} `yaml:"contexts"`
			Users []struct {
				Name string `yaml:"name"`
				User struct {
					Exec struct {
						Command string `yaml:"command"`
					} `yaml:"exec"`
					AuthProvider struct {
						Name string `yaml:"name"`
					} `yaml:"auth-provider"`
				} `yaml:"user"`
			} `yaml:"users"`
		}
		kf := inventory.KubeFile{Path: file}
		data, err := os.ReadFile(file)
		switch {
		case err != nil:
			kf.Missing = true
		case yaml.Unmarshal(data, &config) != nil:
			kf.Invalid = true
		}
		kc.files = append(kc.files, kf)
		if kf.Missing || kf.Invalid {
			continue
		}
		if kc.current == nil && config.Current != "" {
			kc.current = &inventory.CliContext{Tool: "kubectl", Name: config.Current, File: file}
		}
		for _, u := range config.Users {
			if _, ok := users[u.Name]; !ok && u.Name != "" {
				users[u.Name] = or(u.User.Exec.Command, u.User.AuthProvider.Name)
			}
		}
		var contexts []kubeContextUser
		for _, cc := range config.Contexts {
			if cc.Name == "" {
				continue
			}
			contexts = append(contexts, kubeContextUser{inventory.KubeContext{Name: cc.Name, Cluster: cc.Context.Cluster, Namespace: cc.Context.Namespace, File: file, Shadowed: seen[cc.Name]}, cc.Context.User})
			seen[cc.Name] = true
		}
		slices.SortFunc(contexts, func(a, b kubeContextUser) int { return strings.Compare(a.Name, b.Name) })
		for _, cu := range contexts {
			kc.contexts = append(kc.contexts, cu.KubeContext)
			contextUsers = append(contextUsers, cu.user)
		}
	}
	for i, user := range contextUsers {
		kc.contexts[i].Auth = users[user]
	}
	return kc
}

func kubeContext(env sysenv.Env, home string) *inventory.CliContext {
	return readKubeConfig(env, home).current
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
