package inventory

// CliState is the command-line tools without a page of their own:
// Kubernetes', the clouds' and infrastructure's, and where they point now.
type CliState struct {
	// Tools are the copies found, each Tool the command it is run by:
	// kubectl, helm, kind, terraform, tofu, aws, gcloud, az.
	Tools []Installation `json:",omitempty"`
	// HelmRepos is how many chart repositories helm has added.
	HelmRepos int `json:",omitempty"`
	// KubeContexts are the named contexts of every kubeconfig file, in
	// the files' order, without credentials or servers.
	KubeContexts []KubeContext `json:",omitempty"`
	// KubeFiles are the kubeconfig files kubectl reads, in merge order.
	KubeFiles []KubeFile `json:",omitempty"`
	// KubeconfigSet reports that KUBECONFIG names the files.
	KubeconfigSet bool `json:",omitempty"`
	// Contexts are the tools' current cluster, profile, project or
	// subscription, read from their settings; keys are never read.
	Contexts []CliContext `json:",omitempty"`
}

// CliChoice is one local profile, configuration, or subscription read
// from a cloud CLI's config files; credentials and tokens are never read.
type CliChoice struct {
	Name    string `json:",omitempty"`
	ID      string `json:",omitempty"`
	Project string `json:",omitempty"`
	Region  string `json:",omitempty"`
	HasSSO  bool   `json:",omitempty"`
}

// CliContext is where one tool points now.
type CliContext struct {
	// Tool is kubectl, aws, gcloud or az.
	Tool string
	// Name is the context (kubectl), profile (aws), configuration name
	// (gcloud) or subscription name (az); empty when none is chosen.
	Name   string `json:",omitempty"`
	Region string `json:",omitempty"`
	// Project is the selected GCP project for gcloud.
	Project string `json:",omitempty"`
	// SubscriptionID is the selected Azure subscription ID for az.
	SubscriptionID string `json:",omitempty"`
	// File is the settings file or directory it is read from.
	File string
	// NameSource says where Name came from: "file", "default", or an env
	// variable name such as "AWS_PROFILE" or "CLOUDSDK_ACTIVE_CONFIG_NAME".
	NameSource string `json:",omitempty"`
	// RegionSource says where Region came from: "file", "AWS_REGION",
	// "AWS_DEFAULT_REGION", or "CLOUDSDK_COMPUTE_REGION".
	RegionSource string `json:",omitempty"`
	// ProjectSource says where Project came from: "file" or
	// "CLOUDSDK_CORE_PROJECT".
	ProjectSource string `json:",omitempty"`
	// ConfigEnv is the env variable that changed the config path, if any:
	// "AWS_CONFIG_FILE", "CLOUDSDK_CONFIG", or "AZURE_CONFIG_DIR".
	ConfigEnv string `json:",omitempty"`
	// Status is "", "missing", "invalid", or "ambiguous".
	Status string `json:",omitempty"`
	// Issue identifies the specific missing, invalid, or ambiguous state:
	// "config_missing", "profile_missing", "config_invalid", "env_creds",
	// "profile_conflict", "region_conflict", "active_config_missing",
	// "project_override", or "multiple_defaults".
	Issue string `json:",omitempty"`
	// HasSSO reports that the selected AWS profile configures SSO.
	HasSSO bool `json:",omitempty"`
	// Choices are the local profiles, configurations, or subscriptions
	// found in the settings files, for switching.
	Choices []CliChoice `json:",omitempty"`
}

// KubeContext names a local kubectl context and the cluster and namespace it selects.
// File is the source of this context in the merged kubeconfig.
type KubeContext struct {
	Name, Cluster, Namespace, File string
	// Shadowed reports that an earlier file has a context of this name,
	// which kubectl uses instead.
	Shadowed bool `json:",omitempty"`
	// Auth is the program kubectl runs to sign in to the cluster, the
	// command of the user's exec hook or its auth provider's name; ""
	// for a user without one.
	Auth string `json:",omitempty"`
}

// KubeFile is a kubeconfig file kubectl reads.
type KubeFile struct {
	Path string
	// Missing: there is no file; Invalid: it is not a kubeconfig kubectl
	// can read.
	Missing bool `json:",omitempty"`
	Invalid bool `json:",omitempty"`
}
