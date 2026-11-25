package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/inventory"
)

func TestIconsParse(t *testing.T) {
	m, err := icons()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"lucide/search", "lucide/folder-git-2", "brand/nodedotjs", "brand/cursor"} {
		if m[key] == nil {
			t.Errorf("icon %s is missing", key)
		}
	}
	if len(m) < 50 {
		t.Errorf("parsed %d icons, want all of resources/icons", len(m))
	}
}

func TestIconTakesTextColor(t *testing.T) {
	red := ui.Hex("#FF0000")
	view := func(c *ui.Context) {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Icon(c, Icon("database")).FontSize(48).TextColor(red)
		})
	}
	img := ui.NewTester(view, 80, 80).Image()
	reddish := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r > 0xC000 && g < 0x4000 && bl < 0x4000 {
				reddish++
			}
		}
	}
	if reddish < 20 {
		t.Errorf("icon drew %d red pixels, want it in the text color", reddish)
	}
}

func TestUnknownIconPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Icon of an unknown name did not panic")
		}
	}()
	Icon("no-such-icon")
}

func TestEveryBrandHasLogo(t *testing.T) {
	m, err := icons()
	if err != nil {
		t.Fatal(err)
	}
	for key := range m {
		if slug, ok := strings.CutPrefix(key, "brand/"); ok && m["logo/"+slug] == nil {
			t.Errorf("brand %s has no logo/%s in colors", slug, slug)
		}
	}
}

func TestLogoKeepsItsColors(t *testing.T) {
	yellow := func(r, g, b uint32) bool { return r > 0xD000 && g > 0xB000 && b < 0x8000 }
	white := func(r, g, b uint32) bool { return r > 0xE000 && g > 0xE000 && b > 0xE000 }
	black := func(r, g, b uint32) bool { return r < 0x2000 && g < 0x2000 && b < 0x2000 }
	tests := []struct {
		name  string
		slug  string
		dark  bool
		color func(r, g, b uint32) bool
	}{
		{"python in its yellow", "python", false, yellow},
		{"python in its yellow on dark", "python", true, yellow},
		{"openai black on light", "openai", false, black},
		{"openai white on dark", "openai", true, white},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := func(c *ui.Context) {
				ui.Column(c).Fill().Center().Background(ui.Hex("#808080")).Children(func() {
					iconOrLogo(c, "brand/"+tt.slug, 48, ui.Hex("#808080"))
				})
			}
			tester := ui.NewTester(view, 80, 80)
			tester.SetDark(tt.dark)
			img := tester.Image()
			n := 0
			b := img.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					r, g, bl, _ := img.At(x, y).RGBA()
					if tt.color(r, g, bl) {
						n++
					}
				}
			}
			if n < 20 {
				t.Errorf("drew %d pixels of the logo's color, want its own colors", n)
			}
		})
	}
}

// The overview's tool cards and agents show logos in their colors, on
// both themes.
func TestOverviewLogos(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := NewApp()
		a.Snapshot = found("node", "bun", "python", "php", "go", "rust", "docker", "pnpm", "git")
		for _, id := range []string{"claude", "codex", "copilot", "gemini", "cursor"} {
			a.Snapshot.Agents = append(a.Snapshot.Agents, inventory.Agent{ID: id, Installations: []inventory.Installation{{Tool: id, Version: "1.0.0"}}})
		}
		tt := openOverview(t, a)
		tt.SetDark(dark)
		tt.Frame()
		hasTexts(t, tt, "Python", "Docker", "Claude Code", "Gemini CLI")
		name := "overview-logos"
		if dark {
			name += "-dark"
		}
		screenshot(t, tt, name)
	}
}

func TestSetupToolIconsRender(t *testing.T) {
	// Verify every catalog tool icon renders visible pixels on its tile on both themes.
	for _, dark := range []bool{false, true} {
		for _, e := range catalog.Tools {
			view := func(c *ui.Context) {
				ui.Column(c).Fill().Center().Background(c.Theme().Background).Children(func() {
					setupToolTile(c, e.ID, 36, false, false)
				})
			}
			tt := ui.NewTester(view, 48, 48)
			tt.SetDark(dark)
			img := tt.Image()
			bgR, bgG, bgB, _ := img.At(24, 10).RGBA() // inside tile near top border
			diff := 0
			for y := 14; y < 34; y++ {
				for x := 14; x < 34; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if absDiffU32(r, bgR)+absDiffU32(g, bgG)+absDiffU32(b, bgB) > 0x3000 {
						diff++
					}
				}
			}
			if diff < 8 {
				t.Errorf("tool %s (dark=%v, icon=%s): only %d visible pixels inside tile", e.ID, dark, setupToolIcon(e.ID), diff)
			}
		}

		// Render a visual contact sheet of all catalog tool tiles when screenshots are enabled.
		gridView := func(c *ui.Context) {
			t := c.Theme()
			ui.Column(c).Fill().Background(t.Background).Padding(16).Gap(8).Children(func() {
				const cols = 6
				for i := 0; i < len(catalog.Tools); i += cols {
					end := min(i+cols, len(catalog.Tools))
					ui.Row(c).Gap(8).Children(func() {
						for _, e := range catalog.Tools[i:end] {
							ui.Row(c).Width(155).Gap(8).AlignItems(ui.Center).Children(func() {
								setupToolTile(c, e.ID, 28, false, false)
								ui.Text(c, e.ID).FontSize(12).TextColor(t.Text)
							})
						}
					})
				}
			})
		}
		tt := ui.NewTester(gridView, 1020, 420)
		tt.SetDark(dark)
		tt.Frame()
		name := "install-logos-light"
		if dark {
			name = "install-logos-dark"
		}
		screenshot(t, tt, name)
	}
}

func TestEntityBrandIconsRender(t *testing.T) {
	keys := []string{
		dockerEngineIcon("Docker Desktop"),
		dockerEngineIcon("Colima"),
		dockerEngineIcon("OrbStack"),
		dockerEngineIcon("Podman"),
		dockerEngineIcon("Rancher Desktop"),
		dockerEngineIcon("Kiri"),
		pkgManagerIcon("brew"),
		pkgManagerIcon("apt"),
		pkgManagerIcon("dnf"),
		pkgManagerIcon("pacman"),
		pkgManagerIcon("zypper"),
		pkgManagerIcon("apk"),
		pkgManagerIcon("snap"),
		pkgManagerIcon("flatpak"),
		pkgManagerIcon("nix"),
		pkgManagerIcon("port"),
		pkgManagerIcon("winget"),
		pkgManagerIcon("choco"),
		pkgManagerIcon("scoop"),
		kubeContextIcon(inventory.KubeContext{Name: "k3d-dev", Cluster: "k3d-dev"}),
		kubeContextIcon(inventory.KubeContext{Name: "prod-eu", Cluster: "prod-eu"}),
	}
	for _, dark := range []bool{false, true} {
		for _, key := range keys {
			view := func(c *ui.Context) {
				ui.Column(c).Fill().Center().Background(c.Theme().Background).Children(func() {
					entityTile(c, key, 28)
				})
			}
			tt := ui.NewTester(view, 40, 40)
			tt.SetDark(dark)
			img := tt.Image()
			bgR, bgG, bgB, _ := img.At(20, 8).RGBA()
			diff := 0
			for y := 11; y < 29; y++ {
				for x := 11; x < 29; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if absDiffU32(r, bgR)+absDiffU32(g, bgG)+absDiffU32(b, bgB) > 0x2000 {
						diff++
					}
				}
			}
			if diff < 8 {
				t.Errorf("entity icon %s (dark=%v): only %d visible pixels inside tile", key, dark, diff)
			}
		}

		// Contact sheet when UI screenshots are enabled.
		gridView := func(c *ui.Context) {
			th := c.Theme()
			ui.Column(c).Fill().Background(th.Background).Padding(16).Gap(8).Children(func() {
				const cols = 4
				for i := 0; i < len(keys); i += cols {
					end := min(i+cols, len(keys))
					ui.Row(c).Gap(10).Children(func() {
						for _, key := range keys[i:end] {
							ui.Row(c).Width(175).Gap(8).AlignItems(ui.Center).Children(func() {
								entityTile(c, key, 28)
								ui.Text(c, key).FontSize(12).TextColor(th.Text)
							})
						}
					})
				}
			})
		}
		tt := ui.NewTester(gridView, 780, 260)
		tt.SetDark(dark)
		tt.Frame()
		name := "entity-logos-light"
		if dark {
			name = "entity-logos-dark"
		}
		screenshot(t, tt, name)
	}
}

func absDiffU32(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

func TestAllIconLiteralsExist(t *testing.T) {
	m, err := icons()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	reIcon := regexp.MustCompile(`\bIcon\("([^"]+)"\)`)
	reBtn := regexp.MustCompile(`\biconButton\([^,]+,\s*"([a-z0-9-]+)"`)
	reKey := regexp.MustCompile(`"((?:lucide|brand)/[a-z0-9-]+)"`)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		for _, match := range reIcon.FindAllStringSubmatch(src, -1) {
			if m["lucide/"+match[1]] == nil {
				t.Errorf("%s: unknown Icon(%q)", name, match[1])
			}
		}
		for _, match := range reBtn.FindAllStringSubmatch(src, -1) {
			if m["lucide/"+match[1]] == nil {
				t.Errorf("%s: unknown iconButton icon %q", name, match[1])
			}
		}
		for _, match := range reKey.FindAllStringSubmatch(src, -1) {
			if m[match[1]] == nil {
				t.Errorf("%s: unknown icon key %q", name, match[1])
			}
		}
	}
}
