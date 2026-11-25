package pkgmgr

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// SearchResult is a package returned by an explicit user-triggered search
// against a package manager's catalog.
type SearchResult struct {
	Name        string
	Version     string
	Kind        string
	Description string
	Origin      string
}

const maxSearchResults = 30

// validSearchQuery allows common package search tokens while rejecting leading
// dashes (which managers might parse as flags) and control/shell characters.
var validSearchQuery = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+@/:\- ]{0,99}$`)

// ErrInvalidQuery is returned when a search query is empty or contains
// characters that cannot safely be passed to a package manager CLI.
var ErrInvalidQuery = errors.New("invalid package search query")

// CanSearch reports whether Hearth supports explicit catalog search for manager id.
func CanSearch(id string) bool {
	return slices.Contains([]string{
		"brew", "winget", "scoop", "choco",
		"apt", "dnf", "pacman", "zypper", "apk", "flatpak", "snap",
	}, id)
}

// Search queries m's catalog for packages matching query. It is strictly
// read-only and must only be called on explicit user action (never during
// automatic scans or on every keystroke). kind optionally narrows results
// (e.g. "formula" or "cask" for Homebrew).
func Search(ctx context.Context, r run.Runner, term sysenv.Env, m inventory.PackageManager, query, kind string) ([]SearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" || !validSearchQuery.MatchString(q) {
		return nil, ErrInvalidQuery
	}
	if !CanSearch(m.ID) {
		return nil, fmt.Errorf("search unsupported for %s", m.ID)
	}
	if m.ID != "winget" && m.Path == "" {
		return nil, fmt.Errorf("search %s: missing executable path", m.ID)
	}
	env := append(slices.Clone(term),
		"LC_ALL=C", "LANG=C", "NO_COLOR=1",
		"HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_ENV_HINTS=1",
	)
	exec := func(args ...string) (run.Result, error) {
		return r.Run(ctx, run.Cmd{
			Args:      args,
			Env:       env,
			Timeout:   30 * time.Second,
			MaxOutput: 4 << 20,
		})
	}
	switch m.ID {
	case "brew":
		return searchBrew(exec, m.Path, q, kind)
	case "apt":
		return searchAPT(exec, m.Path, q)
	case "dnf":
		return searchDNF(exec, m.Path, q)
	case "pacman":
		return searchPacman(exec, m.Path, q)
	case "zypper":
		return searchZypper(exec, m.Path, q)
	case "apk":
		return searchAPK(exec, m.Path, q)
	case "flatpak":
		return searchFlatpak(exec, m.Path, q)
	case "snap":
		return searchSnap(exec, m.Path, q)
	case "winget":
		return searchWinGet(exec, q)
	case "scoop":
		return searchScoop(exec, m.Path, q)
	case "choco":
		return searchChoco(exec, m.Path, q)
	}
	return nil, fmt.Errorf("search unsupported for %s", m.ID)
}

func searchBrew(exec func(...string) (run.Result, error), brewPath, q, kind string) ([]SearchResult, error) {
	// Piped `brew search` omits `==> Formulae`/`==> Casks` headers, and
	// `brew search --desc` only searches descriptions (missing exact names like
	// "ripgrep"), while name search alone misses description matches (like
	// "Grafana" -> "gcx"). Run explicit kind name searches and `--desc` in
	// parallel, rank name matches ahead of description-only matches, reuse any
	// descriptions returned by `--desc`, and enrich remaining results via `brew desc`.
	kinds := []string{"formula", "cask"}
	if kind == "formula" || kind == "cask" {
		kinds = []string{kind}
	}
	type queryOut struct {
		items     []SearchResult
		succeeded bool
		err       error
	}
	nameOuts := make([]queryOut, len(kinds))
	var descOut queryOut
	var wg sync.WaitGroup
	for i, k := range kinds {
		wg.Go(func() {
			res, err := exec(brewPath, "search", "--"+k, q)
			if err != nil {
				nameOuts[i].err = fmt.Errorf("brew search: %w", err)
				return
			}
			if res.ExitCode == 0 {
				nameOuts[i].succeeded = true
				nameOuts[i].items = parseBrewSearch(string(res.Stdout), k)
				return
			}
			if isBrewNoMatch(string(res.Stdout), string(res.Stderr)) {
				nameOuts[i].succeeded = true
				return
			}
			nameOuts[i].err = fmt.Errorf("brew search: exit %d", res.ExitCode)
		})
	}
	wg.Go(func() {
		args := []string{brewPath, "search"}
		if kind == "formula" || kind == "cask" {
			args = append(args, "--"+kind)
		}
		args = append(args, "--desc", q)
		res, err := exec(args...)
		if err != nil {
			descOut.err = fmt.Errorf("brew search: %w", err)
			return
		}
		if res.ExitCode == 0 {
			descOut.succeeded = true
			descOut.items = parseBrewSearch(string(res.Stdout), kind)
			return
		}
		if isBrewNoMatch(string(res.Stdout), string(res.Stderr)) {
			descOut.succeeded = true
			return
		}
		descOut.err = fmt.Errorf("brew search: exit %d", res.ExitCode)
	})
	wg.Wait()

	var nameResults []SearchResult
	var firstErr error
	anySucceeded := descOut.succeeded
	for _, out := range nameOuts {
		if out.succeeded {
			anySucceeded = true
		}
		if out.err != nil && firstErr == nil {
			firstErr = out.err
		}
		nameResults = append(nameResults, out.items...)
	}
	if !anySucceeded {
		if firstErr != nil {
			return nil, firstErr
		}
		if descOut.err != nil {
			return nil, descOut.err
		}
	}

	// Reuse descriptions returned by `--desc` so matching name results already
	// have descriptions without extra `brew desc` lookups.
	descMap := make(map[[2]string]string, len(descOut.items))
	for _, item := range descOut.items {
		if item.Description != "" {
			descMap[[2]string{item.Kind, strings.ToLower(item.Name)}] = item.Description
		}
	}
	for i := range nameResults {
		if nameResults[i].Description == "" {
			nameResults[i].Description = descMap[[2]string{nameResults[i].Kind, strings.ToLower(nameResults[i].Name)}]
		}
	}

	combined := append(prioritizeSearchMatches(nameResults, q), descOut.items...)
	results := capSearchResults(combined)
	enrichBrewDescriptions(exec, brewPath, results)
	return results, nil
}

func isBrewNoMatch(stdout, stderr string) bool {
	combined := strings.ToLower(stdout + " " + stderr)
	return strings.Contains(combined, "no formulae or casks found") ||
		strings.Contains(combined, "no formula found") ||
		strings.Contains(combined, "no cask found")
}

// prioritizeSearchMatches places exact name matches and prefix matches first so
// short queries (like "git" or "jq") are not dropped when capping catalog
// search output.
func prioritizeSearchMatches(in []SearchResult, q string) []SearchResult {
	lowerQ := strings.ToLower(q)
	var exact, prefix, rest []SearchResult
	for _, r := range in {
		lower := strings.ToLower(r.Name)
		switch {
		case lower == lowerQ:
			exact = append(exact, r)
		case strings.HasPrefix(lower, lowerQ):
			prefix = append(prefix, r)
		default:
			rest = append(rest, r)
		}
	}
	out := make([]SearchResult, 0, len(in))
	out = append(out, exact...)
	out = append(out, prefix...)
	out = append(out, rest...)
	return out
}

func enrichBrewDescriptions(exec func(...string) (run.Result, error), brewPath string, results []SearchResult) {
	byKind := map[string][]string{}
	for _, r := range results {
		if r.Description != "" || r.Name == "" {
			continue
		}
		byKind[r.Kind] = append(byKind[r.Kind], r.Name)
	}
	kinds := []string{"formula", "cask"}
	parsed := make([][]SearchResult, len(kinds))
	var wg sync.WaitGroup
	for i, kind := range kinds {
		names := byKind[kind]
		if len(names) == 0 {
			continue
		}
		wg.Go(func() {
			args := append([]string{brewPath, "desc", "--" + kind}, names...)
			res, err := exec(args...)
			if err != nil || res.ExitCode != 0 {
				return
			}
			parsed[i] = parseBrewSearch(string(res.Stdout), kind)
		})
	}
	wg.Wait()
	descs := map[[2]string]string{}
	for i, kind := range kinds {
		for _, item := range parsed[i] {
			if item.Description != "" {
				descs[[2]string{kind, strings.ToLower(item.Name)}] = item.Description
			}
		}
	}
	for i := range results {
		if results[i].Description != "" {
			continue
		}
		if d := descs[[2]string{results[i].Kind, strings.ToLower(results[i].Name)}]; d != "" {
			results[i].Description = d
		}
	}
}

func cleanBrewName(raw string) string {
	name := strings.TrimSpace(raw)
	for {
		trimmed := strings.TrimSpace(strings.TrimSuffix(name, "✔"))
		for _, tag := range []string{"(deprecated)", "(disabled)"} {
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, tag))
		}
		if trimmed == name {
			return name
		}
		name = trimmed
	}
}

func parseBrewSearch(out, forcedKind string) []SearchResult {
	out = terminalEscape.ReplaceAllString(out, "")
	section := forcedKind
	if section == "" {
		section = "formula"
	}
	var results []SearchResult
	for raw := range strings.SplitSeq(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "==>") {
			lower := strings.ToLower(line)
			switch {
			case strings.Contains(lower, "cask"):
				section = "cask"
			case strings.Contains(lower, "formula"):
				section = "formula"
			}
			continue
		}
		if strings.HasPrefix(line, "If you meant") || strings.HasPrefix(line, "Warning:") || strings.HasPrefix(line, "Error:") {
			continue
		}
		if left, desc, ok := strings.Cut(line, ":"); ok {
			name := cleanBrewName(left)
			if name != "" && !strings.Contains(name, " ") {
				results = append(results, SearchResult{
					Name:        name,
					Kind:        section,
					Description: strings.TrimSpace(desc),
				})
				continue
			}
		}
		for tok := range strings.FieldsSeq(line) {
			if tok == "" || tok == "✔" || strings.HasPrefix(tok, "(") {
				continue
			}
			results = append(results, SearchResult{
				Name: tok,
				Kind: section,
			})
		}
	}
	return results
}

func searchAPT(exec func(...string) (run.Result, error), aptPath, q string) ([]SearchResult, error) {
	aptCache := "apt-cache"
	if filepath.IsAbs(aptPath) {
		aptCache = filepath.Join(filepath.Dir(aptPath), "apt-cache")
	}
	res, err := exec(aptCache, "search", "--", q)
	if err != nil {
		return nil, fmt.Errorf("apt-cache search: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("apt-cache search: exit %d", res.ExitCode)
	}
	var results []SearchResult
	out := terminalEscape.ReplaceAllString(string(res.Stdout), "")
	for raw := range strings.SplitSeq(out, "\n") {
		name, desc, ok := strings.Cut(strings.TrimSpace(raw), " - ")
		if !ok || name == "" {
			continue
		}
		results = append(results, SearchResult{
			Name:        strings.TrimSpace(name),
			Kind:        "package",
			Description: strings.TrimSpace(desc),
		})
	}
	return capSearchResults(prioritizeSearchMatches(results, q)), nil
}

func searchDNF(exec func(...string) (run.Result, error), dnfPath, q string) ([]SearchResult, error) {
	res, err := exec(dnfPath, "search", "-q", "--", q)
	if err != nil {
		return nil, fmt.Errorf("dnf search: %w", err)
	}
	if res.ExitCode == 1 && strings.Contains(strings.ToLower(string(res.Stderr)+string(res.Stdout)), "no matches") {
		return nil, nil
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("dnf search: exit %d", res.ExitCode)
	}
	var results []SearchResult
	for raw := range strings.SplitSeq(string(res.Stdout), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "=") {
			continue
		}
		left, desc, ok := strings.Cut(line, " : ")
		if !ok {
			continue
		}
		name := strings.TrimSpace(left)
		if dot := strings.LastIndex(name, "."); dot > 0 {
			arch := name[dot+1:]
			if slices.Contains([]string{"x86_64", "aarch64", "noarch", "i686", "src", "armv7hl"}, arch) {
				name = name[:dot]
			}
		}
		if name != "" {
			results = append(results, SearchResult{
				Name:        name,
				Kind:        "package",
				Description: strings.TrimSpace(desc),
			})
		}
	}
	return capSearchResults(prioritizeSearchMatches(results, q)), nil
}

func searchPacman(exec func(...string) (run.Result, error), pacmanPath, q string) ([]SearchResult, error) {
	res, err := exec(pacmanPath, "-Ss", "--", q)
	if err != nil {
		return nil, fmt.Errorf("pacman -Ss: %w", err)
	}
	// pacman -Ss exits 1 when no matches are found.
	if res.ExitCode == 1 && len(strings.TrimSpace(string(res.Stderr))) == 0 {
		return nil, nil
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("pacman -Ss: exit %d", res.ExitCode)
	}
	var results []SearchResult
	lines := strings.Split(string(res.Stdout), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		repo, name, ok := strings.Cut(fields[0], "/")
		if !ok || name == "" {
			continue
		}
		desc := ""
		if i+1 < len(lines) && (strings.HasPrefix(lines[i+1], " ") || strings.HasPrefix(lines[i+1], "\t")) {
			desc = strings.TrimSpace(lines[i+1])
			i++
		}
		results = append(results, SearchResult{
			Name:        name,
			Version:     fields[1],
			Kind:        "package",
			Description: desc,
			Origin:      repo,
		})
	}
	return capSearchResults(prioritizeSearchMatches(results, q)), nil
}

func searchZypper(exec func(...string) (run.Result, error), zypperPath, q string) ([]SearchResult, error) {
	res, err := exec(zypperPath, "--non-interactive", "search", "-d", "--", q)
	if err != nil {
		return nil, fmt.Errorf("zypper search: %w", err)
	}
	if res.ExitCode == 104 { // ZYPPER_EXIT_INF_CAP_NOT_FOUND
		return nil, nil
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("zypper search: exit %d", res.ExitCode)
	}
	var results []SearchResult
	for raw := range strings.SplitSeq(string(res.Stdout), "\n") {
		if !strings.Contains(raw, "|") {
			continue
		}
		parts := strings.Split(raw, "|")
		if len(parts) < 4 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		summary := strings.TrimSpace(parts[2])
		kind := strings.TrimSpace(parts[3])
		if name == "" || strings.EqualFold(name, "Name") || strings.HasPrefix(name, "-") {
			continue
		}
		results = append(results, SearchResult{
			Name:        name,
			Kind:        kind,
			Description: summary,
		})
	}
	return capSearchResults(prioritizeSearchMatches(results, q)), nil
}

func searchAPK(exec func(...string) (run.Result, error), apkPath, q string) ([]SearchResult, error) {
	res, err := exec(apkPath, "search", "-v", "--", q)
	if err != nil {
		return nil, fmt.Errorf("apk search: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("apk search: exit %d", res.ExitCode)
	}
	var results []SearchResult
	for raw := range strings.SplitSeq(string(res.Stdout), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		pkgVer, desc, _ := strings.Cut(line, " - ")
		name, ver := splitAPKNameVersion(strings.TrimSpace(pkgVer))
		if name == "" {
			continue
		}
		results = append(results, SearchResult{
			Name:        name,
			Version:     ver,
			Kind:        "package",
			Description: strings.TrimSpace(desc),
		})
	}
	return capSearchResults(prioritizeSearchMatches(results, q)), nil
}

func splitAPKNameVersion(s string) (name, version string) {
	// APK packages are formatted as <name>-<ver>-r<rel>; find the dash before
	// the version token starting with a digit.
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '-' && s[i+1] >= '0' && s[i+1] <= '9' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func searchFlatpak(exec func(...string) (run.Result, error), flatpakPath, q string) ([]SearchResult, error) {
	res, err := exec(flatpakPath, "search", "--columns=application,version,description,remotes", "--", q)
	if err != nil {
		return nil, fmt.Errorf("flatpak search: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("flatpak search: exit %d", res.ExitCode)
	}
	var results []SearchResult
	out := terminalEscape.ReplaceAllString(string(res.Stdout), "")
	for raw := range strings.SplitSeq(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.EqualFold(line, "No matches found") {
			continue
		}
		parts := strings.Split(raw, "\t")
		if len(parts) < 1 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		var ver, desc, origin string
		if len(parts) > 1 {
			ver = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			desc = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			origin, _, _ = strings.Cut(strings.TrimSpace(parts[3]), ",")
			origin = strings.TrimSpace(origin)
		}
		results = append(results, SearchResult{
			Name:        name,
			Version:     ver,
			Kind:        "app",
			Description: desc,
			Origin:      origin,
		})
	}
	return capSearchResults(prioritizeSearchMatches(results, q)), nil
}

func searchSnap(exec func(...string) (run.Result, error), snapPath, q string) ([]SearchResult, error) {
	res, err := exec(snapPath, "find", q)
	if err != nil {
		return nil, fmt.Errorf("snap find: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("snap find: exit %d", res.ExitCode)
	}
	var results []SearchResult
	out := terminalEscape.ReplaceAllString(string(res.Stdout), "")
	for i, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || (i == 0 && strings.HasPrefix(line, "Name")) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		desc := ""
		if len(fields) > 4 {
			desc = strings.Join(fields[4:], " ")
		}
		results = append(results, SearchResult{
			Name:        fields[0],
			Version:     fields[1],
			Kind:        "snap",
			Description: desc,
		})
	}
	return capSearchResults(prioritizeSearchMatches(results, q)), nil
}

func searchWinGet(exec func(...string) (run.Result, error), q string) ([]SearchResult, error) {
	res, err := exec("winget", "search", "--query", q, "--source", "winget", "--disable-interactivity", "--count", "30")
	if err != nil {
		return nil, fmt.Errorf("winget search: %w", err)
	}
	out := string(res.Stdout)
	results, hasTable := parseWinGetSearch(out)
	if hasTable {
		return capSearchResults(prioritizeSearchMatches(results, q)), nil
	}
	// WinGet exits with APPINSTALLER_CLI_ERROR_NO_APPLICATIONS_FOUND (0x8A150014,
	// signed -1978335212) and prints a single localized line when nothing matches.
	if res.ExitCode == 0 || res.ExitCode == 20 || res.ExitCode == -1978335212 || res.ExitCode == 2316632084 ||
		strings.Contains(strings.ToLower(out), "no package found") {
		return nil, nil
	}
	return nil, fmt.Errorf("winget search: exit %d", res.ExitCode)
}

func parseWinGetSearch(out string) ([]SearchResult, bool) {
	text := terminalEscape.ReplaceAllString(out, "")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = renderLine(line)
	}
	headerIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i > 0 && len(trimmed) >= 3 && strings.Trim(trimmed, "-") == "" {
			headerIdx = i - 1
			break
		}
	}
	if headerIdx < 0 || headerIdx+2 >= len(lines) {
		return nil, false
	}
	cells := displayCells(lines[headerIdx])
	matches := tableColumns.FindAllStringIndex(strings.TrimRight(string(cells), " "), -1)
	starts := make([]int, 0, len(matches))
	for _, match := range matches {
		starts = append(starts, len([]rune(string(cells)[:match[0]])))
	}
	if len(starts) < 3 {
		return nil, false
	}
	var results []SearchResult
	for _, line := range lines[headerIdx+2:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "<") {
			continue
		}
		row := displayCells(line)
		cols := make([]string, len(starts))
		for i, start := range starts {
			end := len(row)
			if i+1 < len(starts) {
				end = min(end, starts[i+1])
			}
			if start < end {
				cols[i] = strings.TrimSpace(string(row[start:end]))
			}
		}
		desc := cols[0]
		id := cols[1]
		ver := cols[2]
		if v, _, ok := strings.Cut(ver, " "); ok {
			ver = v
		}
		if id == "" || strings.Contains(id, " ") || strings.Contains(id, "…") || strings.Contains(id, "...") {
			continue
		}
		results = append(results, SearchResult{
			Name:        id,
			Version:     ver,
			Kind:        "package",
			Description: desc,
			Origin:      "winget",
		})
	}
	return results, true
}

func searchScoop(exec func(...string) (run.Result, error), scoopPath, q string) ([]SearchResult, error) {
	res, err := exec("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scoopPath, "search", q)
	if err != nil {
		return nil, fmt.Errorf("scoop search: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("scoop search: exit %d", res.ExitCode)
	}
	return capSearchResults(parseScoopSearch(string(res.Stdout))), nil
}

func parseScoopSearch(out string) []SearchResult {
	out = terminalEscape.ReplaceAllString(out, "")
	var results []SearchResult
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	inTable := false
	bucket := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Name") && strings.Contains(line, "Version") && strings.Contains(line, "Source") {
			inTable = true
			continue
		}
		if strings.HasPrefix(line, "----") {
			continue
		}
		if inTable {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				results = append(results, SearchResult{
					Name:    fields[0],
					Version: fields[1],
					Kind:    "app",
					Origin:  fields[2],
				})
			}
			continue
		}
		if strings.HasPrefix(line, "'") && strings.HasSuffix(line, "bucket:") {
			b, _, _ := strings.Cut(strings.TrimPrefix(line, "'"), "'")
			bucket = b
			continue
		}
		if name, rest, ok := strings.Cut(line, " ("); ok && !strings.Contains(name, " ") {
			ver, _, _ := strings.Cut(rest, ")")
			results = append(results, SearchResult{
				Name:    name,
				Version: ver,
				Kind:    "app",
				Origin:  bucket,
			})
		}
	}
	return results
}

func searchChoco(exec func(...string) (run.Result, error), chocoPath, q string) ([]SearchResult, error) {
	res, err := exec(chocoPath, "search", q, "--limit-output", "--page-size=30")
	if err != nil {
		return nil, fmt.Errorf("choco search: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("choco search: exit %d", res.ExitCode)
	}
	var results []SearchResult
	for raw := range strings.SplitSeq(strings.ReplaceAll(string(res.Stdout), "\r\n", "\n"), "\n") {
		name, ver, ok := strings.Cut(strings.TrimSpace(raw), "|")
		if !ok || name == "" {
			continue
		}
		results = append(results, SearchResult{
			Name:    strings.TrimSpace(name),
			Version: strings.TrimSpace(ver),
			Kind:    "package",
		})
	}
	return capSearchResults(results), nil
}

func capSearchResults(in []SearchResult) []SearchResult {
	seen := make(map[[2]string]bool, len(in))
	out := make([]SearchResult, 0, min(len(in), maxSearchResults))
	for _, r := range in {
		if r.Name == "" {
			continue
		}
		key := [2]string{r.Kind, strings.ToLower(r.Name)}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
		if len(out) >= maxSearchResults {
			break
		}
	}
	return out
}
