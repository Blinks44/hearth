package docker

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type ImageUser struct {
	Name   string
	Active bool
}

type Image struct {
	ID, Architecture, OS  string
	RepoTags, RepoDigests []string
	Size                  int64
	Created               time.Time
	Users                 []ImageUser
}

func (i Image) Name() string {
	if len(i.RepoTags) != 0 {
		return i.RepoTags[0]
	}
	if len(i.RepoDigests) != 0 {
		return i.RepoDigests[0]
	}
	return i.ID
}

func (i Image) Dangling() bool {
	return len(i.RepoTags) == 0 && len(i.Users) == 0
}

func canonicalArch(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	case "armv7l", "armv6l", "armhf":
		return "arm"
	case "i386", "i686":
		return "386"
	default:
		return arch
	}
}

func (i Image) Emulated(engineArch string) bool {
	return i.Architecture != "" && i.Architecture != "unknown" && engineArch != "" &&
		canonicalArch(i.Architecture) != canonicalArch(engineArch)
}

func ImageID(id string) bool {
	return strings.HasPrefix(id, "sha256:") && ContainerID(strings.TrimPrefix(id, "sha256:"))
}

const inspectImage = `{"ID":{{json .Id}},"RepoTags":{{json (index . "RepoTags")}},"RepoDigests":{{json (index . "RepoDigests")}},"Architecture":{{json (index . "Architecture")}},"OS":{{json (index . "Os")}},"Size":{{json (index . "Size")}},"Created":{{with index . "Created"}}{{json .}}{{else}}null{{end}}}`

func InspectImages(ctx context.Context, r run.Runner, env sysenv.Env, t Target, ids []string) ([]Image, error) {
	var images []Image
	for batch := range slices.Chunk(ids, 64) {
		for _, id := range batch {
			if !ImageID(id) {
				return nil, fmt.Errorf("docker image inspect: invalid image ID %q", id)
			}
		}
		cmd := t.Command(env, "image", "inspect", "--format", inspectImage, "--")
		cmd.Args = append(cmd.Args, batch...)
		res, err := r.Run(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("docker image inspect: %w", err)
		}
		if res.Truncated {
			return nil, errors.New("docker image inspect: output too large")
		}
		if res.ExitCode != 0 && !isMissingDockerObject(string(res.Stderr), "image") {
			return nil, fmt.Errorf("docker image inspect: %s", strings.TrimSpace(string(res.Stderr)))
		}
		for line := range strings.Lines(string(res.Stdout)) {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var row Image
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				return nil, fmt.Errorf("docker image inspect: %w", err)
			}
			slices.Sort(row.RepoTags)
			slices.Sort(row.RepoDigests)
			images = append(images, row)
		}
	}
	return images, nil
}

// ImageUsers includes stopped containers and joins by immutable image ID,
// since a tag can point to a different image after a pull.
func ImageUsers(ctx context.Context, r run.Runner, env sysenv.Env, t Target) (map[string][]ImageUser, error) {
	items, err := ReadContainers(ctx, r, env, t)
	if err != nil {
		return nil, err
	}
	users := make(map[string][]ImageUser)
	for _, item := range items {
		users[item.ImageID] = append(users[item.ImageID], ImageUser{Name: item.Name, Active: item.Active()})
	}
	return users, nil
}

func ReadImages(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]Image, error) {
	var ids []string
	seen := make(map[string]bool)
	for _, dangling := range []string{"false", "true"} {
		rows, err := readRows[struct{ ID string }](ctx, r, t.Command(env, "image", "ls", "--no-trunc",
			"--filter", "dangling="+dangling, "--format", "{{json .}}"))
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if !seen[row.ID] {
				ids = append(ids, row.ID)
				seen[row.ID] = true
			}
		}
	}
	images, err := InspectImages(ctx, r, env, t, ids)
	if err != nil {
		return nil, err
	}
	users, err := ImageUsers(ctx, r, env, t)
	if err != nil {
		return nil, err
	}
	for i := range images {
		images[i].Users = users[images[i].ID]
	}
	slices.SortFunc(images, func(a, b Image) int { return cmp.Or(cmp.Compare(a.Name(), b.Name()), cmp.Compare(a.ID, b.ID)) })
	return images, nil
}

func WatchImages(ctx context.Context, r run.Runner, env sysenv.Env, t Target, got func([]Image, error)) {
	watchChanges(ctx, env, t, []string{"type=image", "type=container"}, func(ctx context.Context) {
		list, err := ReadImages(ctx, r, env, t)
		if ctx.Err() == nil {
			got(list, err)
		}
	})
}
