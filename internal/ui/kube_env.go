package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

// podEnv lists the variables the pod's containers get from their specs:
// a value written out, or where it comes from. A Secret's value is not
// read, and a value written out that looks like a secret shows hidden
// until its eye is clicked.
func (a *App) podEnv(c *ui.Context, p kube.Pod) {
	var any bool
	for _, ct := range p.Containers {
		any = any || len(ct.Env) > 0 || len(ct.EnvFrom) > 0
	}
	if !any {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	detailSection(c, locales.KubeEnvTitle())
	for _, ct := range p.Containers {
		if len(ct.Env) == 0 && len(ct.EnvFrom) == 0 {
			continue
		}
		if len(p.Containers) > 1 {
			ui.Text(c, ct.Name).FontSize(12).FontWeight(500).TextColor(t.TextMuted).Padding(2, 16, 4, 16)
		}
		ui.Column(c).Margin(0, 16, 8, 16).Radius(8).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
			for _, from := range ct.EnvFrom {
				a.envRow(c, p, ct.Name, kube.EnvVar{Name: from.Prefix + "*", From: "all " + from.Kind, Ref: from.Name})
			}
			for _, v := range ct.Env {
				a.envRow(c, p, ct.Name, v)
			}
		})
	}
}

func (a *App) envRow(c *ui.Context, p kube.Pod, container string, v kube.EnvVar) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	key := p.UID + "/" + container + "/" + v.Name
	ui.Row(c.Key("env-"+key)).Gap(10).Padding(6, 12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		clipped(c, v.Name).Font(FontMono).FontSize(12).Width(180).Shrink(0)
		switch v.From {
		case "":
			hidden := ai.PlainSecret(v.Name, v.Value) && !s.envShown[key]
			text := v.Value
			if hidden {
				text = "••••••••"
			}
			value := clipped(c, text).Font(FontMono).FontSize(12).Grow(1).MinWidth(0)
			if v.Value == "" {
				value.TextColor(pal.TextFaint)
			}
			if !hidden {
				value.Selectable()
			}
			if ai.PlainSecret(v.Name, v.Value) {
				icon, label := "eye", locales.KubeEnvShow(v.Name)
				if !hidden {
					icon, label = "eye-off", locales.KubeEnvHide(v.Name)
				}
				iconButton(c.Key("env-eye-"+key), icon, label).OnClick(func() {
					if s.envShown == nil {
						s.envShown = map[string]bool{}
					}
					s.envShown[key] = hidden
				})
			}
		case "ConfigMap", "Secret", "all ConfigMap", "all Secret":
			kind, text := v.From, ""
			if all, ok := strings.CutPrefix(v.From, "all "); ok {
				kind, text = all, locales.KubeEnvFromAll(all, v.Ref)
			} else {
				text = locales.KubeEnvFromKey(kind, v.Ref, v.Key)
			}
			clipped(c, text).FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0)
			link := linkTo(kube.Relationship{Kind: kind, Namespace: p.Namespace, Name: v.Ref})
			iconButton(c.Key("env-open-"+key), "arrow-right", locales.KubeOpenOwnerKind(kind, v.Ref)).OnClick(func() { a.openResourceLink(link) })
		case "Field":
			clipped(c, locales.KubeEnvFromField(v.Ref)).FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0)
		case "Resource":
			clipped(c, locales.KubeEnvFromResource(v.Ref)).FontSize(12).TextColor(t.TextMuted).Grow(1).MinWidth(0)
		}
	})
}
