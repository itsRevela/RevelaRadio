// Package history persists listener-count snapshots to SQLite.
package history

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Snapshot is a single listener-count observation.
type Snapshot struct {
	ObservedAt time.Time `json:"observed_at"`
	Listeners  int       `json:"listeners"`
	Online     bool      `json:"online"`
}

// Store wraps a sqlite db file with the minimal queries we need.
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc.org/sqlite doesn't strictly need this but it limits surprises.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func migrate(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS listener_snapshots (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    observed_at  TEXT NOT NULL,
    listeners    INTEGER NOT NULL,
    online       INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_listener_snapshots_observed_at
    ON listener_snapshots(observed_at);
`
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Insert records a snapshot.
func (s *Store) Insert(ctx context.Context, snap Snapshot) error {
	online := 0
	if snap.Online {
		online = 1
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO listener_snapshots (observed_at, listeners, online) VALUES (?, ?, ?)",
		snap.ObservedAt.UTC().Format(time.RFC3339Nano), snap.Listeners, online,
	)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	return nil
}

// Recent returns snapshots observed in the last `window` duration, ordered oldest -> newest.
func (s *Store) Recent(ctx context.Context, window time.Duration) ([]Snapshot, error) {
	cutoff := time.Now().UTC().Add(-window).Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx,
		"SELECT observed_at, listeners, online FROM listener_snapshots WHERE observed_at >= ? ORDER BY observed_at ASC",
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent: %w", err)
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		var (
			ts     string
			count  int
			online int
		)
		if err := rows.Scan(&ts, &count, &online); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		t, perr := time.Parse(time.RFC3339Nano, ts)
		if perr != nil {
			return nil, fmt.Errorf("parse observed_at %q: %w", ts, perr)
		}
		out = append(out, Snapshot{
			ObservedAt: t,
			Listeners:  count,
			Online:     online == 1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshots: %w", err)
	}
	return out, nil
}

// PeakSince returns the highest listener count observed since `since`.
func (s *Store) PeakSince(ctx context.Context, since time.Time) (int, error) {
	var peak sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		"SELECT MAX(listeners) FROM listener_snapshots WHERE observed_at >= ?",
		since.UTC().Format(time.RFC3339Nano),
	).Scan(&peak)
	if err != nil {
		return 0, fmt.Errorf("peak since: %w", err)
	}
	if !peak.Valid {
		return 0, nil
	}
	return int(peak.Int64), nil
}
