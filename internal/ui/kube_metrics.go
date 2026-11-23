package ui

import (
	"errors"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) metricsNotice(c *ui.Context, err error) {
	if err == nil {
		a.cluster.metricsNoticeDismissed = ""
		return
	}
	message := locales.KubeMetricsFailed()
	if errors.Is(err, kube.ErrMetricsUnavailable) {
		message = locales.KubeMetricsUnavailable()
	} else if failure, ok := errors.AsType[*kube.Error](err); ok && failure.Access.State == detect.KubeForbidden {
		message = locales.KubeMetricsForbidden()
	}
	if a.cluster.metricsNoticeDismissed == message {
		return
	}
	pal := Colors(c.Theme())
	ui.Row(c.Key("kube-metrics-notice")).AlignItems(ui.Center).Gap(8).Padding(7, 8, 7, 12).Radius(8).Background(pal.InfoSoft).Shrink(0).Children(func() {
		ui.Icon(c, Icon("info")).Size(14, 14).Shrink(0).TextColor(pal.Info)
		ui.Text(c, message).FontSize(11.5).TextColor(pal.InfoText).Wrap().Grow(1).MinWidth(0).Tooltip(err.Error())
		iconButton(c.Key("kube-metrics-notice-dismiss"), "x", locales.OpClose()).Shrink(0).OnClick(func() {
			a.cluster.metricsNoticeDismissed = message
		})
	})
}

// metricsCPU says CPU as kubectl top does, in millicores (250m), and in
// cores from one core up (1.5 CPU), where millicores grow long.
func metricsCPU(cores float64) string {
	switch {
	case cores > 0 && cores < 0.001:
		return "<" + locales.KubeMetricsMillicores(1)
	case cores >= 1:
		return locales.KubeMetricsCores(math.Round(cores*100) / 100)
	}
	return locales.KubeMetricsMillicores(math.Round(cores * 1000))
}

// metricsMemory says memory in MiB, and in GiB from 1 GiB up.
func metricsMemory(bytes float64) string {
	switch {
	case bytes > 0 && bytes < 1<<20:
		return "<" + locales.KubeMetricsMiB(1)
	case bytes >= 1<<30:
		return locales.KubeMetricsGiB(math.Round(bytes/(1<<30)*100) / 100)
	}
	return locales.KubeMetricsMiB(math.Round(bytes / (1 << 20)))
}

func podMetricCell(c *ui.Context, usage *kube.Resources, limits kube.Resources, memory bool) {
	pal := Colors(c.Theme())
	text, color := "–", pal.TextFaint
	tip := locales.KubeMetricsNoSample()
	if usage != nil {
		value, limit := usage.CPU, limits.CPU
		format := metricsCPU
		if memory {
			value, limit, format = usage.Memory, limits.Memory, metricsMemory
		}
		text = format(value)
		tip = text
		if limit > 0 {
			share := value / limit
			text += " · " + requestPercent(share)
			tip = text + " / " + format(limit)
			if share >= .9 {
				color = pal.WarningText
			}
		}
	}
	ui.Text(c, text).Font(FontMono).FontSize(11.5).SingleLine().TextColor(color).Tooltip(tip)
}

// workloadMetrics is what a pod uses of its CPU and memory: for each, the
// use, a bar of it against the limit (or the request without one) with
// the request marked, and the request and limit in words.
func workloadMetrics(c *ui.Context, usage *kube.Resources, requests, limits kube.Resources) {
	ui.Column(c).Gap(12).Padding(4, 16, 10, 16).Children(func() {
		for _, memory := range []bool{false, true} {
			metricBar(c, usage, requests, limits, memory)
		}
	})
}

func metricBar(c *ui.Context, usage *kube.Resources, requests, limits kube.Resources, memory bool) {
	t := c.Theme()
	pal := Colors(t)
	label, requested, limit, format := "CPU", requests.CPU, limits.CPU, metricsCPU
	if memory {
		label, requested, limit, format = locales.KubeMetricsMemory(), requests.Memory, limits.Memory, metricsMemory
	}
	used, sampled := 0.0, usage != nil
	if sampled {
		used = usage.CPU
		if memory {
			used = usage.Memory
		}
	}
	// The bar's whole is the limit, or the request when there is none.
	scale, of := limit, locales.KubeMetricsOfLimit
	if scale == 0 {
		scale, of = requested, locales.KubeMetricsOfRequest
	}
	ui.Column(c).Gap(5).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, label).FontSize(12.5).FontWeight(500).Grow(1)
			if !sampled {
				ui.Text(c, locales.KubeMetricsNoSample()).FontSize(11.5).TextColor(pal.TextFaint)
				return
			}
			ui.Text(c, format(used)).Font(FontMono).FontSize(12)
			if scale > 0 {
				color := t.TextMuted
				if used/scale >= .9 {
					color = pal.WarningText
				}
				ui.Text(c, of(requestPercent(used/scale))).FontSize(11.5).TextColor(color)
			}
		})
		if scale > 0 {
			share := used / scale
			color := t.Success
			switch {
			case share >= 1 && limit > 0:
				color = t.Danger
			case share >= .9:
				color = t.Warning
			}
			ui.Row(c).Height(6).Radius(3).Background(pal.Line).Children(func() {
				if sampled && share > 0 {
					ui.Box(c).WidthPercent(float32(min(share, 1) * 100)).Height(6).Radius(3).Background(color).Transition(barMotion)
				}
				if limit > 0 && requested > 0 && requested < limit {
					// The request, as a tick on the limit's bar.
					ui.Box(c).Absolute().LeftPercent(float32(requested / limit * 100)).Top(-2).Width(2).Height(10).Radius(1).Background(t.TextMuted)
				}
			})
		}
		parts := []string{locales.KubeMetricsRequestIs(format(requested)), locales.KubeMetricsLimitIs(format(limit))}
		if requested == 0 {
			parts[0] = locales.KubeMetricsNoRequest()
		}
		if limit == 0 {
			parts[1] = locales.KubeMetricsNoLimit()
		}
		ui.Text(c, strings.Join(parts, " · ")).FontSize(11.5).TextColor(t.TextMuted)
	})
}

// containerMetrics is a container's use in one line under its row.
func containerMetrics(c *ui.Context, ct kube.Container) {
	if ct.Usage == nil && ct.Requests == (kube.Resources{}) && ct.Limits == (kube.Resources{}) {
		return
	}
	t := c.Theme()
	parts := make([]string, 0, 2)
	for _, memory := range []bool{false, true} {
		label, used, requested, limit, format := "CPU", 0.0, ct.Requests.CPU, ct.Limits.CPU, metricsCPU
		if memory {
			label, requested, limit, format = locales.KubeMetricsMemory(), ct.Requests.Memory, ct.Limits.Memory, metricsMemory
		}
		value := "–"
		if ct.Usage != nil {
			used = ct.Usage.CPU
			if memory {
				used = ct.Usage.Memory
			}
			value = format(used)
		}
		text := label + " " + value
		switch {
		case requested > 0 && limit > 0:
			text += " (" + locales.KubeMetricsBounds(format(requested), format(limit)) + ")"
		case requested > 0:
			text += " (" + locales.KubeMetricsRequestIs(format(requested)) + ")"
		case limit > 0:
			text += " (" + locales.KubeMetricsLimitIs(format(limit)) + ")"
		}
		parts = append(parts, text)
	}
	ui.Text(c, strings.Join(parts, "   ")).FontSize(11.5).TextColor(t.TextMuted).Padding(0, 16, 8, 30).Wrap()
}
