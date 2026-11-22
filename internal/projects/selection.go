package projects

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
)

// ErrNoCommonVersion means none of the known versions satisfies every
// requirement. It does not assert that the release list is exhaustive.
var ErrNoCommonVersion = errors.New("no common version in installed copies or available releases")

// Selection chooses one version for all of a tool's requirements. Installed
// copies take precedence over downloads, even when a newer release exists.
type Selection struct {
	Version   string
	Installed *inventory.Installation
	Ready     bool
}

// SelectVersion never changes a project or starts a program. Unknown policies
// stay unknown; a matching copy for each individual file is not enough.
func SelectVersion(st Status, releases []string, lines map[int]versions.Line) (Selection, error) {
	matches := make([]func(inventory.Installation) bool, 0, len(st.Checks))
	ready := st.Checked && !st.Downloads && st.Terminal != nil
	for _, ch := range st.Checks {
		if ch.SDK != nil {
			if !ch.Met {
				return Selection{}, fmt.Errorf("%s: SDK selection requires manual review", ch.File)
			}
			continue
		}
		match, err := requirementMatcher(st.Tool, ch.Requirement, lines)
		if err != nil {
			return Selection{}, fmt.Errorf("%s: %w", ch.File, err)
		}
		matches = append(matches, match)
		ready = ready && ch.Met
	}
	all := func(in inventory.Installation) bool {
		if in.Source.Shim || in.Stub {
			return false
		}
		for _, match := range matches {
			if !match(in) {
				return false
			}
		}
		return true
	}
	if ready && all(*st.Terminal) {
		return Selection{Version: st.Terminal.Version, Installed: st.Terminal, Ready: true}, nil
	}
	if in := newest(st.Installations, all); in != nil {
		return Selection{Version: in.Version, Installed: in}, nil
	}
	// When installing from scratch, prefer an exact packageManager pin if it
	// satisfies every constraint (including engines), then available releases,
	// then any other exact pin for offline installs without a release index.
	var candidates []string
	for _, ch := range st.Checks {
		if ch.Key == "packageManager" {
			if v, err := semver.StrictNewVersion(ch.Want); err == nil {
				candidates = append(candidates, v.String())
			}
		}
	}
	candidates = append(candidates, releases...)
	for _, ch := range st.Checks {
		if ch.Key != "packageManager" {
			if v, err := semver.StrictNewVersion(ch.Want); err == nil {
				candidates = append(candidates, v.String())
			}
		}
	}
	for _, v := range candidates {
		if all(inventory.Installation{Version: v}) {
			return Selection{Version: v}, nil
		}
	}
	return Selection{}, ErrNoCommonVersion
}
