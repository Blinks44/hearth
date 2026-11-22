package ui

import "github.com/Blinks44/hearth/locales"

const recommendedDockerEngine = "Colima"

func dockerPlatform() string { return locales.DockerPlatformMac() }

func dockerEngineIsService(engine string) bool {
	return engine == "Docker Engine" || engine == "Podman"
}
