package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// cliPage is "CLI & clouds": cloud CLIs, infrastructure tools and mise tools
// without a page of their own.
const cliPage = "/tools/cli"

// cliGroups are the table's groups of known commands, in its order;
// Kubernetes' have their page.
var cliGroups = []struct {
	title    func() string
	commands []string
}{
	{locales.CliGroupClouds, []string{"aws", "gcloud", "az"}},
	{locales.CliGroupInfra, []string{"terraform", "tofu"}},
}

// cloudCommands are the cloud provider CLIs shown in the Clouds group.
var cloudCommands = []string{"aws", "gcloud", "az"}

// cliProducts name what a missing command comes with; mise's copies of
// them are left to their pages.
var cliProducts = map[string]string{
	"kubectl": "Kubernetes", "helm": "Helm", "kind": "kind",
	"aws": "AWS CLI", "gcloud": "Google Cloud CLI", "az": "Azure CLI",
	"terraform": "Terraform", "tofu": "OpenTofu",
}

// cliTabs are the page's tabs after All: the groups by cliGroups' index,
// then mise's (-1).
var cliTabs = []int{0, 1, -1}

// cloudAuthEntry stores an in-memory on-demand provider auth check result.
type cloudAuthEntry struct {
	checking  bool
	checked   bool
	res       detect.CloudAuth
	checkedAt time.Time
	seq       int
}

// cliInfraReq summarizes one added project's Terraform or OpenTofu requirement.
type cliInfraReq struct {
	projectName string
	projectDir  string
	tool        string
	toolTitle   string
	want        string
	file        string
	installed   string
	met         bool
	err         string
}

func isCLICommand(id string) bool {
	for _, g := range cliGroups {
		if slices.Contains(g.commands, id) {
			return true
		}
	}
	return false
}

// cliState is what the CLI detector found; empty before a scan.
func (a *App) cliState() inventory.CliState {
	if st, _ := a.Snapshot.Tool("cli"); st.Cli != nil {
		return *st.Cli
	}
	return inventory.CliState{}
}

// cliCopy is the copy of a command the terminal runs, else the first the
// apps find; ok is false when none was found.
func cliCopy(cli inventory.CliState, command string) (inventory.Installation, bool) {
	var ins []inventory.Installation
	for _, in := range cli.Tools {
		if in.Tool == command {
			ins = append(ins, in)
		}
	}
	if len(ins) == 0 {
		return inventory.Installation{}, false
	}
	if i := inventory.TerminalCopy(ins); i >= 0 {
		return ins[i], true
	}
	return ins[0], true
}

// cliFound counts the commands of a group found.
func cliFound(cli inventory.CliState, commands []string) int {
	n := 0
	for _, cmd := range commands {
		if _, ok := cliCopy(cli, cmd); ok {
			n++
		}
	}
	return n
}

// cliCount is the sidebar's number: the commands found and mise's tools.
func (a *App) cliCount() int {
	n := len(a.miseTools())
	for _, g := range cliGroups {
		n += cliFound(a.cliState(), g.commands)
	}
	return n
}

// miseTools are the tools mise installed without a page of their own;
// the table's own commands (including opentofu, shown as tofu) are left to
// their groups.
func (a *App) miseTools() []inventory.MiseTool {
	st, _ := a.Snapshot.Tool("mise")
	return slices.DeleteFunc(slices.Clone(st.MiseTools), func(mt inventory.MiseTool) bool {
		if mt.Name == "opentofu" {
			return true
		}
		_, known := cliProducts[mt.Name]
		return known
	})
}

// cliContext is the current context of a tool, nil when none was read.
func cliContext(cli inventory.CliState, tool string) *inventory.CliContext {
	for i := range cli.Contexts {
		if cli.Contexts[i].Tool == tool {
			return &cli.Contexts[i]
		}
	}
	return nil
}

// cliContextText says where a tool points in a concise one-line form.
func cliContextText(cc inventory.CliContext) string {
	switch cc.Tool {
	case "kubectl":
		return locales.CliContext(cc.Name)
	case "aws":
		var parts []string
		if cc.Name != "" {
			parts = append(parts, locales.CliProfile(cc.Name))
		} else {
			parts = append(parts, locales.CliTargetNoProfile())
		}
		if cc.Region != "" {
			parts = append(parts, cc.Region)
		}
		return strings.Join(parts, " · ")
	case "gcloud":
		var parts []string
		if cc.Name != "" {
			parts = append(parts, cc.Name)
		}
		if cc.Project != "" {
			parts = append(parts, locales.CliProject(cc.Project))
		} else {
			parts = append(parts, locales.CliNoProject())
		}
		return strings.Join(parts, " · ")
	case "az":
		if cc.Name == "" {
			return locales.CliNoSubscription()
		}
		return locales.CliSubscription(cc.Name)
	}
	return cc.Name
}

// cliIssueText describes a missing, invalid, or ambiguous cloud context issue.
func cliIssueText(cc inventory.CliContext) string {
	switch cc.Issue {
	case "config_missing":
		return locales.CliIssueConfigMissing()
	case "profile_missing":
		return locales.CliIssueProfileMissing()
	case "config_invalid":
		return locales.CliIssueConfigInvalid()
	case "env_creds":
		return locales.CliIssueEnvCreds()
	case "profile_conflict":
		return locales.CliIssueProfileConflict()
	case "region_conflict":
		return locales.CliIssueRegionConflict()
	case "active_config_missing":
		return locales.CliIssueActiveConfigMissing()
	case "project_override":
		return locales.CliIssueProjectOverride(cmp.Or(cc.ProjectSource, "CLOUDSDK_CORE_PROJECT"))
	case "multiple_defaults":
		return locales.CliIssueMultipleDefaults()
	}
	return ""
}

// cliInfraReqs collects Terraform and OpenTofu requirements across added projects.
func (a *App) cliInfraReqs() []cliInfraReq {
	if a.Snapshot == nil || len(a.Projects) == 0 {
		return nil
	}
	var out []cliInfraReq
	for _, p := range a.Projects {
		statuses := projects.Resolve(p, a.Snapshot, nil)
		for _, st := range statuses {
			if st.Tool != "terraform" && st.Tool != "tofu" {
				continue
			}
			if len(st.Checks) == 0 {
				continue
			}
			chosen := st.Checks[0]
			if unmet, ok := st.Unmet(); ok {
				chosen = unmet
			} else {
				for _, ch := range st.Checks {
					if ch.Err != "" {
						chosen = ch
						break
					}
					if ch.Want != "" {
						chosen = ch
						break
					}
				}
			}
			installed := ""
			if st.Terminal != nil {
				installed = st.Terminal.Version
			}
			out = append(out, cliInfraReq{
				projectName: filepath.Base(p.Dir),
				projectDir:  p.Dir,
				tool:        st.Tool,
				toolTitle:   cliProducts[st.Tool],
				want:        chosen.Want,
				file:        chosen.File,
				installed:   installed,
				met:         chosen.Met,
				err:         chosen.Err,
			})
		}
	}
	return out
}

func (a *App) cliView(c *ui.Context, p page) {
	cli := a.cliState()
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			a.cliInstallMenu(c, cli)
		}})
		a.cloudSwitchView(c)
		a.cloudLoginView(c)
		t := c.Theme()
		pal := Colors(t)
		tools := a.miseTools()
		known := len(tools)
		for _, g := range cliGroups {
			known += len(g.commands)
		}
		infraReqs := a.cliInfraReqs()

		// Sign-in state card metrics (on-demand checks only).
		cloudFound := 0
		var verifiedNames, unverifiedNames []string
		anyAuthChecked := false
		for _, cmd := range cloudCommands {
			if _, ok := cliCopy(cli, cmd); !ok {
				continue
			}
			cloudFound++
			if entry, ok := a.cloudAuth[cmd]; ok && entry.checked {
				anyAuthChecked = true
				if entry.res.State == detect.CloudAuthVerified {
					verifiedNames = append(verifiedNames, cmd)
				} else {
					unverifiedNames = append(unverifiedNames, cmd)
				}
			}
		}

		// Project requirements state card metrics.
		var unmetReqs []cliInfraReq
		for _, r := range infraReqs {
			if !r.met && r.err == "" {
				unmetReqs = append(unmetReqs, r)
			}
		}

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						ui.Icon(c, Icon("square-terminal")).Size(24, 24).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.CliLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.CleanupFound(), strconv.Itoa(a.cliCount()), locales.CliFoundCaption(known))

					authDot := pal.TextFaint
					authCaption := locales.CliAuthNoneFound()
					if cloudFound > 0 {
						switch {
						case !anyAuthChecked:
							authCaption = locales.CliAuthNotChecked()
						case len(verifiedNames) == cloudFound:
							authDot = t.Success
							authCaption = strings.Join(verifiedNames, " · ")
						case len(verifiedNames) > 0:
							authDot = t.Warning
							var missing []string
							for _, cmd := range cloudCommands {
								if _, ok := cliCopy(cli, cmd); ok && !slices.Contains(verifiedNames, cmd) {
									missing = append(missing, cmd)
								}
							}
							authCaption = locales.CliAuthSomeMissing(strings.Join(verifiedNames, " · "), strings.Join(missing, " · "))
						default:
							authDot = t.Warning
							authCaption = strings.Join(unverifiedNames, " · ") + " · " + locales.CliWhoBadgeNotSignedIn()
						}
					}
					stateCard(c, authDot, locales.CliAuthCard(), locales.CliAuthCount(len(verifiedNames), cloudFound), authCaption)

					projDot := pal.TextFaint
					projCaption := locales.CliProjectsNone()
					if len(unmetReqs) > 0 {
						projDot = pal.DangerText
						var labels []string
						for _, r := range unmetReqs {
							lbl := r.toolTitle
							if r.want != "" {
								lbl += " " + r.want
							}
							if !slices.Contains(labels, lbl) {
								labels = append(labels, lbl)
							}
							if len(labels) == 2 {
								break
							}
						}
						projCaption = strings.Join(labels, " · ")
					} else if len(infraReqs) > 0 {
						projDot = t.Success
						projCaption = locales.CliProjectsAllMet()
					}
					stateCard(c, projDot, locales.CliProjectsCard(), strconv.Itoa(len(unmetReqs)), projCaption)
				})
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segs := []segment{{locales.TabAll(), strconv.Itoa(a.cliCount())}}
					for _, g := range cliTabs {
						if g < 0 {
							segs = append(segs, segment{locales.CliFromMise(), strconv.Itoa(len(tools))})
						} else {
							segs = append(segs, segment{cliGroups[g].title(), strconv.Itoa(cliFound(cli, cliGroups[g].commands))})
						}
					}
					segments(c, "cli-tabs", &a.cliTab, segs)
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("cli-filter"), &a.cliFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Height(controlHeight).Width(200)
				})
				columns(c, 12, func() {
					if a.Snapshot == nil {
						toolNote(c, locales.CleanupWaiting())
						return
					}
					a.cliTable(c, cli, tools, infraReqs)
				}, func() {
					if a.Snapshot == nil {
						return
					}
					a.cliTargetCard(c, cli)
					if cloudFound > 0 {
						a.cliWhoCard(c, cli)
					}
					a.cliInfraCard(c, infraReqs)
				})
			})
		})
	})
}

// cliInstallMenu renders the top-right "Install…" menu for CLIs on this page.
func (a *App) cliInstallMenu(c *ui.Context, cli inventory.CliState) {
	if a.Snapshot == nil || a.BuildPlan == nil {
		return
	}
	in := a.setupHere()
	var ids []string
	for _, g := range cliGroups {
		ids = append(ids, g.commands...)
	}
	hasOffer := false
	for _, id := range ids {
		if e, ok := catalog.Find(id); ok {
			if _, found := cliCopy(cli, id); found || slices.ContainsFunc(e.On(goos), in.Usable) {
				hasOffer = true
				break
			}
		}
	}
	if !hasOffer {
		return
	}
	ui.PrimaryButton(c.Key("cli-install"), locales.CliInstallMenu()).Height(controlHeight).Shrink(0).PopoverMenuButton(func(m *ui.Menu) {
		for _, id := range ids {
			e, ok := catalog.Find(id)
			if !ok {
				continue
			}
			title := setupTitle(id)
			item := m.Item(title).ID(id)
			if copyIn, found := cliCopy(cli, id); found {
				item.Disabled(true).Trailing(locales.InstalledVersion(cmp.Or(copyIn.Version, locales.UnknownVersion())))
				continue
			}
			ms := e.On(goos)
			i := slices.IndexFunc(ms, in.Usable)
			if i < 0 {
				item.Disabled(true)
				continue
			}
			item.Subtitle(locales.SetupVia(managerTitle(ms[i])))
			if item.Trailing(locales.AgentInstall()).Chosen() {
				a.installTool(id)
			}
		}
	})
}

func cliConfigOpenable(cmd string, cc *inventory.CliContext) string {
	if cc == nil || cc.File == "" || cc.Issue == "config_missing" || cc.Issue == "active_config_missing" {
		return ""
	}
	if cmd == "aws" && cc.NameSource != "file" && len(cc.Choices) == 0 && cc.Issue != "config_invalid" {
		return ""
	}
	return cc.File
}

// cliTargetCard renders the "Where commands go" card for AWS, gcloud, and Azure.
func (a *App) cliTargetCard(c *ui.Context, cli inventory.CliState) {
	pal := Colors(c.Theme())
	type targetRow struct {
		cmd   string
		found bool
		cc    *inventory.CliContext
	}
	var rows []targetRow
	for _, cmd := range cloudCommands {
		_, found := cliCopy(cli, cmd)
		cc := cliContext(cli, cmd)
		if !found && cc == nil {
			continue
		}
		rows = append(rows, targetRow{cmd: cmd, found: found, cc: cc})
	}
	listCard(c, locales.CliTargetTitle(), "", func() {
		if len(rows) == 0 {
			cardNote(c, locales.CliTargetEmpty())
			return
		}
		for _, r := range rows {
			title, caption, warn, danger, chooseLabel := a.cliTargetRowText(r.cmd, r.cc)
			var captionColor ui.Color
			switch {
			case danger:
				captionColor = pal.DangerText
			case warn:
				captionColor = pal.WarningText
			default:
				captionColor = pal.TextFaint
			}
			canSwitch := a.BuildPlan != nil && (r.cmd == "aws" || r.found)
			cmd, cc := r.cmd, r.cc
			cfgFile := cliConfigOpenable(cmd, cc)
			ui.Row(c.Key("target:"+cmd)).Gap(8).MinHeight(46).Padding(8, 12, 8, 14).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					clipped(c, title).FontSize(13).FontWeight(500)
					if caption != "" {
						clipped(c, caption).FontSize(11.5).TextColor(captionColor)
					}
				})
				if cfgFile != "" && a.canOpenFile() {
					iconButton(c.Key("open-config:"+cmd), "external-link", locales.OpenInEditor()).OnClick(func() {
						a.openFile(c, cfgFile)
					})
				}
				if canSwitch {
					ui.Button(c.Key("switch:"+cmd), chooseLabel).Shrink(0).OnClick(func() {
						a.openCloudSwitch(cmd, cc)
					})
				}
			})
		}
	})
}

func (a *App) cliTargetRowText(cmd string, cc *inventory.CliContext) (title, caption string, warn, danger bool, buttonLabel string) {
	buttonLabel = locales.CliTargetSwitch()
	if cc == nil {
		switch cmd {
		case "aws":
			return "aws · " + locales.CliTargetNoProfile(), locales.CliIssueConfigMissing(), true, false, locales.CliTargetChoose()
		case "gcloud":
			return "gcloud · " + locales.CliNoProject(), locales.CliIssueConfigMissing(), true, false, locales.CliTargetChoose()
		default:
			return "az · " + locales.CliNoSubscription(), locales.CliIssueConfigMissing(), true, false, locales.CliTargetChoose()
		}
	}
	var namePart string
	switch cmd {
	case "aws":
		namePart = cmp.Or(cc.Name, locales.CliTargetNoProfile())
		if cc.Name == "" {
			buttonLabel = locales.CliTargetChoose()
		}
	case "gcloud":
		namePart = cmp.Or(cc.Name, "default")
		if cc.Project == "" {
			buttonLabel = locales.CliTargetChoose()
		}
	case "az":
		if cc.Name != "" {
			namePart = cc.Name
		} else {
			namePart = locales.CliNoSubscription()
			buttonLabel = locales.CliTargetChoose()
		}
	}
	title = cmd + " · " + namePart

	var parts []string
	switch cc.NameSource {
	case "", "file":
		if cc.File != "" {
			shown := cc.File
			if cmd == "gcloud" && filepath.Base(filepath.Dir(shown)) == "configurations" {
				shown = filepath.Dir(filepath.Dir(shown))
			}
			parts = append(parts, locales.CliContextFrom(tilde(shown, a.Home)))
		}
	case "default":
		parts = append(parts, locales.CliTargetFromDefault())
	default:
		parts = append(parts, locales.CliTargetFromEnv(cc.NameSource))
	}

	switch cmd {
	case "aws":
		if cc.Region != "" {
			r := cc.Region
			if cc.RegionSource != "" && cc.RegionSource != "file" {
				r += " (" + cc.RegionSource + ")"
			}
			parts = append(parts, r)
		}
	case "gcloud":
		if cc.Project != "" {
			parts = append(parts, cc.Project)
		} else {
			parts = append(parts, locales.CliNoProject())
		}
		if cc.Region != "" {
			parts = append(parts, cc.Region)
		}
	case "az":
		if cc.SubscriptionID != "" {
			parts = append(parts, detect.MaskID(cc.SubscriptionID))
		}
	}

	if cc.ConfigEnv != "" {
		parts = append(parts, locales.CliTargetConfigEnv(cc.ConfigEnv))
	}
	if issue := cliIssueText(*cc); issue != "" {
		parts = append(parts, issue)
	}

	switch cc.Status {
	case "invalid":
		danger = true
	case "missing", "ambiguous":
		warn = true
	}
	caption = strings.Join(parts, " · ")
	return title, caption, warn, danger, buttonLabel
}

// cliWhoCard renders "Who the provider sees" with explicit on-demand auth
// verification and native browser sign-in trigger.
func (a *App) cliWhoCard(c *ui.Context, cli inventory.CliState) {
	t := c.Theme()
	pal := Colors(t)
	anyChecked := false
	anyChecking := false
	listCard(c, locales.CliWhoTitle(), "", func() {
		for _, cmd := range cloudCommands {
			in, found := cliCopy(cli, cmd)
			if !found {
				continue
			}
			entry := a.cloudAuth[cmd]
			if entry.checked {
				anyChecked = true
			}
			if entry.checking {
				anyChecking = true
			}
			var title, caption string
			titleColor := t.Text
			captionColor := pal.TextFaint
			switch {
			case entry.checking:
				title = cmd + ": " + locales.CliWhoChecking()
			case !entry.checked:
				title = cmd + ": " + locales.CliWhoNotChecked()
			case entry.res.State == detect.CloudAuthVerified:
				title = cmd + ": " + cmp.Or(entry.res.Identity, locales.CliWhoBadgeVerified())
				if entry.res.Account != "" {
					caption = locales.CliWhoVerifiedCaption(ago(c, entry.checkedAt), entry.res.Account)
				} else {
					caption = locales.CliWhoVerifiedNoAccount(ago(c, entry.checkedAt))
				}
			case entry.res.State == detect.CloudAuthNotLoggedIn:
				title = cmd + ": " + locales.CliWhoNotLoggedIn()
				caption = locales.CliWhoNotLoggedInCaption()
				titleColor = pal.WarningText
			case entry.res.State == detect.CloudAuthExpired:
				title = cmd + ": " + locales.CliWhoExpired()
				caption = cmp.Or(entry.res.Detail, locales.CliWhoNotLoggedInCaption())
				titleColor = pal.WarningText
			case entry.res.State == detect.CloudAuthNetwork:
				title = cmd + ": " + locales.CliWhoNetwork()
				caption = cmp.Or(entry.res.Detail, locales.CliWhoNotLoggedInCaption())
				titleColor = pal.WarningText
			default:
				title = cmd + ": " + locales.CliWhoUnknown()
				caption = cmp.Or(entry.res.Detail, locales.CliWhoNotLoggedInCaption())
				titleColor = pal.WarningText
			}

			cc := cliContext(cli, cmd)
			tool, bin := cmd, in.Path
			ui.Row(c.Key("who:"+tool)).Gap(8).MinHeight(46).Padding(8, 12, 8, 14).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					clipped(c, title).FontSize(13).FontWeight(500).TextColor(titleColor)
					if caption != "" {
						clipped(c, caption).FontSize(11.5).TextColor(captionColor)
					}
				})
				switch {
				case entry.checking:
					// Busy state shown in title; no competing button.
				case !entry.checked:
					if a.CheckCloudAuth != nil {
						ui.Button(c.Key("check:"+tool), locales.CliWhoCheck()).Shrink(0).OnClick(func() {
							a.checkCloudAuth(tool, bin)
						})
					}
				case entry.res.State != detect.CloudAuthVerified:
					ui.Button(c.Key("login:"+tool), locales.CliWhoSignIn()).Shrink(0).OnClick(func() {
						a.openCloudLogin(tool, in, cc)
					})
				}
			})
		}

		// Footer row: check all installed cloud CLIs on demand.
		btnLabel := locales.KubeCheckAll()
		if anyChecked {
			btnLabel = locales.CliWhoCheckAgain()
		}
		footer := ui.Row(c.Key("who-check-all")).Gap(8).MinHeight(46).Padding(8, 12, 8, 14).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
		footer.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				clipped(c, btnLabel).FontSize(13).FontWeight(500)
				clipped(c, locales.CliWhoNotCheckedCaption()).FontSize(11.5).TextColor(pal.TextFaint)
			})
			if a.CheckCloudAuth != nil {
				iconButton(c.Key("who-check-btn"), "refresh-cw", btnLabel).Disabled(anyChecking).OnClick(func() {
					a.checkAllCloudAuth(cli)
				})
			}
		})
	})
}

// cliInfraCard renders the "Needed by projects" card for Terraform and OpenTofu.
func (a *App) cliInfraCard(c *ui.Context, reqs []cliInfraReq) {
	pal := Colors(c.Theme())
	listCard(c, locales.CliInfraCardTitle(), "", func() {
		if len(reqs) == 0 {
			cardNote(c, locales.CliInfraCardEmpty())
		}
		for i, r := range reqs {
			title := r.projectName + ": " + r.toolTitle
			if r.want != "" {
				title += " " + r.want
			}
			var caption string
			captionColor := pal.TextFaint
			switch {
			case r.err != "":
				caption = locales.CliInfraInvalidConstraint()
				captionColor = pal.WarningText
			case r.installed == "":
				caption = locales.CliInfraNotInstalled()
				captionColor = pal.WarningText
			case r.met:
				caption = locales.CliInfraMet(r.installed)
			default:
				caption = locales.CliInfraMismatch(r.installed)
				captionColor = pal.WarningText
			}
			req := r
			ui.Row(c.Key("infra-req:"+strconv.Itoa(i)+":"+req.projectDir+":"+req.tool)).Gap(8).MinHeight(46).Padding(8, 12, 8, 14).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					clipped(c, title).FontSize(13).FontWeight(500)
					clipped(c, caption).FontSize(11.5).TextColor(captionColor)
				})
				if !req.met && req.err == "" && a.BuildPlan != nil {
					spec := ops.MiseSpecForConstraint(req.want)
					ui.Button(c.Key("infra-install:"+strconv.Itoa(i)), locales.CliInfraInstallVersion(spec)).Shrink(0).OnClick(func() {
						a.installInfraVersion(req.tool, req.toolTitle, req.want)
					})
				}
			})
		}
		listRow(c, locales.CliInfraNoExec(), false, locales.CliInfraNoExecCaption(), func() {})
	})
}

// ClearCloudAuth drops cached per-provider cloud auth check results, e.g.
// after a full rescan when external CLI state may have changed.
func (a *App) ClearCloudAuth() {
	a.cloudAuth = nil
}

func (a *App) checkCloudAuth(tool, bin string) {
	if a.CheckCloudAuth == nil || bin == "" {
		return
	}
	if a.cloudAuth == nil {
		a.cloudAuth = map[string]cloudAuthEntry{}
	}
	entry := a.cloudAuth[tool]
	entry.checking = true
	entry.seq++
	seq := entry.seq
	a.cloudAuth[tool] = entry
	a.CheckCloudAuth(tool, bin, func(res detect.CloudAuth) {
		cur := a.cloudAuth[tool]
		if cur.seq != seq {
			return
		}
		a.cloudAuth[tool] = cloudAuthEntry{
			checking:  false,
			checked:   true,
			res:       res,
			checkedAt: time.Now(),
			seq:       seq,
		}
	})
}

func (a *App) checkAllCloudAuth(cli inventory.CliState) {
	for _, cmd := range cloudCommands {
		if in, ok := cliCopy(cli, cmd); ok {
			a.checkCloudAuth(cmd, in.Path)
		}
	}
}

func (a *App) installInfraVersion(tool, title, want string) {
	if a.BuildPlan == nil {
		return
	}
	setup := a.setupHere()
	detectFn := a.detect("cli")
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.InfraVersionPlan(tool, title, want, setup, env, detectFn)
	}, a.proposeOrSay)
}

// cliTable is the table of the chosen tab: the known commands by group,
// found or not, then mise's tools.
func (a *App) cliTable(c *ui.Context, cli inventory.CliState, tools []inventory.MiseTool, infraReqs []cliInfraReq) {
	pal := Colors(c.Theme())
	filter := strings.ToLower(strings.TrimSpace(a.cliFilter))
	match := func(name string) bool {
		if filter == "" {
			return true
		}
		if strings.Contains(strings.ToLower(name), filter) {
			return true
		}
		if product := cliProducts[name]; product != "" && strings.Contains(strings.ToLower(product), filter) {
			return true
		}
		return false
	}
	tab := -2 // All
	if a.cliTab > 0 && a.cliTab <= len(cliTabs) {
		tab = cliTabs[a.cliTab-1]
	}
	shown := 0
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		for i, g := range cliGroups {
			if tab != -2 && tab != i {
				continue
			}
			commands := slices.DeleteFunc(slices.Clone(g.commands), func(cmd string) bool { return !match(cmd) })
			if len(commands) == 0 {
				continue
			}
			cliGroupHead(c, g.title(), cliFound(cli, commands))
			for _, cmd := range commands {
				a.cliRowWithReqs(c.Key("cli:"+cmd), cli, cmd, infraReqs)
			}
			shown += len(commands)
		}
		if tab == -2 || tab == -1 {
			rows := slices.DeleteFunc(slices.Clone(tools), func(mt inventory.MiseTool) bool { return !match(mt.Name) })
			if len(rows) > 0 {
				cliGroupHead(c, locales.CliGroupMise(), len(rows))
			}
			for _, mt := range rows {
				a.miseToolRow(c.Key("mise:"+mt.Name), mt)
			}
			shown += len(rows)
		}
		if shown == 0 {
			cardNote(c, locales.NothingMatches())
		}
	})
}

func cliGroupHead(c *ui.Context, title string, n int) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Text(c, strings.ToUpper(title)).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
		ui.Text(c, strconv.Itoa(n)).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
	})
}

// cliRow is a known command: where it came from, where it points and an
// update its package manager has, or that it was not found.
func (a *App) cliRow(c *ui.Context, cli inventory.CliState, cmd string) {
	a.cliRowWithReqs(c, cli, cmd, nil)
}

func (a *App) cliRowWithReqs(c *ui.Context, cli inventory.CliState, cmd string, infraReqs []cliInfraReq) {
	pal := Colors(c.Theme())
	in, found := cliCopy(cli, cmd)
	var parts []string
	version := "–"
	aboutColor := pal.TextFaint
	var firstUnmet *cliInfraReq
	for i := range infraReqs {
		if infraReqs[i].tool == cmd && !infraReqs[i].met && infraReqs[i].err == "" {
			firstUnmet = &infraReqs[i]
			break
		}
	}

	if found {
		parts = append(parts, sourceName(in.Source))
		if cc := cliContext(cli, cmd); cc != nil {
			parts = append(parts, cliContextText(*cc))
			if issue := cliIssueText(*cc); issue != "" {
				parts = append(parts, issue)
				switch cc.Status {
				case "invalid":
					aboutColor = pal.DangerText
				case "missing", "ambiguous":
					aboutColor = pal.WarningText
				}
			}
		}
		if cmd == "helm" && cli.HelmRepos > 0 {
			parts = append(parts, locales.CliHelmRepos(cli.HelmRepos))
		}
		if entry, ok := a.cloudAuth[cmd]; ok && entry.checked {
			if entry.res.State == detect.CloudAuthVerified {
				parts = append(parts, locales.CliWhoBadgeVerified())
			} else {
				parts = append(parts, locales.CliWhoBadgeNotSignedIn())
			}
		}
		if firstUnmet != nil {
			if firstUnmet.want != "" {
				parts = append(parts, locales.CliProjectAsks(firstUnmet.projectName, firstUnmet.want))
			} else {
				parts = append(parts, locales.CliProjectNeeds(firstUnmet.projectName))
			}
			aboutColor = pal.WarningText
		}
		if u, _ := a.packageUpdate(in); u != "" {
			parts = append(parts, u)
		}
		version = in.Version
		if version == "" {
			version = locales.UnknownVersion()
		}
	} else {
		switch {
		case firstUnmet != nil:
			parts = append(parts, locales.CliMissingNeeded(cliProducts[cmd], firstUnmet.projectName))
			aboutColor = pal.WarningText
		case isCLICommand(cmd):
			parts = append(parts, locales.CliMissingNotNeeded(cliProducts[cmd]))
		default:
			parts = append(parts, locales.CliMissing(cliProducts[cmd]))
		}
	}
	cliTableRowColored(c, cmd, strings.Join(parts, " · "), aboutColor, version, func() {
		if !found {
			a.installToolButton(c, cmd)
			return
		}
		if m, pkg, ok := a.copyPackage(in); ok {
			if a.packageCan(m, pkg) {
				iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) { a.packageItems(menu, m, pkg) })
			}
		}
	})
}

// cliTableRow is a row of the table: the command, a line about it, its
// version and its actions.
func cliTableRow(c *ui.Context, name, about, version string, actions func()) {
	cliTableRowColored(c, name, about, Colors(c.Theme()).TextFaint, version, actions)
}

func cliTableRowColored(c *ui.Context, name, about string, aboutColor ui.Color, version string, actions func()) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, name).Font(FontMono).FontSize(13)
			clipped(c, about).FontSize(10.5).TextColor(aboutColor)
		})
		ui.Text(c, version).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
		if actions != nil {
			actions()
		}
	})
}

// miseToolRow is a tool of mise's: its name, where it stands and its other
// versions, and the version that runs, else the default, else the newest.
func (a *App) miseToolRow(c *ui.Context, mt inventory.MiseTool) {
	shown := 0
	for i, in := range mt.Installations {
		if in.OnPath > 0 {
			shown = i
			break
		}
		if in.Default {
			shown = i
		}
	}
	in := mt.Installations[shown]
	parts := []string{"mise"}
	switch {
	case in.OnPath > 0:
		parts = append(parts, locales.ChipInPath())
	case in.Default:
		parts = append(parts, locales.CliDefault())
	default:
		parts = append(parts, locales.CliNotSet())
	}
	if n := len(mt.Installations) - 1; n > 0 {
		var others []string
		for i, o := range mt.Installations {
			if i != shown {
				others = append(others, o.Version)
			}
		}
		parts = append(parts, locales.CliMoreVersions(n, strings.Join(others, ", ")))
	}
	cliTableRow(c, mt.Name, strings.Join(parts, " · "), in.Version, nil)
}
