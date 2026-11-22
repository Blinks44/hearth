package ui

import "github.com/Blinks44/hearth/locales"

const recommendedDockerEngine = ""

func dockerPlatform() string { return locales.DockerPlatformWindows() }

func dockerEngineIsService(engine string) bool {
	return engine == "Docker Engine" || engine == "Podman"
}
