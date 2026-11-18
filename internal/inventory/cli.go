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

// CliContext is where one tool points now.
type CliContext struct {
	// Tool is kubectl, aws, gcloud or az.
	Tool string
	// Name is the context, profile, project or subscription; empty when
	// none is chosen.
	Name   string `json:",omitempty"`
	Region string `json:",omitempty"`
	// File is the settings file it is read from.
	File string
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
