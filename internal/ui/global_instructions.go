package ui

import (
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type globalInstructionPreview struct {
	open, create, loading     bool
	text, shared, native, err string
	files                     inventory.GlobalInstructions
}

func globalInstructionState(state string) string {
	switch state {
	case "shared":
		return locales.GlobalInstructionsShared()
	case "native":
		return locales.GlobalInstructionsReady()
	case "override":
		return locales.GlobalInstructionsOverride()
	case "linked":
		return locales.GlobalInstructionsLinked()
	case "same":
		return locales.GlobalInstructionsSame()
	case "different":
		return locales.InstructionsDifferent()
	case "unreadable":
		return locales.InstructionsUnreadable()
	case "empty":
		return locales.GlobalInstructionsEmpty()
	default:
		return locales.InstructionsMissing()
	}
}

func (a *App) globalInstructionsCard(c *ui.Context, in *inventory.GlobalInstructions) {
	if in == nil {
		return
	}
	t, pal := c.Theme(), Colors(c.Theme())
	listFrame(c, func() {
		ui.Row(c).Gap(8).Padding(12, 14, 10, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.GlobalInstructionsTitle()).FontSize(14).FontWeight(600).Grow(1).SingleLine()
			switch in.State {
			case "linked":
				chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.GlobalInstructionsChipLinked())
			case "native", "same":
				chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.GlobalInstructionsChipReady())
			case "shared":
				chip(c, Accent, pal.AccentSoft, pal.AccentInk, locales.GlobalInstructionsChipShared())
			case "missing":
				chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.GlobalInstructionsChipMissing())
			default:
				chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.GlobalInstructionsChipCheck())
			}
		})
		// One sentence says what the state means and what to do.
		lead := globalInstructionState(in.State)
		switch in.State {
		case "linked":
			lead = locales.GlobalInstructionsLinkedLead()
		case "shared":
			lead = locales.GlobalInstructionsLead()
		}
		ui.Text(c, lead).FontSize(12.5).TextColor(t.TextMuted).Padding(0, 14, 12, 14)
		for _, row := range []struct {
			label string
			f     inventory.InstructionFile
		}{
			{locales.GlobalInstructionsSharedFile(), in.Shared}, {locales.GlobalInstructionsCodexFile(), in.Native}, {locales.GlobalInstructionsOverrideFile(), in.Override},
		} {
			if row.f == in.Override && !row.f.Exists && row.f.Err == "" {
				continue
			}
			status := locales.GlobalInstructionsNotFound()
			switch {
			case row.f.Err != "":
				status = locales.InstructionsUnreadable()
			case row.f.Link && in.State == "linked":
				status = locales.GlobalInstructionsLinkFile()
			case row.f.Path == in.Active:
				status = locales.GlobalInstructionsActive()
			case row.f.Exists && row.f.Empty:
				status = locales.GlobalInstructionsEmptyFile()
			case row.f.Exists:
				status = locales.GlobalInstructionsFound()
			}
			ui.Row(c.Key(row.label)).Gap(10).Padding(8, 12, 8, 14).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, row.label).FontSize(12.5).Width(120).Shrink(0).SingleLine()
				clipped(c, tilde(row.f.Path, a.Home)).Font(FontMono).FontSize(12).Grow(1)
				color := pal.TextFaint
				if row.f.Path == in.Active {
					color = t.TextMuted
				}
				ui.Text(c, status).FontSize(11.5).TextColor(color).SingleLine()
				if row.f.Exists && a.Shell != nil {
					path := row.f.Path
					iconButton(c, "external-link", locales.OpenInEditor()).OnClick(func() { a.openFile(c, path) })
				} else {
					ui.Box(c).Size(24, 24).Shrink(0)
				}
			})
		}
		// A linked file is the shared one: comparing it with itself says nothing.
		compare := in.Shared.Exists && in.Native.Exists && in.State != "unreadable" && in.State != "linked"
		if in.State != "shared" && in.State != "missing" && !compare {
			return
		}
		ui.Row(c).Gap(8).Padding(10, 14, 12, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			switch in.State {
			case "shared":
				ui.PrimaryButton(c.Key("global-connect"), locales.GlobalInstructionsConnect()).Disabled(a.BuildPlan == nil).OnClick(func() { a.globalInstructionPlan("link", "") })
			case "missing":
				ui.Button(c.Key("global-create"), locales.GlobalInstructionsCreate()).Disabled(a.BuildPlan == nil).OnClick(func() { a.previewGlobalInstructions(true) })
			}
			if compare {
				ui.Button(c.Key("global-compare"), locales.GlobalInstructionsCompare()).Disabled(a.BuildPlan == nil).OnClick(func() { a.previewGlobalInstructions(false) })
			}
		})
	})
}

func (a *App) globalInstructionPlan(mode, text string) {
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.CodexInstructionPlan(env, mode, text)
	}, a.proposeOrSay)
}

func (a *App) previewGlobalInstructions(create bool) {
	p := &globalInstructionPreview{open: true, create: create, loading: true}
	a.globalInstructions = p
	var files inventory.GlobalInstructions
	var shared, native string
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		files = ai.CodexInstructions(env)
		var err error
		if !create {
			shared, err = ai.InstructionText(files.Shared.Path)
			if err == nil {
				native, err = ai.InstructionText(files.Native.Path)
			}
			// The shared diff widget computes a line LCS on the UI thread.
			// Keep this preview within the same bounds as operation plans.
			if err == nil && (len(shared) > 32*1024 || len(native) > 32*1024 || strings.Count(shared, "\n") > 500 || strings.Count(native, "\n") > 500) {
				err = errors.New(locales.GlobalInstructionsTextLimit())
			}
		}
		return ops.Plan{}, err
	}, func(_ ops.Plan, err error) {
		if a.globalInstructions != p {
			return
		}
		p.loading = false
		p.files, p.shared, p.native = files, shared, native
		if err != nil {
			p.err = err.Error()
		}
		if create {
			p.text = locales.GlobalInstructionsTemplate()
		}
	})
}

func (a *App) globalInstructionsDialog(c *ui.Context) {
	p := a.globalInstructions
	if p == nil || !p.open {
		return
	}
	dialogBase(c.Key("global-instructions"), &p.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Clip().Background(c.Theme().Background).BorderWidth(1).BorderColor(Colors(c.Theme()).Line)
		ui.Column(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, locales.GlobalInstructionsTitle()).FontSize(18).FontWeight(600)
			switch {
			case p.loading:
				ui.Text(c, locales.OpVerifying()).FontSize(12)
			case p.err != "":
				ui.Text(c, p.err).FontSize(12)
			case p.create:
				ui.Text(c, locales.GlobalInstructionsCreateWhy()).FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Text(c, tilde(p.files.Native.Path, a.Home)).Font(FontMono).FontSize(12)
				ui.TextArea(c.Key("global-instructions-text"), &p.text).Height(240).Font(FontMono).Label(locales.GlobalInstructionsTitle())
			default:
				ui.Text(c, locales.GlobalInstructionsCompareWhy()).FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Scroll(c).MaxHeight(340).Children(func() {
					a.editDiff(c, ops.FileEdit{Path: tilde(p.files.Native.Path, a.Home) + " ↔ " + tilde(p.files.Shared.Path, a.Home), Before: p.native, After: p.shared}, true)
				})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.Cancel()).OnClick(func() { p.open = false })
				if p.create {
					ui.PrimaryButton(c, locales.GlobalInstructionsPlan()).Disabled(p.loading || p.err != "" || strings.TrimSpace(p.text) == "").OnClick(func() { p.open = false; a.globalInstructionPlan("create", p.text) })
				}
			})
		})
	})
}
