package docker

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func desktopDisks(env sysenv.Env, provider string) []string {
	if provider == "Docker Desktop" {
		return []string{filepath.Join(env.Get("HOME"), ".docker", "desktop", "vms", "0", "data", "Docker.raw")}
	}
	return nil
}
