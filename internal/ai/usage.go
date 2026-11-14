package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// transcript is what Hearth takes from an agent's transcript: the folder
// the session ran in and the tokens it used.
type transcript struct {
	id, dir string
	usage   []inventory.TokenUsage
}

// cached is a transcript read before, while its file stays the same.
type cached struct {
	size int64
	mod  time.Time
	t    transcript
}

// transcripts keeps what each transcript gave, by its path, for the next
// scan: a long history is gigabytes, and only the latest files change.
var transcripts = struct {
	sync.Mutex
	files map[string]cached
}{files: map[string]cached{}}

// Sessions finds the sessions in Claude Code's and Codex's local history,
// in its folders of term's home or of CLAUDE_CONFIG_DIR and CODEX_HOME.
// A file it cannot read is left out.
func Sessions(term sysenv.Env) []inventory.AgentSession {
	home := homeDir(term)
	var out []inventory.AgentSession
	var claudeDirs []string
	if d := dir(agentDirs("claude"), term, home); d != "" {
		claudeDirs = append(claudeDirs, d)
	}
	// Claude Code also keeps its files in the XDG folder.
	if home != "" {
		claudeDirs = append(claudeDirs, filepath.Join(home, ".config", "claude"))
	}
	seen := map[string]bool{}
	for _, d := range claudeDirs {
		if real, err := filepath.EvalSymlinks(d); err == nil && !seen[real] {
			seen[real] = true
			out = append(out, claudeSessions(filepath.Join(d, "projects"), time.Local)...)
		}
	}
	if d := dir(agentDirs("codex"), term, home); d != "" {
		for _, sub := range []string{"sessions", "archived_sessions"} {
			out = append(out, codexSessions(filepath.Join(d, sub), time.Local)...)
		}
	}
	return out
}

// agentDirs are the settings directories of the agent id, as the agents
// table writes them.
func agentDirs(id string) []string {
	for _, a := range agents {
		if a.id == id {
			return a.dir
		}
	}
	return nil
}

// claudeSessions are Claude Code's sessions under root, its projects
// folder: a folder each project, <session>.jsonl in it, and beside it a
// <session> folder with its subagents' transcripts and kept tool results.
func claudeSessions(root string, loc *time.Location) []inventory.AgentSession {
	projects, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []inventory.AgentSession
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(root, p.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			id, ok := strings.CutSuffix(e.Name(), ".jsonl")
			if !ok || e.IsDir() {
				continue
			}
			file := filepath.Join(dir, e.Name())
			info, err := e.Info()
			if err != nil {
				continue
			}
			t, err := read(file, info, func(r io.Reader) (transcript, error) { return parseClaude(r, loc) })
			if err != nil {
				continue
			}
			s := inventory.AgentSession{Agent: "claude", ID: id, Dir: t.dir, Files: []string{file}, Size: info.Size(), Modified: info.ModTime()}
			usage := t.usage
			side := filepath.Join(dir, id)
			if fi, err := os.Stat(side); err == nil && fi.IsDir() {
				s.Files = append(s.Files, side)
				_ = filepath.WalkDir(side, func(path string, d fs.DirEntry, err error) error {
					if err != nil || d.IsDir() {
						return nil
					}
					info, err := d.Info()
					if err != nil {
						return nil
					}
					s.Size += info.Size()
					if info.ModTime().After(s.Modified) {
						s.Modified = info.ModTime()
					}
					// A subagent's transcript: its tokens are the session's.
					if strings.HasSuffix(path, ".jsonl") {
						if st, err := read(path, info, func(r io.Reader) (transcript, error) { return parseClaude(r, loc) }); err == nil {
							usage = append(usage, st.usage...)
						}
					}
					return nil
				})
			}
			s.Usage = merge(usage)
			out = append(out, s)
		}
	}
	return out
}

// codexSessions are Codex's sessions under root: its rollout-*.jsonl
// files, in folders by date or not.
func codexSessions(root string, loc *time.Location) []inventory.AgentSession {
	var out []inventory.AgentSession
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		t, err := read(path, info, func(r io.Reader) (transcript, error) { return parseCodex(r, loc) })
		if err != nil {
			return nil
		}
		id := t.id
		if id == "" {
			id = strings.TrimSuffix(d.Name(), ".jsonl")
		}
		out = append(out, inventory.AgentSession{Agent: "codex", ID: id, Dir: t.dir, Files: []string{path}, Size: info.Size(),
			Modified: info.ModTime(), Usage: merge(t.usage)})
		return nil
	})
	return out
}

// read parses the transcript at path, or takes what it gave before while
// the file stays the same.
func read(path string, info fs.FileInfo, parse func(io.Reader) (transcript, error)) (transcript, error) {
	transcripts.Lock()
	c, ok := transcripts.files[path]
	transcripts.Unlock()
	if ok && c.size == info.Size() && c.mod.Equal(info.ModTime()) {
		return c.t, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return transcript{}, err
	}
	defer func() { _ = f.Close() }() // read only
	t, err := parse(f)
	if err != nil {
		return transcript{}, err
	}
	transcripts.Lock()
	transcripts.files[path] = cached{size: info.Size(), mod: info.ModTime(), t: t}
	transcripts.Unlock()
	return t, nil
}

// eachLine calls fn with each line of r, however long; the line is valid
// only during the call.
func eachLine(r io.Reader, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, 1<<20)
	var long []byte
	for {
		chunk, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			long = append(long, chunk...)
			continue
		}
		if len(long) > 0 {
			long = append(long, chunk...)
			chunk = long
		}
		if len(chunk) > 0 {
			fn(chunk)
		}
		long = long[:0]
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// claudeLine is what Hearth reads of a line of Claude Code's transcript.
type claudeLine struct {
	Type      string
	Timestamp time.Time
	Cwd       string
	RequestID string `json:"requestId"`
	Message   struct {
		ID    string
		Model string
		Usage *struct {
			Input         int64 `json:"input_tokens"`
			CacheCreation int64 `json:"cache_creation_input_tokens"`
			CacheRead     int64 `json:"cache_read_input_tokens"`
			Output        int64 `json:"output_tokens"`
			Split         *struct {
				Minutes int64 `json:"ephemeral_5m_input_tokens"`
				Hour    int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		}
	}
}

// parseClaude reads a transcript of Claude Code: a JSON object a line,
// the agent's replies with the tokens of their request. A reply is
// written again as it streams, with the same message and request IDs: the
// last line of each counts.
func parseClaude(r io.Reader, loc *time.Location) (transcript, error) {
	var t transcript
	type key struct{ message, request string }
	replies := map[key]int{}
	err := eachLine(r, func(line []byte) {
		usage := bytes.Contains(line, []byte(`"usage"`))
		if !usage && (t.dir != "" || !bytes.Contains(line, []byte(`"cwd"`))) {
			return
		}
		var l claudeLine
		if json.Unmarshal(line, &l) != nil {
			return
		}
		if t.dir == "" {
			t.dir = l.Cwd
		}
		u := l.Message.Usage
		// "<synthetic>" marks a reply Claude Code made up, as an error's.
		if l.Type != "assistant" || u == nil || l.Message.Model == "" || l.Message.Model == "<synthetic>" {
			return
		}
		tu := inventory.TokenUsage{Day: l.Timestamp.In(loc).Format(time.DateOnly), Model: l.Message.Model,
			Input: u.Input, CacheRead: u.CacheRead, Output: u.Output, CacheWrite: u.CacheCreation}
		if u.Split != nil && u.Split.Minutes+u.Split.Hour == u.CacheCreation {
			tu.CacheWrite, tu.CacheWrite1h = u.Split.Minutes, u.Split.Hour
		}
		k := key{l.Message.ID, l.RequestID}
		if i, ok := replies[k]; ok && k.message != "" {
			t.usage[i] = tu
			return
		}
		replies[k] = len(t.usage)
		t.usage = append(t.usage, tu)
	})
	return t, err
}

// codexTokens are Codex's counts of a session's tokens; Input has the
// cached ones and the ones written to the cache in it.
type codexTokens struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
}

// codexLine is what Hearth reads of a line of Codex's rollout.
type codexLine struct {
	Type      string
	Timestamp time.Time
	Payload   struct {
		Type  string
		ID    string
		Cwd   string
		Model string
		Info  *struct {
			Total codexTokens `json:"total_token_usage"`
		}
	}
}

// parseCodex reads a rollout of Codex: a JSON object a line, the
// session's folder first, each turn's model, and the session's tokens so
// far after each request, of which the growth counts.
func parseCodex(r io.Reader, loc *time.Location) (transcript, error) {
	var t transcript
	var model string
	var last codexTokens
	err := eachLine(r, func(line []byte) {
		if !bytes.Contains(line, []byte(`"token_count"`)) && !bytes.Contains(line, []byte(`"turn_context"`)) &&
			!bytes.Contains(line, []byte(`"session_meta"`)) {
			return
		}
		var l codexLine
		if json.Unmarshal(line, &l) != nil {
			return
		}
		switch {
		case l.Type == "session_meta":
			t.id, t.dir = l.Payload.ID, l.Payload.Cwd
		case l.Type == "turn_context":
			model = l.Payload.Model
		case l.Payload.Type == "token_count" && l.Payload.Info != nil && model != "":
			cur := l.Payload.Info.Total
			d := codexTokens{cur.Input - last.Input, cur.Cached - last.Cached, cur.CacheWrite - last.CacheWrite, cur.Output - last.Output}
			// A count below the last one starts over, as after compacting.
			if d.Input < 0 || d.Cached < 0 || d.CacheWrite < 0 || d.Output < 0 {
				d = cur
			}
			last = cur
			if d == (codexTokens{}) {
				return
			}
			t.usage = append(t.usage, inventory.TokenUsage{Day: l.Timestamp.In(loc).Format(time.DateOnly), Model: model,
				Input: max(d.Input-d.Cached-d.CacheWrite, 0), CacheWrite: d.CacheWrite, CacheRead: d.Cached, Output: d.Output})
		}
	})
	return t, err
}

// merge sums the tokens of each day and model, by day and then model.
func merge(us []inventory.TokenUsage) []inventory.TokenUsage {
	type key struct{ day, model string }
	at := map[key]int{}
	var out []inventory.TokenUsage
	for _, u := range us {
		k := key{u.Day, u.Model}
		i, ok := at[k]
		if !ok {
			at[k] = len(out)
			out = append(out, inventory.TokenUsage{Day: u.Day, Model: u.Model})
			i = len(out) - 1
		}
		o := &out[i]
		o.Input += u.Input
		o.CacheWrite += u.CacheWrite
		o.CacheWrite1h += u.CacheWrite1h
		o.CacheRead += u.CacheRead
		o.Output += u.Output
	}
	slices.SortFunc(out, func(a, b inventory.TokenUsage) int {
		if c := strings.Compare(a.Day, b.Day); c != 0 {
			return c
		}
		return strings.Compare(a.Model, b.Model)
	})
	return out
}
