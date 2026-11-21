package ui

import (
	"cmp"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/locales"
)

type issueSeverity uint8

const (
	issueWarning issueSeverity = iota
	issueDanger
)

// rowIssue describes a table row's warning or failure details shown via a
// compact icon indicator beside the status cell, keeping all rows the same
// height. Hover shows a rich tooltip card; clicking opens an interactive
// popover with selectable text and an optional action button.
type rowIssue struct {
	key         string
	label       string
	severity    issueSeverity
	title       string
	body        string
	actionLabel string
	onAction    func()
}

// workloadStatusIssue renders a status pill with an optional compact issue
// indicator right next to it, keeping single-line row height.
func (a *App) workloadStatusIssue(c *ui.Context, label string, bg, fg ui.Color, issue *rowIssue) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
		aiTag(c, label, bg, fg).Shrink(1).MinWidth(0).MaxWidthPercent(100).Tooltip(label)
		a.rowIssueBadge(c, issue)
	})
}

// rowIssueBadge renders a compact 20×20 icon trigger that reveals issue details
// in a styled tooltip card on hover and an interactive popover on click.
func (a *App) rowIssueBadge(c *ui.Context, issue *rowIssue) {
	if issue == nil || (issue.title == "" && issue.body == "") {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	icon, soft, fg, accent := "triangle-alert", pal.WarningSoft, pal.WarningText, t.Warning
	if issue.severity == issueDanger {
		icon, soft, fg, accent = "circle-alert", pal.DangerSoft, pal.DangerText, t.Danger
	}
	if a.issuePopover == issue.key && !a.issuePopoverOn {
		a.issuePopover = ""
	}
	open := a.issuePopover == issue.key && a.issuePopoverOn
	label := cmp.Or(issue.label, issue.title, locales.Details())
	b := ui.ButtonBase(c.Key("row-issue-"+issue.key)).Size(20, 20).Radius(6).Center().Shrink(0).
		Label(label).Description(issue.body).
		OnClick(func() {
			if a.issuePopover == issue.key && a.issuePopoverOn {
				a.issuePopover = ""
				a.issuePopoverOn = false
			} else {
				a.issuePopover = issue.key
				a.issuePopoverOn = true
			}
		})
	bg := soft
	border := accent.Alpha(0.28)
	if open || b.Hovered() {
		bg = soft.Mix(accent, 0.14)
		border = accent.Alpha(0.55)
	}
	b.Background(bg).BorderWidth(1).BorderColor(border).Children(func() {
		ui.Icon(c, Icon(icon)).Size(12, 12).TextColor(fg)
	})
	if open {
		ui.PopoverBase(c.Key("row-issue-popover-"+issue.key), b, &a.issuePopoverOn, func(panel ui.Element) {
			panel.AttachTo(b, ui.AnchorBottom, ui.AnchorTop).Margin(6, 0, 0, 0)
			a.rowIssueCard(c, panel, issue, true)
		})
		return
	}
	ui.TooltipBase(c, b, func(tip ui.Element) {
		tip.AttachTo(b, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 6, 0)
		a.rowIssueCard(c, tip, issue, false)
	})
}

func (a *App) rowIssueCard(c *ui.Context, panel ui.Element, issue *rowIssue, interactive bool) {
	t := c.Theme()
	pal := Colors(t)
	icon, soft, fg := "triangle-alert", pal.WarningSoft, pal.WarningText
	if issue.severity == issueDanger {
		icon, soft, fg = "circle-alert", pal.DangerSoft, pal.DangerText
	}
	shadow := ui.RGBA(0x2A, 0x1E, 0x10, 0.14)
	if t.Dark {
		shadow = ui.RGBA(0, 0, 0, 0.48)
	}
	panel.Column().MinWidth(220).MaxWidth(332).Padding(10, 12).Gap(6).Radius(10).
		Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
		Shadow(0, 8, 24, 0, shadow).Children(func() {
		if issue.title != "" {
			ui.Row(c).Gap(7).AlignItems(ui.Start).Children(func() {
				ui.Box(c).Size(18, 18).Radius(5).Center().Shrink(0).Background(soft).Children(func() {
					ui.Icon(c, Icon(icon)).Size(11, 11).TextColor(fg)
				})
				title := ui.Text(c, issue.title).FontSize(12).FontWeight(600).TextColor(fg).Grow(1).MinWidth(0).Wrap()
				if interactive {
					title.Selectable()
				}
			})
		}
		if issue.body != "" {
			body := ui.Text(c, issue.body).FontSize(12).TextColor(t.Text).Wrap()
			if interactive {
				body.Selectable()
			}
		}
		if interactive && issue.actionLabel != "" && issue.onAction != nil {
			ui.Row(c).Padding(2, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Button(c.Key("issue-action"), issue.actionLabel).Height(24).Shrink(0).OnClick(func() {
					a.issuePopover = ""
					a.issuePopoverOn = false
					issue.onAction()
				})
			})
		}
	})
}
