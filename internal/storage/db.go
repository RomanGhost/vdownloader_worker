// Package storage records every download in a SQLite database.
// Callers interact only through DB methods; the schema is versioned via Migrate.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

// DB wraps a SQLite connection pool.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	// SQLite performs best with a single writer connection.
	conn.SetMaxOpenConns(1)
	return &DB{db: conn}, nil
}

// Close closes the underlying database connection.
func (s *DB) Close() error { return s.db.Close() }

// Migrate creates the downloads table if it does not exist.
func (s *DB) Migrate(_ context.Context) error {
	const ddl = `
	CREATE TABLE IF NOT EXISTS downloads (
		id            INTEGER  PRIMARY KEY AUTOINCREMENT,
		file_id       TEXT     NOT NULL UNIQUE DEFAULT '',
		url           TEXT     NOT NULL,
		title         TEXT     NOT NULL,
		file_name     TEXT     NOT NULL DEFAULT '',
		format_arg    TEXT     NOT NULL DEFAULT '',
		quality_label TEXT     NOT NULL DEFAULT '',
		output_path   TEXT     NOT NULL DEFAULT '',
		created_at    DATETIME NOT NULL
	);`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Save inserts dl into the database and sets dl.ID and dl.CreatedAt.
func (s *DB) Save(ctx context.Context, dl *Download) error {
	dl.CreatedAt = time.Now()

	const q = `
	INSERT INTO downloads (file_id, url, title, file_name, format_arg, quality_label, output_path, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id`

	row := s.db.QueryRowContext(ctx, q,
		dl.FileID,
		dl.URL,
		dl.Title,
		dl.FileName,
		dl.FormatArg,
		dl.QualityLabel,
		dl.OutputPath,
		dl.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err := row.Scan(&dl.ID); err != nil {
		return fmt.Errorf("save download: %w", err)
	}
	return nil
}

// UpdateOutputPath sets the output_path for a record after the file is written to disk.
func (s *DB) UpdateOutputPath(ctx context.Context, id int64, outputPath string) error {
	const q = `UPDATE downloads SET output_path = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, outputPath, id); err != nil {
		return fmt.Errorf("update output path: %w", err)
	}
	return nil
}

// GetByFileID returns the download record for the given public file identifier.
func (s *DB) GetByFileID(ctx context.Context, fileID string) (*Download, error) {
	const q = `
	SELECT id, file_id, url, title, file_name, format_arg, quality_label, output_path, created_at
	FROM downloads
	WHERE file_id = ?`

	var dl Download
	var createdAt string
	err := s.db.QueryRowContext(ctx, q, fileID).Scan(
		&dl.ID, &dl.FileID, &dl.URL, &dl.Title, &dl.FileName,
		&dl.FormatArg, &dl.QualityLabel, &dl.OutputPath, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get by file_id: %w", err)
	}
	dl.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &dl, nil
}

// List returns all download records ordered by most recent first.
func (s *DB) List(ctx context.Context) ([]Download, error) {
	const q = `
	SELECT id, file_id, url, title, file_name, format_arg, quality_label, output_path, created_at
	FROM downloads
	ORDER BY id DESC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list downloads: %w", err)
	}
	defer rows.Close()

	var records []Download
	for rows.Next() {
		var dl Download
		var createdAt string
		if err := rows.Scan(
			&dl.ID, &dl.FileID, &dl.URL, &dl.Title, &dl.FileName,
			&dl.FormatArg, &dl.QualityLabel, &dl.OutputPath, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan download: %w", err)
		}
		dl.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		records = append(records, dl)
	}
	return records, rows.Err()
}
