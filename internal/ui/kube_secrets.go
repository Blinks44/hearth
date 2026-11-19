package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

const (
	secretRevealTime    = 30 * time.Second
	secretClipboardTime = 60 * time.Second
)

type secretView struct {
	uid, scope string
	filter     int
	request    *secretRequest
	err        string
	revealed   *secretReveal
}

type secretReveal struct {
	key    string
	value  []byte
	binary bool
	until  time.Time
	stop   func()
}

type secretRequest struct {
	key  string
	stop func()
}

func (s *secretView) cancel() {
	if r := s.request; r != nil {
		s.request = nil
		if r.stop != nil {
			r.stop()
		}
	}
}

func (s *secretView) hide() {
	if reveal := s.revealed; reveal != nil {
		s.revealed = nil
		if reveal.stop != nil {
			reveal.stop()
		}
		clear(reveal.value)
		reveal.value = nil
	}
}

func (s *secretView) reset() {
	s.cancel()
	s.hide()
	s.uid, s.scope, s.err = "", "", ""
}

type secretClipboard struct {
	digest   [32]byte
	services ui.Services
	stop     func()
}

func (a *App) ClearSecretClipboard() {
	if b := a.secretClipboard; b != nil {
		a.secretClipboard = nil
		b.stop()
		if sha256.Sum256([]byte(b.services.ReadClipboard())) == b.digest {
			b.services.WriteClipboard("")
		}
	}
}

func (a *App) syncSecrets(c *ui.Context) {
	s := &a.cluster
	v := &s.secret
	if v.uid == "" {
		return
	}
	if s.kind != kindSecrets || a.router.Path() != kubeClusterPage || s.target != v.scope || s.data == nil {
		v.reset()
		return
	}
	i := slices.IndexFunc(s.data.Secrets, func(item kube.Secret) bool { return item.UID == v.uid && v.matches(item, s.query) })
	if i < 0 {
		v.reset()
		return
	}
	selected := s.data.Secrets[i]
	if r := v.request; r != nil && (s.err != nil || !secretHasKey(selected, r.key)) {
		v.cancel()
	}
	if reveal := v.revealed; reveal != nil {
		if s.err != nil || !secretHasKey(selected, reveal.key) {
			v.hide()
			return
		}
		if reveal.until.IsZero() {
			reveal.until = c.Now().Add(secretRevealTime)
		}
		if remaining := reveal.until.Sub(c.Now()); remaining > 0 {
			c.After(min(remaining, time.Second))
		} else {
			v.hide()
		}
	}
}

func secretHasKey(secret kube.Secret, name string) bool {
	return slices.ContainsFunc(secret.Keys, func(key kube.SecretKey) bool { return key.Name == name })
}

func (v *secretView) matches(secret kube.Secret, query string) bool {
	return secretFilters(secret)[v.filter] &&
		strings.Contains(strings.ToLower(secret.Namespace+"/"+secret.Name), strings.ToLower(strings.TrimSpace(query)))
}

func secretFilters(secret kube.Secret) [4]bool {
	return [4]bool{true, !secret.Helm, secret.Helm, secret.Type == "kubernetes.io/tls"}
}

func (a *App) loadSecret(services ui.Services, secret kube.Secret, key string, copyValue bool) {
	s := &a.cluster
	v := &s.secret
	if a.ReadSecretValue == nil || a.SecretTimer == nil || s.err != nil {
		return
	}
	v.cancel()
	if !copyValue {
		v.hide()
	}
	v.err = ""
	ref := a.resourceRef("Secret", secret.Namespace, secret.Name, secret.UID)
	t := kube.Target{Kubectl: ref.Kubectl, Context: ref.Context}
	request := &secretRequest{key: key}
	v.request = request
	request.stop = a.ReadSecretValue(t, secret, key, func(value []byte, err error) {
		if v.request != request {
			clear(value)
			return
		}
		v.cancel()
		if a.router.Path() != kubeClusterPage || a.router.Query("context") != t.Context || s.kind != kindSecrets {
			clear(value)
			return
		}
		if err != nil {
			clear(value)
			v.err = locales.KubeSecretReadFailed(a.accessWords(err))
			return
		}
		binary := !utf8.Valid(value) || bytes.ContainsFunc(value, func(r rune) bool {
			return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
		})
		if binary {
			encoded := make([]byte, base64.StdEncoding.EncodedLen(len(value)))
			base64.StdEncoding.Encode(encoded, value)
			clear(value)
			value = encoded
		}
		if copyValue {
			a.ClearSecretClipboard()
			services.WriteClipboard(string(value))
			clipboard := &secretClipboard{digest: sha256.Sum256(value), services: services}
			a.secretClipboard = clipboard
			clipboard.stop = a.SecretTimer(secretClipboardTime, func() {
				if a.secretClipboard == clipboard {
					a.ClearSecretClipboard()
				}
			})
			a.notice = locales.KubeSecretCopied()
			clear(value)
			return
		}
		reveal := &secretReveal{key: key, value: value, binary: binary}
		v.revealed = reveal
		reveal.stop = a.SecretTimer(secretRevealTime, func() {
			if v.revealed == reveal {
				v.hide()
			}
		})
	})
}

func (a *App) secretsView(c *ui.Context, list []kube.Secret) {
	s := &a.cluster
	v := &s.secret
	var counts [4]int
	var shown []kube.Secret
	var selected kube.Secret
	for _, item := range list {
		for i, included := range secretFilters(item) {
			if included {
				counts[i]++
			}
		}
		if v.matches(item, s.query) {
			shown = append(shown, item)
			if item.UID == v.uid {
				selected = item
			}
		}
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
		segments(c, "secret-filter", &v.filter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[0])},
			{locales.KubeSecretOther(), strconv.Itoa(counts[1])},
			{"Helm", strconv.Itoa(counts[2])},
			{"TLS", strconv.Itoa(counts[3])},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).
			Placeholder(locales.KubeSecretSearch()).Width(215).MinWidth(100)
	})
	if len(shown) == 0 {
		message := locales.KubeSecretNoMatches()
		if len(list) == 0 {
			message = locales.KubeSecretEmpty()
		}
		toolNote(c, message)
		return
	}
	t := c.Theme()
	pal := Colors(t)
	tracks := []ui.Track{ui.Fr(1.5), ui.Fr(1)}
	showNamespace := len(a.clusterNamespaces(a.router.Query("context"))) != 1
	var rowHeight float32 = 34
	if showNamespace {
		rowHeight = 41
	}
	workloadPanes(c, func() {
		resourceHead(c, tracks, locales.KubeClusterName(), locales.KubeSecretTypeKeys())
	}, func() {
		for _, item := range shown {
			row := resourceGrid(ui.ButtonBase(c.Key(item.UID)), tracks).Padding(4, 16).Height(rowHeight).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Label(item.Name)
			switch {
			case selected.UID == item.UID:
				row.Background(pal.AccentSoft)
			case row.Hovered():
				row.Background(pal.Muted)
			}
			row.OnClick(func() {
				wasSelected := v.uid == item.UID
				v.reset()
				s.revealWorkload = item.UID
				if !wasSelected {
					v.uid, v.scope = item.UID, s.target
				}
			}).Children(func() {
				ui.Column(c).MinWidth(0).Gap(1).Children(func() {
					clipped(c, item.Name).FontSize(13).FontWeight(500)
					if showNamespace {
						clipped(c, item.Namespace).FontSize(10.5).TextColor(pal.TextFaint)
					}
				})
				workloadStatus(c, secretTypeLabel(item)+" · "+strconv.Itoa(len(item.Keys)), pal.Muted, t.TextMuted)
			})
			s.revealWorkloadRow(row, item.UID)
		}
	}, selected.UID, func() { a.secretDetail(c, selected) })
}

func secretTypeLabel(secret kube.Secret) string {
	switch secret.Type {
	case "kubernetes.io/tls":
		return "TLS"
	case "kubernetes.io/dockerconfigjson", "kubernetes.io/dockercfg":
		return "Docker"
	case "helm.sh/release.v1":
		return "Helm"
	default:
		return secret.Type
	}
}

func (a *App) secretDetail(c *ui.Context, secret kube.Secret) {
	t := c.Theme()
	pal := Colors(t)
	v := &a.cluster.secret
	detailFrame(c, 0, false, func() {
		ui.Column(c).Gap(6).Padding(16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon("key-round")).Size(18, 18).Shrink(0).TextColor(t.TextMuted)
				clipped(c, secret.Name).FontSize(16).FontWeight(500).MinWidth(0)
				aiTag(c, secretTypeLabel(secret), pal.Muted, t.TextMuted).MaxWidthPercent(35).Tooltip(secret.Type)
				ui.Box(c).Grow(1)
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(v.reset)
			})
			ui.Text(c, strings.Join([]string{"Secret", secret.Namespace, locales.KubeCreatedAgo(age(c, secret.Created))}, " · ")).
				FontSize(12).TextColor(t.TextMuted).Wrap()
		})
	}, func() {
		ui.Column(c).Gap(14).Padding(0, 16, 6, 16).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.KubeSecretKeys()).FontSize(13).FontWeight(600)
				ui.Text(c, strconv.Itoa(len(secret.Keys))).FontSize(11).TextColor(pal.TextFaint)
			})
			ui.Row(c).AlignItems(ui.Center).Gap(9).Padding(10, 12).Radius(8).Background(pal.InfoSoft).Children(func() {
				ui.Icon(c, Icon("info")).Size(14, 14).Shrink(0).TextColor(pal.Info)
				ui.Text(c, locales.KubeSecretNotice()).FontSize(11.5).TextColor(pal.InfoText).MinWidth(0).Wrap()
			})
			if v.err != "" {
				ui.Text(c, v.err).FontSize(11.5).TextColor(pal.WarningText).Wrap()
			}
			if len(secret.Keys) == 0 {
				toolNote(c, locales.KubeSecretNoKeys())
				return
			}
			ui.Column(c).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
				for i, key := range secret.Keys {
					a.secretKeyRow(c.Key(key.Name), secret, key, i < len(secret.Keys)-1)
				}
			})
			ui.Text(c, locales.KubeSecretClipboard()).FontSize(11.5).TextColor(t.TextMuted).Wrap()
		})
	})
}

func (a *App) secretKeyRow(c *ui.Context, secret kube.Secret, key kube.SecretKey, divider bool) {
	s := &a.cluster
	v := &s.secret
	t := c.Theme()
	pal := Colors(t)
	reveal := v.revealed
	revealed := reveal != nil && reveal.key == key.Name
	row := ui.Row(c).MinHeight(56).Gap(6).AlignItems(ui.Center).Padding(8, 10, 8, 14)
	if divider {
		row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
	}
	if revealed {
		row.Background(pal.WarningSoft.Alpha(0.4))
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			clipped(c, key.Name).Font(FontMono).FontSize(12)
			switch {
			case !revealed:
				ui.Text(c, "••••••••").Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			case len(reveal.value) == 0:
				ui.Text(c, locales.KubeSecretEmptyValue()).FontSize(11).TextColor(t.TextMuted)
			default:
				if reveal.binary {
					ui.Text(c, "Base64").FontSize(10.5).TextColor(t.TextMuted)
				}
				secretValue(c, reveal)
			}
		})
		status, color := locales.KubeSecretBytes(strconv.Itoa(key.Bytes)), pal.TextFaint
		switch {
		case v.request != nil && v.request.key == key.Name:
			status = locales.KubeSecretReading()
		case revealed:
			seconds := int(math.Ceil(reveal.until.Sub(c.Now()).Seconds()))
			status, color = locales.KubeSecretHidesIn(strconv.Itoa(max(1, seconds))), pal.WarningText
		}
		ui.Text(c, status).FontSize(10.5).TextColor(color).Shrink(0).SingleLine()
		services := c.Services()
		if revealed {
			iconButton(c, "eye-off", locales.KubeSecretHide(key.Name)).OnClick(v.hide)
		} else {
			iconButton(c, "eye", locales.KubeSecretShow(key.Name)).Disabled(a.ReadSecretValue == nil || a.SecretTimer == nil || s.err != nil).
				OnClick(func() { a.loadSecret(services, secret, key.Name, false) })
		}
		iconButton(c, "copy", locales.KubeSecretCopy(key.Name)).Disabled(a.ReadSecretValue == nil || a.SecretTimer == nil || s.err != nil).
			OnClick(func() { a.loadSecret(services, secret, key.Name, true) })
	})
}

// Draw one rune at a time so the toolkit's text cache and accessibility
// tree never receive the value. The copy button gives the complete value.
func secretValue(c *ui.Context, reveal *secretReveal) {
	lines := 1
	if len(reveal.value) > 20 || bytes.ContainsRune(reveal.value, '\n') {
		lines = 4
	}
	const lineHeight = 15
	ui.Box(c).Height(float32(lines * lineHeight)).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		x, y := r.X, r.Y
		ellipsis := ui.Span{Text: "…", Font: FontMono, Size: 12}
		more, _ := p.MeasureText(0, ellipsis)
		value := reveal.value
		for len(value) > 0 {
			ch, size := utf8.DecodeRune(value)
			value = value[size:]
			if ch == '\r' {
				continue
			}
			if ch == '\t' {
				ch = '⇥'
			}
			span := ui.Span{Text: string(ch), Font: FontMono, Size: 12}
			w, _ := p.MeasureText(0, span)
			if ch == '\n' || x+w > r.X+r.W || (y+lineHeight*2 > r.Y+r.H && len(value) > 0 && x+w+more > r.X+r.W) {
				if y+lineHeight*2 > r.Y+r.H {
					p.RichText(x, y, 0, ellipsis)
					return
				}
				x, y = r.X, y+lineHeight
				if ch == '\n' {
					continue
				}
			}
			p.RichText(x, y, 0, span)
			x += w
		}
	})
}
