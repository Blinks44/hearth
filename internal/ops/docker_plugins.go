package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// DockerRemoveBrokenPluginPlan builds a confirmed plan that removes a single
// dangling plugin symlink inside Docker's cli-plugins directory. Both plan
// creation and Before verify that the target is still a broken symlink inside
// the active cli-plugins directory so real files and valid links are never
// touched.
func DockerRemoveBrokenPluginPlan(env sysenv.Env, pluginDir, name string, detect Detect) (Plan, error) {
	target, ok := brokenPluginTarget(env, pluginDir, name)
	if !ok {
		return Plan{}, errors.New(locales.DockerBrokenPluginChanged())
	}
	p := Plan{
		Title:  locales.DockerBrokenPluginRemoveTitle(name),
		Why:    locales.DockerBrokenPluginRemoveWhy(),
		Action: locales.DockerBrokenPluginRemoveAction(),
		Before: func(context.Context) error {
			if _, ok := brokenPluginTarget(env, pluginDir, name); !ok {
				return errors.New(locales.DockerBrokenPluginChanged())
			}
			return nil
		},
		Steps:   []Step{{Remove: target}},
		Changes: []string{"− " + target},
		Verify: func(ctx context.Context) (string, error) {
			if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
				return "", errors.New(locales.DockerBrokenPluginStillThere(name))
			}
			if detect != nil {
				st, err := detect(ctx)
				if err != nil {
					return "", err
				}
				if st.Docker != nil && slices.Contains(st.Docker.BrokenPlugins, name) {
					return "", errors.New(locales.DockerBrokenPluginStillThere(name))
				}
			}
			return locales.DockerBrokenPluginRemoved(name), nil
		},
	}
	return p, nil
}

func brokenPluginTarget(env sysenv.Env, pluginDir, name string) (string, bool) {
	configDir := docker.ConfigDir(env)
	if configDir == "" || !filepath.IsAbs(pluginDir) {
		return "", false
	}
	cleanDir := filepath.Clean(pluginDir)
	wantDir := filepath.Join(filepath.Clean(configDir), "cli-plugins")
	if cleanDir != wantDir {
		return "", false
	}
	if !strings.HasPrefix(name, "docker-") || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return "", false
	}
	target := filepath.Join(cleanDir, name)
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", false
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	return target, true
}
