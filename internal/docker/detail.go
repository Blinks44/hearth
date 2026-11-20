package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type Binding struct {
	HostIP, HostPort string
}

// URL points to a published TCP port. Wildcard addresses are reached via loopback.
func (b Binding) URL(port string) string {
	if !strings.HasSuffix(port, "/tcp") {
		return ""
	}
	n, err := strconv.ParseUint(b.HostPort, 10, 16)
	if err != nil || n == 0 {
		return ""
	}
	host := b.HostIP
	if host == "" {
		host = "127.0.0.1"
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	if ip.IsUnspecified() {
		if ip.Is4() {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
	}
	return "http://" + net.JoinHostPort(host, b.HostPort)
}

type Mount struct {
	Type, Name, Source, Destination string
	RW                              bool
}

type HealthCheck struct {
	Start, End time.Time
	ExitCode   int
	Output     string
}

type ContainerDetail struct {
	ID, Image, ImageID, Platform, Path, RestartPolicy string
	Args, EnvKeys, Digests                            []string
	Created, Finished                                 time.Time
	Ports                                             map[string][]Binding
	Mounts                                            []Mount
	Networks                                          map[string]string
	Labels                                            map[string]string
	Health                                            []HealthCheck
	// Inspect keeps the complete object with Config.Env values masked.
	Inspect string
}

type containerInspect struct {
	ID, Image, Platform, Path string
	Args                      []string
	Created                   time.Time
	Config                    struct {
		Image        string
		Labels       map[string]string
		ExposedPorts map[string]json.RawMessage
	}
	HostConfig struct {
		RestartPolicy struct{ Name string }
		PortBindings  map[string][]Binding
	}
	NetworkSettings struct {
		Ports    map[string][]Binding
		Networks map[string]struct{ IPAddress, GlobalIPv6Address string }
	}
	Mounts []Mount
	State  struct {
		FinishedAt time.Time
		Health     *struct{ Log []HealthCheck }
	}
}

func inspectValue(ctx context.Context, r run.Runner, env sysenv.Env, t Target, id, format string) ([]byte, error) {
	if !ContainerID(id) {
		return nil, errors.New("docker inspect: invalid container ID")
	}
	cmd := t.Command(env, "container", "inspect", "--format", format, "--", id)
	return commandOutput(ctx, r, cmd, "docker container inspect")
}

func commandOutput(ctx context.Context, r run.Runner, cmd run.Cmd, label string) ([]byte, error) {
	res, err := r.Run(ctx, cmd)
	if err != nil {
		clear(res.Stdout)
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	if res.ExitCode != 0 || res.Truncated {
		clear(res.Stdout)
		if res.Truncated {
			return nil, fmt.Errorf("%s: output too large", label)
		}
		return nil, fmt.Errorf("%s: %s", label, strings.TrimSpace(string(res.Stderr)))
	}
	return res.Stdout, nil
}

func ReadContainerDetail(ctx context.Context, r run.Runner, env sysenv.Env, t Target, id string) (ContainerDetail, error) {
	raw, err := inspectValue(ctx, r, env, t, id, "{{json .}}")
	if err != nil {
		return ContainerDetail{}, err
	}
	defer clear(raw)
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return ContainerDetail{}, errors.New("docker inspect: invalid JSON")
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(object["Config"], &config); err != nil || config == nil {
		return ContainerDetail{}, errors.New("docker inspect: invalid container configuration")
	}
	var keys []string
	if encoded, ok := config["Env"]; ok {
		defer clear(encoded)
		defer clear(object["Config"])
		var entries []string
		if err := json.Unmarshal(encoded, &entries); err != nil {
			return ContainerDetail{}, errors.New("docker inspect: invalid environment")
		}
		masked := make([]string, 0, len(entries))
		for _, entry := range entries {
			key, _, _ := strings.Cut(entry, "=")
			keys = append(keys, strings.Clone(key))
			masked = append(masked, key+"=••••••••")
		}
		if entries != nil {
			config["Env"], _ = json.Marshal(masked)
			object["Config"], _ = json.Marshal(config)
		}
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	var item containerInspect
	if err := json.Unmarshal(raw, &item); err != nil || item.ID != id {
		return ContainerDetail{}, errors.New("docker inspect: invalid container details")
	}
	safe, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return ContainerDetail{}, fmt.Errorf("docker inspect: %w", err)
	}
	d := ContainerDetail{ID: id, Image: item.Config.Image, ImageID: item.Image, Platform: item.Platform,
		Path: item.Path, Args: item.Args, EnvKeys: keys, Inspect: string(safe),
		Created: item.Created, Finished: item.State.FinishedAt, RestartPolicy: item.HostConfig.RestartPolicy.Name,
		Mounts: item.Mounts, Labels: item.Config.Labels, Ports: make(map[string][]Binding), Networks: make(map[string]string)}
	if item.State.Health != nil {
		d.Health = item.State.Health.Log
	}
	for port := range item.Config.ExposedPorts {
		d.Ports[port] = nil
	}
	maps.Copy(d.Ports, item.HostConfig.PortBindings)
	for port, bindings := range item.NetworkSettings.Ports {
		if len(bindings) > 0 {
			d.Ports[port] = bindings
		}
	}
	for name, network := range item.NetworkSettings.Networks {
		d.Networks[name] = strings.TrimSpace(network.IPAddress + " " + network.GlobalIPv6Address)
	}
	if item.Image != "" {
		var image struct {
			Os, Architecture, Variant string
			RepoDigests               []string
		}
		cmd := t.Command(env, "image", "inspect", "--format", "{{json .}}", "--", item.Image)
		if raw, err := commandOutput(ctx, r, cmd, "docker image inspect"); err == nil {
			if json.Unmarshal(raw, &image) == nil {
				if image.Os != "" && image.Architecture != "" {
					d.Platform = image.Os + "/" + image.Architecture
					if image.Variant != "" {
						d.Platform += "/" + image.Variant
					}
				}
				d.Digests = image.RepoDigests
			}
		}
	}
	return d, nil
}

func ReadContainerEnv(ctx context.Context, r run.Runner, env sysenv.Env, t Target, id, key string) ([]byte, error) {
	raw, err := inspectValue(ctx, r, env, t, id, "{{json .Config.Env}}")
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var entries []string
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, errors.New("docker inspect: invalid environment")
	}
	for _, entry := range entries {
		name, value, _ := strings.Cut(entry, "=")
		if name == key {
			return []byte(value), nil
		}
	}
	return nil, errors.New("docker inspect: environment key no longer exists")
}
