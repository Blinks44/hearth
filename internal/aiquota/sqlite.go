package aiquota

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver (no cgo).
)

var errWALWithoutSHM = errors.New("sqlite wal exists without shm")

// querySQLiteString opens a SQLite file strictly read-only and returns the
// first string value from query, or ("", false, nil) if no row matched.
// Handles TEXT as well as BLOB columns (including UTF-16LE used by some
// Electron/Cursor builds) and trims surrounding quotes/whitespace.
func querySQLiteString(ctx context.Context, dbPath, query string, args ...any) (string, bool, error) {
	rows, err := querySQLiteRows(ctx, dbPath, query, 1, args...)
	if err != nil {
		return "", false, err
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return "", false, nil
	}
	return rows[0][0], true, nil
}

// querySQLiteRows runs a small bounded read-only query against a SQLite file.
// When no -wal sidecar exists, immutable=1 is set so SQLite never creates lock
// or journal files in another application's profile directory. When -wal exists
// without -shm, the query is refused to avoid creating a -shm sidecar.
func querySQLiteRows(ctx context.Context, dbPath, query string, limit int, args ...any) ([][]string, error) {
	if dbPath == "" {
		return nil, os.ErrNotExist
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}
	uri, err := sqliteReadOnlyURI(dbPath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sqlite columns: %w", err)
	}
	if limit <= 0 {
		limit = 4
	}
	var out [][]string
	for rows.Next() {
		if len(out) >= limit {
			break
		}
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan sqlite: %w", err)
		}
		row := make([]string, len(cols))
		for i, v := range raw {
			s, ok := decodeSQLiteCell(v)
			if !ok {
				return nil, errors.New("sqlite value exceeds limit")
			}
			row[i] = s
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite: %w", err)
	}
	return out, nil
}

func decodeSQLiteCell(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", true
	case string:
		if len(x) > maxConfigFileBytes {
			return "", false
		}
		return x, true
	case []byte:
		if len(x) > maxConfigFileBytes {
			return "", false
		}
		// Some Electron/Cursor versions store string values as UTF-16LE BLOBs.
		if len(x) >= 2 && len(x)%2 == 0 && bytes.IndexByte(x, 0) >= 0 {
			u16 := make([]uint16, len(x)/2)
			for i := range u16 {
				u16[i] = binary.LittleEndian.Uint16(x[2*i : 2*i+2])
			}
			return string(utf16.Decode(u16)), true
		}
		return string(x), true
	default:
		return fmt.Sprint(x), true
	}
}

func trimSQLiteToken(s string) string {
	return strings.Trim(strings.TrimSpace(s), "\"")
}

func sqliteReadOnlyURI(dbPath string) (string, error) {
	slashed := filepath.ToSlash(dbPath)
	u := &url.URL{
		Scheme: "file",
		Path:   slashed,
	}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_pragma", "busy_timeout(250)")

	_, walErr := os.Stat(dbPath + "-wal")
	hasWAL := walErr == nil
	if hasWAL {
		// Opening a WAL database without an existing -shm can cause SQLite to
		// attempt creating a -shm sidecar in another app's directory.
		if _, shmErr := os.Stat(dbPath + "-shm"); shmErr != nil {
			return "", errWALWithoutSHM
		}
	} else if os.IsNotExist(walErr) {
		q.Set("immutable", "1")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
