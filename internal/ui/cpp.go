package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// cppPage is C and C++'s page among the languages.
const cppPage = "/tools/cpp"

// cppCards are the commands the side cards show rather than the table.
var cppCards = []string{"make", "pkg-config", "ccache"}

// cppView is C and C++'s page: the compilers and build systems, how the
// terminal finds cc, the OS's toolset, ccache and the build variables.
func (a *App) cppView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, _ := a.Snapshot.Tool("cpp")
		s := st.Cpp
		if st.Err != "" || s == nil || len(s.Tools) == 0 && s.Base == nil && len(listed(st.Installations)) == 0 {
			a.toolEmpty(c, "cpp", "C/C++")
			return
		}
		ins := st.Installations
		active := inventory.TerminalCopy(ins)
		rows := slices.DeleteFunc(slices.Clone(s.Tools), func(in inventory.Installation) bool { return slices.Contains(cppCards, in.Tool) })
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.cppHeader(c, st, active)
				segments(c, "cpp-tabs", &a.cppTab, []segment{
					{locales.Installed(), strconv.Itoa(len(rows))},
					{"PATH", ""},
				})
				if a.cppTab == 1 {
					a.cppPath(c, st, active)
					return
				}
				columns(c, 12, func() {
					if len(rows) == 0 {
						toolNote(c, locales.CppNoTools())
					} else {
						a.cppTable(c, s, rows)
					}
					if s.Base != nil {
						toolNote(c, map[string]string{
							"clt": locales.CppNoteClt(), "xcode": locales.CppNoteClt(),
							"build-essential": locales.CppNoteBuildEssential(), "msvc": locales.CppNoteMsvc(),
						}[s.Base.ID])
					}
					a.cppPath(c, st, active)
				}, func() {
					a.cppBaseCard(c, s)
					a.cppCacheCard(c, st)
					a.cppVarsCard(c, st, active)
				})
			})
		})
	})
}

// cppHeader is the logo, the toolset and where the other tools come from,
// and the cards: what cc runs, the build system and ccache.
func (a *App) cppHeader(c *ui.Context, st inventory.ToolState, active int) {
	t := c.Theme()
	pal := Colors(t)
	s := st.Cpp
	var lead []string
	if s.Base != nil {
		lead = append(lead, strings.TrimSpace(s.Base.Name+" "+s.Base.Version))
	}
	all := make([]int, len(s.Tools))
	for i := range all {
		all[i] = i
	}
	for _, n := range sourceNames(s.Tools, all, -1) {
		if n != locales.SourceSystem() {
			lead = append(lead, n)
		}
	}
	toolHead(c, "cpp", "C/C++", func() {}, strings.Join(lead, " · "), func() {
		if active >= 0 {
			in := st.Installations[active]
			caption := sourceName(in.Source)
			if s.Base != nil && in.Source.Kind == inventory.KindSystem {
				caption = strings.TrimSpace(s.Base.Name + " " + s.Base.Version)
			}
			stateCard(c, t.Success, locales.CppCompiler(), strings.TrimSpace(s.Compiler+" "+major(in.Version)), caption)
		} else {
			stateCard(c, pal.TextFaint, locales.CppCompiler(), "—", locales.NotInPath())
		}
		build := "—"
		caption := locales.NotInPath()
		for _, name := range []string{"cmake", "meson", "make"} {
			if in, ok := cppFirst(s, name); ok {
				build, caption = name+" "+minor(in.Version), sourceName(in.Source)
				break
			}
		}
		stateCard(c, pal.TextFaint, locales.CppBuild(), build, caption)
		if in, ok := cppFirst(s, "ccache"); ok {
			stateCard(c, pal.TextFaint, locales.CppCache(), "ccache "+minor(in.Version), sourceName(in.Source))
		} else {
			stateCard(c, pal.TextFaint, locales.CppCache(), "—", locales.NotInPath())
		}
	})
}

// cppTable lists the compilers and build systems: the version and what
// the program is, the source, the path and how it is run. The copy cc
// runs stands out.
func (a *App) cppTable(c *ui.Context, s *inventory.CppState, rows []inventory.Installation) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 12, 8, 16).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.ColumnVersion()).FontSize(11.5).TextColor(pal.TextFaint).Width(132).Shrink(0).SingleLine()
			ui.Text(c, locales.ColumnSource()).FontSize(11.5).TextColor(pal.TextFaint).Width(100).Shrink(0).SingleLine()
			ui.Text(c, locales.ColumnPath()).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
		})
		for _, in := range rows {
			first, _ := cppFirst(s, in.Tool)
			used := first.RealPath == in.RealPath && in.OnPath > 0
			row := ui.Row(c).Gap(12).Padding(12, 12, 12, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
			if used && in.Tool == s.Compiler {
				row.Background(pal.ActiveRow)
			}
			row.Children(func() {
				ui.Column(c).Width(132).Shrink(0).Gap(2).Children(func() {
					v := in.Version
					if v == "" {
						v = locales.UnknownVersion()
					}
					ui.Text(c, v).Font(FontMono).FontSize(13.5).SingleLine()
					ui.Text(c, cppLabel(s, in)).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
				})
				ui.Row(c).Width(100).Shrink(0).Children(func() { sourceBadge(c, in.Source) })
				ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
					clipped(c, tilde(in.Path, a.Home)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted)
					ui.Row(c).Gap(5).Wrap().Children(func() {
						switch {
						case used && strings.HasPrefix(in.Tool, "gcc-"):
							chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.CppCommand(in.Tool))
						case used:
							chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ChipInPath())
						case in.OnPath > 0:
							chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.TagShadowed())
						}
					})
				})
				a.rowActions(c, in)
			})
		}
	})
}

// cppPath is how the terminal finds cc, and that macOS's gcc is clang.
func (a *App) cppPath(c *ui.Context, st inventory.ToolState, active int) {
	s := st.Cpp
	pathCard(c, s.Command, st.Installations, active, a.Home)
	if !s.GCCIsClang {
		return
	}
	if i := slices.IndexFunc(s.Tools, func(in inventory.Installation) bool { return strings.HasPrefix(in.Tool, "gcc-") }); i >= 0 {
		warningRow(c, locales.CppGccIsClang(major(s.Tools[i].Version), s.Tools[i].Tool))
	} else {
		toolNote(c, locales.CppGccIsClangOnly())
	}
}

// cppBaseCard is the OS's toolset, with make and pkg-config.
func (a *App) cppBaseCard(c *ui.Context, s *inventory.CppState) {
	listCard(c, locales.CppBase(), "", func() {
		if b := s.Base; b != nil {
			listRow(c, strings.TrimSpace(b.Name+" "+b.Version), true, tilde(b.Path, a.Home), func() {})
		} else {
			cardNote(c, locales.CppNoBase())
		}
		for _, name := range []string{"make", "pkg-config"} {
			if in, ok := cppFirst(s, name); ok {
				listRow(c, strings.TrimSpace(name+" "+in.Version), true, sourceName(in.Source)+" · "+tilde(in.Path, a.Home), func() {})
			}
		}
	})
}

// cppCacheCard is ccache: its cache's size and whether Cleanup offers it,
// and whether CMake builds go through it.
func (a *App) cppCacheCard(c *ui.Context, st inventory.ToolState) {
	in, ok := cppFirst(st.Cpp, "ccache")
	aside := ""
	if ok {
		aside = in.Version
	}
	listCard(c, locales.CppCache()+" · ccache", aside, func() {
		if !ok {
			cardNote(c, locales.CppNoCcache())
			return
		}
		var cache inventory.Cache
		var usage inventory.Usage
		counted := false
		if a.Sizes != nil {
			if i := slices.IndexFunc(a.Sizes.Caches, func(ch inventory.Cache) bool { return ch.ID == "ccache" }); i >= 0 {
				cache = a.Sizes.Caches[i]
				usage, counted = a.Sizes.Paths[cache.Path]
			}
		}
		if counted {
			caption := tilde(cache.Path, a.Home)
			if slices.ContainsFunc(a.candidates(c), func(cd cleanup.Candidate) bool { return cd.Kind == cleanup.Cache && cd.ID == "ccache" }) {
				caption += " · " + locales.CppInCleanup()
			}
			listRow(c, locales.CppCacheSize(formatBytes(usage.Bytes)), false, caption, func() {})
		} else {
			cardNote(c, locales.CppNoCache())
		}
		launcher := st.Vars["CMAKE_C_COMPILER_LAUNCHER"]
		on := strings.Contains(launcher, "ccache")
		caption := "CMAKE_C_COMPILER_LAUNCHER=" + launcher
		if launcher == "" {
			caption = "CMAKE_C_COMPILER_LAUNCHER · " + locales.CppNotSet()
		}
		listRow(c, locales.CppCMake(), false, caption, func() { stateText(c, on) })
	})
}

// cppVarsCard is the variables that change what builds use, as the
// terminal sets them, and what is used when they are not set.
func (a *App) cppVarsCard(c *ui.Context, st inventory.ToolState, active int) {
	s := st.Cpp
	names := []string{"CC", "CXX", "CMAKE_GENERATOR"}
	if s.Base != nil && (s.Base.ID == "clt" || s.Base.ID == "xcode") {
		names = append(names, "SDKROOT")
	}
	listCard(c, locales.CppVars(), locales.CppVarsAside(), func() {
		for _, name := range names {
			if v := st.Vars[name]; v != "" {
				listRow(c, name, true, v, func() {})
				continue
			}
			caption := locales.CppNotSet()
			switch {
			case name == "CC" && active >= 0 && s.Compiler != "":
				caption += " · " + s.Command + " → " + s.Compiler
			case name == "SDKROOT":
				caption += " · " + locales.CppSdkFrom(s.Base.Name)
			}
			listRow(c, name, true, caption, func() {
				ui.Text(c, "—").TextColor(Colors(c.Theme()).TextFaint)
			})
		}
	})
}

// cppFirst is the copy of the command name that the terminal runs, else
// the first found.
func cppFirst(s *inventory.CppState, name string) (inventory.Installation, bool) {
	var found []inventory.Installation
	for _, in := range s.Tools {
		if in.Tool == name {
			found = append(found, in)
		}
	}
	if len(found) == 0 {
		return inventory.Installation{}, false
	}
	if i := inventory.TerminalCopy(found); i >= 0 {
		return found[i], true
	}
	return found[0], true
}

// cppLabel names the program a command runs, as Apple clang or GCC.
func cppLabel(s *inventory.CppState, in inventory.Installation) string {
	switch {
	case in.Tool == "clang" && s.AppleClang && in.Source.Kind == inventory.KindSystem:
		return "Apple clang"
	case in.Tool == "clang":
		return "Clang"
	case in.Tool == "gcc" || strings.HasPrefix(in.Tool, "gcc-"):
		return "GCC"
	}
	return map[string]string{"cmake": "CMake", "ninja": "Ninja", "meson": "Meson"}[in.Tool]
}

// major is a version's first number, as 17 of 17.0.0; minor its first
// two, as 3.31 of 3.31.2.
func major(v string) string {
	m, _, _ := strings.Cut(v, ".")
	return m
}

func minor(v string) string {
	parts := strings.SplitN(v, ".", 3)
	return strings.Join(parts[:min(len(parts), 2)], ".")
}
