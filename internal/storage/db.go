// Package storage records every download in a SQLite database.
// Callers interact only through DB methods; the schema is versioned via Migrate.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

// Migrate creates or updates the downloads table.
// Safe to call on both fresh and existing databases.
func (s *DB) Migrate(_ context.Context) error {
	// Fresh installs get the full schema including all columns.
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
		status        TEXT     NOT NULL DEFAULT 'pending',
		error_msg     TEXT     NOT NULL DEFAULT '',
		created_at    DATETIME NOT NULL
	);`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Existing installs get the new columns via additive ALTER TABLE.
	// SQLite returns "duplicate column name" when the column already exists — ignore that.
	for _, alter := range []string{
		`ALTER TABLE downloads ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE downloads ADD COLUMN error_msg TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(alter); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// Save inserts dl into the database and sets dl.ID, dl.Status, and dl.CreatedAt.
func (s *DB) Save(ctx context.Context, dl *Download) error {
	dl.CreatedAt = time.Now()
	dl.Status = "pending"

	const q = `
	INSERT INTO downloads (file_id, url, title, file_name, format_arg, quality_label, output_path, status, error_msg, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id`

	row := s.db.QueryRowContext(ctx, q,
		dl.FileID,
		dl.URL,
		dl.Title,
		dl.FileName,
		dl.FormatArg,
		dl.QualityLabel,
		dl.OutputPath,
		dl.Status,
		dl.ErrorMsg,
		dl.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err := row.Scan(&dl.ID); err != nil {
		return fmt.Errorf("save download: %w", err)
	}
	return nil
}

// UpdateOutputPath sets output_path and marks the record as ready.
func (s *DB) UpdateOutputPath(ctx context.Context, id int64, outputPath string) error {
	const q = `UPDATE downloads SET output_path = ?, status = 'ready' WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, outputPath, id); err != nil {
		return fmt.Errorf("update output path: %w", err)
	}
	return nil
}

// UpdateError marks the record as failed and stores the error message.
func (s *DB) UpdateError(ctx context.Context, id int64, errMsg string) error {
	const q = `UPDATE downloads SET status = 'failed', error_msg = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, errMsg, id); err != nil {
		return fmt.Errorf("update error: %w", err)
	}
	return nil
}

// GetByFileID returns the download record for the given public file identifier.
func (s *DB) GetByFileID(ctx context.Context, fileID string) (*Download, error) {
	const q = `
	SELECT id, file_id, url, title, file_name, format_arg, quality_label, output_path, status, error_msg, created_at
	FROM downloads
	WHERE file_id = ?`
	return s.scanOne(s.db.QueryRowContext(ctx, q, fileID))
}

// GetByID returns the download record for the given primary-key ID.
func (s *DB) GetByID(ctx context.Context, id int64) (*Download, error) {
	const q = `
	SELECT id, file_id, url, title, file_name, format_arg, quality_label, output_path, status, error_msg, created_at
	FROM downloads
	WHERE id = ?`
	return s.scanOne(s.db.QueryRowContext(ctx, q, id))
}

// List returns all download records ordered by most recent first.
func (s *DB) List(ctx context.Context) ([]Download, error) {
	const q = `
	SELECT id, file_id, url, title, file_name, format_arg, quality_label, output_path, status, error_msg, created_at
	FROM downloads
	ORDER BY id DESC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list downloads: %w", err)
	}
	defer rows.Close()

	var records []Download
	for rows.Next() {
		dl, err := s.scanOne(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *dl)
	}
	return records, rows.Err()
}

// scanner is the common interface for *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func (s *DB) scanOne(row scanner) (*Download, error) {
	var dl Download
	var createdAt string
	err := row.Scan(
		&dl.ID, &dl.FileID, &dl.URL, &dl.Title, &dl.FileName,
		&dl.FormatArg, &dl.QualityLabel, &dl.OutputPath,
		&dl.Status, &dl.ErrorMsg, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan download: %w", err)
	}
	dl.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &dl, nil
}
