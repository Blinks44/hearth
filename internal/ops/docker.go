package ops

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// engineStartWait is how long a start's check waits for the engine to
// answer, and engineStartPause how far apart it asks: a desktop app
// starts its virtual machine first. Tests shorten them.
var (
	engineStartWait  = 2 * time.Minute
	engineStartPause = 2 * time.Second
)

// contextRef is what a Docker context's name may hold, as the CLI
// allows it.
var contextRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+-]*$`)

// enginePackages are the packages that install engine through the
// manager id; none when Hearth does not install it that way. cli reports
// a Docker CLI already there, which Colima needs and does not bring.
func enginePackages(manager, engine string, cli bool) []inventory.SystemPackage {
	cask := func(name string) []inventory.SystemPackage {
		return []inventory.SystemPackage{{Name: name, Kind: "cask"}}
	}
	switch manager + " " + engine {
	case "brew Docker Desktop":
		return cask("docker-desktop")
	case "brew OrbStack":
		return cask("orbstack")
	case "brew Podman Desktop":
		return cask("podman-desktop")
	case "brew Rancher Desktop":
		return cask("rancher")
	case "brew Podman":
		return []inventory.SystemPackage{{Name: "podman", Kind: "formula"}}
	case "brew Colima":
		out := []inventory.SystemPackage{{Name: "colima", Kind: "formula"}}
		if !cli {
			out = append(out, inventory.SystemPackage{Name: "docker", Kind: "formula"})
		}
		return out
	case "winget Docker Desktop":
		return []inventory.SystemPackage{{Name: "Docker.DockerDesktop"}}
	case "winget Podman":
		return []inventory.SystemPackage{{Name: "RedHat.Podman"}}
	case "winget Podman Desktop":
		return []inventory.SystemPackage{{Name: "RedHat.Podman-Desktop"}}
	case "winget Rancher Desktop":
		return []inventory.SystemPackage{{Name: "SUSE.RancherDesktop"}}
	// Linux distributions package Docker Engine with its CLI, each under
	// a name of its own.
	case "apt Docker Engine":
		return []inventory.SystemPackage{{Name: "docker.io"}}
	case "dnf Docker Engine":
		return []inventory.SystemPackage{{Name: "moby-engine"}}
	case "zypper Docker Engine", "pacman Docker Engine", "apk Docker Engine":
		return []inventory.SystemPackage{{Name: "docker"}}
	case "apt Podman", "dnf Podman", "zypper Podman", "pacman Podman", "apk Podman":
		return []inventory.SystemPackage{{Name: "podman"}}
	}
	return nil
}

// engineTerms is the note on the terms of use of an engine that has
// some beyond open source; ok is false for the others.
func engineTerms(engine string) (Note, bool) {
	switch engine {
	case "Docker Desktop":
		return Note{Title: locales.DockerTermsDesktop(), Text: locales.DockerTermsDesktopText(), URL: "https://docs.docker.com/subscription-billing/desktop-license/"}, true
	case "OrbStack":
		return Note{Title: locales.DockerTermsOrb(), Text: locales.DockerTermsOrbText(), URL: "https://orbstack.dev/pricing"}, true
	}
	return Note{}, false
}

// EngineInstallPlan returns the plan that installs engine through m, the
// system's package manager; cli reports a Docker CLI already there. ok
// is false when Hearth does not install engine through m.
func EngineInstallPlan(engine string, m inventory.PackageManager, cli bool, reread Reread) (p Plan, ok bool) {
	pkgs := enginePackages(m.ID, engine, cli)
	if len(pkgs) == 0 {
		return Plan{}, false
	}
	if p, ok = PackagePlan(PackageInstall, m, pkgs, reread); !ok {
		return Plan{}, false
	}
	p.Title = locales.PkgInstallTitle(engine)
	if note, ok := engineTerms(engine); ok {
		p.Notes = append([]Note{note}, p.Notes...)
		p.Action = locales.DockerAcceptInstall()
	}
	return p, true
}

// Engine is an installed container engine to start: its name, as
// inventory.DockerEngine has it, and its app or command.
type Engine struct{ Name, Path string }

// EngineStartPlan returns the plan that starts e, the engine of the
// context contextName, and waits for it to answer there; ok is false when
// Hearth does not start it.
func EngineStartPlan(e Engine, contextName string, detect Detect) (p Plan, ok bool) {
	steps := engineStart(e)
	if len(steps) == 0 || e.Path == "" {
		return Plan{}, false
	}
	p = Plan{Title: locales.DockerStartTitle(e.Name), Why: locales.DockerStartWhy(), Steps: steps}
	for _, s := range steps {
		if s.Admin {
			p.Notes = append(p.Notes, AdminNote())
			break
		}
	}
	p.Changes = []string{locales.DockerStartChange(e.Name)}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			ctx, cancel := context.WithTimeout(ctx, engineStartWait)
			defer cancel()
			for {
				st, err := detect(ctx)
				if err == nil {
					if c, ok := selectedContext(st, contextName); ok && c.Status == "available" {
						return locales.DockerCheckAnswers(e.Name, c.Version), nil
					}
				}
				select {
				case <-ctx.Done():
					return "", errors.New(locales.DockerCheckSilent(e.Name))
				case <-time.After(engineStartPause):
				}
			}
		}
	}
	return p, true
}

// ContextPlan returns the plan that makes to the Docker CLI's context,
// for every terminal, through cli, the CLI's path; from is the context
// it was. ok is false for a name the CLI would not take.
func ContextPlan(cli, from, to string, detect Detect) (p Plan, ok bool) {
	if cli == "" || !contextRef.MatchString(to) {
		return Plan{}, false
	}
	p = Plan{
		Title:   locales.DockerContextTitle(to),
		Why:     locales.DockerContextWhy(),
		Steps:   []Step{{Argv: []string{cli, "context", "use", to}, Timeout: time.Minute}},
		Changes: []string{"docker context: " + from + " → " + to},
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if st.Docker == nil || st.Docker.Selected != to {
				return "", errors.New(locales.DockerCheckNotSelected(to))
			}
			return locales.DockerCheckSelected(to), nil
		}
	}
	return p, true
}

// selectedContext is st's context name when it is the one selected.
func selectedContext(st inventory.ToolState, name string) (inventory.DockerContext, bool) {
	if st.Docker == nil || st.Docker.Selected != name {
		return inventory.DockerContext{}, false
	}
	for _, c := range st.Docker.Contexts {
		if c.Name == name {
			return c, true
		}
	}
	return inventory.DockerContext{}, false
}
