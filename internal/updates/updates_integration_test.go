//go:build integration

package updates_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/scan"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/updates"
	"github.com/Blinks44/hearth/internal/versions"
)

// TestFindReal scans this computer, gets the version lists and asks mise
// online, and logs the updates found: compare them with mise outdated and
// the managers by hand.
func TestFindReal(t *testing.T) {
	term, err := sysenv.Terminal(t.Context(), run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	s := scan.Run(t.Context(), run.Exec{}, term, term, scan.Detectors)
	c := &versions.Cache{Dir: t.TempDir()}
	var l updates.Lists
	if node, err := versions.FetchNode(t.Context(), c); err == nil {
		l.Node = &node
	}
	if g, err := versions.FetchGo(t.Context(), c); err == nil {
		l.Go = &g
	}
	if p, err := versions.FetchPython(t.Context(), c); err == nil {
		l.Python = &p
	}
	if j, err := versions.FetchJava(t.Context(), c); err == nil {
		l.Java = &j
	}
	if b, err := versions.FetchBun(t.Context(), c); err == nil {
		l.Bun = &b
	}
	if st, ok := s.Tool("mise"); ok && len(st.MiseTools) > 0 {
		mise := sysenv.LookPathAll("mise", term)
		if len(mise) == 0 {
			t.Fatal("mise's tools without mise on PATH")
		}
		var installed []string
		dirs := map[string]bool{}
		for _, mt := range st.MiseTools {
			dirs[mt.Name] = true
			for _, in := range mt.Installations {
				installed = append(installed, mt.Name+"@"+in.Version)
			}
		}
		m, err := versions.FetchMise(t.Context(), c, run.Exec{}, term, mise[0], installed)
		if err != nil {
			t.Fatal(err)
		}
		l.Mise = &m
		for name, mt := range m.Tools {
			t.Logf("mise outdated: %s %+v", name, mt)
		}
	}
	for _, u := range updates.Find(&s, nil, l, time.Now()) {
		kind := "patch"
		if u.Kind == updates.NewLine {
			kind = "new line"
		}
		t.Logf("%s %s: %s -> %s %s", kind, u.Tool, u.Copy.Version, u.Version, strings.TrimSpace(u.LTS))
	}
}
