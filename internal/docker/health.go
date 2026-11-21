package docker

import (
	"context"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ReadHealth samples the active containers of one local engine. Memory is
// container usage, a lower bound on engine memory use, and zero if the
// sample could not account for every running container.
func ReadHealth(ctx context.Context, r run.Runner, env sysenv.Env, target Target, engine Engine) inventory.DockerHealth {
	var health inventory.DockerHealth
	if engine.Running == 0 && engine.Paused == 0 {
		return health
	}
	items, err := ReadContainers(ctx, r, env, target)
	if err != nil {
		return health
	}
	var ids []string
	for _, item := range items {
		if (item.State.Status == "running" || item.State.Status == "paused") && ImageID(item.ImageID) && !slices.Contains(ids, item.ImageID) {
			ids = append(ids, item.ImageID)
		}
	}
	if images, err := InspectImages(ctx, r, env, target, ids); err == nil {
		byID := make(map[string]Image, len(images))
		for _, image := range images {
			byID[image.ID] = image
		}
		for _, item := range items {
			image := byID[item.ImageID]
			if item.State.Status == "running" && image.Emulated(engine.Arch) {
				health.Emulated = append(health.Emulated, inventory.DockerEmulatedContainer{
					ID: item.ID, Name: item.Name, ImageArch: image.Architecture, EngineArch: canonicalArch(engine.Arch),
				})
			}
		}
	}
	if engine.Memory > 0 && sampleContainers(ctx, r, env, target, items) == nil {
		var used int64
		var sampled int
		for _, item := range items {
			if item.State.Status != "running" && item.State.Status != "paused" {
				continue
			}
			size, err := ParseSize(strings.TrimSpace(item.Memory))
			if err == nil && size >= 0 {
				used += size
				sampled++
			}
		}
		if sampled == engine.Running+engine.Paused {
			health.Memory, health.ContainerMemory = engine.Memory, used
		}
	}
	return health
}
