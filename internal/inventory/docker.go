package inventory

// DockerState describes the terminal's Docker CLI configuration. Only the
// selected local endpoint is probed; remote contexts stay unchecked.
type DockerState struct {
	PluginDir string
	Contexts  []DockerContext
	Selected  string
	Override  string // DOCKER_CONTEXT or DOCKER_HOST, without its value
	Err       string
	Engines   []DockerEngine
	Plugins   []Package
	// Databases are the database containers of the selected engine, once
	// it answered; DatabaseErr reports that they could not all be listed.
	Databases   []Database
	DatabaseErr bool
}

// DockerContext is a configured endpoint. Remote addresses are kept only
// as their scheme, without credentials, query strings or host names.
type DockerContext struct {
	Name, Endpoint, Engine string
	Status                 string // "available", "unavailable", or "" when not checked
	Version                string
	Local                  bool
	Denied                 bool // the socket refused this user: not in its group
}

// DockerEngine is an installed engine application or command, or a
// platform's installation option when Path is empty.
type DockerEngine struct {
	Name, Path string
}
