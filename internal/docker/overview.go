package docker

import (
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type Overview struct {
	Problems            []Container
	ContainersErr       error
	Cores               float64
	Memory              int64
	CPURead, MemoryRead bool
	Disk                OverviewDisk
	Events              OverviewEvents
}

type OverviewDisk struct {
	Usage []inventory.DockerUsage
	Err   error
	VM    VMDisk
}

type OverviewEvents struct {
	Items []Event
	Err   error
}

type Event struct {
	Action, Name, ExitCode string
	Count                  int
}

func readOverview(ctx context.Context, r run.Runner, env sysenv.Env, target Target, e Engine, readDetails bool, disk OverviewDisk, events OverviewEvents) (Overview, inventory.DockerHealth) {
	o := Overview{Disk: disk, Events: events}
	var health inventory.DockerHealth
	var reads sync.WaitGroup
	reads.Go(func() {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		items, err := ReadContainers(ctx, r, env, target)
		o.ContainersErr = err
		if err != nil {
			return
		}
		for _, item := range items {
			if item.Problem() {
				o.Problems = append(o.Problems, item)
			}
		}
		o.Cores, o.CPURead, o.Memory, o.MemoryRead = readContainerUsage(ctx, r, env, target, items)
		health = readContainerHealth(ctx, r, env, target, e, items)
		if o.MemoryRead {
			health.Memory, health.ContainerMemory = e.Memory, o.Memory
		}
	})
	if readDetails {
		reads.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			o.Disk.Usage, o.Disk.Err = ReadUsage(ctx, r, env, target)
		})
		reads.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			o.Disk.VM = readVMDisk(ctx, r, env, target)
		})
		reads.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			o.Events.Items, o.Events.Err = readOverviewEvents(ctx, r, env, target, e.Taken)
		})
	}
	reads.Wait()
	return o, health
}

// Docker's CPU percentage measures usage against one core. Memory is the
// sum of container working sets. Neither sample includes the VM itself.
func readContainerUsage(ctx context.Context, r run.Runner, env sysenv.Env, t Target, items []Container) (cores float64, cpuRead bool, memory int64, memoryRead bool) {
	active := make([]Container, 0, len(items))
	for _, item := range items {
		if item.State.Status == "running" || item.State.Status == "paused" {
			active = append(active, item)
		}
	}
	if len(active) > 0 && sampleContainers(ctx, r, env, t, active) != nil {
		return 0, false, 0, false
	}
	cpus, memories := 0, 0
	for _, item := range active {
		if pct, ok := strings.CutSuffix(strings.TrimSpace(item.CPU), "%"); ok {
			if n, err := strconv.ParseFloat(pct, 64); err == nil && n >= 0 && !math.IsInf(n, 0) {
				cores += n / 100
				cpus++
			}
		}
		if n, err := ParseSize(strings.TrimSpace(item.Memory)); err == nil {
			memory += n
			memories++
		}
	}
	count := len(active)
	return cores, cpus == count, memory, memories == count
}

func readOverviewEvents(ctx context.Context, r run.Runner, env sysenv.Env, t Target, now time.Time) ([]Event, error) {
	rows, err := readRows[struct {
		Action string
		Actor  struct {
			ID         string
			Attributes struct{ Name, ExitCode string }
		}
	}](ctx, r, t.Command(env, "events", "--since", now.Add(-time.Hour).Format(time.RFC3339Nano),
		"--until", now.Format(time.RFC3339Nano), "--filter", "type=container", "--filter", "event=restart",
		"--filter", "event=oom", "--filter", "event=die", "--format", "{{json .}}"))
	if err != nil {
		return nil, err
	}
	type key struct{ id, action, code string }
	byKey := make(map[key]int)
	var events []Event
	for _, row := range slices.Backward(rows) {
		name, code := row.Actor.Attributes.Name, row.Actor.Attributes.ExitCode
		if row.Action == "die" && (code == "" || code == "0" || code == "143" || code == "130") {
			continue
		}
		k := key{row.Actor.ID, row.Action, code}
		index, ok := byKey[k]
		if !ok {
			index = len(events)
			byKey[k] = index
			events = append(events, Event{Action: row.Action, Name: name, ExitCode: code})
		}
		events[index].Count++
	}
	return events, nil
}
