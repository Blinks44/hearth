package disk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

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
	res, err := r.Run(ctx, run.Cmd{Args: []string{st.Installations[active].Path, "--host", st.Docker.Contexts[i].Endpoint, "system", "df", "--format", "{{json .}}"},
		Env: append(slices.Clone(env), "DOCKER_CONTEXT="), Timeout: time.Minute})
	if err != nil {
		return nil, fmt.Errorf("docker system df: %w", err)
	}
	if res.ExitCode != 0 || res.Truncated {
		return nil, fmt.Errorf("docker system df: exit %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return parseDockerDF(res.Stdout)
}

func parseDockerDF(data []byte) ([]inventory.DockerUsage, error) {
	var out []inventory.DockerUsage
	for line := range bytes.SplitSeq(bytes.TrimSpace(data), []byte("\n")) {
		var row struct{ Type, TotalCount, Active, Size, Reclaimable string }
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("docker system df: %w", err)
		}
		u := inventory.DockerUsage{Type: row.Type}
		var errs [4]error
		u.Count, errs[0] = strconv.Atoi(row.TotalCount)
		u.Active, errs[1] = strconv.Atoi(row.Active)
		u.Bytes, errs[2] = parseDockerSize(row.Size)
		// Reclaimable comes with a share: "1.664GB (14%)".
		reclaimable, _, _ := strings.Cut(row.Reclaimable, " ")
		u.Reclaimable, errs[3] = parseDockerSize(reclaimable)
		if err := errors.Join(errs[:]...); err != nil {
			return nil, fmt.Errorf("docker system df %s: %w", row.Type, err)
		}
		out = append(out, u)
	}
	return out, nil
}

// parseDockerSize reads a size as Docker writes it, in decimal units:
// "0B", "769.4MB", "11.66GB".
func parseDockerSize(s string) (int64, error) {
	units := []struct {
		suffix string
		scale  float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}}
	for _, u := range units {
		if num, ok := strings.CutSuffix(s, u.suffix); ok {
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, fmt.Errorf("size %q: %w", s, err)
			}
			return int64(math.Round(f * u.scale)), nil
		}
	}
	return 0, fmt.Errorf("size %q: unknown unit", s)
}
