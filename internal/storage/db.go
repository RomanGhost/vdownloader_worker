// Package storage records every download in a JSON file acting as a simple
// append-only database. The schema is intentionally compatible with a future
// SQLite or Postgres migration: callers interact only through DB methods.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// DB is a JSON-file backed store. All public methods are safe for concurrent
// use, though yt-dlp is typically run one at a time.
type DB struct {
	mu   sync.Mutex
	path string
}

// Open opens (or creates) the JSON database at path.
func Open(path string) (*DB, error) {
	// Ensure the file exists so that List never fails on a fresh run.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	f.Close()
	return &DB{path: path}, nil
}

// Close is a no-op for the JSON backend; it exists so callers can use defer db.Close().
func (s *DB) Close() error { return nil }

// Migrate is a no-op for the JSON backend; it exists to satisfy the same
// interface that a SQL backend would require.
func (s *DB) Migrate(_ context.Context) error { return nil }

// Save appends dl to the database and sets dl.ID and dl.CreatedAt.
func (s *DB) Save(_ context.Context, dl *Download) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.readAll()
	if err != nil {
		return err
	}

	dl.CreatedAt = time.Now()
	if len(records) > 0 {
		dl.ID = records[len(records)-1].ID + 1
	} else {
		dl.ID = 1
	}

	records = append(records, *dl)
	return s.writeAll(records)
}

// List returns all download records ordered by most recent first.
func (s *DB) List(_ context.Context) ([]Download, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.readAll()
	if err != nil {
		return nil, err
	}

	// Reverse in-place so index 0 is the newest.
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
}

// readAll deserialises the JSON file. Callers must hold s.mu.
func (s *DB) readAll() ([]Download, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read database: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var records []Download
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse database: %w", err)
	}
	return records, nil
}

// writeAll serialises records back to the JSON file. Callers must hold s.mu.
func (s *DB) writeAll(records []Download) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal database: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write database: %w", err)
	}
	return nil
}
