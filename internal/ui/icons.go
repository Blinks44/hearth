package ui

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/resources"
)

// icons maps "lucide/<name>" and "brand/<slug>" to parsed SVGs.
var icons = sync.OnceValues(func() (map[string]*ui.SVG, error) {
	m := map[string]*ui.SVG{}
	err := fs.WalkDir(resources.Icons, "icons", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := resources.Icons.ReadFile(p)
		if err != nil {
			return err
		}
		svg, err := ui.ParseSVG(data)
		if err != nil {
			return fmt.Errorf("parse icon %s: %w", p, err)
		}
		m[strings.TrimSuffix(strings.TrimPrefix(p, "icons/"), path.Ext(p))] = svg
		return nil
	})
	return m, err
})

// Icon returns the Lucide icon of that name, as in the Sketch document's
// icon/<name>. ui.Icon draws it in the color of the text.
func Icon(name string) *ui.SVG { return mustIcon("lucide/" + name) }

// iconOrLogo draws the icon of key ("lucide/<name>" or "brand/<slug>") size
// wide: a brand with a logo in colors, logo/<slug>, shows in them, in its
// logo/<slug>-dark variant on a dark theme when it has one; anything else is
// an icon in color.
func iconOrLogo(c *ui.Context, key string, size float32, color ui.Color) {
	if slug, ok := strings.CutPrefix(key, "brand/"); ok {
		m, _ := icons()
		logo, ok := m["logo/"+slug]
		if dark, has := m["logo/"+slug+"-dark"]; has && c.Theme().Dark {
			logo = dark
		}
		if ok {
			ui.Image(c, logo).Size(size, size)
			return
		}
	}
	ui.Icon(c, mustIcon(key)).Size(size, size).TextColor(color)
}

func mustIcon(key string) *ui.SVG {
	m, err := icons()
	if err != nil {
		panic(err)
	}
	svg, ok := m[key]
	if !ok {
		panic("unknown icon " + key)
	}
	return svg
}
