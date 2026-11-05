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
}

// DockerContext is a configured endpoint. Remote addresses are kept only
// as their scheme, without credentials, query strings or host names.
type DockerContext struct {
	Name, Endpoint, Engine string
	Status                 string // "available", "unavailable", or "" when not checked
	Version                string
	Local                  bool
}

// DockerEngine is an installed engine application or command, or a
// platform's installation option when Path is empty.
type DockerEngine struct {
	Name, Path string
}
