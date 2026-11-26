package ui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type cloneProjectState struct {
	open         bool
	repoURL      string
	lastURL      string
	parent       string
	folder       string
	autoFolder   string
	folderEdited bool
	err          string
}

func (a *App) openCloneDialog() {
	a.cloneForm = &cloneProjectState{
		open:   true,
		parent: a.defaultCloneParent(),
	}
}

func (a *App) defaultCloneParent() string {
	counts := map[string]int{}
	best, bestN := "", 0
	for _, p := range a.Projects {
		if p.Dir == "" {
			continue
		}
		dir := filepath.Dir(p.Dir)
		if dir == "" || dir == "." || dir == p.Dir {
			continue
		}
		counts[dir]++
		if counts[dir] > bestN || (counts[dir] == bestN && dir < best) {
			best, bestN = dir, counts[dir]
		}
	}
	if best != "" {
		return best
	}
	return a.Home
}

// realGitPath returns the path to a real (non-stub) Git executable from the
// latest snapshot, or ("", false) when Git is missing or only the macOS stub.
func (a *App) realGitPath() (string, bool) {
	if a.Snapshot == nil {
		return "", false
	}
	st, ok := a.Snapshot.Tool("git")
	if !ok {
		return "", false
	}
	if active := inventory.TerminalCopy(st.Installations); active >= 0 && !st.Installations[active].Stub {
		return st.Installations[active].Path, true
	}
	for _, in := range st.Installations {
		if !in.Stub && in.Path != "" {
			return in.Path, true
		}
	}
	return "", false
}

func (a *App) cloneProjectButton(c *ui.Context) {
	if a.BuildPlan == nil {
		return
	}
	ui.Button(c.Key("clone-project"), "").Label(locales.GitCloneButton()).Height(controlHeight).OnClick(a.openCloneDialog).Children(func() {
		ui.Icon(c, Icon("git-branch")).Size(14, 14).TextColor(c.Theme().TextMuted)
		ui.Text(c, locales.GitCloneButton()).SingleLine()
	})
}

func (a *App) cloneDialog(c *ui.Context) {
	st := a.cloneForm
	if st == nil || !st.open {
		return
	}
	t := c.Theme()
	pal := Colors(t)

	gitPath, hasGit := a.realGitPath()
	canInstallGit := !hasGit && a.canInstallTool("git")

	// Auto-derive the folder name from the repository URL while the user has not
	// manually overridden it.
	if st.repoURL != st.lastURL {
		st.lastURL = st.repoURL
		st.err = ""
		if _, name, err := ops.ParseGitCloneURL(st.repoURL); err == nil {
			if !st.folderEdited || strings.TrimSpace(st.folder) == "" || st.folder == st.autoFolder {
				st.folder = name
				st.autoFolder = name
				st.folderEdited = false
			}
		} else if !st.folderEdited && st.folder == st.autoFolder {
			st.folder = ""
			st.autoFolder = ""
		}
	}

	parentExpanded := ops.ExpandCloneParent(st.parent, a.Home)
	folderClean := strings.TrimSpace(st.folder)
	destination := ""
	if parentExpanded != "" && folderClean != "" {
		destination = filepath.Join(parentExpanded, folderClean)
	}

	submit := func() {
		if !hasGit && !canInstallGit {
			st.err = locales.GitCloneNoGitUnavailable()
			return
		}
		cloneURL, defaultName, err := ops.ParseGitCloneURL(st.repoURL)
		if err != nil {
			st.err = err.Error()
			return
		}
		folder := strings.TrimSpace(st.folder)
		if folder == "" {
			folder = defaultName
		}
		if err := ops.ValidateCloneFolderName(folder); err != nil {
			st.err = err.Error()
			return
		}
		parent := ops.ExpandCloneParent(st.parent, a.Home)
		if parent == "" || !filepath.IsAbs(parent) {
			st.err = locales.GitCloneErrParent()
			return
		}
		parent = filepath.Clean(parent)
		target := filepath.Join(parent, folder)
		st.err = ""
		st.open = false
		if a.BuildPlan == nil {
			return
		}
		after := func() {
			if a.AddProjects != nil {
				a.AddProjects([]string{target})
			}
			a.router.Push(projectPath(target))
		}
		if hasGit {
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.GitClonePlan(gitPath, cloneURL, parent, folder, env)
			}, a.proposeThen(after))
			return
		}
		setup := a.installSetup("git")
		a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			installPlan, _, err := ops.SetupPlan(setup, env)
			if err != nil {
				return ops.Plan{}, err
			}
			return ops.GitCloneWithInstallPlan(installPlan, cloneURL, parent, folder, env)
		}, a.proposeThen(after))
	}

	canSubmit := strings.TrimSpace(st.repoURL) != "" && strings.TrimSpace(st.parent) != "" && (hasGit || canInstallGit)

	dialogBase(c.Key("git-clone-dialog"), &st.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(520).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Padding(20).Gap(14).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.GitCloneTitle()).FontSize(18).FontWeight(600)
				ui.Text(c, locales.GitCloneNote()).FontSize(12).TextColor(t.TextMuted)
			})

			if !hasGit {
				msg := locales.GitCloneNoGitText()
				if !canInstallGit {
					msg = locales.GitCloneNoGitUnavailable()
				}
				ui.Row(c).Gap(10).Padding(10, 12).Radius(8).AlignItems(ui.Start).
					Background(pal.WarningSoft).BorderWidth(1).BorderColor(pal.Line).Children(func() {
					ui.Icon(c, Icon("triangle-alert")).Size(15, 15).TextColor(pal.WarningText).Margin(1, 0, 0, 0)
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, locales.GitCloneNoGitTitle()).FontSize(12.5).FontWeight(600).TextColor(pal.WarningText)
						ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted)
					})
				})
			}

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.GitCloneUrlLabel()).FontSize(12).FontWeight(500)
				ui.TextInput(c.Key("git-clone-url"), &st.repoURL).
					Placeholder(locales.GitCloneUrlPlaceholder()).
					Height(controlHeight).FillWidth().AutoFocus().OnSubmit(submit)
			})

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.GitCloneParentLabel()).FontSize(12).FontWeight(500)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					prev := st.parent
					ui.TextInput(c.Key("git-clone-parent"), &st.parent).
						Grow(1).Height(controlHeight).OnSubmit(submit)
					if st.parent != prev {
						st.err = ""
					}
					if a.PickPath != nil {
						ui.Button(c.Key("git-clone-browse"), locales.PathAddBrowse()).Height(controlHeight).OnClick(func() {
							a.PickPath(locales.GitClonePickParent(), true, func(path string, err error) {
								if err != nil {
									st.err = err.Error()
									return
								}
								if path != "" {
									st.parent = path
									st.err = ""
								}
							})
						})
					}
				})
			})

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.GitCloneFolderLabel()).FontSize(12).FontWeight(500)
				prev := st.folder
				ui.TextInput(c.Key("git-clone-folder"), &st.folder).
					Height(controlHeight).FillWidth().OnSubmit(submit)
				if st.folder != prev {
					st.err = ""
					st.folderEdited = strings.TrimSpace(st.folder) != "" && st.folder != st.autoFolder
				}
			})

			if destination != "" {
				ui.Row(c).Gap(6).Padding(8, 10).Radius(8).Background(pal.Subtle).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, Icon("folder-git-2")).Size(14, 14).TextColor(pal.TextFaint)
					clipped(c, locales.GitCloneDestination(tilde(destination, a.Home))).
						Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).Grow(1).MinWidth(0)
				})
			}

			if st.err != "" {
				ui.Text(c, st.err).FontSize(12).TextColor(pal.WarningText)
			}

			submitLabel := locales.GitCloneAction()
			if !hasGit && canInstallGit {
				submitLabel = locales.GitCloneInstallAction()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.Cancel()).Height(controlHeight).OnClick(func() { st.open = false })
				ui.PrimaryButton(c.Key("git-clone-submit"), submitLabel).Height(controlHeight).
					Disabled(!canSubmit).OnClick(submit)
			})
		})
	})

	if !st.open {
		a.cloneForm = nil
	}
}
