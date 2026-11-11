package ops

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// AppsPathSettable reports that Hearth can give apps started from the
// Dock the terminal's PATH.
const AppsPathSettable = true

// AppsPathPlan returns the plan that gives apps started from the Dock,
// Finder or Spotlight the PATH of env, the terminal's: launchctl setenv
// gives it launchd now, and Hearth's launch agent gives it again as the
// user signs in. It is the PATH as it is now: a later change to the
// terminal's needs the plan again.
func AppsPathPlan(env sysenv.Env) (Plan, error) {
	path := env.Get("PATH")
	file := sysenv.AppsAgent(env.Get("HOME"))
	before, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Plan{}, fmt.Errorf("read %s: %w", file, err)
	}
	launchctl := []string{"/bin/launchctl", "setenv", "PATH", path}
	edit := &FileEdit{Path: file, Before: string(before), After: launchAgent(sysenv.AppsAgentLabel, launchctl), New: err != nil}
	count := len(filepath.SplitList(path))
	return Plan{
		Title: locales.AppsPathTitle(), Why: locales.AppsPathWhy(),
		Steps: []Step{{Edit: edit}, {Argv: launchctl}},
		Notes: []Note{
			{Title: locales.AppsPathReopen(), Text: locales.AppsPathReopenText()},
			{Title: locales.AppsPathLoginItem(), Text: locales.AppsPathLoginItemText(sysenv.AppsAgentLabel)},
		},
		Changes: []string{locales.AppsPathChange(count)},
		Verify: func(ctx context.Context) (string, error) {
			if sysenv.AppsPath(ctx, run.Exec{}) != path {
				return "", errors.New(locales.AppsPathOther())
			}
			return locales.AppsPathChecked(count), nil
		},
	}, nil
}

// AppsPathStopPlan returns the plan that takes back what AppsPathPlan
// did for the user of home: apps opened after get the system's PATH
// again.
func AppsPathStopPlan(home string) Plan {
	return Plan{
		Title: locales.AppsPathStopTitle(), Why: locales.AppsPathStopWhy(),
		Steps: []Step{
			{Argv: []string{"/bin/rm", "-f", sysenv.AppsAgent(home)}},
			{Argv: []string{"/bin/launchctl", "unsetenv", "PATH"}},
		},
		Notes:   []Note{{Title: locales.AppsPathReopen(), Text: locales.AppsPathReopenText()}},
		Changes: []string{locales.AppsPathStopChange()},
		Verify: func(ctx context.Context) (string, error) {
			if sysenv.AppsPath(ctx, run.Exec{}) != "" {
				return "", errors.New(locales.AppsPathOther())
			}
			return locales.AppsPathStopChecked(), nil
		},
	}
}

// launchAgent is the property list of a launch agent labeled label that
// runs argv as the user signs in.
func launchAgent(label string, argv []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + xmlText(label) + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range argv {
		b.WriteString("\t\t<string>" + xmlText(a) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`)
	return b.String()
}

// xmlText is s escaped for an XML element's text.
func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
