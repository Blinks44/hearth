package inventory

// CliState is the command-line tools without a page of their own:
// Kubernetes', the clouds' and infrastructure's, and where they point now.
type CliState struct {
	// Tools are the copies found, each Tool the command it is run by:
	// kubectl, helm, terraform, tofu, aws, gcloud, az.
	Tools []Installation `json:",omitempty"`
	// HelmRepos is how many chart repositories helm has added.
	HelmRepos int `json:",omitempty"`
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
