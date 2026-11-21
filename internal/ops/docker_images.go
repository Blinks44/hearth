package ops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const (
	DockerImagePull   DockerAction = "image-pull"
	DockerImageRemove DockerAction = "image-remove"
	DockerImageUntag  DockerAction = "image-untag"
)

// Accept registry, repository, tag and digest syntax. Docker validates
// the exact reference, and the argument never goes through a shell.
var dockerImageRef = regexp.MustCompile(`^[a-zA-Z0-9\[][a-zA-Z0-9_.:/\[\]-]*(@[a-zA-Z0-9_+.-]+:[a-fA-F0-9]+)?$`)

func ValidDockerImageRef(ref string) bool {
	return dockerImageRef.MatchString(ref) && !strings.Contains(ref, "://")
}

func PlanDockerPull(target docker.Target, env sysenv.Env, ref string) (Plan, error) {
	if target.Docker == "" || target.Endpoint == "" || !ValidDockerImageRef(ref) {
		return Plan{}, errors.New(locales.DockerImageReferenceInvalid())
	}
	cmd := target.Command(env, "image", "pull", "--", ref)
	return Plan{
		Title: locales.DockerImagePullTitle(ref), Why: locales.DockerImagePullWhy(target.Context),
		Action: locales.DockerImageDownload(), DockerAction: DockerImagePull, Env: cmd.Env,
		Steps: []Step{{Argv: cmd.Args, Timeout: 2 * time.Hour}},
	}, nil
}

func PlanDockerImageUntag(ctx context.Context, r run.Runner, target docker.Target, env sysenv.Env, item docker.Image, tag string) (Plan, error) {
	if target.Docker == "" || target.Endpoint == "" || !docker.ImageID(item.ID) || !ValidDockerImageRef(tag) || len(item.RepoTags) <= 1 || !slices.Contains(item.RepoTags, tag) {
		return Plan{}, errors.New(locales.DockerImageChanged())
	}
	priorTags := slices.Clone(item.RepoTags)
	priorDigests := slices.Clone(item.RepoDigests)
	check := func(ctx context.Context) error {
		fresh, err := docker.InspectImages(ctx, r, env, target, []string{item.ID})
		if err != nil {
			return err
		}
		if len(fresh) != 1 || fresh[0].ID != item.ID ||
			!slices.Equal(fresh[0].RepoTags, priorTags) ||
			!slices.Equal(fresh[0].RepoDigests, priorDigests) ||
			len(fresh[0].RepoTags) <= 1 || !slices.Contains(fresh[0].RepoTags, tag) {
			return errors.New(locales.DockerImageChanged())
		}
		return nil
	}
	if err := check(ctx); err != nil {
		return Plan{}, err
	}
	cmd := target.Command(env, "image", "rm", "--no-prune", "--", tag)
	return Plan{
		Title:        locales.DockerImageUntagTitle(tag),
		Why:          locales.DockerImageUntagWhy(tag, target.Context),
		Action:       locales.DockerImageUntagAction(),
		DockerAction: DockerImageUntag,
		Env:          cmd.Env,
		Before:       check,
		Steps:        []Step{{Argv: cmd.Args, Note: tag, Timeout: time.Minute}},
		Verify: func(ctx context.Context) (string, error) {
			fresh, err := docker.InspectImages(ctx, r, env, target, []string{item.ID})
			if err != nil {
				return "", err
			}
			if len(fresh) == 1 && slices.Contains(fresh[0].RepoTags, tag) {
				return "", errors.New(locales.DockerImageUntagCheck())
			}
			return "", nil
		},
	}, nil
}

func PlanDockerImageRemoval(ctx context.Context, r run.Runner, target docker.Target, env sysenv.Env, items []docker.Image) (Plan, error) {
	if target.Docker == "" || target.Endpoint == "" || len(items) == 0 {
		return Plan{}, errors.New(locales.DockerImageChanged())
	}
	ids := make([]string, 0, len(items))
	reviewed := make(map[string]docker.Image, len(items))
	for _, item := range items {
		if _, dup := reviewed[item.ID]; dup || !docker.ImageID(item.ID) {
			return Plan{}, errors.New(locales.DockerImageChanged())
		}
		ids = append(ids, item.ID)
		item.RepoTags, item.RepoDigests = slices.Clone(item.RepoTags), slices.Clone(item.RepoDigests)
		reviewed[item.ID] = item
	}
	check := func(ctx context.Context) ([]Note, error) {
		fresh, err := docker.InspectImages(ctx, r, env, target, ids)
		if err != nil {
			return nil, err
		}
		if len(fresh) != len(ids) {
			return nil, errors.New(locales.DockerImageChanged())
		}
		users, err := docker.ImageUsers(ctx, r, env, target)
		if err != nil {
			return nil, err
		}
		var blocked []Note
		seen := make(map[string]bool, len(fresh))
		for _, item := range fresh {
			prior, ok := reviewed[item.ID]
			if !ok || seen[item.ID] || !slices.Equal(item.RepoTags, prior.RepoTags) || !slices.Equal(item.RepoDigests, prior.RepoDigests) {
				return nil, errors.New(locales.DockerImageChanged())
			}
			seen[item.ID] = true
			if names := users[item.ID]; len(names) > 0 {
				containers := make([]string, 0, len(names))
				for _, user := range names {
					containers = append(containers, user.Name)
				}
				blocked = append(blocked, Note{Title: item.Name(), Text: locales.DockerImageInUse(strings.Join(containers, ", "))})
			} else if len(item.RepoTags) > 1 {
				blocked = append(blocked, Note{Title: item.Name(), Text: locales.DockerImageMultipleTags(strings.Join(item.RepoTags, ", "))})
			}
		}
		return blocked, nil
	}
	blocked, err := check(ctx)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{
		Title: locales.DockerImageRemoveTitle(len(ids)), Why: locales.DockerImageRemoveWhy(target.Context),
		Action: locales.DockerContainerRemove(), DockerAction: DockerImageRemove,
	}
	if len(blocked) != 0 {
		p.Why, p.Notes = locales.DockerImageRemoveBlocked(), blocked
		return p, nil
	}
	p.Before = func(ctx context.Context) error {
		blocked, err := check(ctx)
		if err != nil {
			return err
		}
		if len(blocked) != 0 {
			return fmt.Errorf("%s: %s", blocked[0].Title, blocked[0].Text)
		}
		return nil
	}
	for _, id := range ids {
		cmd := target.Command(env, "image", "rm", "--no-prune", "--", id)
		p.Env = cmd.Env
		p.Steps = append(p.Steps, Step{Argv: cmd.Args, Note: reviewed[id].Name(), Timeout: time.Minute})
	}
	return p, nil
}
