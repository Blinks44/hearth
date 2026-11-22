package ui

import "github.com/Blinks44/hearth/locales"

const recommendedDockerEngine = "Docker Engine"

func dockerPlatform() string { return locales.DockerPlatformLinux() }

func dockerEngineIsService(engine string) bool {
	return engine == "Docker Engine" || engine == "Podman" || engine == "Kiri"
}
