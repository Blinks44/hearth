package detect

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
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
		if cc := kubeContextFrom(kc); cc != nil {
			cli.Contexts = append(cli.Contexts, *cc)
		}
		for _, c := range []func(sysenv.Env, string) *inventory.CliContext{awsProfile, gcloudProject, azSubscription} {
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

func kubeContextFrom(kc kubeConfig) *inventory.CliContext {
	return kc.current
}

func kubeContext(env sysenv.Env, home string) *inventory.CliContext {
	return kubeContextFrom(readKubeConfig(env, home))
}

var errInvalidINI = errors.New("invalid INI syntax")

// awsProfile is the AWS CLI's selected profile, region, sources, and local
// profiles from ~/.aws/config (or AWS_CONFIG_FILE). ~/.aws/credentials and
// secret env values are never read or retained.
func awsProfile(env sysenv.Env, home string) *inventory.CliContext {
	file := env.Get("AWS_CONFIG_FILE")
	configEnv := ""
	if file != "" {
		configEnv = "AWS_CONFIG_FILE"
	} else {
		file = filepath.Join(home, ".aws", "config")
	}
	envProfile := env.Get("AWS_PROFILE")
	envDefProfile := env.Get("AWS_DEFAULT_PROFILE")
	envRegion := env.Get("AWS_REGION")
	envDefRegion := env.Get("AWS_DEFAULT_REGION")
	hasEnvCreds := env.Get("AWS_ACCESS_KEY_ID") != "" || env.Get("AWS_SESSION_TOKEN") != ""

	sections, order, err := readINI(file)
	notExist := errors.Is(err, fs.ErrNotExist)
	hasAnyEnv := configEnv != "" || envProfile != "" || envDefProfile != "" || envRegion != "" || envDefRegion != "" || hasEnvCreds
	if notExist && !hasAnyEnv {
		return nil
	}

	var choices []inventory.CliChoice
	profileVals := map[string]map[string]string{}
	for _, sec := range order {
		name := ""
		switch {
		case sec == "default":
			name = "default"
		case strings.HasPrefix(sec, "profile "):
			name = strings.TrimSpace(strings.TrimPrefix(sec, "profile "))
		default:
			continue
		}
		if name == "" {
			continue
		}
		if _, exists := profileVals[name]; exists {
			continue
		}
		vals := sections[sec]
		profileVals[name] = vals
		choices = append(choices, inventory.CliChoice{
			Name:   name,
			Region: vals["region"],
			HasSSO: vals["sso_start_url"] != "" || vals["sso_session"] != "",
		})
	}
	sortCliChoices(choices)
	if err == nil && len(choices) == 0 && !hasAnyEnv {
		return nil
	}

	profile, nameSource := "default", "default"
	switch {
	case envProfile != "":
		profile, nameSource = envProfile, "AWS_PROFILE"
	case envDefProfile != "":
		profile, nameSource = envDefProfile, "AWS_DEFAULT_PROFILE"
	case profileVals["default"] != nil:
		nameSource = "file"
	}

	vals, hasProfile := profileVals[profile]

	region, regionSource := "", ""
	switch {
	case envRegion != "":
		region, regionSource = envRegion, "AWS_REGION"
	case envDefRegion != "":
		region, regionSource = envDefRegion, "AWS_DEFAULT_REGION"
	case vals["region"] != "":
		region, regionSource = vals["region"], "file"
	}

	cc := &inventory.CliContext{
		Tool:         "aws",
		Name:         profile,
		Region:       region,
		File:         file,
		NameSource:   nameSource,
		RegionSource: regionSource,
		ConfigEnv:    configEnv,
		HasSSO:       vals["sso_start_url"] != "" || vals["sso_session"] != "",
		Choices:      choices,
	}
	switch {
	case err != nil && !notExist:
		cc.Status, cc.Issue = "invalid", "config_invalid"
	case notExist && configEnv != "":
		cc.Status, cc.Issue = "missing", "config_missing"
	case hasEnvCreds:
		cc.Status, cc.Issue = "ambiguous", "env_creds"
	case envProfile != "" && envDefProfile != "" && envProfile != envDefProfile:
		cc.Status, cc.Issue = "ambiguous", "profile_conflict"
	case envRegion != "" && envDefRegion != "" && envRegion != envDefRegion:
		cc.Status, cc.Issue = "ambiguous", "region_conflict"
	case !hasProfile:
		cc.Status, cc.Issue = "missing", "profile_missing"
	}
	return cc
}

// gcloudProject is gcloud's active configuration, selected project, their
// sources, and local configurations from ~/.config/gcloud (or CLOUDSDK_CONFIG).
// Credentials and token databases are never read.
func gcloudProject(env sysenv.Env, home string) *inventory.CliContext {
	dir := env.Get("CLOUDSDK_CONFIG")
	configEnv := ""
	if dir != "" {
		configEnv = "CLOUDSDK_CONFIG"
	} else {
		dir = gcloudConfigDir(env, home)
	}
	envActive := env.Get("CLOUDSDK_ACTIVE_CONFIG_NAME")
	envProject := env.Get("CLOUDSDK_CORE_PROJECT")
	envRegion := env.Get("CLOUDSDK_COMPUTE_REGION")

	dirExists := exists(dir)
	hasAnyEnv := configEnv != "" || envActive != "" || envProject != "" || envRegion != ""
	if !dirExists && !hasAnyEnv {
		return nil
	}

	fileActive := ""
	if dirExists {
		if data, err := os.ReadFile(filepath.Join(dir, "active_config")); err == nil {
			fileActive = strings.TrimSpace(string(data))
		}
	}
	active, nameSource := "default", "default"
	switch {
	case envActive != "":
		active, nameSource = envActive, "CLOUDSDK_ACTIVE_CONFIG_NAME"
	case fileActive != "":
		active, nameSource = fileActive, "file"
	}

	var choices []inventory.CliChoice
	if dirExists {
		entries, _ := os.ReadDir(filepath.Join(dir, "configurations"))
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "config_") {
				continue
			}
			cname := strings.TrimPrefix(e.Name(), "config_")
			if cname == "" {
				continue
			}
			secs, _, err := readINI(filepath.Join(dir, "configurations", e.Name()))
			ch := inventory.CliChoice{Name: cname}
			if err == nil {
				ch.Project = secs["core"]["project"]
				ch.Region = secs["compute"]["region"]
			}
			choices = append(choices, ch)
		}
		sortCliChoices(choices)
	}

	file := filepath.Join(dir, "configurations", "config_"+active)
	if !dirExists {
		file = dir
	}
	secs, _, activeErr := readINI(file)
	core := secs["core"]
	compute := secs["compute"]

	project, projectSource := "", ""
	switch {
	case envProject != "":
		project, projectSource = envProject, "CLOUDSDK_CORE_PROJECT"
	case core["project"] != "":
		project, projectSource = core["project"], "file"
	}
	region, regionSource := "", ""
	switch {
	case envRegion != "":
		region, regionSource = envRegion, "CLOUDSDK_COMPUTE_REGION"
	case compute["region"] != "":
		region, regionSource = compute["region"], "file"
	}

	cc := &inventory.CliContext{
		Tool:          "gcloud",
		Name:          active,
		Project:       project,
		Region:        region,
		File:          file,
		NameSource:    nameSource,
		ProjectSource: projectSource,
		RegionSource:  regionSource,
		ConfigEnv:     configEnv,
		Choices:       choices,
	}
	switch {
	case !dirExists:
		cc.Status, cc.Issue = "missing", "config_missing"
	case activeErr != nil && !errors.Is(activeErr, fs.ErrNotExist):
		cc.Status, cc.Issue = "invalid", "config_invalid"
	case errors.Is(activeErr, fs.ErrNotExist):
		cc.Status, cc.Issue = "missing", "active_config_missing"
	case envProject != "" && core["project"] != "" && envProject != core["project"]:
		cc.Status, cc.Issue = "ambiguous", "project_override"
	}
	return cc
}

// azSubscription is Azure CLI's default subscription and local subscriptions
// from azureProfile.json (under ~/.azure or AZURE_CONFIG_DIR). Token caches
// are never read.
func azSubscription(env sysenv.Env, home string) *inventory.CliContext {
	dir := env.Get("AZURE_CONFIG_DIR")
	configEnv := ""
	if dir != "" {
		configEnv = "AZURE_CONFIG_DIR"
	} else {
		dir = filepath.Join(home, ".azure")
	}
	file := filepath.Join(dir, "azureProfile.json")
	data, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && configEnv == "" {
			return nil
		}
		cc := &inventory.CliContext{Tool: "az", File: file, ConfigEnv: configEnv}
		if errors.Is(err, fs.ErrNotExist) {
			cc.Status, cc.Issue = "missing", "config_missing"
		} else {
			cc.Status, cc.Issue = "invalid", "config_invalid"
		}
		return cc
	}
	var profile struct {
		Subscriptions []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			State     string `json:"state"`
			IsDefault bool   `json:"isDefault"`
		} `json:"subscriptions"`
	}
	cc := &inventory.CliContext{Tool: "az", File: file, ConfigEnv: configEnv}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(data), "\ufeff")), &profile); err != nil { // az writes a BOM
		cc.Status, cc.Issue = "invalid", "config_invalid"
		return cc
	}
	defaults := 0
	for _, s := range profile.Subscriptions {
		if s.Name == "" && s.ID == "" {
			continue
		}
		if s.State == "" || strings.EqualFold(s.State, "Enabled") {
			cc.Choices = append(cc.Choices, inventory.CliChoice{Name: s.Name, ID: s.ID})
		}
		if s.IsDefault {
			defaults++
			if defaults == 1 {
				cc.Name, cc.SubscriptionID, cc.NameSource = s.Name, s.ID, "file"
			}
		}
	}
	if defaults > 1 {
		cc.Status, cc.Issue = "ambiguous", "multiple_defaults"
	}
	return cc
}

// sortCliChoices puts "default" first and sorts the rest by name.
func sortCliChoices(choices []inventory.CliChoice) {
	slices.SortFunc(choices, func(a, b inventory.CliChoice) int {
		if a.Name == "default" && b.Name != "default" {
			return -1
		}
		if b.Name == "default" && a.Name != "default" {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// readINI parses an INI file into sections and key/value maps, preserving
// section order. Indented lines (AWS CLI sub-properties) stay inside their
// section. Unclosed section headers or binary NUL bytes report errInvalidINI.
func readINI(path string) (sections map[string]map[string]string, order []string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, nil, errInvalidINI
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	sections = map[string]map[string]string{}
	current := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, nil, errInvalidINI
			}
			name := strings.TrimSpace(line[1 : len(line)-1])
			if name == "" {
				return nil, nil, errInvalidINI
			}
			current = name
			if _, ok := sections[current]; !ok {
				sections[current] = map[string]string{}
				order = append(order, current)
			}
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, nil, errInvalidINI
		}
		if current == "" {
			return nil, nil, errInvalidINI
		}
		// Keep top-level keys only; indented AWS sub-properties (under s3 =)
		// do not overwrite top-level keys like region.
		if len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t') {
			continue
		}
		sections[current][strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	return sections, order, nil
}
