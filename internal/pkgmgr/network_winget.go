package pkgmgr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/width"

	"github.com/Blinks44/hearth/internal/inventory"
)

var tableColumns = regexp.MustCompile(`\S(?:.*?\S)?(?: {2,}|$)`)
var terminalEscape = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")

func queryWinget(probe networkProbe) ([]inventory.SystemPackage, error) {
	res, err := probe("source", "export", "--disable-interactivity")
	if err != nil {
		return nil, err
	}
	sources, err := wingetSourceNames(res.Stdout)
	if err != nil {
		return nil, err
	}
	var out []inventory.SystemPackage
	seen := map[string]bool{}
	var failures []error
	for _, source := range sources {
		res, err = probe("list", "--source", source, "--disable-interactivity", "--authentication-mode", "silent")
		if err != nil {
			failures = append(failures, err)
			continue
		}
		text := strings.TrimSpace(string(res.Stdout))
		if text == "No installed package found matching input criteria." {
			continue
		}
		packages, err := wingetTable(res.Stdout)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, p := range packages {
			p.Kind = "package"
			p.Origin = source
			key := p.Name + "\x00" + p.Version
			if !seen[key] {
				seen[key] = true
				out = append(out, p)
			}
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("WinGet has no configured sources")
	}
	return out, errors.Join(failures...)
}

func wingetSourceNames(data []byte) ([]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var out []string
	for {
		var source struct{ Name string }
		err := decoder.Decode(&source)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read WinGet sources: %w", err)
		}
		if source.Name == "" {
			return nil, fmt.Errorf("WinGet source has no name")
		}
		out = append(out, source.Name)
	}
	return out, nil
}

// wingetTable reads one source at a time: the localized table then always has
// name, id, installed and optionally available columns, without a source column.
// Column positions are display cells, not UTF-8 bytes; CJK headers are valid.
func wingetTable(data []byte) ([]inventory.SystemPackage, error) {
	text := terminalEscape.ReplaceAllString(string(data), "")
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	header := -1
	for i, line := range lines {
		if i > 0 && len(strings.TrimSpace(line)) >= 3 && strings.Trim(strings.TrimSpace(line), "-") == "" {
			header = i - 1
			break
		}
	}
	if header >= 0 {
		for _, line := range lines[:header] {
			if strings.Trim(line, " \t\b-|/\\") != "" {
				return nil, fmt.Errorf("WinGet reported a source diagnostic before its table")
			}
		}
	}
	if header < 0 {
		return nil, fmt.Errorf("WinGet table header not found")
	}
	cells := displayCells(lines[header])
	matches := tableColumns.FindAllStringIndex(strings.TrimRight(string(cells), " "), -1)
	// Convert byte positions to cell positions before slicing the data rows.
	starts := make([]int, 0, len(matches))
	for _, match := range matches {
		starts = append(starts, len([]rune(string(cells)[:match[0]])))
	}
	if len(starts) != 3 && len(starts) != 4 {
		return nil, fmt.Errorf("unsupported WinGet table columns")
	}
	var out []inventory.SystemPackage
	for _, line := range lines[header+2:] {
		if strings.TrimSpace(line) == "" {
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
		if cols[1] == "" || cols[2] == "" || strings.Contains(cols[1]+cols[2], "…") || strings.Contains(cols[1]+cols[2], "...") {
			return nil, fmt.Errorf("incomplete WinGet package row")
		}
		p := inventory.SystemPackage{Name: cols[1], Version: cols[2]}
		if len(cols) == 4 {
			p.Available = cols[3]
		}
		if strings.Contains(p.Available, "…") || strings.Contains(p.Available, "...") {
			return nil, fmt.Errorf("truncated WinGet version")
		}
		out = append(out, p)
	}
	return out, nil
}

func displayCells(s string) []rune {
	var out []rune
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		out = append(out, r)
		kind := width.LookupRune(r).Kind()
		if kind == width.EastAsianWide || kind == width.EastAsianFullwidth {
			out = append(out, ' ')
		}
	}
	return out
}
