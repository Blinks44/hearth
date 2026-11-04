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

// Brand returns a tool's logo by its Simple Icons slug, as in brand/<slug>.
func Brand(slug string) *ui.SVG { return mustIcon("brand/" + slug) }

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
