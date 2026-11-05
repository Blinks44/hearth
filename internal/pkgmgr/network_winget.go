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

// wingetSourceNames are the sources WinGet itself lists from: an explicit
// source, as winget-font, is only searched when named.
func wingetSourceNames(data []byte) ([]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var out []string
	for {
		var source struct {
			Name     string
			Explicit bool
		}
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
		if !source.Explicit {
			out = append(out, source.Name)
		}
	}
	return out, nil
}

// wingetTable reads one source at a time: the localized table then always has
// name, id, installed and optionally available columns, without a source column.
// Column positions are display cells, not UTF-8 bytes; CJK headers are valid.
// Rows of what the source does not know, MSIX\ and ARP\ ids that WinGet
// truncates, are no packages of it.
func wingetTable(data []byte) ([]inventory.SystemPackage, error) {
	text := terminalEscape.ReplaceAllString(string(data), "")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = renderLine(line)
	}
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
		// One line and no table is WinGet's message that nothing is
		// installed from the source, in the language of Windows.
		if text := strings.TrimSpace(strings.Join(lines, "\n")); text != "" && !strings.Contains(text, "\n") {
			return nil, nil
		}
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
		if strings.HasPrefix(cols[1], `MSIX\`) || strings.HasPrefix(cols[1], `ARP\`) {
			continue
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

// renderLine is what a terminal shows of line: WinGet's spinner goes back
// with \r and \b on the line its table header then overwrites.
func renderLine(line string) string {
	var out []rune
	cursor := 0
	for _, r := range strings.TrimSuffix(line, "\r") {
		switch r {
		case '\r':
			cursor = 0
		case '\b':
			cursor = max(cursor-1, 0)
		default:
			if cursor < len(out) {
				out[cursor] = r
			} else {
				out = append(out, r)
			}
			cursor++
		}
	}
	return string(out)
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
