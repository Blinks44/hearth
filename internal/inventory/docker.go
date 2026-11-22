package inventory

// DockerState describes the terminal's Docker CLI configuration. Remote
// contexts stay unchecked.
type DockerState struct {
	PluginDir          string
	ConfigDir          string                  `json:",omitempty"`
	CredentialIssues   []DockerCredentialIssue `json:",omitempty"`
	Registries         []DockerRegistry        `json:",omitempty"`
	BrokenPlugins      []string                `json:",omitempty"`
	DefaultSocketIssue string                  `json:",omitempty"`
	Health             DockerHealth            `json:",omitzero"`
	Contexts           []DockerContext
	Selected           string
	Override           string // DOCKER_CONTEXT or DOCKER_HOST, without its value
	Err                string
	Engines            []DockerEngine
	Plugins            []Package
	Colima             *ColimaState `json:",omitempty"`
	// Databases are the database containers of the selected engine, once
	// it answered; DatabaseErr reports that they could not all be listed.
	Databases   []Database
	DatabaseErr bool
	// PortContainers are the running containers of the selected engine that
	// publish at least one host TCP port, read from the same list as Databases.
	PortContainers      []DockerPortContainer `json:",omitempty"`
	PortContainersFresh bool                  `json:"-"`
	// KindClusters are the kind clusters whose nodes are the selected
	// engine's containers, read from the same list as Databases.
	KindClusters []KindCluster `json:",omitempty"`
	// Running and Containers are the selected engine's counts from docker info.
	Running    int `json:",omitempty"`
	Containers int `json:",omitempty"`
}

// DockerPortBinding is one published host IP and TCP port of a Docker container.
type DockerPortBinding struct {
	HostIP   string `json:",omitempty"`
	HostPort string
}

// DockerPortContainer is a running Docker container that publishes host TCP ports.
type DockerPortContainer struct {
	ID, Name, Image, Status string
	Ports                   []string
	Bindings                []DockerPortBinding `json:",omitempty"`
}

// KindCluster is a local kind cluster, as its node containers show it.
type KindCluster struct {
	Name string
	// Nodes is how many node containers it has; Running how many of them
	// run.
	Nodes, Running int
	// Version is Kubernetes' of the control plane's node image, as
	// "v1.31.2"; "" when the image does not say.
	Version string `json:",omitempty"`
}

// DockerContext is a configured endpoint. Remote addresses are kept only
// as their scheme, without credentials, query strings or host names.
type DockerContext struct {
	Name, Endpoint, Engine string
	Status                 string // "available", "unavailable", or "" when not checked
	Version                string
	Local                  bool
	Denied                 bool // the socket refused this user: not in its group
	// Joined reports that the user is in the socket's group already but
	// this session is not: signing in again gives access.
	Joined bool `json:",omitempty"`
}

// DockerEngine is an installed engine application or command, or a
// platform's installation option when Path is empty.
type DockerEngine struct {
	Name, Path string
}

// DockerRegistry describes a host mentioned in config, without testing login.
type DockerRegistry struct {
	Name          string
	MissingHelper bool `json:",omitempty"`
}

type DockerCredentialIssue struct {
	Registry, Key     string `json:",omitempty"`
	Name, Replacement string `json:",omitempty"`
}

type DockerHealth struct {
	Memory, ContainerMemory int64
	Emulated                []DockerEmulatedContainer `json:",omitempty"`
}

type DockerEmulatedContainer struct {
	ID, Name, ImageArch, EngineArch string
}
