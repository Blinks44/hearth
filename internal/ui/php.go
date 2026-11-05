package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// php is the PHP page: the tool's header with where it stands, the
// copies installed, how the terminal finds php, the versions
// available, the version manager, CLI configuration and global Composer
// packages.
func (a *App) php(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.toolState("php")
		if !ok {
			a.toolEmpty(c, "php", "PHP")
			return
		}
		ins := st.Installations
		rows := listed(ins)
		active := inventory.TerminalCopy(ins)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				phpHeader(c, ins, rows, active)
				toolTabs(c, "php-tabs", &a.phpTab, len(rows))
				switch a.phpTab {
				case 0:
					ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
							a.versionTable(c, ins, rows, active, a.phpChannel(c))
							toolNote(c, locales.PhpManagementNote())
							pathCard(c, "php", ins, active, a.Home)
						})
						ui.Column(c).Width(342).Shrink(0).Gap(14).Children(func() {
							a.phpAvailable(c, ins, rows, active)
							a.managerCard(c, st, rows, active, nil)
							if active >= 0 {
								a.phpConfig(c, ins[active])
							}
							a.composerCard(c)
						})
					})
				case 1:
					a.phpAvailable(c, ins, rows, active)
				case 3:
					pathCard(c, "php", ins, active, a.Home)
				default:
					comingSoon(c)
				}
			})
		})
	})
}

// phpHeader is PHP's logo, name and how its versions are managed,
// and the cards of where it stands: installed, in the terminal, in a
// project.
func phpHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	t := c.Theme()
	pal := Colors(t)
	toolHead(c, "php", "PHP", func() {}, managedBy(ins, rows, active), func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.CopiesCount(len(rows)),
			strings.Join(sourceNames(ins, rows, -1), " · "))
		if active >= 0 {
			in := ins[active]
			stateCard(c, t.Success, locales.InTerminal(), copyVersion(in), "php → "+sourceName(in.Source))
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
	})
}

// phpChannel marks versions whose security support has ended.
func (a *App) phpChannel(c *ui.Context) func(inventory.Installation) (string, bool) {
	return func(in inventory.Installation) (string, bool) {
		if a.PHPVersions != nil {
			if l, ok := a.PHPVersions.Line(in.Version); ok && !l.End.IsZero() && !c.Now().Before(l.End) {
				return "", true
			}
		}
		return "", false
	}
}

// phpAvailable lists the newest release of each line released and still
// supported, newest first, with how each stands to the copies installed.
func (a *App) phpAvailable(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	pal := Colors(c.Theme())
	asOf := ""
	if a.PHPVersions != nil {
		asOf = locales.VersionsAsOf(a.PHPVersions.Fetched)
	}
	listCard(c, locales.AvailableTitle(), asOf, func() {
		if a.PHPVersions == nil {
			text := locales.VersionsLoading()
			if a.PHPVersionsErr != "" {
				text = locales.VersionsFailed(a.PHPVersionsErr)
			}
			cardNote(c, text)
			return
		}
		now := c.Now()
		for _, l := range a.PHPVersions.Lines {
			if l.Latest == "" || !l.End.IsZero() && !now.Before(l.End) || !l.Release.IsZero() && now.Before(l.Release) {
				continue
			}
			caption, update := goReleaseNote(l.Latest, ins, rows, active)
			listRow(c, l.Latest, true, caption, func() {
				if update {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
			})
		}
	})
}

func (a *App) phpConfig(c *ui.Context, in inventory.Installation) {
	listCard(c, locales.PhpConfiguration(copyVersion(in)), "php --ini", func() {
		cfg := in.PHP
		if cfg == nil || cfg.Err != "" {
			cardNote(c, locales.PhpConfigurationFailed())
			return
		}
		if len(cfg.Files) == 0 {
			cardNote(c, locales.PhpNoIni())
		}
		for _, path := range cfg.Files {
			listRow(c, filepath.Base(path), false, tilde(path, a.Home), func() {
				if a.Shell != nil {
					iconButton(c, "external-link", locales.PhpOpenIni()).OnClick(func() {
						if err := a.Shell.OpenPath(path); err != nil {
							c.Toast(locales.OpenFailed(tilde(path, a.Home), err.Error()))
						}
					})
				}
			})
		}
		listRow(c, locales.PhpExtensions(strconv.Itoa(len(cfg.Extensions))), false, strings.Join(cfg.Extensions, ", "), func() {
			iconButton(c, "copy", locales.PhpCopyExtensions()).OnClick(func() { c.WriteClipboard(strings.Join(cfg.Extensions, "\n")); c.Toast(locales.PhpExtensionsCopied()) })
		})
	})
}

func (a *App) composerCard(c *ui.Context) {
	st, ok := a.toolState("composer")
	active := inventory.TerminalCopy(st.Installations)
	if !ok || active < 0 {
		listCard(c, "Composer", "", func() { cardNote(c, locales.ToolNotInstalled("Composer")) })
		return
	}
	in := st.Installations[active]
	globalsCard(c, "Composer · "+in.Version, locales.PhpNoComposerPackages(), in.Packages, "composer global")
}
