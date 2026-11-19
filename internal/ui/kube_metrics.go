package ui

import (
	"errors"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func metricsNotice(c *ui.Context, err error) {
	if err == nil {
		return
	}
	message := locales.KubeMetricsFailed()
	if errors.Is(err, kube.ErrMetricsUnavailable) {
		message = locales.KubeMetricsUnavailable()
	} else if failure, ok := errors.AsType[*kube.Error](err); ok && failure.Access.State == detect.KubeForbidden {
		message = locales.KubeMetricsForbidden()
	}
	pal := Colors(c.Theme())
	ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(9, 12).Radius(8).Background(pal.InfoSoft).Shrink(0).Children(func() {
		ui.Icon(c, Icon("info")).Size(14, 14).Shrink(0).TextColor(pal.Info)
		ui.Text(c, message).FontSize(11.5).TextColor(pal.InfoText).Wrap().Tooltip(err.Error())
	})
}

func metricsCPU(cores float64) string {
	if cores > 0 && cores < 0.001 {
		return "<" + locales.KubeMetricsMillicores(1)
	}
	return locales.KubeMetricsMillicores(cores * 1000)
}

func metricsMemory(bytes float64) string {
	if bytes > 0 && bytes < 1<<20 {
		return "<" + locales.KubeMetricsMiB(1)
	}
	return locales.KubeMetricsMiB(bytes / (1 << 20))
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

func workloadMetrics(c *ui.Context, usage *kube.Resources, requests, limits kube.Resources) {
	tracks := []ui.Track{ui.Fixed(60), ui.Fr(1), ui.Fr(1), ui.Fr(1)}
	resourceHead(c, tracks, "", locales.KubeMetricsUsage(), locales.KubeMetricsRequest(), locales.KubeMetricsLimit())
	for _, memory := range []bool{false, true} {
		resourceGrid(ui.Row(c), tracks).Padding(4, 16).Children(func() {
			label, requested, limit, format := "CPU", requests.CPU, limits.CPU, metricsCPU
			if memory {
				label, requested, limit, format = locales.KubeMetricsMemory(), requests.Memory, limits.Memory, metricsMemory
			}
			ui.Text(c, label).FontSize(11.5).SingleLine()
			podMetricCell(c, usage, limits, memory)
			ui.Text(c, format(requested)).Font(FontMono).FontSize(11.5).SingleLine()
			text := "–"
			if limit > 0 {
				text = format(limit)
			}
			ui.Text(c, text).Font(FontMono).FontSize(11.5).SingleLine()
		})
	}
}
