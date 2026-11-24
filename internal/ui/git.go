package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// gitPage is Git's page in the Tools section.
const gitPage = "/tools/git"

// gitPageView is Git's page: the copies and how the terminal finds git,
// the user's global settings, the credential helpers and GitHub CLI, and
// the SSH keys by name.
func (a *App) gitPageView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.toolState("git")
		if !ok {
			a.toolEmpty(c, "git", "Git")
			return
		}
		ins := st.Installations
		rows := listed(ins)
		active := inventory.TerminalCopy(ins)
		g := st.Git
		if g == nil {
			g = &inventory.GitState{}
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.gitHeader(c, ins, rows, active, g)
				segments(c, "git-tabs", &a.gitTab, []segment{
					{locales.Installed(), strconv.Itoa(len(rows))},
					{locales.GitSettingsTab(), ""},
					{"PATH", ""},
				})
				switch a.gitTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, a.packageUpdate)
						if slices.ContainsFunc(rows, func(i int) bool { return inventory.NeededBySystem(ins[i]) }) {
							toolNote(c, locales.GitSystemNote())
						}
						pathCard(c, "git", ins, active, a.Home)
					}, func() { a.gitCards(c, g) })
				case 1:
					columns(c, 14, func() { a.gitCards(c, g) }, nil)
				default:
					pathCard(c, "git", ins, active, a.Home)
				}
			})
		})
	})
}

// gitHeader is Git's logo, where its copies come from and GitHub CLI's
// version, and the cards: installed, in the terminal, the user.
func (a *App) gitHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int, g *inventory.GitState) {
	t := c.Theme()
	pal := Colors(t)
	subtitle := strings.Join(sourceNames(ins, rows, active), " · ")
	if g.GitHub != nil {
		subtitle = strings.TrimPrefix(subtitle+" · GitHub CLI "+g.GitHub.Version, " · ")
	}
	toolHead(c, "git", "Git", func() {}, subtitle, func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.CopiesCount(len(rows)), strings.Join(sourceNames(ins, rows, -1), " · "))
		if active >= 0 {
			in := ins[active]
			stateCard(c, t.Success, locales.InTerminal(), copyVersion(in), whichCommand+" git → "+sourceName(in.Source))
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		user := "—"
		if v := gitSetting(g, "user.name"); v != "" {
			user = v
		}
		userCaption := gitSetting(g, "user.email")
		if userCaption == "" {
			userCaption = tilde(g.ConfigFile, a.Home)
		}
		stateCard(c, pal.TextFaint, locales.GitUser(), user, userCaption)
	})
}

// gitCards are the global settings, the credentials and the SSH keys.
func (a *App) gitCards(c *ui.Context, g *inventory.GitState) {
	pal := Colors(c.Theme())
	configAside := func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if label := tilde(g.ConfigFile, a.Home); label != "" {
				ui.Text(c, label).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			}
			if g.ConfigFile != "" && !g.ConfigMissing && a.canOpenFile() {
				iconButton(c.Key("open-gitconfig"), "external-link", locales.OpenFile(filepath.Base(g.ConfigFile))).
					OnClick(func() { a.openFile(c, g.ConfigFile) })
			}
		})
	}
	listCardHeader(c, locales.GitGlobal(), configAside, func() {
		for _, s := range g.Config {
			listRow(c, gitSettingName(s.Name), true, s.Value, func() {})
		}
		if len(g.Config) == 0 {
			cardNote(c, locales.GitNoSettings())
		}
		if g.Err != "" {
			cardNote(c, locales.GitConfigFailed(g.Err))
		}
	})
	listCard(c, locales.GitCredentials(), "credential.helper", func() {
		for _, h := range g.Helpers {
			listRow(c, gitHelperName(h), false, gitHelperCaption(h), func() {})
		}
		if len(g.Helpers) == 0 {
			cardNote(c, locales.GitNoHelper())
		}
		if gh := g.GitHub; gh != nil {
			var hosts []string
			for _, h := range gh.Hosts {
				hosts = append(hosts, strings.TrimSpace(h.Host+" "+prefixed("· ", h.User)))
			}
			caption := locales.GitHubSignedOut()
			if len(hosts) > 0 {
				caption = locales.GitHubSignedIn(strings.Join(hosts, ", "))
			}
			listRow(c, "GitHub CLI "+gh.Version, false, caption, func() {})
		} else if a.canInstallTool("gh") {
			listRow(c, "GitHub CLI", false, locales.GitNoGitHub(), func() { a.installToolButton(c, "gh") })
		}
	})
	sshAside := func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.GitSSHAside()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			if a.Home != "" && !g.SSHConfigMissing && a.canOpenFile() {
				sshConfig := filepath.Join(a.Home, ".ssh", "config")
				iconButton(c.Key("open-ssh-config"), "external-link", locales.OpenFile("~/.ssh/config")).
					OnClick(func() { a.openFile(c, sshConfig) })
			}
		})
	}
	listCardHeader(c, locales.GitSSHKeys(), sshAside, func() {
		for _, k := range g.Keys {
			var caption []string
			if len(k.Hosts) > 0 {
				caption = append(caption, locales.GitSSHFor(strings.Join(k.Hosts, ", ")))
			}
			if k.InAgent {
				caption = append(caption, locales.GitSSHInAgent())
			}
			if len(caption) == 0 {
				caption = append(caption, locales.GitSSHUnused())
			}
			kind := k.Type
			if k.Bits > 0 {
				kind += " " + strconv.Itoa(k.Bits)
			}
			pub := k.PublicKey
			listRow(c, k.Name, true, strings.Join(caption, " · "), func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, kind).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
					if pub != "" {
						iconButton(c.Key("copy-pub:"+k.Name), "copy", locales.GitCopyPublicKey()).OnClick(func() {
							c.WriteClipboard(pub)
							c.Toast(locales.GitPublicKeyCopied())
						})
					}
				})
			})
		}
		if len(g.Keys) == 0 {
			cardNote(c, locales.GitNoSSHKeys())
		}
	})
}

// gitSetting is a global setting's value by its lower-case name; "" when
// not set.
func gitSetting(g *inventory.GitState, name string) string {
	for _, s := range g.Config {
		if s.Name == name {
			return s.Value
		}
	}
	return ""
}

// gitSettingName writes a setting as git's documentation does:
// init.defaultBranch, not init.defaultbranch, as git config lists it.
func gitSettingName(name string) string {
	if n, ok := map[string]string{"init.defaultbranch": "init.defaultBranch", "commit.gpgsign": "commit.gpgSign"}[name]; ok {
		return n
	}
	return name
}

// gitHelperName names a credential helper for people.
func gitHelperName(h string) string {
	switch h {
	case "osxkeychain":
		return locales.GitHelperKeychain()
	case "manager", "manager-core", "credential-manager":
		return "Git Credential Manager"
	case "libsecret":
		return locales.GitHelperLibsecret()
	case "store":
		return locales.GitHelperStore()
	case "cache":
		return locales.GitHelperCache()
	}
	return h
}

func gitHelperCaption(h string) string {
	if h == "store" {
		return h + " · " + locales.GitHelperStorePlain()
	}
	return h + " · " + locales.GitHelperSecrets()
}

func prefixed(p, s string) string {
	if s == "" {
		return ""
	}
	return p + s
}

// packageUpdate tells a copy a package manager has an update for: "update
// 2.48.1".
func (a *App) packageUpdate(in inventory.Installation) (string, bool) {
	if _, p, ok := a.copyPackage(in); ok && p.Available != "" {
		return locales.PkgAvailable() + " " + p.Available, false
	}
	return "", false
}

// copyPackage finds the package that put a copy, and its manager, with
// the updates the online check found.
func (a *App) copyPackage(in inventory.Installation) (inventory.PackageManager, inventory.SystemPackage, bool) {
	st, ok := a.Snapshot.Tool("packages")
	if !ok || in.Source.Root == "" {
		return inventory.PackageManager{}, inventory.SystemPackage{}, false
	}
	for _, m := range st.PackageManagers {
		if m.ID != in.Source.Manager {
			continue
		}
		if checked, ok := a.PackageUpdates[m.Path]; ok {
			m = checked
		}
		for _, p := range m.Packages {
			if p.Path == in.Source.Root {
				return m, p, true
			}
		}
	}
	return inventory.PackageManager{}, inventory.SystemPackage{}, false
}
