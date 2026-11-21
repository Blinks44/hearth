package docker

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func desktopDisks(env sysenv.Env, provider string) []string {
	home := env.Get("HOME")
	switch provider {
	case "OrbStack":
		return []string{filepath.Join(home, "Library", "Group Containers", "HUAQ24HBR6.dev.orbstack", "data", "data.img")}
	case "Docker Desktop":
		data := filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data")
		return []string{filepath.Join(data, "Docker.raw"), filepath.Join(data, "Docker.qcow2")}
	}
	return nil
}
