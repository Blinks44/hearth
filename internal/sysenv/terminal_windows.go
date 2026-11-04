package sysenv

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"

	"github.com/Blinks44/hearth/internal/run"
)

// Terminal returns the environment a new terminal gets: Hearth's own with
// Path read anew from the registry, system then user, as a new terminal
// reads it. Hearth's own Path is what it was when Explorer started.
func Terminal(context.Context, run.Runner) (Env, error) {
	sys, err := registryPath(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`)
	if err != nil {
		return nil, fmt.Errorf("terminal environment: %w", err)
	}
	usr, err := registryPath(registry.CURRENT_USER, `Environment`)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, fmt.Errorf("terminal environment: %w", err)
	}
	path := sys
	if usr != "" {
		path += ";" + usr
	}
	return GUI().with("Path", path), nil
}

// registryPath reads the Path value of a key, expanding %VAR% references.
func registryPath(root registry.Key, key string) (string, error) {
	k, err := registry.OpenKey(root, key, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", key, err)
	}
	defer k.Close() //nolint:errcheck // a read-only key: nothing to lose on close
	v, typ, err := k.GetStringValue("Path")
	if err != nil {
		return "", fmt.Errorf("read %s\\Path: %w", key, err)
	}
	if typ == registry.EXPAND_SZ {
		if v, err = registry.ExpandString(v); err != nil {
			return "", fmt.Errorf("expand %s\\Path: %w", key, err)
		}
	}
	return v, nil
}
