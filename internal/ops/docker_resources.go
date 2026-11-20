package ops

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const DockerResourceRemove DockerAction = "resource-remove"

func PlanDockerResourceRemoval(ctx context.Context, r run.Runner, target docker.Target, env sysenv.Env, kind docker.ResourceKind, item docker.Resource) (Plan, error) {
	changed := errors.New(locales.DockerResourceChanged())
	if target.Docker == "" || target.Endpoint == "" || !kind.ValidID(item.ID) {
		return Plan{}, changed
	}
	check := func(ctx context.Context) (string, error) {
		items, err := docker.InspectResources(ctx, r, env, target, kind, []string{item.ID})
		if err != nil {
			return "", err
		}
		if len(items) != 1 {
			return "", changed
		}
		fresh := items[0]
		if fresh.ID != item.ID || fresh.Name != item.Name || fresh.Driver != item.Driver ||
			fresh.Project != item.Project || !fresh.Created.Equal(item.Created) {
			return "", changed
		}
		if kind == docker.NetworkResource && fresh.SystemNetwork() {
			return locales.DockerNetworkProtected(), nil
		}
		if len(fresh.Users) > 0 {
			names := make([]string, 0, len(fresh.Users))
			for _, user := range fresh.Users {
				names = append(names, user.Name)
			}
			return locales.DockerResourceInUse(strings.Join(names, ", ")), nil
		}
		return "", nil
	}
	blocked, err := check(ctx)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{DockerAction: DockerResourceRemove, Title: locales.DockerNetworkRemoveTitle(item.Name),
		Why: locales.DockerNetworkRemoveWhy(target.Context), Action: locales.DockerContainerRemove()}
	if kind == docker.VolumeResource {
		p.Title, p.Why = locales.DockerVolumeRemoveTitle(item.Name), locales.DockerVolumeRemoveWhy(target.Context)
		if item.Project != "" {
			p.ConfirmName = item.Name
		}
	}
	if blocked != "" {
		p.Why, p.ConfirmName = blocked, ""
		return p, nil
	}
	if item.Project != "" {
		p.Notes = []Note{{Title: locales.DockerResourceProject(item.Project)}}
	}
	p.Before = func(ctx context.Context) error {
		blocked, err := check(ctx)
		if err == nil && blocked != "" {
			err = errors.New(blocked)
		}
		return err
	}
	cmd := target.Command(env, string(kind), "rm", "--", item.ID)
	p.Env = cmd.Env
	p.Steps = []Step{{Argv: cmd.Args, Note: item.Name, Timeout: time.Minute}}
	return p, nil
}
