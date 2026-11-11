package ops

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/inventory"
)

// FileEdit changes a file of the user's, as a step of a plan. Before is
// the text the plan was made from, After what the file becomes; New
// makes a file that is not there.
type FileEdit struct {
	Path, Before, After string
	New                 bool
	// Hide are values that show as inventory.Hidden in the diff, as the
	// tokens of an MCP server copied: they go in the file, not on screen.
	Hide []string
}

// Masked is text with e.Hide's values hidden, as the diff shows it.
func (e FileEdit) Masked(text string) string {
	for _, h := range e.Hide {
		if h != "" {
			text = strings.ReplaceAll(text, h, inventory.Hidden)
		}
	}
	return text
}

// The lines around Hearth's block in a file with # comments: what is
// between them is Hearth's, the rest the user's.
const (
	BlockStart = "# >>> hearth >>>"
	BlockEnd   = "# <<< hearth <<<"
)

// ErrChanged reports a file that changed after the plan was made from it.
var ErrChanged = errors.New("the file changed after the plan was made")

// Block returns text with Hearth's block holding body: in place of the
// block where it is, at the end where it is not; body "" removes the
// block. The other lines stay as they are.
func Block(text, body string) string {
	lines := splitLines(text)
	start, end := blockLines(lines)
	var block []string
	if body != "" {
		block = append(append([]string{BlockStart}, splitLines(strings.TrimRight(body, "\n"))...), BlockEnd)
	}
	if start < 0 {
		if block == nil {
			return text
		}
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		return joinLines(append(lines, block...))
	}
	out := append(append(append([]string(nil), lines[:start]...), block...), lines[end+1:]...)
	// The empty line Block put before the block goes with it.
	if block == nil && start > 0 && start == len(out) && out[start-1] == "" {
		out = out[:start-1]
	}
	return joinLines(out)
}

// AddToBlock returns text with lines in Hearth's block, after the lines
// the block has; those it has already stay once. A block that is not
// there is made at the end.
func AddToBlock(text string, lines ...string) string {
	all := splitLines(text)
	var body []string
	if start, end := blockLines(all); start >= 0 {
		body = all[start+1 : end]
	}
	for _, l := range lines {
		if !slices.Contains(body, l) {
			body = append(slices.Clip(body), l)
		}
	}
	return Block(text, strings.Join(body, "\n"))
}

// CommentOut returns text with the lines numbered in lines, counting
// from 1, made comments that say why: the user's lines are never
// removed. Lines in Hearth's block and comments already are left.
func CommentOut(text string, lines []int, why string) string {
	all := splitLines(text)
	start, end := blockLines(all)
	for _, n := range lines {
		i := n - 1
		if i < 0 || i >= len(all) || start >= 0 && i >= start && i <= end || strings.HasPrefix(strings.TrimSpace(all[i]), "#") {
			continue
		}
		all[i] = "# hearth: " + why + " · " + all[i]
	}
	return joinLines(all)
}

// blockLines returns the indexes of Hearth's block's first and last
// lines, -1 when there is none.
func blockLines(lines []string) (start, end int) {
	start = -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case BlockStart:
			start = i
		case BlockEnd:
			if start >= 0 {
				return start, i
			}
		}
	}
	return -1, -1
}

// splitLines splits text into lines without their line breaks; text that
// ends with one has no empty last line.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// DiffLine is a line a file edit removes or adds.
type DiffLine struct {
	// Added is true for a line After has, false for one Before had.
	Added bool
	// Line is its number, in After when added, in Before when removed.
	Line int
	Text string
}

// Diff returns the lines that differ between before and after, in file
// order, each removed line before the one added in its place.
func Diff(before, after string) []DiffLine {
	a, b := splitLines(before), splitLines(after)
	// The longest common subsequence, from the ends: files are short.
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i, ai := range slices.Backward(a) {
		for j, bj := range slices.Backward(b) {
			if ai == bj {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i, j = i+1, j+1
		case j == len(b) || i < len(a) && lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, DiffLine{Line: i + 1, Text: a[i]})
			i++
		default:
			out = append(out, DiffLine{Added: true, Line: j + 1, Text: b[j]})
			j++
		}
	}
	return out
}

// applyEdit writes e.After to the file, after a copy of what was there,
// and returns the copy's path, "" for a new file. A file that is a link,
// as dotfile managers make, is written where it leads.
func applyEdit(e FileEdit, now time.Time) (backup string, err error) {
	path := realFile(e.Path)
	data, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read %s: %w", e.Path, err)
	}
	if string(data) != e.Before || existed == e.New {
		return "", fmt.Errorf("%s: %w", e.Path, ErrChanged)
	}
	mode := os.FileMode(0o644)
	if existed {
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", e.Path, err)
		}
		mode = info.Mode().Perm()
		backup = path + ".hearth-" + now.Format("2006-01-02-150405") + ".bak"
		if err := os.WriteFile(backup, data, mode); err != nil {
			return "", fmt.Errorf("back up %s: %w", e.Path, err)
		}
	} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("make the folder of %s: %w", e.Path, err)
	}
	if err := writeFile(path, []byte(e.After), mode); err != nil {
		return backup, err
	}
	return backup, nil
}

// restoreEdit puts back what the file held before applyEdit: the copy,
// or no file for a new one.
func restoreEdit(e FileEdit, backup string) error {
	path := realFile(e.Path)
	if backup == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", e.Path, err)
		}
		return nil
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		return fmt.Errorf("read the copy of %s: %w", e.Path, err)
	}
	info, err := os.Stat(backup)
	if err != nil {
		return fmt.Errorf("stat the copy of %s: %w", e.Path, err)
	}
	return writeFile(path, data, info.Mode().Perm())
}

// writeFile replaces the file whole, through a file beside it, so that a
// failure leaves the old one.
func writeFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".hearth-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(tmp.Name(), mode)); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// realFile is where path leads through links; path itself when it leads
// nowhere yet.
func realFile(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// SetJSON returns text, a JSON object, with its top-level key set to
// value: in place where the key is, after the last key where it is not,
// indented as the keys around it. The rest of the text stays as it is.
func SetJSON(text, key string, value any) (string, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return "", fmt.Errorf("encode %s: %w", key, err)
	}
	raw := strings.TrimSuffix(buf.String(), "\n")
	dec := json.NewDecoder(strings.NewReader(text))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return "", fmt.Errorf("not a JSON object")
	}
	// The end of the last value, and the start of the first key.
	last, first := -1, -1
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("read JSON: %w", err)
		}
		end := int(dec.InputOffset())
		if first < 0 {
			first = strings.LastIndex(text[:end-1], `"`)
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return "", fmt.Errorf("read JSON: %w", err)
		}
		last = int(dec.InputOffset())
		if tok == key {
			return text[:last-len(v)] + raw + text[last:], nil
		}
	}
	if _, err := dec.Token(); err != nil {
		return "", fmt.Errorf("read JSON: %w", err)
	}
	closing := int(dec.InputOffset()) - 1
	name, _ := json.Marshal(key)
	if last < 0 {
		return text[:closing] + "\n  " + string(name) + ": " + raw + "\n" + text[closing:], nil
	}
	indent := text[strings.LastIndex(text[:first], "\n")+1 : first]
	if strings.TrimSpace(indent) != "" {
		indent = " "
	} else {
		indent = "\n" + indent
	}
	return text[:last] + "," + indent + string(name) + ": " + raw + text[last:], nil
}

// SetTOML returns text, a TOML file, with key in [table] set to value,
// a string: the key's line replaced where it is, a line after the
// table's last where it is not, the table at the end where there is
// none. The other lines stay as they are.
func SetTOML(text, table, key, value string) (string, error) {
	return setTOMLRaw(text, table, key, strconv.Quote(value))
}

// setTOMLRaw is SetTOML with value written as it is, as false.
func setTOMLRaw(text, table, key, value string) (string, error) {
	line := key + " = " + value
	lines := splitLines(text)
	start := -1
	for i, l := range lines {
		if tomlTable(l) == table {
			start = i
			break
		}
	}
	if start < 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "["+table+"]", line)
	} else {
		at := start + 1
		for i := start + 1; i < len(lines) && tomlTable(lines[i]) == ""; i++ {
			l := strings.TrimSpace(lines[i])
			if k, _, ok := strings.Cut(l, "="); ok && strings.Trim(strings.TrimSpace(k), `"'`) == key {
				lines[i] = lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))] + line
				at = -1
				break
			}
			if l != "" {
				at = i + 1
			}
		}
		if at >= 0 {
			lines = slices.Insert(lines, at, line)
		}
	}
	out := joinLines(lines)
	if _, err := toml.Decode(out, new(map[string]any)); err != nil {
		return "", fmt.Errorf("set %s.%s: %w", table, key, err)
	}
	return out, nil
}

// tomlTable is the name of the table a line opens, "" for other lines.
func tomlTable(line string) string {
	l := strings.TrimSpace(line)
	if strings.HasPrefix(l, "[[") {
		return l
	}
	if !strings.HasPrefix(l, "[") {
		return ""
	}
	name, _, ok := strings.Cut(l[1:], "]")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}
