package sysenv

import (
	"bytes"
	"cmp"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

// The start of the terminal's shell Hearth traces to find the lines that
// take the time, and how many of those it keeps.
const (
	traceFrom     = 500 * time.Millisecond
	minLine       = 10 * time.Millisecond
	maxLines      = 8
	maxTraceBytes = 16 << 20
)

// Culprit is a known cause of a slow shell start, as "nvm"; "" for none.
type Culprit string

// The culprits Hearth knows, with the lines of startup files that load
// them.
const (
	CulpritNvm      Culprit = "nvm"
	CulpritConda    Culprit = "conda"
	CulpritPyenv    Culprit = "pyenv"
	CulpritRbenv    Culprit = "rbenv"
	CulpritRvm      Culprit = "rvm"
	CulpritSdkman   Culprit = "sdkman"
	CulpritCompinit Culprit = "compinit"
	CulpritOhMyZsh  Culprit = "oh-my-zsh"
	CulpritOhMyPosh Culprit = "oh-my-posh"
	CulpritPoshGit  Culprit = "posh-git"
	CulpritBrew     Culprit = "brew"
)

var culprits = []struct {
	c  Culprit
	re *regexp.Regexp
}{
	{CulpritNvm, regexp.MustCompile(`nvm\.sh`)},
	{CulpritConda, regexp.MustCompile(`conda\S*['"]?\s+['"]?shell\.|conda\.sh|conda-hook`)},
	{CulpritPyenv, regexp.MustCompile(`pyenv\s+(virtualenv-)?init`)},
	{CulpritRbenv, regexp.MustCompile(`rbenv\s+init`)},
	{CulpritRvm, regexp.MustCompile(`\.rvm/scripts/rvm`)},
	{CulpritSdkman, regexp.MustCompile(`sdkman-init\.sh`)},
	{CulpritOhMyZsh, regexp.MustCompile(`oh-my-zsh\.sh`)},
	{CulpritOhMyPosh, regexp.MustCompile(`(?i)oh-my-posh`)},
	{CulpritPoshGit, regexp.MustCompile(`(?i)posh-git`)},
	{CulpritBrew, regexp.MustCompile(`brew\s+--prefix`)},
	{CulpritCompinit, regexp.MustCompile(`(^|[\s;&])compinit(\s|$|;)`)},
}

// cachedCompinit is compinit with -C, which skips the check of the
// completion files.
var cachedCompinit = regexp.MustCompile(`compinit\s+(-\w*\s+)*-\w*C`)

// StartupCulprit is the known cause of a slow start that line of a
// startup file loads, "" when none.
func StartupCulprit(line string) Culprit {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "#") {
		return ""
	}
	for _, k := range culprits {
		if k.re.MatchString(line) {
			if k.c == CulpritCompinit && cachedCompinit.MatchString(line) {
				return ""
			}
			return k.c
		}
	}
	return ""
}

// traceLine is a line of the shell's trace: when it ran, where, and how
// deep: in a sourced file, a function, eval or a command substitution.
type traceLine struct {
	at    float64
	file  string
	line  int
	level int
}

// parseTrace reads the shell's trace in out, lines its PS4 prefixed with
// '+', the marker and |time|file|line|frames|; bash repeats the '+' for
// each level of eval and command substitution. Lines without the marker
// are what the startup files print, or commands of several lines.
func parseTrace(out []byte, marker string) []traceLine {
	var lines []traceLine
	for l := range bytes.SplitSeq(out, []byte{'\n'}) {
		i := bytes.Index(l, []byte(marker+"|"))
		if i < 1 {
			continue
		}
		pluses := 0
		for j := i - 1; j >= 0 && l[j] == '+'; j-- {
			pluses++
		}
		f := strings.SplitN(string(l[i+len(marker)+1:]), "|", 5)
		if len(f) < 5 {
			continue
		}
		// bash writes EPOCHREALTIME with the locale's decimal separator.
		at, err := strconv.ParseFloat(strings.Replace(f[0], ",", ".", 1), 64)
		if err != nil {
			continue
		}
		n, _ := strconv.Atoi(f[2])
		frames, _ := strconv.Atoi(f[3])
		lines = append(lines, traceLine{at: at, file: f[1], line: n, level: frames*1000 + pluses})
	}
	return lines
}

// attribute gives the time of each traced line to the line of a startup
// file it ran for: the line itself at the top level of a startup file,
// else the last such line before it, which sourced the file it is in,
// called its function or evaluated its code. The time a line takes is
// until the next one starts; the trace ends at the command the shell was
// started for. It returns the startup files with their time in the order
// they ran, the lines with theirs in no order, and the time the startup
// files took in all.
func attribute(trace []traceLine, startup func(file string) bool) (files []inventory.StartupFile, lines []inventory.StartupLine, took time.Duration) {
	base := map[string]int{}
	anchor := -1
	byLine := map[[2]any]int{}
	byFile := map[string]int{}
	for i, t := range trace {
		if !strings.HasPrefix(t.file, "/") && t.level < 1000 {
			break // the command after the startup files
		}
		if startup(t.file) {
			b, ok := base[t.file]
			if !ok {
				b, base[t.file] = t.level, t.level
			}
			if t.level == b {
				key := [2]any{t.file, t.line}
				j, ok := byLine[key]
				if !ok {
					j = len(lines)
					byLine[key] = j
					lines = append(lines, inventory.StartupLine{File: t.file, Line: t.line})
				}
				anchor = j
				if _, ok := byFile[t.file]; !ok {
					byFile[t.file] = len(files)
					files = append(files, inventory.StartupFile{File: t.file})
				}
			}
		}
		if anchor < 0 || i+1 >= len(trace) {
			continue
		}
		d := max(0, time.Duration((trace[i+1].at-t.at)*float64(time.Second)))
		lines[anchor].Took += d
		files[byFile[lines[anchor].File]].Took += d
		took += d
	}
	return files, lines, took
}

// slowest scales the traced times to the start measured without the
// trace, which slows the shell, and keeps the slowest lines, with their
// text from their files.
func slowest(files []inventory.StartupFile, lines []inventory.StartupLine, traced, measured time.Duration) ([]inventory.StartupFile, []inventory.StartupLine) {
	if traced <= 0 {
		return nil, nil
	}
	scale := func(d time.Duration) time.Duration {
		return time.Duration(float64(d) * float64(measured) / float64(traced))
	}
	for i := range files {
		files[i].Took = scale(files[i].Took)
	}
	for i := range lines {
		lines[i].Took = scale(lines[i].Took)
	}
	slices.SortStableFunc(lines, func(a, b inventory.StartupLine) int { return cmp.Compare(b.Took, a.Took) })
	i := slices.IndexFunc(lines, func(l inventory.StartupLine) bool { return l.Took < minLine })
	if i >= 0 {
		lines = lines[:i]
	}
	lines = lines[:min(len(lines), maxLines)]
	texts := map[string][]string{}
	for i, l := range lines {
		text, ok := texts[l.File]
		if !ok {
			text = readLines(l.File)
			texts[l.File] = text
		}
		if l.Line >= 1 && l.Line <= len(text) {
			lines[i].Text = trimLine(text[l.Line-1])
		}
	}
	return files, lines
}

// readLines are the lines of a startup file, none when it is not one
// Hearth reads.
func readLines(file string) []string {
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	return strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
}

// trimLine is a line as the page shows it: no indentation, and not more
// than a row holds.
func trimLine(l string) string {
	l = strings.TrimSpace(l)
	if r := []rune(l); len(r) > 120 {
		l = string(r[:119]) + "…"
	}
	return l
}
