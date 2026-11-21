package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ErrNoEngine says no local Docker engine answered the scan: there is
// nothing to ask.
var ErrNoEngine = errors.New("no local Docker engine available")

// Docker asks the engine of the terminal's selected local context what its
// images, containers, volumes and build cache take, as the scan pinned it.
func Docker(ctx context.Context, r run.Runner, env sysenv.Env, snap *inventory.Snapshot) ([]inventory.DockerUsage, error) {
	st, ok := snap.Tool("docker")
	active := inventory.TerminalCopy(st.Installations)
	if !ok || st.Docker == nil || active < 0 {
		return nil, ErrNoEngine
	}
	i := slices.IndexFunc(st.Docker.Contexts, func(c inventory.DockerContext) bool {
		return c.Name == st.Docker.Selected && c.Local && c.Status == "available"
	})
	if i < 0 {
		return nil, ErrNoEngine
	}
	target := docker.Target{Docker: st.Installations[active].Path, Endpoint: st.Docker.Contexts[i].Endpoint}
	rows, err := docker.ReadUsage(ctx, r, env, target)
	if err != nil {
		return nil, err
	}
	// Docker counts an image a stopped container uses as reclaimable,
	// though image prune leaves it: images without containers free what
	// only they hold, and none nothing.
	for j, u := range rows {
		if u.Type != "Images" {
			continue
		}
		if u.Count <= u.Active {
			rows[j].Reclaimable = 0
			continue
		}
		cmd := target.Command(env, "system", "df", "--verbose", "--format", "{{json .Images}}")
		cmd.Timeout = time.Minute
		res, err := r.Run(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("docker system df --verbose: %w", err)
		}
		if res.ExitCode != 0 || res.Truncated {
			return nil, fmt.Errorf("docker system df --verbose: exit %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
		}
		if rows[j].Reclaimable, err = parseUnusedImages(res.Stdout); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

// parseUnusedImages adds up the layers only the images without
// containers hold; a layer two of them share is not counted.
func parseUnusedImages(data []byte) (int64, error) {
	var images []struct{ Containers, UniqueSize string }
	if err := json.Unmarshal(data, &images); err != nil {
		return 0, fmt.Errorf("docker system df --verbose: %w", err)
	}
	var n int64
	for _, img := range images {
		if img.Containers != "0" {
			continue
		}
		size, err := docker.ParseSize(img.UniqueSize)
		if err != nil {
			return 0, fmt.Errorf("docker system df --verbose: %w", err)
		}
		n += size
	}
	return n, nil
}
