package docker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ReadUsage reads the space and reclaimable estimates reported by Docker.
func ReadUsage(ctx context.Context, r run.Runner, env sysenv.Env, target Target) ([]inventory.DockerUsage, error) {
	cmd := target.Command(env, "system", "df", "--format", "{{json .}}")
	cmd.Timeout = time.Minute
	rows, err := readRows[struct{ Type, TotalCount, Active, Size, Reclaimable string }](ctx, r, cmd)
	if err != nil {
		return nil, fmt.Errorf("docker system df: %w", err)
	}
	var out []inventory.DockerUsage
	for _, row := range rows {
		u := inventory.DockerUsage{Type: row.Type}
		var errs [4]error
		u.Count, errs[0] = strconv.Atoi(row.TotalCount)
		u.Active, errs[1] = strconv.Atoi(row.Active)
		u.Bytes, errs[2] = ParseSize(row.Size)
		// Reclaimable comes with a share: "1.664GB (14%)".
		reclaimable, _, _ := strings.Cut(row.Reclaimable, " ")
		u.Reclaimable, errs[3] = ParseSize(reclaimable)
		if err := errors.Join(errs[:]...); err != nil {
			return nil, fmt.Errorf("docker system df %s: %w", row.Type, err)
		}
		out = append(out, u)
	}
	return out, nil
}
