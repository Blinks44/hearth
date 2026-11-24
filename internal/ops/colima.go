package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// ColimaInstallPlan installs the engine and both CLI plugins and starts the
// default profile with a Docker CLI independent of any engine app.
// The wizard uses the same plan after its Homebrew steps.
func ColimaInstallPlan(manager inventory.PackageManager, migrate bool, env sysenv.Env, r run.Runner) (Plan, error) {
	var base []Step
	var notes []Note
	if manager.Path == "" {
		manager.Path, base, notes = homebrew(env)
	}
	if manager.Path == "" {
		return Plan{}, errors.New(locales.ColimaBrewRequired())
	}
	manager.ID, manager.Name = "brew", "Homebrew"
	prefix := manager.Prefix
	if prefix == "" {
		prefix = filepath.Dir(filepath.Dir(manager.Path))
	}
	bin := filepath.Join(prefix, "bin")
	dir := filepath.Join(prefix, "lib", "docker", "cli-plugins")
	edit, err := colimaPluginEdit(env, dir)
	if err != nil {
		return Plan{}, err
	}
	names := []string{"colima", "docker-compose", "docker-buildx"}
	termDirs := filepath.SplitList(env.Get("PATH"))
	brewDocker := filepath.Join(bin, "docker")
	optDocker := filepath.Join(prefix, "opt", "docker", "bin", "docker")
	search := append(slices.Clone(env), "PATH="+strings.Join(slices.Concat(termDirs, []string{bin, filepath.Dir(optDocker)}), string(filepath.ListSeparator)))
	exe := detect.StandaloneDockerCLI(search)
	var separateInstall, pathEdits []Step
	if exe == "" {
		switch _, err := os.Lstat(brewDocker); {
		case errors.Is(err, os.ErrNotExist):
			names = append(names, "docker")
			exe = brewDocker
		case err != nil:
			return Plan{}, fmt.Errorf("check Homebrew Docker CLI path: %w", err)
		default:
			// Homebrew's name can already be occupied by a cask link.
			// Keep its link and install Docker under Homebrew's opt path.
			separateInstall = []Step{{Argv: []string{manager.Path, "install", "--formula", "--skip-link", "docker"}}}
			exe = optDocker
		}
	}
	if exe == optDocker {
		pathEdits, err = colimaClientPathEdits(env, bin, filepath.Dir(exe))
		if err != nil {
			return Plan{}, err
		}
	}
	var packages []inventory.SystemPackage
	for _, name := range names {
		packages = append(packages, inventory.SystemPackage{Name: name, Kind: "formula"})
	}
	p, ok := PackagePlan(PackageInstall, manager, packages, nil)
	if !ok {
		return Plan{}, errors.New(locales.ColimaBrewRequired())
	}
	p.Steps = append(base, p.Steps...)
	p.Steps = append(p.Steps, separateInstall...)
	p.Steps = append(p.Steps, pathEdits...)
	p.Notes = append(notes, p.Notes...)
	p.Title, p.Why = locales.ColimaInstall(), locales.ColimaInstallWhy()
	p.Env = slices.Clone(env)
	p.Isolated = true
	if edit != nil {
		p.Steps = append(p.Steps, Step{Edit: edit})
	}
	path := filepath.Join(bin, "colima")
	// On a retry after a partial installation keep the profile's existing
	// VM type and runtime settings.
	args := []string{path, "start", "--profile", "default"}
	instance := filepath.Join(docker.ColimaConfigDir(env), "default", "colima.yaml")
	_, err = os.Stat(instance)
	if errors.Is(err, os.ErrNotExist) {
		args = append(args, "--ssh-config=false")
		args = append(args, colimaFirstStart()...)
	} else if err != nil {
		return Plan{}, fmt.Errorf("read Colima configuration: %w", err)
	}
	if migrate {
		p.Title = locales.ColimaMigrate()
		p.Notes = append(p.Notes, dockerSwitchNotes()...)
	}
	p.Notes = append(p.Notes, Note{Title: locales.DockerContext(), Text: locales.ColimaSelectContext()})
	// Colima invokes docker itself to create its context. Put the same
	// standalone client that verification will use first for that command.
	clientDir := filepath.Dir(exe)
	pathDirs := []string{clientDir}
	if bin != clientDir {
		pathDirs = append(pathDirs, bin)
	}
	pathEnv := strings.Join(append(pathDirs, termDirs...), string(filepath.ListSeparator))
	checkEnv := append(slices.Clone(env), "PATH="+pathEnv)
	p.Steps = append(p.Steps, Step{Argv: args, Env: checkEnv[len(env):]})
	p.Verify = func(ctx context.Context) (string, error) {
		profile := inventory.ColimaProfile{Name: "default"}
		if err := docker.ReadColimaStatus(ctx, r, checkEnv, path, &profile); err != nil {
			return "", err
		}
		if profile.Runtime != "docker" || profile.Socket == "" {
			return "", errors.New(locales.ColimaUnavailable())
		}
		t := docker.Target{Docker: exe, Context: "colima", Endpoint: profile.Socket}
		for _, args := range [][]string{{"version", "--format", "{{.Server.Version}}"}, {"compose", "version"}, {"buildx", "version"}} {
			res, err := r.Run(ctx, t.Command(checkEnv, args...))
			if err != nil {
				return "", fmt.Errorf("verify Docker %s: %w", args[0], err)
			}
			if res.ExitCode != 0 || res.Truncated || len(res.Stdout) == 0 {
				return "", fmt.Errorf("verify Docker %s: exit %d", args[0], res.ExitCode)
			}
		}
		return locales.ColimaRunning(), nil
	}
	return p, nil
}

const dockerPluginDirs = "cliPluginsExtraDirs"

// colimaPluginEdit appends a directory inside Docker's plugin directory list.
// JSON has no comments, so the array entry is the unit Hearth manages.
func colimaPluginEdit(env sysenv.Env, dir string) (*FileEdit, error) {
	config := docker.ConfigDir(env)
	if config == "" {
		return nil, errors.New(locales.ColimaConfigError())
	}
	path := filepath.Join(config, "config.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		value, _ := json.Marshal(dir)
		return &FileEdit{Path: path, New: true, After: "{\n  \"" + dockerPluginDirs + "\": [" + string(value) + "]\n}\n"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Docker configuration: %w", err)
	}
	e := &FileEdit{Path: path, Before: string(data)}
	dec := json.NewDecoder(strings.NewReader(e.Before))
	if token, err := dec.Token(); err != nil || token != json.Delim('{') || !json.Valid(data) {
		return nil, errors.New(locales.ColimaConfigError())
	}
	found, fields := false, 0
	value, _ := json.Marshal(dir)
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, errors.New(locales.ColimaConfigError())
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, errors.New(locales.ColimaConfigError())
		}
		fields++
		name, _ := key.(string)
		if !strings.EqualFold(name, dockerPluginDirs) {
			e.Hide = append(e.Hide, string(raw))
			continue
		}
		var dirs []string
		if found || json.Unmarshal(raw, &dirs) != nil {
			return nil, errors.New(locales.ColimaConfigError())
		}
		found = true
		if slices.Contains(dirs, dir) {
			continue
		}
		end := int(dec.InputOffset())
		if dirs == nil {
			e.After = e.Before[:end-len(raw)] + "[" + string(value) + "]" + e.Before[end:]
			continue
		}
		i := end - 1
		add := string(value)
		if len(dirs) > 0 {
			add = ", " + add
		}
		e.After = e.Before[:i] + add + e.Before[i:]
	}
	if !found {
		if _, err := dec.Token(); err != nil {
			return nil, errors.New(locales.ColimaConfigError())
		}
		i := int(dec.InputOffset()) - 1
		before := strings.TrimRight(e.Before[:i], " \n\r\t")
		if fields > 0 {
			before += ","
		}
		e.After = before + "\n  \"" + dockerPluginDirs + "\": [" + string(value) + "]\n" + e.Before[i:]
	}
	if e.After == "" {
		return nil, nil
	}
	return e, nil
}

type ColimaLimits struct {
	CPU    int
	Memory float64
	Disk   int64
}

type ColimaAction string

const (
	ColimaStart   ColimaAction = "start"
	ColimaStop    ColimaAction = "stop"
	ColimaTrim    ColimaAction = "trim"
	ColimaResize  ColimaAction = "resize"
	ColimaEnable  ColimaAction = "enable"
	ColimaDisable ColimaAction = "disable"
)

var colimaProfileName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ColimaPlan builds a change against one observed profile and rechecks it
// before execution. Only the default profile has a Homebrew service.
func ColimaPlan(r run.Runner, env sysenv.Env, colima inventory.ColimaState, profile inventory.ColimaProfile, action ColimaAction, limits ColimaLimits) (Plan, error) {
	if colima.Path == "" || !colimaProfileName.MatchString(profile.Name) || (profile.Runtime != "" && profile.Runtime != "docker") || colima.Unreadable {
		return Plan{}, errors.New(locales.ColimaUnavailable())
	}
	cmd := func(action string, flags ...string) Step {
		return Step{Argv: append([]string{colima.Path, action, "--profile", profile.Name}, flags...)}
	}
	start, stop := cmd("start", "--activate=false"), cmd("stop")
	p := Plan{Env: slices.Clone(env), Isolated: true}
	useService := action == ColimaEnable || action == ColimaDisable
	managed := profile.Name == "default" && colima.Autostart == "on"
	service := func(verb string) Step {
		return Step{Argv: []string{colima.Brew, "services", verb, "colima"}, Timeout: 2 * time.Minute}
	}
	var wantRunning bool
	var trimDisk docker.VMDisk
	switch action {
	case ColimaStart:
		if profile.Status != "" && profile.Status != "stopped" {
			return Plan{}, errors.New(locales.ColimaUnavailable())
		}
		p.Title, p.Why, p.Steps = locales.ColimaStart(), locales.DockerStartWhy(), []Step{start}
		if profile.Status == "" {
			p.Steps[0].Argv = append(p.Steps[0].Argv, "--ssh-config=false")
			p.Steps[0].Argv = append(p.Steps[0].Argv, colimaFirstStart()...)
			p.Notes = append(p.Notes, Note{Title: locales.DockerContext(), Text: locales.ColimaChooseContext()})
			if managed {
				p.Steps = append(p.Steps, stop, service("start"))
			}
		} else if managed {
			p.Steps = []Step{service("restart")}
		}
		wantRunning = true
	case ColimaStop:
		if profile.Status != "running" {
			return Plan{}, errors.New(locales.ColimaUnavailable())
		}
		p.Title, p.Why, p.Steps = locales.ColimaStop(), locales.ColimaStopWhy(), []Step{stop}
		if managed {
			p.Steps = []Step{service("stop"), stop}
			p.Notes = append(p.Notes, Note{Title: locales.ColimaDisable(), Text: locales.ColimaDisableWhy()})
		}
	case ColimaTrim:
		if profile.Status != "running" {
			return Plan{}, errors.New(locales.ColimaUnavailable())
		}
		var err error
		trimDisk, err = docker.ColimaDisk(env, profile.Name)
		if err != nil {
			return Plan{}, err
		}
		trim := cmd("ssh", "--", "sudo", "fstrim", "-a")
		trim.Timeout = 2 * time.Minute
		p.Title, p.Why, p.Steps = locales.DockerOverviewTrimTitle(), locales.DockerOverviewTrimWhy(profile.Name), []Step{trim}
		wantRunning = true
	case ColimaResize:
		if (profile.Status != "running" && profile.Status != "stopped") || profile.Disk <= 0 ||
			limits.CPU < 1 || !(limits.Memory > 0 && limits.Memory <= 1024) ||
			limits.Disk > 1<<32 || limits.Disk < 1+(profile.Disk-1)/docker.ColimaGiB {
			return Plan{}, errors.New(locales.ColimaInvalidLimits())
		}
		actionLabel := locales.ColimaRestart()
		if profile.Status == "stopped" {
			actionLabel = locales.ColimaStart()
		}
		p.Title, p.Why, p.Action = locales.ColimaResources(), locales.ColimaResizeWhy(profile.Name), actionLabel
		p.Notes = []Note{{Title: locales.ColimaDiskGrowth(), Text: locales.ColimaDiskGrowthHint()}}
		if managed {
			startService := service("start")
			stopService := service("stop")
			stopService.Undo = &startService
			p.Steps = append(p.Steps, stopService)
			if profile.Status == "running" {
				p.Steps = append(p.Steps, stop)
			}
		} else if profile.Status == "running" {
			stop.Undo = &start
			p.Steps = append(p.Steps, stop)
		}
		p.Steps = append(p.Steps, cmd("start", "--activate=false", "--cpu", strconv.Itoa(limits.CPU),
			"--memory", strconv.FormatFloat(limits.Memory, 'f', -1, 64), "--disk", strconv.FormatInt(limits.Disk, 10)))
		if managed {
			p.Steps = append(p.Steps, stop, service("start"))
			p.Notes = append(p.Notes, Note{Title: locales.ColimaAutostartOn(), Text: locales.ColimaRestoreAutostart()})
		}
		wantRunning = true
	case ColimaEnable, ColimaDisable:
		if profile.Name != "default" || colima.Brew == "" || colima.Autostart == "" ||
			(action == ColimaEnable) == (colima.Autostart == "on") {
			return Plan{}, errors.New(locales.ColimaUnavailable())
		}
		verb := "stop"
		p.Title, p.Why = locales.ColimaDisable(), locales.ColimaDisableWhy()
		if action == ColimaEnable {
			verb = "start"
			p.Title, p.Why = locales.ColimaEnable(), locales.ColimaEnableWhy()
		}
		p.Steps = []Step{service(verb)}
	default:
		return Plan{}, errors.New(locales.ColimaUnavailable())
	}
	read := func(ctx context.Context) (inventory.ColimaProfile, error) {
		profiles, err := docker.ReadColimaProfiles(ctx, r, env, colima.Path)
		for _, current := range profiles {
			if current.Name == profile.Name {
				return current, err
			}
		}
		return inventory.ColimaProfile{Name: profile.Name}, err
	}
	p.Before = func(ctx context.Context) error {
		if (managed || useService) && !docker.ColimaUsesServiceProfile(env) {
			return errors.New(locales.ColimaChanged())
		}
		current, err := read(ctx)
		if err != nil {
			return err
		}
		if current.Status != profile.Status || current.Disk != profile.Disk || current.Memory != profile.Memory || current.CPU != profile.CPU ||
			(current.Runtime != "" && current.Runtime != "docker") ||
			(profile.Name == "default" && colima.Brew != "" && docker.ColimaUsesServiceProfile(env) && docker.ColimaAutostart(ctx, r, env, colima.Brew) != colima.Autostart) {
			return errors.New(locales.ColimaChanged())
		}
		if action == ColimaTrim {
			currentDisk, err := docker.ColimaDisk(env, profile.Name)
			if err != nil || currentDisk.Path != trimDisk.Path {
				return errors.New(locales.ColimaChanged())
			}
		}
		return nil
	}
	p.Verify = func(ctx context.Context) (string, error) {
		if useService {
			want, result := "off", locales.ColimaAutostartOff()
			if action == ColimaEnable {
				want, result = "on", locales.ColimaAutostartOn()
			}
			if docker.ColimaAutostart(ctx, r, env, colima.Brew) != want {
				return "", errors.New(locales.ColimaUnavailable())
			}
			return result, nil
		}
		current, err := read(ctx)
		if err != nil {
			return "", err
		}
		if managed {
			want := "on"
			if action == ColimaStop {
				want = "off"
			}
			if docker.ColimaAutostart(ctx, r, env, colima.Brew) != want {
				return "", errors.New(locales.ColimaUnavailable())
			}
		}
		if !wantRunning && current.Status == "stopped" {
			return locales.ColimaStopped(), nil
		}
		if wantRunning && current.Status == "running" &&
			(action != ColimaResize || current.CPU == limits.CPU && current.Memory == int64(float32(limits.Memory)*float32(docker.ColimaGiB)) && current.Disk >= limits.Disk*docker.ColimaGiB) {
			if action == ColimaTrim {
				after, err := docker.ColimaDisk(env, profile.Name)
				if err != nil || after.Path != trimDisk.Path {
					return "", errors.New(locales.ColimaChanged())
				}
				return locales.DockerOverviewTrimResult(), nil
			}
			return locales.ColimaRunning(), nil
		}
		return "", errors.New(locales.ColimaUnavailable())
	}
	return p, nil
}
