package inventory

// GitState is how Git is set up for the user: no secrets are kept, only
// the settings Hearth shows by name, helpers' names, the hosts GitHub CLI
// is signed in to, and SSH keys by name and type.
type GitState struct {
	// ConfigFile is the user's global config, there or not; Config are
	// the settings of it Hearth shows, in git config's order.
	ConfigFile string
	Config     []Setting `json:",omitempty"`
	// Helpers are credential.helper's values, as "osxkeychain".
	Helpers []string `json:",omitempty"`
	// GitHub is GitHub CLI's copy in the terminal; nil when none.
	GitHub *GitHubCLI `json:",omitempty"`
	// Keys are the SSH keys in ~/.ssh, by their public halves.
	Keys []SSHKey `json:",omitempty"`
	Err  string   `json:",omitempty"`
}

// Setting is a setting by name and value.
type Setting struct{ Name, Value string }

// GitHubCLI is gh as found, with the hosts it is signed in to.
type GitHubCLI struct {
	Path, Version string
	// Hosts are as "github.com", with the account, never the token.
	Hosts []GitHubHost `json:",omitempty"`
}

// GitHubHost is a host gh is signed in to.
type GitHubHost struct{ Host, User string }

// SSHKey is a key pair in ~/.ssh, as its public file tells it; the
// private one is never read.
type SSHKey struct {
	// Name is the private key's file name, as "id_ed25519".
	Name string
	// Type is as "ED25519" or "RSA"; Bits an RSA key's size.
	Type string
	Bits int `json:",omitempty"`
	// Hosts are the ones ~/.ssh/config gives it to.
	Hosts []string `json:",omitempty"`
	// InAgent reports that ssh-agent holds it.
	InAgent bool `json:",omitempty"`
	// PublicKey is the trimmed contents of the .pub file.
	PublicKey string `json:",omitempty"`
}
