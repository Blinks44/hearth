package ai

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func sqliteFileURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// openSQLiteRO opens a SQLite database strictly read-only, falling back to
// immutable=1 if the directory does not allow creating WAL/SHM sidecars or
// the database is a cleanly closed WAL database without a -shm file.
func openSQLiteRO(path string) (*sql.DB, error) {
	uri := sqliteFileURI(path)
	db, err := sql.Open("sqlite", uri+"?mode=ro&_pragma=busy_timeout(1000)&_pragma=query_only(1)")
	if err == nil {
		err = probeSQLiteRO(db)
		if err == nil {
			return db, nil
		}
		_ = db.Close()
	}
	if wfi, werr := os.Stat(path + "-wal"); werr == nil && wfi.Size() > 0 {
		return nil, err
	}
	db, err = sql.Open("sqlite", uri+"?mode=ro&immutable=1")
	if err != nil {
		return nil, err
	}
	if err := probeSQLiteRO(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func probeSQLiteRO(db *sql.DB) error {
	var n int
	err := db.QueryRow("SELECT 1 FROM sqlite_master LIMIT 1").Scan(&n)
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

// tableColumns returns the set of lowercase column names of table.
func tableColumns(db *sql.DB, table string) map[string]bool {
	// table comes from our own constants or sqlite_master; quote safely.
	quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
	rows, err := db.Query("PRAGMA table_info(" + quoted + ")")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk) == nil {
			cols[strings.ToLower(name)] = true
		}
	}
	return cols
}

func attachSharedDBSize(sessions []inventory.AgentSession, dbPath string) {
	if len(sessions) == 0 {
		return
	}
	size, _, walSize, _, ok := dbStat(dbPath)
	if !ok {
		return
	}
	newest := 0
	for i := 1; i < len(sessions); i++ {
		if sessions[i].Modified.After(sessions[newest].Modified) {
			newest = i
		}
	}
	sessions[newest].Size = size + walSize
}

// opencodeSessions reads OpenCode sessions from opencode.db or legacy
// storage/session + storage/message JSON files.
func opencodeSessions(term sysenv.Env, home string, loc *time.Location) []inventory.AgentSession {
	candidates := candidateDirs(term, home, []string{
		"$OPENCODE_DATA",
		"$XDG_DATA_HOME/opencode",
		"~/.local/share/opencode",
		"$LOCALAPPDATA/opencode",
	})
	var out []inventory.AgentSession
	for _, root := range candidates {
		dbPath := filepath.Join(root, "opencode.db")
		if _, err := os.Stat(dbPath); err == nil {
			ss, err := readDB(dbPath, func(p string) ([]inventory.AgentSession, error) {
				return parseOpenCodeDB(p, loc)
			})
			if err == nil {
				out = append(out, ss...)
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "storage")); err == nil {
			out = append(out, opencodeLegacySessions(filepath.Join(root, "storage"), loc)...)
		}
	}
	return out
}

type opencodeMessageData struct {
	Role    string `json:"role"`
	ModelID string `json:"modelID"`
	Model   *struct {
		ModelID string `json:"modelID"`
	} `json:"model"`
	Time *struct {
		Created float64 `json:"created"`
	} `json:"time"`
	Tokens *struct {
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     *struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
}

func (m opencodeMessageData) toUsage(fallbackTS float64, loc *time.Location) (inventory.TokenUsage, bool) {
	if m.Role != "assistant" || m.Tokens == nil {
		return inventory.TokenUsage{}, false
	}
	model := m.ModelID
	if model == "" && m.Model != nil {
		model = m.Model.ModelID
	}
	if model == "" {
		return inventory.TokenUsage{}, false
	}
	var cr, cw int64
	if m.Tokens.Cache != nil {
		cr, cw = m.Tokens.Cache.Read, m.Tokens.Cache.Write
	}
	out := m.Tokens.Output + m.Tokens.Reasoning
	if m.Tokens.Input == 0 && cr == 0 && cw == 0 && out == 0 {
		return inventory.TokenUsage{}, false
	}
	ts := fallbackTS
	if ts == 0 && m.Time != nil {
		ts = m.Time.Created
	}
	if ts == 0 {
		return inventory.TokenUsage{}, false
	}
	return inventory.TokenUsage{
		Day:        unixTime(ts).In(loc).Format(time.DateOnly),
		Model:      model,
		Input:      m.Tokens.Input,
		CacheRead:  cr,
		CacheWrite: cw,
		Output:     out,
	}, true
}

func parseOpenCodeDB(path string, loc *time.Location) ([]inventory.AgentSession, error) {
	db, err := openSQLiteRO(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	sessCols := tableColumns(db, "session")
	msgCols := tableColumns(db, "message")
	if !sessCols["id"] || !msgCols["session_id"] || !msgCols["data"] {
		return nil, nil
	}

	type sessMeta struct {
		dir string
		mod time.Time
	}
	meta := map[string]sessMeta{}
	dirCol := "''"
	if sessCols["directory"] {
		dirCol = "COALESCE(directory, '')"
	}
	updCol := "0"
	if sessCols["time_updated"] {
		updCol = "COALESCE(time_updated, 0)"
	}
	sRows, err := db.Query("SELECT id, " + dirCol + ", " + updCol + " FROM session")
	if err != nil {
		return nil, err
	}
	for sRows.Next() {
		var id, dir string
		var upd float64
		if sRows.Scan(&id, &dir, &upd) == nil && id != "" {
			var mod time.Time
			if upd > 0 {
				mod = unixTime(upd)
			}
			meta[id] = sessMeta{dir: dir, mod: mod}
		}
	}
	_ = sRows.Close()

	timeCol := "0"
	if msgCols["time_created"] {
		timeCol = "COALESCE(time_created, 0)"
	}
	mRows, err := db.Query("SELECT session_id, " + timeCol + ", data FROM message")
	if err != nil {
		return nil, err
	}
	defer func() { _ = mRows.Close() }()

	bySession := map[string][]inventory.TokenUsage{}
	maxMod := map[string]time.Time{}
	for mRows.Next() {
		var sid, raw string
		var created float64
		if mRows.Scan(&sid, &created, &raw) != nil || sid == "" {
			continue
		}
		var md opencodeMessageData
		if json.Unmarshal([]byte(raw), &md) != nil {
			continue
		}
		tu, ok := md.toUsage(created, loc)
		if !ok {
			continue
		}
		bySession[sid] = append(bySession[sid], tu)
		if created > 0 {
			if ts := unixTime(created); ts.After(maxMod[sid]) {
				maxMod[sid] = ts
			}
		}
	}
	if err := mRows.Err(); err != nil {
		return nil, err
	}

	fi, _ := os.Stat(path)
	var fileMod time.Time
	if fi != nil {
		fileMod = fi.ModTime()
	}

	var out []inventory.AgentSession
	for sid, us := range bySession {
		m := meta[sid]
		mod := m.mod
		if maxMod[sid].After(mod) {
			mod = maxMod[sid]
		}
		if mod.IsZero() {
			mod = fileMod
		}
		out = append(out, inventory.AgentSession{
			Agent:    "opencode",
			ID:       sid,
			Dir:      m.dir,
			StoreDir: filepath.Dir(path),
			Modified: mod,
			Usage:    merge(us),
		})
	}
	slices.SortFunc(out, func(a, b inventory.AgentSession) int {
		return strings.Compare(a.ID, b.ID)
	})
	attachSharedDBSize(out, path)
	return out, nil
}

func opencodeLegacySessions(storageDir string, loc *time.Location) []inventory.AgentSession {
	type sessMeta struct {
		dir string
		mod time.Time
	}
	meta := map[string]sessMeta{}
	_ = filepath.WalkDir(filepath.Join(storageDir, "session"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var doc struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
			Time      *struct {
				Updated float64 `json:"updated"`
			} `json:"time"`
		}
		if json.Unmarshal(data, &doc) != nil || doc.ID == "" {
			return nil
		}
		var mod time.Time
		if doc.Time != nil && doc.Time.Updated > 0 {
			mod = unixTime(doc.Time.Updated)
		} else if info, err := d.Info(); err == nil {
			mod = info.ModTime()
		}
		meta[doc.ID] = sessMeta{dir: doc.Directory, mod: mod}
		return nil
	})

	msgRoot := filepath.Join(storageDir, "message")
	sessEntries, err := os.ReadDir(msgRoot)
	if err != nil {
		return nil
	}
	var out []inventory.AgentSession
	for _, se := range sessEntries {
		if !se.IsDir() {
			continue
		}
		sid := se.Name()
		sessMsgDir := filepath.Join(msgRoot, sid)
		msgs, err := os.ReadDir(sessMsgDir)
		if err != nil {
			continue
		}
		var us []inventory.TokenUsage
		var size int64
		mod := meta[sid].mod
		for _, me := range msgs {
			if me.IsDir() || !strings.HasSuffix(me.Name(), ".json") {
				continue
			}
			info, err := me.Info()
			if err != nil {
				continue
			}
			size += info.Size()
			if info.ModTime().After(mod) {
				mod = info.ModTime()
			}
			data, err := os.ReadFile(filepath.Join(sessMsgDir, me.Name()))
			if err != nil {
				continue
			}
			var md opencodeMessageData
			if json.Unmarshal(data, &md) != nil {
				continue
			}
			if tu, ok := md.toUsage(0, loc); ok {
				us = append(us, tu)
			}
		}
		if len(us) == 0 {
			continue
		}
		out = append(out, inventory.AgentSession{
			Agent:    "opencode",
			ID:       sid,
			Dir:      meta[sid].dir,
			StoreDir: storageDir,
			Size:     size,
			Modified: mod,
			Usage:    merge(us),
		})
	}
	return out
}

// devinSessions reads Devin CLI sessions from sessions.db.
func devinSessions(term sysenv.Env, home string, loc *time.Location) []inventory.AgentSession {
	candidates := candidateDirs(term, home, []string{
		"$XDG_DATA_HOME/devin/cli",
		"~/.local/share/devin/cli",
		"$LOCALAPPDATA/devin/cli",
	})
	var out []inventory.AgentSession
	for _, root := range candidates {
		dbPath := filepath.Join(root, "sessions.db")
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		ss, err := readDB(dbPath, func(p string) ([]inventory.AgentSession, error) {
			return parseDevinDB(p, loc)
		})
		if err == nil {
			out = append(out, ss...)
		}
	}
	return out
}

func parseDevinDB(path string, loc *time.Location) ([]inventory.AgentSession, error) {
	db, err := openSQLiteRO(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	sessCols := tableColumns(db, "sessions")
	nodeCols := tableColumns(db, "message_nodes")
	if !nodeCols["session_id"] || !nodeCols["chat_message"] {
		return nil, nil
	}

	type sessMeta struct {
		dir string
		mod time.Time
	}
	meta := map[string]sessMeta{}
	idCol := firstColumn(sessCols, "id", "session_id")
	if idCol != "" {
		dirCol := firstColumn(sessCols, "working_dir", "cwd", "workspace_dir")
		dirExpr := "''"
		if dirCol != "" {
			dirExpr = "COALESCE(" + dirCol + ", '')"
		}
		updCol := firstColumn(sessCols, "updated_at", "created_at")
		updExpr := "''"
		if updCol != "" {
			updExpr = "COALESCE(CAST(" + updCol + " AS TEXT), '')"
		}
		if sRows, err := db.Query("SELECT " + idCol + ", " + dirExpr + ", " + updExpr + " FROM sessions"); err == nil {
			for sRows.Next() {
				var id, dir, upd string
				if sRows.Scan(&id, &dir, &upd) == nil && id != "" {
					meta[id] = sessMeta{dir: dir, mod: parseFlexibleTime(upd)}
				}
			}
			_ = sRows.Close()
		}
	}

	tsCol := firstColumn(nodeCols, "created_at", "timestamp")
	tsExpr := "''"
	if tsCol != "" {
		tsExpr = "COALESCE(CAST(" + tsCol + " AS TEXT), '')"
	}
	rows, err := db.Query("SELECT session_id, " + tsExpr + ", chat_message FROM message_nodes")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type reqKey struct{ sid, req string }
	seenReq := map[reqKey]int{}
	bySession := map[string][]inventory.TokenUsage{}
	maxMod := map[string]time.Time{}

	for rows.Next() {
		var sid, tsStr, raw string
		if rows.Scan(&sid, &tsStr, &raw) != nil || sid == "" || raw == "" {
			continue
		}
		var msg struct {
			ID        string `json:"id"`
			RequestID string `json:"request_id"`
			Model     string `json:"model"`
			Timestamp string `json:"timestamp"`
			Usage     *struct {
				InputTokens         int64 `json:"input_tokens"`
				PromptTokens        int64 `json:"prompt_tokens"`
				OutputTokens        int64 `json:"output_tokens"`
				CompletionTokens    int64 `json:"completion_tokens"`
				CacheRead           int64 `json:"cache_read_input_tokens"`
				CacheReadAlt        int64 `json:"cache_read_tokens"`
				CachedTokens        int64 `json:"cached_tokens"`
				PromptTokensDetails *struct {
					CachedTokens int64 `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
				InputTokensDetails *struct {
					CachedTokens int64 `json:"cached_tokens"`
				} `json:"input_tokens_details"`
				CacheWrite    int64 `json:"cache_creation_input_tokens"`
				CacheWriteAlt int64 `json:"cache_write_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(raw), &msg) != nil || msg.Usage == nil || msg.Model == "" {
			continue
		}
		in := max(msg.Usage.InputTokens, msg.Usage.PromptTokens)
		outTok := max(msg.Usage.OutputTokens, msg.Usage.CompletionTokens)
		crAnthropic := max(msg.Usage.CacheRead, msg.Usage.CacheReadAlt)
		crOpenAI := msg.Usage.CachedTokens
		if msg.Usage.PromptTokensDetails != nil {
			crOpenAI = max(crOpenAI, msg.Usage.PromptTokensDetails.CachedTokens)
		}
		if msg.Usage.InputTokensDetails != nil {
			crOpenAI = max(crOpenAI, msg.Usage.InputTokensDetails.CachedTokens)
		}
		cr := max(crAnthropic, crOpenAI)
		// OpenAI-style usage reports cached tokens as a subset of prompt/input
		// tokens, whereas Anthropic input_tokens excludes cache_read_input_tokens.
		if cr > 0 && (crOpenAI > 0 || (msg.Usage.PromptTokens > 0 && msg.Usage.InputTokens == 0)) {
			in = max(in-cr, 0)
		}
		cw := max(msg.Usage.CacheWrite, msg.Usage.CacheWriteAlt)
		if in == 0 && outTok == 0 && cr == 0 && cw == 0 {
			continue
		}
		ts := parseFlexibleTime(msg.Timestamp)
		if ts.IsZero() {
			ts = parseFlexibleTime(tsStr)
		}
		if ts.IsZero() {
			continue
		}
		if ts.After(maxMod[sid]) {
			maxMod[sid] = ts
		}
		tu := inventory.TokenUsage{
			Day:        ts.In(loc).Format(time.DateOnly),
			Model:      msg.Model,
			Input:      in,
			CacheRead:  cr,
			CacheWrite: cw,
			Output:     outTok,
		}
		req := msg.RequestID
		if req == "" {
			req = msg.ID
		}
		if req != "" {
			k := reqKey{sid, req}
			if idx, ok := seenReq[k]; ok {
				bySession[sid][idx] = tu
				continue
			}
			seenReq[k] = len(bySession[sid])
		}
		bySession[sid] = append(bySession[sid], tu)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	fi, _ := os.Stat(path)
	var fileMod time.Time
	if fi != nil {
		fileMod = fi.ModTime()
	}

	var out []inventory.AgentSession
	for sid, us := range bySession {
		m := meta[sid]
		mod := m.mod
		if maxMod[sid].After(mod) {
			mod = maxMod[sid]
		}
		if mod.IsZero() {
			mod = fileMod
		}
		out = append(out, inventory.AgentSession{
			Agent:    "devin",
			ID:       sid,
			Dir:      m.dir,
			StoreDir: filepath.Dir(path),
			Modified: mod,
			Usage:    merge(us),
		})
	}
	slices.SortFunc(out, func(a, b inventory.AgentSession) int {
		return strings.Compare(a.ID, b.ID)
	})
	attachSharedDBSize(out, path)
	return out, nil
}

// antigravitySessions reads per-conversation SQLite databases under
// ~/.gemini/antigravity-cli/conversations, ~/.gemini/antigravity-acp/conversations,
// and ~/.gemini/antigravity/conversations.
func antigravitySessions(home string, loc *time.Location) []inventory.AgentSession {
	if home == "" {
		return nil
	}
	roots := []string{
		filepath.Join(home, ".gemini", "antigravity-cli", "conversations"),
		filepath.Join(home, ".gemini", "antigravity-acp", "conversations"),
		filepath.Join(home, ".gemini", "antigravity", "conversations"),
	}
	seenDir := map[string]bool{}
	var out []inventory.AgentSession
	for _, root := range roots {
		real, err := filepath.EvalSymlinks(root)
		if err != nil || seenDir[real] {
			continue
		}
		seenDir[real] = true
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
				continue
			}
			dbPath := filepath.Join(root, e.Name())
			ss, err := readDB(dbPath, func(p string) ([]inventory.AgentSession, error) {
				return parseAntigravityDB(p, loc)
			})
			if err == nil {
				out = append(out, ss...)
			}
		}
	}
	return out
}

func parseAntigravityDB(path string, loc *time.Location) ([]inventory.AgentSession, error) {
	files := []string{path}
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := path + suffix
		if fi, err := os.Stat(sidecar); err == nil && !fi.IsDir() {
			files = append(files, sidecar)
		}
	}
	db, err := openSQLiteRO(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	tRows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err
	}
	var tables []string
	for tRows.Next() {
		var name string
		if tRows.Scan(&name) == nil && name != "" {
			tables = append(tables, name)
		}
	}
	_ = tRows.Close()

	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	size, mod, walSize, walMod, _ := dbStat(path)
	fileMod := fi.ModTime()
	if walMod > mod {
		fileMod = time.Unix(0, walMod)
	}
	fallbackDay := fileMod.In(loc).Format(time.DateOnly)

	var dir string
	var usage []inventory.TokenUsage
	seenResp := map[string]int{}

	for _, tbl := range tables {
		lowerTbl := strings.ToLower(tbl)
		cols := tableColumns(db, tbl)
		quoted := `"` + strings.ReplaceAll(tbl, `"`, `""`) + `"`
		if dir == "" {
			var trajCol string
			switch {
			case lowerTbl == "trajectory_metadata_blob" && cols["data"]:
				trajCol = "data"
			case cols["trajectory_metadata_blob"]:
				trajCol = "trajectory_metadata_blob"
			}
			if trajCol != "" {
				if rows, err := db.Query("SELECT " + trajCol + " FROM " + quoted + " WHERE " + trajCol + " IS NOT NULL LIMIT 5"); err == nil {
					for rows.Next() {
						var blob []byte
						if rows.Scan(&blob) == nil && len(blob) > 0 {
							if d := decodeAntigravityWorkspace(blob); d != "" {
								dir = d
								break
							}
						}
					}
					_ = rows.Close()
				}
			}
		}
		var genCol string
		switch {
		case lowerTbl == "gen_metadata" && cols["data"]:
			genCol = "data"
		case cols["gen_metadata"]:
			genCol = "gen_metadata"
		}
		if genCol != "" {
			rows, err := db.Query("SELECT " + genCol + " FROM " + quoted + " WHERE " + genCol + " IS NOT NULL")
			if err != nil {
				continue
			}
			for rows.Next() {
				var blob []byte
				if rows.Scan(&blob) != nil || len(blob) == 0 {
					continue
				}
				tu, respID, ok := decodeAntigravityGenMetadata(blob, fallbackDay, loc)
				if !ok {
					continue
				}
				if respID != "" {
					if idx, exists := seenResp[respID]; exists {
						usage[idx] = tu
						continue
					}
					seenResp[respID] = len(usage)
				}
				usage = append(usage, tu)
			}
			_ = rows.Close()
		}
	}
	if len(usage) == 0 {
		return nil, nil
	}
	id := strings.TrimSuffix(filepath.Base(path), ".db")
	return []inventory.AgentSession{{
		Agent:    "antigravity",
		ID:       id,
		Dir:      dir,
		StoreDir: filepath.Dir(path),
		Files:    files,
		Size:     size + walSize,
		Modified: fileMod,
		Usage:    merge(usage),
	}}, nil
}

// protoField is a single decoded protobuf wire field.
type protoField struct {
	num      uint64
	wireType uint8
	varint   uint64
	bytes    []byte
}

// decodeProtoFields decodes a protobuf wire message without a schema.
func decodeProtoFields(data []byte) []protoField {
	var out []protoField
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 {
			break
		}
		data = data[n:]
		num := tag >> 3
		wt := uint8(tag & 7)
		if num == 0 {
			break
		}
		switch wt {
		case 0: // varint
			v, m := binary.Uvarint(data)
			if m <= 0 {
				return out
			}
			data = data[m:]
			out = append(out, protoField{num: num, wireType: wt, varint: v})
		case 1: // 64-bit
			if len(data) < 8 {
				return out
			}
			out = append(out, protoField{num: num, wireType: wt, bytes: data[:8]})
			data = data[8:]
		case 2: // length-delimited
			l, m := binary.Uvarint(data)
			if m <= 0 || l > uint64(len(data)-m) {
				return out
			}
			data = data[m:]
			out = append(out, protoField{num: num, wireType: wt, bytes: data[:l]})
			data = data[l:]
		case 5: // 32-bit
			if len(data) < 4 {
				return out
			}
			out = append(out, protoField{num: num, wireType: wt, bytes: data[:4]})
			data = data[4:]
		default:
			return out
		}
	}
	return out
}

func decodeAntigravityWorkspace(blob []byte) string {
	fields := decodeProtoFields(blob)
	for _, wantNum := range []uint64{7, 1} {
		for _, f := range fields {
			if f.num != wantNum || f.wireType != 2 || len(f.bytes) == 0 {
				continue
			}
			if p := extractPathString(f.bytes); p != "" {
				return p
			}
			// Sometimes the field wraps a submessage whose field 1 or 2 is the path/URI.
			for _, sub := range decodeProtoFields(f.bytes) {
				if sub.wireType == 2 {
					if p := extractPathString(sub.bytes); p != "" {
						return p
					}
				}
			}
		}
	}
	return ""
}

func extractPathString(b []byte) string {
	if !utf8.Valid(b) {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if strings.HasPrefix(s, "file://") {
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			s = u.Path
			if len(s) >= 4 && s[0] == '/' && s[2] == ':' && (s[3] == '/' || s[3] == '\\') {
				s = s[1:]
			}
		}
	}
	if strings.HasPrefix(s, "/") || (len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/')) {
		if !strings.ContainsAny(s, "\x00\r\n") {
			return s
		}
	}
	return ""
}

func decodeAntigravityGenMetadata(blob []byte, fallbackDay string, loc *time.Location) (inventory.TokenUsage, string, bool) {
	var chatModelBytes []byte
	for _, f := range decodeProtoFields(blob) {
		if f.num == 1 && f.wireType == 2 {
			chatModelBytes = f.bytes
			break
		}
	}
	if len(chatModelBytes) == 0 {
		return inventory.TokenUsage{}, "", false
	}

	var usageBytes, timeWrapBytes []byte
	var model19, model21, model22 string
	for _, f := range decodeProtoFields(chatModelBytes) {
		if f.wireType != 2 {
			continue
		}
		switch f.num {
		case 4:
			usageBytes = f.bytes
		case 9:
			timeWrapBytes = f.bytes
		case 19:
			if utf8.Valid(f.bytes) {
				model19 = strings.TrimSpace(string(f.bytes))
			}
		case 21:
			if utf8.Valid(f.bytes) {
				model21 = strings.TrimSpace(string(f.bytes))
			}
		case 22:
			if utf8.Valid(f.bytes) {
				model22 = strings.TrimSpace(string(f.bytes))
			}
		}
	}
	if len(usageBytes) == 0 {
		return inventory.TokenUsage{}, "", false
	}

	model := model19
	if model == "" {
		model = model22
	}
	if model == "" {
		model = model21
	}
	if model == "" {
		return inventory.TokenUsage{}, "", false
	}

	var in, out, cw, cr int64
	var respID string
	for _, f := range decodeProtoFields(usageBytes) {
		switch {
		case f.num == 2 && f.wireType == 0 && f.varint <= math.MaxInt64:
			in = int64(f.varint)
		case f.num == 3 && f.wireType == 0 && f.varint <= math.MaxInt64:
			out = int64(f.varint)
		case f.num == 4 && f.wireType == 0 && f.varint <= math.MaxInt64:
			cw = int64(f.varint)
		case f.num == 5 && f.wireType == 0 && f.varint <= math.MaxInt64:
			cr = int64(f.varint)
		case f.num == 11 && f.wireType == 2 && utf8.Valid(f.bytes):
			respID = strings.TrimSpace(string(f.bytes))
		}
	}
	if in == 0 && out == 0 && cw == 0 && cr == 0 {
		return inventory.TokenUsage{}, "", false
	}

	day := decodeAntigravityDay(timeWrapBytes, loc)
	if day == "" {
		day = fallbackDay
	}
	return inventory.TokenUsage{
		Day:        day,
		Model:      model,
		Input:      in,
		CacheWrite: cw,
		CacheRead:  cr,
		Output:     out,
	}, respID, true
}

func decodeAntigravityDay(timeWrap []byte, loc *time.Location) string {
	for _, f := range decodeProtoFields(timeWrap) {
		if f.num != 4 {
			continue
		}
		if f.wireType == 0 && f.varint > 0 {
			return unixTime(float64(f.varint)).In(loc).Format(time.DateOnly)
		}
		if f.wireType == 2 {
			// google.protobuf.Timestamp: field 1 is seconds.
			for _, tf := range decodeProtoFields(f.bytes) {
				if tf.num == 1 && tf.wireType == 0 && tf.varint > 0 {
					return unixTime(float64(tf.varint)).In(loc).Format(time.DateOnly)
				}
			}
		}
	}
	return ""
}

// candidateDirs resolves a list of env/home directory patterns and returns
// existing, deduplicated paths.
func candidateDirs(term sysenv.Env, home string, patterns []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, pat := range patterns {
		var p string
		switch {
		case strings.HasPrefix(pat, "$"):
			name, rest, _ := strings.Cut(pat[1:], "/")
			if v := term.Get(name); v != "" {
				p = filepath.Join(v, filepath.FromSlash(rest))
			}
		case strings.HasPrefix(pat, "~/") && home != "":
			p = filepath.Join(home, filepath.FromSlash(pat[2:]))
		}
		if p == "" {
			continue
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		out = append(out, p)
	}
	return out
}

func firstColumn(cols map[string]bool, names ...string) string {
	for _, n := range names {
		if cols[n] {
			return n
		}
	}
	return ""
}

func parseFlexibleTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts
	}
	if ts, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return ts
	}
	var num float64
	if json.Unmarshal([]byte(s), &num) == nil && num > 0 {
		return unixTime(num)
	}
	return time.Time{}
}
