package docker

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func ConfigDir(env sysenv.Env) string {
	if dir := env.Get("DOCKER_CONFIG"); dir != "" {
		return dir
	}
	home := env.Get("HOME")
	if home == "" {
		home = env.Get("USERPROFILE")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".docker")
}

var registryHost = regexp.MustCompile(`^(?:[a-zA-Z0-9][a-zA-Z0-9.-]*|\[[a-fA-F0-9:]+\])(?::[0-9]{1,5})?$`)
var helperSuffix = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

const HubRegistry = "docker.io"

// RegistryName accepts login hosts and the legacy Docker Hub auth key.
// Ignore URL credentials, paths, and query strings in arbitrary auth keys.
func RegistryName(key string) string {
	switch strings.ToLower(key) {
	case "https://index.docker.io/v1/", "index.docker.io", HubRegistry:
		return HubRegistry
	}
	if registryHost.MatchString(key) {
		return strings.ToLower(key)
	}
	return ""
}

func CredentialHelperAvailable(env sysenv.Env, suffix string) bool {
	return helperSuffix.MatchString(suffix) && len(sysenv.LookPathAll("docker-credential-"+suffix, env)) != 0
}

func CredentialHelperName(suffix string) bool { return helperSuffix.MatchString(suffix) }

func CredentialReplacement(env sysenv.Env) string {
	for _, suffix := range nativeCredentialHelpers {
		if CredentialHelperAvailable(env, suffix) {
			return suffix
		}
	}
	return ""
}
