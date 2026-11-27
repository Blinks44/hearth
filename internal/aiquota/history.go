package aiquota

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Blinks44/hearth/internal/store"
)

const (
	cacheFileName   = "ai-quotas.json"
	historyFileName = "ai-quotas-history.json"
	maxHistoryItems = 48
)

// HistoryPoint is one compact historical sample stored locally for pace trend
// visibility. Contains only provider/window percentages — zero secrets.
type HistoryPoint struct {
	At      time.Time          `json:"at"`
	MinLeft map[string]float64 `json:"minLeft"`
}

// LoadCache loads the most recent persisted quota snapshot from dir.
func LoadCache(dir string) (Snapshot, error) {
	if dir == "" {
		return Snapshot{}, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, cacheFileName))
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("read quota cache: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("decode quota cache: %w", err)
	}
	return snap, nil
}

// SaveCache atomically writes snap to dir/ai-quotas.json.
func SaveCache(dir string, snap Snapshot) error {
	if dir == "" {
		return nil
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("encode quota cache: %w", err)
	}
	if err := store.WriteFile(dir, cacheFileName, data); err != nil {
		return fmt.Errorf("write quota cache: %w", err)
	}
	return nil
}

// LoadHistory reads the bounded quota history samples from dir.
func LoadHistory(dir string) ([]HistoryPoint, error) {
	if dir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, historyFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read quota history: %w", err)
	}
	var pts []HistoryPoint
	if err := json.Unmarshal(data, &pts); err != nil {
		return nil, fmt.Errorf("decode quota history: %w", err)
	}
	return pts, nil
}

// AppendHistory records a compact history sample from snap and trims to maxHistoryItems.
func AppendHistory(dir string, snap Snapshot, now time.Time) error {
	if dir == "" || snap.ConnectedCount() == 0 {
		return nil
	}
	pts, _ := LoadHistory(dir)
	// Avoid duplicating points closer than 10 minutes apart.
	if len(pts) > 0 && now.Sub(pts[len(pts)-1].At) < 10*time.Minute {
		return nil
	}
	minLeft := make(map[string]float64, snap.ConnectedCount())
	for _, p := range snap.Providers {
		if !p.Connected() {
			continue
		}
		if ml, ok := p.MinLeft(now); ok {
			minLeft[string(p.Provider)] = ml
		}
	}
	if len(minLeft) == 0 {
		return nil
	}
	pts = append(pts, HistoryPoint{At: now.UTC(), MinLeft: minLeft})
	if len(pts) > maxHistoryItems {
		pts = pts[len(pts)-maxHistoryItems:]
	}
	data, err := json.Marshal(pts)
	if err != nil {
		return fmt.Errorf("encode quota history: %w", err)
	}
	if err := store.WriteFile(dir, historyFileName, data); err != nil {
		return fmt.Errorf("write quota history: %w", err)
	}
	return nil
}
