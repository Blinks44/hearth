package docker

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestOverviewReadsIndependentSources(t *testing.T) {
	t.Parallel()
	var r run.Fake
	r.Set(run.Result{Stdout: fixture(t, "system-df.jsonl")}, nil, colima.Command(nil, "system", "df", "--format", "{{json .}}").Args...)
	active, failed, restarting := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	r.Stdout(fmt.Sprintf("{\"ID\":%q}\n{\"ID\":%q}\n{\"ID\":%q}", active, failed, restarting), colima.Command(nil, "ps", "--all", "--no-trunc", "--format", "{{json .}}").Args...)
	r.Stdout(fmt.Sprintf(`{"ID":%q,"Name":"/web","State":{"Status":"running"}}
{"ID":%q,"Name":"/worker","State":{"Status":"exited","ExitCode":7}}
{"ID":%q,"Name":"/retry","State":{"Status":"restarting","ExitCode":8}}`, active, failed, restarting),
		colima.Command(nil, "container", "inspect", "--format", inspectContainer, "--", active, failed, restarting).Args...)
	r.Stdout(fmt.Sprintf(`{"ID":%q,"CPUPerc":"150.20%%","MemUsage":"6.5GiB / 8GiB"}`, active),
		colima.Command(nil, "stats", "--no-stream", "--no-trunc", "--format", "{{json .}}").Args...)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	r.Stdout(`{"Action":"die","Actor":{"ID":"one","Attributes":{"name":"worker","exitCode":"7"}}}
{"Action":"die","Actor":{"ID":"one","Attributes":{"name":"worker","exitCode":"0"}}}
{"Action":"die","Actor":{"ID":"one","Attributes":{"name":"worker","exitCode":"7"}}}
{"Action":"oom","Actor":{"ID":"two","Attributes":{"name":"web"}}}`,
		colima.Command(nil, "events", "--since", "2026-01-01T11:00:00Z", "--until", "2026-01-01T12:00:00Z",
			"--filter", "type=container", "--filter", "event=restart", "--filter", "event=oom", "--filter", "event=die", "--format", "{{json .}}").Args...)
	engine := Engine{Taken: now, Running: 2, Stopped: 1, CPUs: 4, Memory: 8 << 30}
	o, h := readOverview(t.Context(), &r, nil, colima, engine, true, OverviewDisk{}, OverviewEvents{})
	if o.ContainersErr != nil || len(o.Problems) != 2 || o.Problems[0].Name != "retry" || o.Problems[1].Name != "worker" {
		t.Fatalf("problems = %+v, err = %v", o.Problems, o.ContainersErr)
	}
	if !o.CPURead || !o.MemoryRead || math.Abs(o.Cores-1.502) > 1e-9 || o.Memory != 13<<29 || h.ContainerMemory != o.Memory {
		t.Fatalf("usage CPU=%v (%v), memory=%v (%v), health=%+v", o.Cores, o.CPURead, o.Memory, o.MemoryRead, h)
	}
	if o.Disk.Err != nil || len(o.Disk.Usage) != 4 || o.Disk.Usage[2].Reclaimable != 1_200_000_000 {
		t.Fatalf("disk = %+v", o.Disk)
	}
	if o.Events.Err != nil || len(o.Events.Items) != 2 || o.Events.Items[0].Action != "oom" || o.Events.Items[1] != (Event{ID: "one", Action: "die", Name: "worker", ExitCode: "7", Count: 2}) {
		t.Fatalf("events = %+v", o.Events)
	}
	r.Stdout(`{"Type":"Images","TotalCount":"1","Active":"1","Size":"12XB","Reclaimable":"0B"}`, colima.Command(nil, "system", "df", "--format", "{{json .}}").Args...)
	o, _ = readOverview(t.Context(), &r, nil, colima, engine, true, OverviewDisk{}, OverviewEvents{})
	if o.Disk.Err == nil || o.Disk.Usage != nil || !o.MemoryRead || len(o.Problems) != 2 {
		t.Fatalf("invalid disk response affected another read: %+v", o)
	}
}

func TestColimaVMDiskUsesSelectedProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := sysenv.Env{"LIMA_HOME=" + dir}
	data := filepath.Join(dir, "_disks", "colima-work", "datadisk")
	if err := os.MkdirAll(filepath.Dir(data), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ColimaDisk(env, "work"); err != nil || got.Path != data || got.Allocated < 0 {
		t.Fatalf("work disk = %+v, %v", got, err)
	}
	if _, err := ColimaDisk(env, "default"); err == nil {
		t.Fatal("default profile showed another VM's disk")
	}
}
