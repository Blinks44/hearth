package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// cloudSwitchDialog switches the active local selection for aws, gcloud, or az
// via a previewed ops.Plan.
type cloudSwitchDialog struct {
	open           bool
	tool           string
	fromName       string
	fromProject    string
	fromSubID      string
	choices        []inventory.CliChoice
	selectedName   string
	selectedSubID  string
	selectedSub    string
	region         string
	project        string
	remoteProjects []string
	loadingProj    bool
	projErr        string
	seq            int
}

// cloudLoginDialog is Frame 99: explains the native browser sign-in command
// before running it through ops.Plan (or opening a terminal for aws configure sso).
type cloudLoginDialog struct {
	open    bool
	tool    string
	bin     string
	version string
	source  string
	profile string
	config  string
	dir     string
	hasSSO  bool
}

func (a *App) openCloudSwitch(tool string, cc *inventory.CliContext) {
	d := &a.cloudSwitch
	d.seq++
	*d = cloudSwitchDialog{
		open: true,
		tool: tool,
		seq:  d.seq,
	}
	if cc != nil {
		d.fromName = cc.Name
		d.fromProject = cc.Project
		d.fromSubID = cc.SubscriptionID
		d.choices = slices.Clone(cc.Choices)
		d.selectedName = cc.Name
		d.selectedSubID = cc.SubscriptionID
		d.selectedSub = cc.Name
		d.region = cc.Region
		d.project = cc.Project
	}
	if tool == "aws" && d.selectedName == "" {
		d.selectedName = "default"
	}
	if tool == "aws" && !slices.ContainsFunc(d.choices, func(c inventory.CliChoice) bool { return c.Name == "default" }) {
		d.choices = append([]inventory.CliChoice{{Name: "default"}}, d.choices...)
	}
	if tool == "gcloud" && d.selectedName == "" {
		d.selectedName = "default"
	}
}

func (a *App) openCloudLogin(tool string, in inventory.Installation, cc *inventory.CliContext) {
	dir := cloudDefaultDir(tool, a.Home, goos)
	profile := "default"
	cfg := "default"
	hasSSO := false
	if cc != nil {
		if cc.File != "" {
			switch {
			case tool == "aws" || tool == "az":
				dir = filepath.Dir(cc.File)
			case tool == "gcloud" && filepath.Base(filepath.Dir(cc.File)) == "configurations":
				dir = filepath.Dir(filepath.Dir(cc.File))
			default:
				dir = cc.File
			}
		}
		if cc.Name != "" {
			profile = cc.Name
			cfg = cc.Name
		}
		hasSSO = cc.HasSSO
	}
	a.cloudLogin = cloudLoginDialog{
		open:    true,
		tool:    tool,
		bin:     in.Path,
		version: cmp.Or(in.Version, locales.UnknownVersion()),
		source:  sourceName(in.Source),
		profile: profile,
		config:  cfg,
		dir:     tilde(dir, a.Home),
		hasSSO:  hasSSO,
	}
}

func cloudDefaultDir(tool, home, goos string) string {
	switch tool {
	case "aws":
		if home != "" {
			return filepath.Join(home, ".aws")
		}
		return "~/.aws"
	case "gcloud":
		if goos == "windows" {
			return `%APPDATA%\gcloud`
		}
		if home != "" {
			return filepath.Join(home, ".config", "gcloud")
		}
		return "~/.config/gcloud"
	case "az":
		if home != "" {
			return filepath.Join(home, ".azure")
		}
		return "~/.azure"
	}
	return ""
}

func cloudProviderName(tool string) string {
	switch tool {
	case "aws":
		return "AWS"
	case "gcloud":
		return "Google Cloud"
	case "az":
		return "Azure"
	}
	return tool
}

func (a *App) cloudSwitchView(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	d := &a.cloudSwitch
	dialogBase(c.Key("cloud-switch"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(500).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))

		cli := a.cliState()
		in, found := cliCopy(cli, d.tool)

		selectedChoiceRegion := ""
		if d.tool == "aws" {
			wantName := strings.TrimSpace(d.selectedName)
			for _, ch := range d.choices {
				if ch.Name == wantName {
					selectedChoiceRegion = ch.Region
					break
				}
			}
		}

		var title, lead string
		canApply := a.BuildPlan != nil
		switch d.tool {
		case "aws":
			title = locales.CloudAwsSwitchTitle()
			lead = locales.CloudAwsSwitchWhy()
			toName := strings.TrimSpace(d.selectedName)
			toRegion := strings.TrimSpace(d.region)
			profileChanged := toName != "" && toName != cmp.Or(d.fromName, "default")
			regionChanged := toRegion != "" && toRegion != selectedChoiceRegion
			canApply = canApply && toName != "" && (profileChanged || (found && regionChanged))
		case "gcloud":
			title = locales.CloudGcloudSwitchTitle()
			lead = locales.CloudGcloudSwitchWhy()
			cfgChanged := strings.TrimSpace(d.selectedName) != "" && strings.TrimSpace(d.selectedName) != d.fromName
			projChanged := strings.TrimSpace(d.project) != "" && strings.TrimSpace(d.project) != d.fromProject
			canApply = canApply && found && (cfgChanged || projChanged)
		case "az":
			title = locales.CloudAzSwitchTitle()
			lead = locales.CloudAzSwitchWhy()
			target := cmp.Or(strings.TrimSpace(d.selectedSubID), strings.TrimSpace(d.selectedSub))
			canApply = canApply && found && target != "" && (d.selectedSubID != d.fromSubID || d.selectedSub != d.fromName)
		}

		apply := func() {
			if !canApply {
				return
			}
			tool := d.tool
			fromName, fromProj := d.fromName, d.fromProject
			toName := strings.TrimSpace(d.selectedName)
			region := strings.TrimSpace(d.region)
			if tool == "aws" && region == selectedChoiceRegion {
				region = ""
			}
			project := strings.TrimSpace(d.project)
			subID := strings.TrimSpace(d.selectedSubID)
			subName := strings.TrimSpace(d.selectedSub)
			bin := in.Path
			d.open = false
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				switch tool {
				case "aws":
					return ops.AWSProfilePlan(bin, fromName, toName, region, env, run.Exec{})
				case "gcloud":
					return ops.GcloudSwitchPlan(bin, fromName, toName, fromProj, project, env, run.Exec{})
				case "az":
					return ops.AzSubscriptionPlan(bin, fromName, subID, subName, env, run.Exec{})
				}
				return ops.Plan{}, nil
			}, a.proposeThen(func() {
				delete(a.cloudAuth, tool)
			}))
		}

		ui.Column(c).Children(func() {
			ui.Column(c).Gap(10).Padding(20, 20, 16, 20).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, title).FontSize(18).FontWeight(600)
					ui.Text(c, lead).FontSize(12).TextColor(t.TextMuted).Wrap()
				})

				switch d.tool {
				case "aws":
					if len(d.choices) > 0 {
						ui.Text(c, locales.CloudAwsProfileLabel()).FontSize(11.5).FontWeight(500).TextColor(t.TextMuted)
						ui.Row(c).Gap(6).Wrap().Children(func() {
							for _, ch := range d.choices {
								choice := ch
								active := d.selectedName == choice.Name
								btn := ui.Button(c.Key("aws-choice:"+choice.Name), choice.Name).Height(28)
								if active {
									btn.Background(pal.AccentSoft).BorderColor(Accent).TextColor(pal.AccentInk)
								}
								btn.OnClick(func() {
									d.selectedName = choice.Name
									d.region = choice.Region
								})
							}
						})
					}
					prof := ui.TextInput(c.Key("aws-profile-input"), &d.selectedName).Label(locales.CloudAwsProfileLabel()).Placeholder("default")
					if prof.Submitted() {
						apply()
					}
					reg := ui.TextInput(c.Key("aws-region-input"), &d.region).Label(locales.CloudAwsRegionLabel()).Placeholder("eu-central-1")
					if reg.Submitted() {
						apply()
					}

				case "gcloud":
					if len(d.choices) > 0 {
						ui.Text(c, locales.CloudGcloudConfigLabel()).FontSize(11.5).FontWeight(500).TextColor(t.TextMuted)
						ui.Row(c).Gap(6).Wrap().Children(func() {
							for _, ch := range d.choices {
								choice := ch
								active := d.selectedName == choice.Name
								btn := ui.Button(c.Key("gcloud-choice:"+choice.Name), choice.Name).Height(28)
								if active {
									btn.Background(pal.AccentSoft).BorderColor(Accent).TextColor(pal.AccentInk)
								}
								btn.OnClick(func() {
									d.selectedName = choice.Name
									d.project = choice.Project
								})
							}
						})
					}
					proj := ui.TextInput(c.Key("gcloud-project-input"), &d.project).Label(locales.CloudGcloudProjectLabel()).Placeholder("my-gcp-project")
					if proj.Submitted() {
						apply()
					}
					if a.ListGcloudProjects != nil && found {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Button(c.Key("gcloud-load-projects"), locales.CloudGcloudLoadProjects()).
								Disabled(d.loadingProj).OnClick(func() {
								d.loadingProj = true
								d.projErr = ""
								d.seq++
								seq := d.seq
								a.ListGcloudProjects(in.Path, func(list []string, err error) {
									if d.seq != seq {
										return
									}
									d.loadingProj = false
									if err != nil {
										d.projErr = err.Error()
										return
									}
									d.remoteProjects = list
								})
							})
							if d.loadingProj {
								ui.Text(c, locales.CliWhoChecking()).FontSize(11.5).TextColor(pal.TextFaint)
							}
						})
						if d.projErr != "" {
							ui.Text(c, d.projErr).FontSize(11.5).TextColor(pal.WarningText).Wrap()
						}
						if len(d.remoteProjects) > 0 {
							ui.Row(c).Gap(6).Wrap().Children(func() {
								for _, pid := range d.remoteProjects {
									id := pid
									active := d.project == id
									btn := ui.Button(c.Key("gcloud-proj:"+id), id).Height(26)
									if active {
										btn.Background(pal.AccentSoft).BorderColor(Accent).TextColor(pal.AccentInk)
									}
									btn.OnClick(func() { d.project = id })
								}
							})
						}
					}

				case "az":
					ui.Text(c, locales.CloudAzSubLabel()).FontSize(11.5).FontWeight(500).TextColor(t.TextMuted)
					if len(d.choices) == 0 {
						ui.Text(c, locales.CloudNoChoices()).FontSize(12).TextColor(pal.TextFaint).Wrap()
					} else {
						ui.Column(c).Radius(10).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
							for i, ch := range d.choices {
								choice := ch
								active := (d.selectedSubID != "" && d.selectedSubID == choice.ID) || (d.selectedSubID == "" && d.selectedSub == choice.Name)
								row := ui.Row(c.Key("az-sub:"+strconv.Itoa(i))).Gap(8).Padding(8, 12).AlignItems(ui.Center).
									BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Cursor(ui.CursorPointer)
								if active {
									row.Background(pal.ActiveRow)
								}
								row.OnClick(func() {
									d.selectedSubID = choice.ID
									d.selectedSub = choice.Name
								}).Children(func() {
									ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
										clipped(c, cmp.Or(choice.Name, choice.ID)).FontSize(13).FontWeight(500)
										if choice.ID != "" {
											clipped(c, detect.MaskID(choice.ID)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
										}
									})
									if active {
										ui.Icon(c, Icon("check")).Size(14, 14).TextColor(Accent)
									}
								})
							}
						})
					}
				}
			})

			ui.Row(c).Gap(8).Padding(12, 20).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Box(c).Grow(1).Height(1)
				ui.Button(c, locales.Cancel()).Height(controlHeight).OnClick(func() { d.open = false })
				ui.PrimaryButton(c, locales.CloudSwitchApply()).Height(controlHeight).Disabled(!canApply).OnClick(apply)
			})
		})
	})
}

// cloudLoginView renders Frame 99: explains the native sign-in command and
// launches it without ever touching passwords or storing tokens.
func (a *App) cloudLoginView(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	d := &a.cloudLogin
	dialogBase(c.Key("cloud-login"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.28))
		panel.Width(640).Radius(14).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 14, 36, 0, ui.RGBA(0, 0, 0, 0.2))

		provider := cloudProviderName(d.tool)
		step1Cmd, step2Cmd, step3Title, step3Desc := cloudLoginSteps(d)
		awsNeedsSetup := d.tool == "aws" && !d.hasSSO

		runSignIn := func() {
			tool, bin, profile, hasSSO := d.tool, d.bin, d.profile, d.hasSSO
			d.open = false
			if tool == "aws" && !hasSSO {
				if a.OpenCloudLogin != nil && bin != "" {
					a.OpenCloudLogin([]string{bin, "configure", "sso"}, func(err error) {
						if err != nil {
							a.planErr = err.Error()
						}
					})
				}
				return
			}
			if a.BuildPlan == nil {
				return
			}
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.CloudLoginPlan(tool, bin, profile, hasSSO, env, run.Exec{})
			}, func(p ops.Plan, err error) {
				if err != nil {
					a.planErr = err.Error()
					return
				}
				a.Propose(p)
				if a.op != nil && !a.op.started {
					a.op.after = func() { a.checkCloudAuth(tool, bin) }
					if a.RunPlan != nil {
						a.startOperation()
					}
				}
			})
		}

		ui.Column(c).Children(func() {
			ui.Column(c).Gap(16).Padding(22, 24, 18, 24).Children(func() {
				// Header
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, locales.CloudLoginTitle(provider)).FontSize(18).FontWeight(600)
					ui.Text(c, locales.CloudLoginLead(d.tool, d.dir)).FontSize(13).TextColor(t.TextMuted).Wrap()
				})

				// Numbered steps card (matching Frame 99)
				ui.Column(c).Radius(10).Clip().Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
					cloudLoginStepRow(c, 1, step1Cmd, true, locales.CloudLoginStep1Desc(d.dir), false)
					cloudLoginStepRow(c, 2, step2Cmd, true, locales.CloudLoginStep2Desc(d.tool), true)
					cloudLoginStepRow(c, 3, step3Title, false, step3Desc, true)
				})

				// Banner (blue info banner for browser login, warning for non-SSO AWS)
				bannerBg := pal.InfoSoft
				bannerColor := pal.InfoText
				bannerIcon := "info"
				bannerText := locales.CloudLoginBanner()
				if awsNeedsSetup {
					bannerBg = pal.WarningSoft
					bannerColor = pal.WarningText
					bannerIcon = "triangle-alert"
					bannerText = locales.CloudLoginAwsNoSso(d.profile)
				}
				ui.Row(c).Gap(10).Padding(10, 12).Radius(8).Background(bannerBg).AlignItems(ui.Start).Children(func() {
					ui.Icon(c, Icon(bannerIcon)).Size(16, 16).TextColor(bannerColor).Margin(1, 0, 0, 0).Shrink(0)
					ui.Text(c, bannerText).FontSize(12).TextColor(bannerColor).Grow(1).MinWidth(0).Wrap()
				})
			})

			// Footer: mono tool/version/source on left, Cancel + primary button on right
			ui.Row(c).Gap(10).Padding(14, 24).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				meta := d.tool + " " + d.version
				if d.source != "" {
					meta += " · " + d.source
				}
				clipped(c, meta).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
				ui.Button(c.Key("cloud-login-cancel"), locales.Cancel()).Height(controlHeight).OnClick(func() { d.open = false })

				if awsNeedsSetup {
					if a.OpenCloudLogin != nil {
						ui.PrimaryButton(c.Key("cloud-login-terminal"), "").Height(controlHeight).
							Label(locales.CloudLoginOpenTerminal()).OnClick(runSignIn).Children(func() {
							ui.Icon(c, Icon("terminal")).Size(14, 14)
							ui.Text(c, locales.CloudLoginOpenTerminal()).FontSize(12.5).FontWeight(500).SingleLine()
						})
					}
				} else if a.BuildPlan != nil {
					ui.PrimaryButton(c.Key("cloud-login-go"), "").Height(controlHeight).
						Label(locales.CloudLoginOpenBrowser()).OnClick(runSignIn).Children(func() {
						ui.Icon(c, Icon("external-link")).Size(14, 14)
						ui.Text(c, locales.CloudLoginOpenBrowser()).FontSize(12.5).FontWeight(500).SingleLine()
					})
				}
			})
		})
	})
}

func cloudLoginStepRow(c *ui.Context, num int, title string, mono bool, desc string, topBorder bool) {
	t := c.Theme()
	pal := Colors(t)
	row := ui.Row(c).Gap(12).Padding(12, 16).AlignItems(ui.Start)
	if topBorder {
		row.BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	}
	row.Children(func() {
		ui.Box(c).Size(22, 22).Radius(11).Background(pal.Muted).Center().Shrink(0).Margin(1, 0, 0, 0).Children(func() {
			ui.Text(c, strconv.Itoa(num)).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			txt := ui.Text(c, title).FontSize(13).FontWeight(500)
			if mono {
				txt.Font(FontMono)
			}
			txt.Wrap()
			ui.Text(c, desc).FontSize(12).TextColor(pal.TextFaint).Wrap()
		})
	})
}

func cloudLoginSteps(d *cloudLoginDialog) (step1, step2, step3Title, step3Desc string) {
	switch d.tool {
	case "gcloud":
		return "gcloud auth login",
			"gcloud auth list",
			locales.CloudLoginStep3TitleGcloud(),
			locales.CloudLoginStep3DescGcloud(cmp.Or(d.config, "default"))
	case "az":
		return "az login",
			"az ad signed-in-user show",
			locales.CloudLoginStep3TitleAz(),
			locales.CloudLoginStep3DescAz()
	case "aws":
		cmd := "aws sso login"
		if !d.hasSSO {
			cmd = "aws configure sso"
		} else if d.profile != "" && d.profile != "default" {
			cmd = "aws sso login --profile " + d.profile
		}
		return cmd,
			"aws sts get-caller-identity",
			locales.CloudLoginStep3TitleAws(),
			locales.CloudLoginStep3DescAws(cmp.Or(d.profile, "default"))
	}
	return d.tool, d.tool, "", ""
}
