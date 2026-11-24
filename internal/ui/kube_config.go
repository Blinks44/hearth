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

// kubeConfigDialog creates a new kubeconfig file from a starter template or
// pasted YAML, proposing a confirmed plan with 0600 permissions and masked
// secrets.
type kubeConfigDialog struct {
	open     bool
	path     string
	mode     int // 0 = starter template, 1 = paste YAML
	template string
	pasted   string

	// Cached parse result for the active text to avoid reparsing YAML on
	// unrelated frames.
	cachedText    string
	cachedSet     bool
	cachedSummary ops.KubeConfigSummary
	cachedErr     error
}

func (d *kubeConfigDialog) parse(text string) (ops.KubeConfigSummary, error) {
	if d.cachedSet && d.cachedText == text {
		return d.cachedSummary, d.cachedErr
	}
	d.cachedText = text
	d.cachedSet = true
	d.cachedSummary, d.cachedErr = ops.ParseKubeConfigYAML(text)
	return d.cachedSummary, d.cachedErr
}

// openKubeConfigCreate opens the create-kubeconfig dialog targeting path.
func (a *App) openKubeConfigCreate(path string) {
	a.kubeConfig = kubeConfigDialog{
		open:     true,
		path:     path,
		mode:     0,
		template: ops.KubeConfigTemplate(),
	}
}

// kubeConfigCreateDialog renders the modal for creating a new kubeconfig file.
func (a *App) kubeConfigCreateDialog(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	d := &a.kubeConfig
	dialogBase(c.Key("kube-config-create"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(600).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		textPtr := &d.template
		areaKey := "kube-config-template"
		if d.mode == 1 {
			textPtr = &d.pasted
			areaKey = "kube-config-paste"
		}
		summary, parseErr := d.parse(*textPtr)
		ok := a.BuildPlan != nil && d.path != "" && parseErr == nil
		next := func() {
			if !ok {
				return
			}
			path, text := d.path, *textPtr
			d.open = false
			a.BuildPlan(func(_ sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.KubeConfigCreatePlan(path, tilde(path, a.Home), text)
			}, a.proposeOrSay)
		}
		ui.Column(c).Children(func() {
			ui.Column(c).Gap(4).Padding(20, 20, 12, 20).Children(func() {
				ui.Text(c, locales.KubeConfigDialogTitle()).FontSize(18).FontWeight(600)
				ui.Text(c, locales.KubeConfigDialogLead()).FontSize(12).TextColor(t.TextMuted)
			})
			ui.Column(c).Gap(10).Padding(0, 20, 16, 20).Children(func() {
				ui.Row(c).Gap(8).Padding(6, 10).Radius(7).AlignItems(ui.Center).
					BorderWidth(1).BorderColor(pal.ControlBorder).Children(func() {
					ui.Icon(c, Icon("file-text")).Size(14, 14).TextColor(t.TextMuted)
					clipped(c, tilde(d.path, a.Home)).Font(FontMono).FontSize(12)
				})
				segments(c, "kube-config-mode", &d.mode, []segment{
					{locales.KubeConfigModeTemplate(), ""},
					{locales.KubeConfigModePaste(), ""},
				})
				area := ui.TextArea(c.Key(areaKey), textPtr).Height(220).Font(FontMono).Label(locales.KubeConfigDialogTitle())
				if d.mode == 1 {
					area.Placeholder(locales.KubeConfigPastePlaceholder())
				}
				switch {
				case parseErr != nil && d.mode == 1 && strings.TrimSpace(*textPtr) == "":
					ui.Text(c, locales.KubeConfigInvalidEmpty()).FontSize(12).TextColor(t.TextMuted)
				case parseErr != nil:
					ui.Text(c, parseErr.Error()).FontSize(12).TextColor(pal.WarningText)
				case len(summary.Contexts) > 0:
					names := strings.Join(summary.Contexts, ", ")
					ui.Text(c, locales.KubeConfigParsedSummary(len(summary.Contexts), names)).FontSize(12).TextColor(pal.SuccessText)
				case d.mode == 0:
					ui.Text(c, locales.KubeConfigTemplateHint()).FontSize(12).TextColor(t.TextMuted)
				default:
					ui.Text(c, locales.KubeConfigParsedNoContexts()).FontSize(12).TextColor(t.TextMuted)
				}
			})
			ui.Row(c).Gap(8).Padding(12, 20).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Box(c).Grow(1).Height(1)
				ui.Button(c, locales.Cancel()).Height(controlHeight).OnClick(func() { d.open = false })
				ui.PrimaryButton(c.Key("kube-config-next"), locales.Next()).Height(controlHeight).Disabled(!ok).OnClick(next)
			})
		})
	})
	if !d.open && (d.pasted != "" || d.cachedText != "" || d.cachedSet) {
		d.pasted = ""
		d.cachedText = ""
		d.cachedSet = false
		d.cachedSummary = ops.KubeConfigSummary{}
		d.cachedErr = nil
	}
}

type kubeCloudHint struct {
	tool, cmd string
}

func kubeCloudHints(cli inventory.CliState) []kubeCloudHint {
	candidates := []kubeCloudHint{
		{tool: "aws", cmd: "aws eks update-kubeconfig --name <cluster>"},
		{tool: "gcloud", cmd: "gcloud container clusters get-credentials <cluster>"},
		{tool: "az", cmd: "az aks get-credentials -g <group> -n <cluster>"},
	}
	var out []kubeCloudHint
	for _, h := range candidates {
		if _, ok := cliCopy(cli, h.tool); ok {
			out = append(out, h)
		}
	}
	return out
}

// kubeEmptyState renders an actionable empty card when kubeconfig has no
// contexts, distinguishing missing files, invalid YAML, and empty valid files.
func (a *App) kubeEmptyState(c *ui.Context, cli inventory.CliState) {
	t := c.Theme()
	pal := Colors(t)
	var firstMissing, firstInvalid, firstExisting string
	for _, f := range cli.KubeFiles {
		switch {
		case f.Missing:
			if firstMissing == "" {
				firstMissing = f.Path
			}
		case f.Invalid:
			if firstInvalid == "" {
				firstInvalid = f.Path
			}
		default:
			if firstExisting == "" {
				firstExisting = f.Path
			}
		}
	}
	if firstMissing == "" && firstInvalid == "" && firstExisting == "" && a.Home != "" {
		firstMissing = filepath.Join(a.Home, ".kube", "config")
	}

	allMissing := firstExisting == "" && firstInvalid == "" && firstMissing != ""
	iconName := "file-text"
	iconBg, iconFg := pal.Subtle, t.TextMuted
	var title, lead string
	switch {
	case allMissing:
		if len(cli.KubeFiles) > 1 {
			title = locales.KubeEmptyMissingManyTitle()
		} else {
			title = locales.KubeEmptyMissingTitle(tilde(firstMissing, a.Home))
		}
		lead = locales.KubeEmptyMissingLead()
	case firstInvalid != "":
		iconName = "triangle-alert"
		iconBg, iconFg = pal.WarningSoft, pal.WarningText
		title = locales.KubeEmptyInvalidTitle()
		lead = locales.KubeEmptyInvalidLead(tilde(firstInvalid, a.Home))
	default:
		title = locales.KubeNoContexts()
		if firstExisting != "" {
			lead = locales.KubeEmptyNoContextsLead(tilde(firstExisting, a.Home))
		} else {
			lead = locales.KubeEmptyMissingLead()
		}
	}

	ui.Column(c).Children(func() {
		ui.Column(c).Gap(12).Padding(20).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Box(c).Size(36, 36).Shrink(0).Radius(9).Background(iconBg).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
					ui.Icon(c, Icon(iconName)).Size(17, 17).TextColor(iconFg)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, title).FontSize(14).FontWeight(600)
					ui.Text(c, lead).FontSize(12.5).TextColor(t.TextMuted)
				})
			})
			actions := ui.Row(c).Gap(8).AlignItems(ui.Center)
			if narrow(c) {
				actions.Wrap()
			}
			actions.Children(func() {
				switch {
				case allMissing && a.BuildPlan != nil:
					target := firstMissing
					ui.PrimaryButton(c.Key("kube-empty-create"), locales.KubeCreateFileButton(tilde(target, a.Home))).
						Height(controlHeight).OnClick(func() { a.openKubeConfigCreate(target) })
				case firstInvalid != "" && a.canOpenFile():
					target := firstInvalid
					ui.PrimaryButton(c.Key("kube-empty-open"), locales.OpenInEditor()).
						Height(controlHeight).OnClick(func() { a.openFile(c, target) })
				case firstExisting != "" && a.canOpenFile():
					target := firstExisting
					ui.PrimaryButton(c.Key("kube-empty-open"), locales.OpenInEditor()).
						Height(controlHeight).OnClick(func() { a.openFile(c, target) })
				}
				ui.Button(c.Key("kube-empty-kind"), locales.KindCreateButton()).Height(controlHeight).OnClick(a.openKindDialog)
			})
		})
		if hints := kubeCloudHints(cli); len(hints) > 0 {
			ui.Column(c).Gap(6).Padding(12, 20, 14, 20).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, locales.KubeEmptyCloudHint()).FontSize(11.5).TextColor(t.TextMuted)
				for _, h := range hints {
					cmd := h.cmd
					ui.Row(c.Key("kube-cloud-hint:" + h.tool)).Gap(8).AlignItems(ui.Center).Children(func() {
						clipped(c, cmd).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
						iconButton(c.Key("kube-cloud-copy:"+h.tool), "copy", locales.KubeShellCopy()).OnClick(func() {
							c.WriteClipboard(cmd)
							c.Toast(locales.KubeNameCopied(cmd))
						})
					})
				}
			})
		}
	})
}
