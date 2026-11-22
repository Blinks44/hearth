package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

// geminiSessions reads Gemini CLI session transcripts under root/tmp/*/chats.
func geminiSessions(root string, loc *time.Location) []inventory.AgentSession {
	tmpDir := filepath.Join(root, "tmp")
	projDirs, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil
	}
	projMap := loadGeminiProjects(filepath.Join(root, "projects.json"))
	var out []inventory.AgentSession
	for _, p := range projDirs {
		if !p.IsDir() {
			continue
		}
		pDir := filepath.Join(tmpDir, p.Name())
		cwd := projMap[p.Name()]
		if raw, err := os.ReadFile(filepath.Join(pDir, ".project_root")); err == nil {
			if s := strings.TrimSpace(string(raw)); s != "" {
				cwd = s
			}
		}
		chatsDir := filepath.Join(pDir, "chats")
		_ = filepath.WalkDir(chatsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasPrefix(name, "session-") {
				return nil
			}
			var parse func(io.Reader) (transcript, error)
			var baseID string
			switch {
			case strings.HasSuffix(name, ".json"):
				baseID = strings.TrimSuffix(name, ".json")
				parse = func(r io.Reader) (transcript, error) { return parseGeminiJSON(r, loc) }
			case strings.HasSuffix(name, ".jsonl"):
				baseID = strings.TrimSuffix(name, ".jsonl")
				parse = func(r io.Reader) (transcript, error) { return parseGeminiJSONL(r, loc) }
			default:
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			tr, err := read(path, info, parse)
			if err != nil {
				return nil
			}
			id := tr.id
			if id == "" {
				id = baseID
			}
			dir := tr.dir
			if dir == "" {
				dir = cwd
			}
			out = append(out, inventory.AgentSession{
				Agent:    "gemini",
				ID:       id,
				Dir:      dir,
				Files:    []string{path},
				Size:     info.Size(),
				Modified: info.ModTime(),
				Usage:    merge(tr.usage),
			})
			return nil
		})
	}
	return out
}

// loadGeminiProjects reads ~/.gemini/projects.json into a slug -> path map.
func loadGeminiProjects(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw struct {
		Projects map[string]string `json:"projects"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	out := make(map[string]string, len(raw.Projects))
	for k, v := range raw.Projects {
		if k == "" || v == "" {
			continue
		}
		// Gemini CLI writes projects as {"/abs/path": "slug"}, handle both directions.
		if filepath.IsAbs(k) && !filepath.IsAbs(v) {
			out[v] = k
		} else {
			out[k] = v
		}
	}
	return out
}

type geminiTokens struct {
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Cached   int64 `json:"cached"`
	Thoughts int64 `json:"thoughts"`
}

type geminiMessage struct {
	ID        string        `json:"id"`
	Timestamp string        `json:"timestamp"`
	Type      string        `json:"type"`
	Model     string        `json:"model"`
	Tokens    *geminiTokens `json:"tokens"`
}

func geminiTokenUsage(m geminiMessage, fallbackDay string, loc *time.Location) (inventory.TokenUsage, bool) {
	if m.Tokens == nil || m.Model == "" {
		return inventory.TokenUsage{}, false
	}
	if m.Type != "" && m.Type != "gemini" && m.Type != "assistant" {
		return inventory.TokenUsage{}, false
	}
	uncached := max(m.Tokens.Input-m.Tokens.Cached, 0)
	output := m.Tokens.Output + m.Tokens.Thoughts
	if uncached == 0 && m.Tokens.Cached == 0 && output == 0 {
		return inventory.TokenUsage{}, false
	}
	day := parseDayString(m.Timestamp, loc)
	if day == "" {
		day = fallbackDay
	}
	if day == "" {
		return inventory.TokenUsage{}, false
	}
	return inventory.TokenUsage{
		Day:       day,
		Model:     m.Model,
		Input:     uncached,
		CacheRead: m.Tokens.Cached,
		Output:    output,
	}, true
}

func parseGeminiJSON(r io.Reader, loc *time.Location) (transcript, error) {
	var doc struct {
		SessionID   string          `json:"sessionId"`
		StartTime   string          `json:"startTime"`
		LastUpdated string          `json:"lastUpdated"`
		Messages    []geminiMessage `json:"messages"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return transcript{}, err
	}
	fallbackDay := parseDayString(doc.LastUpdated, loc)
	if fallbackDay == "" {
		fallbackDay = parseDayString(doc.StartTime, loc)
	}
	tr := transcript{id: doc.SessionID}
	for _, m := range doc.Messages {
		if tu, ok := geminiTokenUsage(m, fallbackDay, loc); ok {
			tr.usage = append(tr.usage, tu)
		}
	}
	return tr, nil
}

func parseGeminiJSONL(r io.Reader, loc *time.Location) (transcript, error) {
	var tr transcript
	var fallbackDay string
	byID := map[string]int{}
	err := eachLine(r, func(line []byte) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			return
		}
		if tr.id == "" && bytes.Contains(trimmed, []byte(`"sessionId"`)) {
			var hdr struct {
				SessionID string `json:"sessionId"`
				StartTime string `json:"startTime"`
			}
			if json.Unmarshal(trimmed, &hdr) == nil {
				if hdr.SessionID != "" {
					tr.id = hdr.SessionID
				}
				if d := parseDayString(hdr.StartTime, loc); d != "" {
					fallbackDay = d
				}
			}
		}
		if !bytes.Contains(trimmed, []byte(`"tokens"`)) {
			return
		}
		var m geminiMessage
		if json.Unmarshal(trimmed, &m) != nil {
			return
		}
		tu, ok := geminiTokenUsage(m, fallbackDay, loc)
		if !ok {
			return
		}
		if m.ID != "" {
			if idx, exists := byID[m.ID]; exists {
				tr.usage[idx] = tu
				return
			}
			byID[m.ID] = len(tr.usage)
		}
		tr.usage = append(tr.usage, tu)
	})
	return tr, err
}

// copilotSessions reads GitHub Copilot CLI sessions under root/session-state.
func copilotSessions(root string, loc *time.Location) []inventory.AgentSession {
	stateDir := filepath.Join(root, "session-state")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return nil
	}
	var out []inventory.AgentSession
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sessDir := filepath.Join(stateDir, e.Name())
		eventsPath := filepath.Join(sessDir, "events.jsonl")
		info, err := os.Stat(eventsPath)
		if err != nil || info.IsDir() {
			continue
		}
		tr, err := read(eventsPath, info, func(r io.Reader) (transcript, error) { return parseCopilot(r, loc) })
		if err != nil {
			continue
		}
		id := tr.id
		if id == "" {
			id = e.Name()
		}
		dir := tr.dir
		if dir == "" {
			dir = readCopilotWorkspaceCwd(filepath.Join(sessDir, "workspace.yaml"))
		}
		size, mod := dirStats(sessDir, info.ModTime())
		out = append(out, inventory.AgentSession{
			Agent:    "copilot",
			ID:       id,
			Dir:      dir,
			Files:    []string{sessDir},
			Size:     size,
			Modified: mod,
			Usage:    merge(tr.usage),
		})
	}
	return out
}

func readCopilotWorkspaceCwd(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var fallback string
	for line := range strings.SplitSeq(string(data), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if v == "" {
			continue
		}
		if k == "cwd" {
			return v
		}
		if k == "git_root" && fallback == "" {
			fallback = v
		}
	}
	return fallback
}

type copilotUsage struct {
	Input      int64 `json:"inputTokens"`
	Output     int64 `json:"outputTokens"`
	CacheRead  int64 `json:"cacheReadTokens"`
	CacheWrite int64 `json:"cacheWriteTokens"`
	// Alternate snake_case fields.
	InputSnake      int64 `json:"input_tokens"`
	OutputSnake     int64 `json:"output_tokens"`
	CacheReadSnake  int64 `json:"cache_read_tokens"`
	CacheWriteSnake int64 `json:"cache_write_tokens"`
}

func (u copilotUsage) normalized() (in, out, cacheRead, cacheWrite int64) {
	in = max(u.Input, u.InputSnake)
	out = max(u.Output, u.OutputSnake)
	cacheRead = max(u.CacheRead, u.CacheReadSnake)
	cacheWrite = max(u.CacheWrite, u.CacheWriteSnake)
	return
}

func parseCopilot(r io.Reader, loc *time.Location) (transcript, error) {
	var tr transcript
	var turnUsage []inventory.TokenUsage
	var shutdownUsage []inventory.TokenUsage
	type cum struct{ in, out, cr, cw int64 }
	lastByModel := map[string]cum{}

	err := eachLine(r, func(line []byte) {
		if !bytes.Contains(line, []byte(`"session.start"`)) &&
			!bytes.Contains(line, []byte(`"session.usage_record"`)) &&
			!bytes.Contains(line, []byte(`"assistant.usage"`)) &&
			!bytes.Contains(line, []byte(`"session.shutdown"`)) {
			return
		}
		var ev struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Data      struct {
				SessionID string `json:"sessionId"`
				Cwd       string `json:"cwd"`
				Context   *struct {
					Cwd string `json:"cwd"`
				} `json:"context"`
				Model string `json:"model"`
				copilotUsage
				Usage        *copilotUsage `json:"usage"`
				ModelMetrics map[string]struct {
					Usage copilotUsage `json:"usage"`
				} `json:"modelMetrics"`
			} `json:"data"`
		}
		if json.Unmarshal(line, &ev) != nil {
			return
		}
		switch ev.Type {
		case "session.start":
			if ev.Data.SessionID != "" && tr.id == "" {
				tr.id = ev.Data.SessionID
			}
			if tr.dir == "" {
				if ev.Data.Context != nil && ev.Data.Context.Cwd != "" {
					tr.dir = ev.Data.Context.Cwd
				} else if ev.Data.Cwd != "" {
					tr.dir = ev.Data.Cwd
				}
			}
		case "session.usage_record", "assistant.usage":
			if ev.Data.Model == "" {
				return
			}
			day := parseDayString(ev.Timestamp, loc)
			if day == "" {
				return
			}
			src := ev.Data.copilotUsage
			if ev.Data.Usage != nil {
				src = *ev.Data.Usage
			}
			in, out, cr, cw := src.normalized()
			if in == 0 && out == 0 && cr == 0 && cw == 0 {
				return
			}
			turnUsage = append(turnUsage, inventory.TokenUsage{
				Day:        day,
				Model:      ev.Data.Model,
				Input:      in,
				CacheRead:  cr,
				CacheWrite: cw,
				Output:     out,
			})
		case "session.shutdown":
			day := parseDayString(ev.Timestamp, loc)
			if day == "" {
				return
			}
			for model, mm := range ev.Data.ModelMetrics {
				if model == "" {
					continue
				}
				in, out, cr, cw := mm.Usage.normalized()
				prev := lastByModel[model]
				din, dout, dcr, dcw := in-prev.in, out-prev.out, cr-prev.cr, cw-prev.cw
				if din < 0 || dout < 0 || dcr < 0 || dcw < 0 {
					din, dout, dcr, dcw = in, out, cr, cw
				}
				lastByModel[model] = cum{in, out, cr, cw}
				if din == 0 && dout == 0 && dcr == 0 && dcw == 0 {
					continue
				}
				shutdownUsage = append(shutdownUsage, inventory.TokenUsage{
					Day:        day,
					Model:      model,
					Input:      din,
					CacheRead:  dcr,
					CacheWrite: dcw,
					Output:     dout,
				})
			}
		}
	})
	if len(turnUsage) > 0 {
		tr.usage = turnUsage
	} else {
		tr.usage = shutdownUsage
	}
	return tr, err
}

// kimiSessions reads Kimi Code sessions under root/sessions/<ws>/<session>/wire.jsonl.
func kimiSessions(root string, loc *time.Location) []inventory.AgentSession {
	indexDirs := loadKimiSessionIndex(filepath.Join(root, "session_index.jsonl"))
	sessionsRoot := filepath.Join(root, "sessions")
	wsDirs, err := os.ReadDir(sessionsRoot)
	if err != nil {
		return nil
	}
	var out []inventory.AgentSession
	for _, ws := range wsDirs {
		if !ws.IsDir() {
			continue
		}
		wsPath := filepath.Join(sessionsRoot, ws.Name())
		wsMeta := readKimiMetadata(filepath.Join(wsPath, "metadata.json"))
		sessEntries, err := os.ReadDir(wsPath)
		if err != nil {
			continue
		}
		for _, se := range sessEntries {
			if !se.IsDir() {
				continue
			}
			sessDir := filepath.Join(wsPath, se.Name())
			wirePath := filepath.Join(sessDir, "wire.jsonl")
			info, err := os.Stat(wirePath)
			if err != nil || info.IsDir() {
				continue
			}
			meta := readKimiMetadata(filepath.Join(sessDir, "metadata.json"))
			defaultModel := meta.model
			if defaultModel == "" {
				defaultModel = wsMeta.model
			}
			tr, err := read(wirePath, info, func(r io.Reader) (transcript, error) {
				return parseKimiWire(r, defaultModel, loc)
			})
			if err != nil {
				continue
			}
			usage := append([]inventory.TokenUsage(nil), tr.usage...)
			// Subagents live under <session>/agents/<agent>/wire.jsonl.
			agentsDir := filepath.Join(sessDir, "agents")
			_ = filepath.WalkDir(agentsDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || d.Name() != "wire.jsonl" {
					return nil
				}
				subInfo, err := d.Info()
				if err != nil {
					return nil
				}
				if sub, err := read(path, subInfo, func(r io.Reader) (transcript, error) {
					return parseKimiWire(r, defaultModel, loc)
				}); err == nil {
					usage = append(usage, sub.usage...)
				}
				return nil
			})
			id := se.Name()
			if meta.sessionID != "" {
				id = meta.sessionID
			}
			dir := tr.dir
			if dir == "" {
				dir = meta.dir
			}
			if dir == "" {
				dir = indexDirs[id]
			}
			if dir == "" {
				dir = indexDirs[se.Name()]
			}
			if dir == "" {
				dir = wsMeta.dir
			}
			size, mod := dirStats(sessDir, info.ModTime())
			out = append(out, inventory.AgentSession{
				Agent:    "kimi",
				ID:       id,
				Dir:      dir,
				Files:    []string{sessDir},
				Size:     size,
				Modified: mod,
				Usage:    merge(usage),
			})
		}
	}
	return out
}

func loadKimiSessionIndex(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }() // read only
	out := map[string]string{}
	_ = eachLine(f, func(line []byte) {
		var row struct {
			ID        string `json:"id"`
			SessionID string `json:"session_id"`
			WorkDir   string `json:"work_dir"`
			Cwd       string `json:"cwd"`
		}
		if json.Unmarshal(line, &row) != nil {
			return
		}
		id := row.SessionID
		if id == "" {
			id = row.ID
		}
		dir := row.WorkDir
		if dir == "" {
			dir = row.Cwd
		}
		if id != "" && dir != "" {
			out[id] = dir
		}
	})
	return out
}

type kimiMeta struct {
	sessionID string
	dir       string
	model     string
}

func readKimiMetadata(path string) kimiMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return kimiMeta{}
	}
	var raw struct {
		SessionID string `json:"session_id"`
		WorkDir   string `json:"work_dir"`
		Cwd       string `json:"cwd"`
		Model     string `json:"model"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return kimiMeta{}
	}
	dir := raw.WorkDir
	if dir == "" {
		dir = raw.Cwd
	}
	return kimiMeta{sessionID: raw.SessionID, dir: dir, model: raw.Model}
}

func parseKimiWire(r io.Reader, defaultModel string, loc *time.Location) (transcript, error) {
	var tr transcript
	model := defaultModel
	err := eachLine(r, func(line []byte) {
		if !bytes.Contains(line, []byte(`"token_usage"`)) &&
			!bytes.Contains(line, []byte(`"model"`)) &&
			!bytes.Contains(line, []byte(`"work_dir"`)) &&
			!bytes.Contains(line, []byte(`"cwd"`)) {
			return
		}
		var env struct {
			Timestamp json.RawMessage `json:"timestamp"`
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
			Message   *struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &env) != nil {
			return
		}
		msgType := env.Type
		payloadRaw := env.Payload
		if env.Message != nil {
			if env.Message.Type != "" {
				msgType = env.Message.Type
			}
			if len(env.Message.Payload) > 0 {
				payloadRaw = env.Message.Payload
			}
		}
		if len(payloadRaw) == 0 {
			return
		}
		var payload struct {
			WorkDir    string `json:"work_dir"`
			Cwd        string `json:"cwd"`
			Model      string `json:"model"`
			Scope      string `json:"scope"`
			IsTotal    bool   `json:"is_total"`
			TokenUsage *struct {
				Scope              string `json:"scope"`
				Model              string `json:"model"`
				InputOther         int64  `json:"input_other"`
				InputTokens        int64  `json:"input_tokens"`
				InputCacheRead     int64  `json:"input_cache_read"`
				CacheReadTokens    int64  `json:"cache_read_input_tokens"`
				InputCacheCreation int64  `json:"input_cache_creation"`
				CacheWriteTokens   int64  `json:"cache_creation_input_tokens"`
				Output             int64  `json:"output"`
				OutputTokens       int64  `json:"output_tokens"`
			} `json:"token_usage"`
		}
		if json.Unmarshal(payloadRaw, &payload) != nil {
			return
		}
		if tr.dir == "" {
			if payload.WorkDir != "" {
				tr.dir = payload.WorkDir
			} else if payload.Cwd != "" {
				tr.dir = payload.Cwd
			}
		}
		if payload.Model != "" {
			model = payload.Model
		}
		tu := payload.TokenUsage
		if tu == nil {
			return
		}
		// Skip session-level totals to avoid double-counting turn records.
		if payload.Scope == "session" || tu.Scope == "session" || payload.IsTotal {
			return
		}
		if msgType != "" && msgType != "StatusUpdate" && msgType != "status_update" && msgType != "TurnUsage" {
			return
		}
		m := tu.Model
		if m == "" {
			m = model
		}
		if m == "" {
			return
		}
		in := max(tu.InputOther, tu.InputTokens)
		cr := max(tu.InputCacheRead, tu.CacheReadTokens)
		cw := max(tu.InputCacheCreation, tu.CacheWriteTokens)
		out := max(tu.Output, tu.OutputTokens)
		if in == 0 && cr == 0 && cw == 0 && out == 0 {
			return
		}
		day := parseDayRaw(env.Timestamp, loc)
		if day == "" {
			return
		}
		tr.usage = append(tr.usage, inventory.TokenUsage{
			Day:        day,
			Model:      m,
			Input:      in,
			CacheRead:  cr,
			CacheWrite: cw,
			Output:     out,
		})
	})
	return tr, err
}

// grokSessions reads Grok Build sessions under root/sessions/<encoded-cwd>/<session-id>.
func grokSessions(root string, loc *time.Location) []inventory.AgentSession {
	sessionsRoot := filepath.Join(root, "sessions")
	projEntries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		return nil
	}
	var out []inventory.AgentSession
	for _, pe := range projEntries {
		if !pe.IsDir() {
			continue
		}
		projDir := filepath.Join(sessionsRoot, pe.Name())
		projCwd := readTrimmedFile(filepath.Join(projDir, ".cwd"))
		sessEntries, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, se := range sessEntries {
			if !se.IsDir() {
				continue
			}
			sessDir := filepath.Join(projDir, se.Name())
			usagePath := filepath.Join(sessDir, "usage.json")
			updatesPath := filepath.Join(sessDir, "updates.jsonl")
			var tr transcript
			var baseMod time.Time
			if info, err := os.Stat(usagePath); err == nil && !info.IsDir() {
				baseMod = info.ModTime()
				fallbackDay := baseMod.In(loc).Format(time.DateOnly)
				t, err := read(usagePath, info, func(r io.Reader) (transcript, error) {
					return parseGrokUsageJSON(r, fallbackDay, loc)
				})
				if err != nil {
					continue
				}
				tr = t
			} else if info, err := os.Stat(updatesPath); err == nil && !info.IsDir() {
				baseMod = info.ModTime()
				t, err := read(updatesPath, info, func(r io.Reader) (transcript, error) {
					return parseGrokUpdates(r, loc)
				})
				if err != nil {
					continue
				}
				tr = t
			} else {
				continue
			}
			dir := tr.dir
			if dir == "" {
				dir = readTrimmedFile(filepath.Join(sessDir, ".cwd"))
			}
			if dir == "" {
				dir = readGrokSummaryCwd(filepath.Join(sessDir, "summary.json"))
			}
			if dir == "" {
				dir = projCwd
			}
			size, mod := dirStats(sessDir, baseMod)
			out = append(out, inventory.AgentSession{
				Agent:    "grok",
				ID:       se.Name(),
				Dir:      dir,
				Files:    []string{sessDir},
				Size:     size,
				Modified: mod,
				Usage:    merge(tr.usage),
			})
		}
	}
	return out
}

func readTrimmedFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readGrokSummaryCwd(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s struct {
		Cwd        string `json:"cwd"`
		WorkingDir string `json:"working_dir"`
	}
	if json.Unmarshal(data, &s) != nil {
		return ""
	}
	if s.Cwd != "" {
		return s.Cwd
	}
	return s.WorkingDir
}

type grokTokenCounts struct {
	Model            string `json:"model"`
	Timestamp        string `json:"timestamp"`
	InputTokens      int64  `json:"input_tokens"`
	PromptTokens     int64  `json:"prompt_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	ReasoningTokens  int64  `json:"reasoning_tokens"`
}

func (g grokTokenCounts) toUsage(fallbackModel, fallbackDay string, loc *time.Location) (inventory.TokenUsage, bool) {
	model := g.Model
	if model == "" {
		model = fallbackModel
	}
	if model == "" {
		return inventory.TokenUsage{}, false
	}
	day := parseDayString(g.Timestamp, loc)
	if day == "" {
		day = fallbackDay
	}
	if day == "" {
		return inventory.TokenUsage{}, false
	}
	in := max(g.InputTokens, g.PromptTokens)
	cached := max(g.CachedTokens, g.CacheReadTokens)
	out := max(g.OutputTokens, g.CompletionTokens) + g.ReasoningTokens
	uncached := max(in-cached, 0)
	if uncached == 0 && cached == 0 && out == 0 {
		return inventory.TokenUsage{}, false
	}
	return inventory.TokenUsage{
		Day:       day,
		Model:     model,
		Input:     uncached,
		CacheRead: cached,
		Output:    out,
	}, true
}

func parseGrokUsageJSON(r io.Reader, fallbackDay string, loc *time.Location) (transcript, error) {
	var doc struct {
		Cwd      string                     `json:"cwd"`
		Model    string                     `json:"model"`
		Turns    []grokTokenCounts          `json:"turns"`
		Entries  []grokTokenCounts          `json:"entries"`
		Requests []grokTokenCounts          `json:"requests"`
		ByModel  map[string]grokTokenCounts `json:"by_model"`
		Models   map[string]grokTokenCounts `json:"models"`
		grokTokenCounts
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return transcript{}, err
	}
	tr := transcript{dir: doc.Cwd}
	list := doc.Turns
	if len(list) == 0 {
		list = doc.Entries
	}
	if len(list) == 0 {
		list = doc.Requests
	}
	if len(list) > 0 {
		for _, item := range list {
			if tu, ok := item.toUsage(doc.Model, fallbackDay, loc); ok {
				tr.usage = append(tr.usage, tu)
			}
		}
		return tr, nil
	}
	byModel := doc.ByModel
	if len(byModel) == 0 {
		byModel = doc.Models
	}
	if len(byModel) > 0 {
		for model, item := range byModel {
			if tu, ok := item.toUsage(model, fallbackDay, loc); ok {
				tr.usage = append(tr.usage, tu)
			}
		}
		return tr, nil
	}
	if tu, ok := doc.toUsage(doc.Model, fallbackDay, loc); ok {
		tr.usage = append(tr.usage, tu)
	}
	return tr, nil
}

func parseGrokUpdates(r io.Reader, loc *time.Location) (transcript, error) {
	var tr transcript
	err := eachLine(r, func(line []byte) {
		if !bytes.Contains(line, []byte(`"turn_completed"`)) {
			return
		}
		var ev struct {
			Type      string           `json:"type"`
			Event     string           `json:"event"`
			Timestamp string           `json:"timestamp"`
			Cwd       string           `json:"cwd"`
			Model     string           `json:"model"`
			Usage     *grokTokenCounts `json:"usage"`
		}
		if json.Unmarshal(line, &ev) != nil {
			return
		}
		if ev.Type != "turn_completed" && ev.Event != "turn_completed" {
			return
		}
		if tr.dir == "" && ev.Cwd != "" {
			tr.dir = ev.Cwd
		}
		if ev.Usage == nil {
			return
		}
		u := *ev.Usage
		if u.Timestamp == "" {
			u.Timestamp = ev.Timestamp
		}
		if tu, ok := u.toUsage(ev.Model, "", loc); ok {
			tr.usage = append(tr.usage, tu)
		}
	})
	return tr, err
}

// dirStats returns the total file size under dir and the latest mtime.
func dirStats(dir string, fallback time.Time) (int64, time.Time) {
	var size int64
	mod := fallback
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size += info.Size()
		if info.ModTime().After(mod) {
			mod = info.ModTime()
		}
		return nil
	})
	return size, mod
}

// parseDayString parses an RFC3339 or ISO date string into YYYY-MM-DD in loc.
func parseDayString(s string, loc *time.Location) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return ts.In(loc).Format(time.DateOnly)
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts.In(loc).Format(time.DateOnly)
	}
	if len(s) >= 10 {
		if _, err := time.Parse(time.DateOnly, s[:10]); err == nil {
			return s[:10]
		}
	}
	return ""
}

// parseDayRaw parses a JSON timestamp that may be an RFC3339 string or a
// Unix epoch number (seconds or milliseconds).
func parseDayRaw(raw json.RawMessage, loc *time.Location) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return parseDayString(s, loc)
		}
		return ""
	}
	var num float64
	if json.Unmarshal(raw, &num) == nil && num > 0 {
		return unixTime(num).In(loc).Format(time.DateOnly)
	}
	return ""
}

// unixTime converts a Unix timestamp in seconds or milliseconds to time.Time.
func unixTime(v float64) time.Time {
	if v > 1e12 {
		sec := int64(v / 1000)
		nsec := int64((v - float64(sec)*1000) * 1e6)
		return time.Unix(sec, nsec)
	}
	sec := int64(v)
	nsec := int64((v - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}
