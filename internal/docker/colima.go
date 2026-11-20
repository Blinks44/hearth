package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const ColimaGiB int64 = 1 << 30

// ColimaConfigDir follows Colima's configuration search order for existing
// installations. Before first start, either default directory can be absent.
func ColimaConfigDir(env sysenv.Env) string {
	home := env.Get("HOME")
	for _, dir := range []string{env.Get("COLIMA_HOME"), filepath.Join(home, ".colima")} {
		if !filepath.IsAbs(dir) {
			continue
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	if xdg := env.Get("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "colima")
	}
	return filepath.Join(home, ".config", "colima")
}

// ColimaUsesServiceProfile reports whether the terminal and the Homebrew
// service refer to the same default storage location.
func ColimaUsesServiceProfile(env sysenv.Env) bool {
	xdg := env.Get("XDG_CONFIG_HOME")
	return env.Get("COLIMA_HOME") == "" && env.Get("LIMA_HOME") == "" &&
		(xdg == "" || xdg == filepath.Join(env.Get("HOME"), ".config"))
}

// ReadColimaProfiles reads the newline-delimited list, including stopped profiles.
func ReadColimaProfiles(ctx context.Context, r run.Runner, env sysenv.Env, path string) ([]inventory.ColimaProfile, error) {
	out, err := colimaRead(ctx, r, append(slices.Clone(env), "COLIMA_PROFILE="), path, "list", "--json")
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var profiles []inventory.ColimaProfile
	for {
		var row struct {
			Name, Status, Runtime string
			CPU                   int `json:"cpus"`
			Memory, Disk          int64
		}
		err := dec.Decode(&row)
		if errors.Is(err, io.EOF) {
			return profiles, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read Colima profiles: %w", err)
		}
		if row.Name != "" {
			profiles = append(profiles, inventory.ColimaProfile{
				Name: row.Name, Status: strings.ToLower(row.Status), Runtime: row.Runtime, CPU: row.CPU, Memory: row.Memory, Disk: row.Disk,
			})
		}
	}
}

// ReadColimaStatus adds live driver, mount and endpoint information to a profile.
func ReadColimaStatus(ctx context.Context, r run.Runner, env sysenv.Env, path string, profile *inventory.ColimaProfile) error {
	out, err := colimaRead(ctx, r, env, path, "status", "--profile", profile.Name, "--json")
	if err != nil {
		return err
	}
	var row struct {
		Driver, Runtime string
		Mount           string `json:"mount_type"`
		Socket          string `json:"docker_socket"`
	}
	if err := json.Unmarshal(out, &row); err != nil {
		return fmt.Errorf("read Colima status: %w", err)
	}
	profile.Driver, profile.Mount = row.Driver, row.Mount
	if strings.HasPrefix(row.Socket, "unix:///") {
		profile.Socket = row.Socket
	}
	if row.Runtime != "" {
		profile.Runtime = row.Runtime
	}
	return nil
}

func colimaRead(ctx context.Context, r run.Runner, env sysenv.Env, path string, args ...string) ([]byte, error) {
	res, err := r.Run(ctx, run.Cmd{Args: append([]string{path}, args...), Env: env, Timeout: 4 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("colima %s: %w", args[0], err)
	}
	if res.ExitCode != 0 || res.Truncated {
		return nil, fmt.Errorf("colima %s: incomplete output (exit %d)", args[0], res.ExitCode)
	}
	return res.Stdout, nil
}

// ColimaAutostart reads Homebrew's default-profile service registration.
func ColimaAutostart(ctx context.Context, r run.Runner, env sysenv.Env, brew string) string {
	if brew == "" {
		return ""
	}
	res, err := r.Run(ctx, run.Cmd{Args: []string{brew, "services", "list", "--json"},
		Env: append(slices.Clone(env), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1"), Timeout: 5 * time.Second})
	var rows []struct {
		Name, File string
		Registered *bool
	}
	if err != nil || res.ExitCode != 0 || res.Truncated || json.Unmarshal(res.Stdout, &rows) != nil {
		return ""
	}
	for _, row := range rows {
		if row.Name != "colima" {
			continue
		}
		if row.Registered != nil && *row.Registered ||
			row.Registered == nil && (strings.Contains(row.File, "/LaunchAgents/") ||
				strings.Contains(row.File, "/LaunchDaemons/") || strings.Contains(row.File, "/systemd/user/")) {
			return "on"
		}
		return "off"
	}
	return ""
}
