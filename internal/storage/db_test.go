package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestSaveAndGetByFileID(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	dl := &Download{
		FileID:       "test-file-id",
		URL:          "https://example.com/video",
		Title:        "Placeholder Title",
		FormatArg:    "bestvideo+bestaudio",
		QualityLabel: "1080p",
	}
	if err := db.Save(ctx, dl); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if dl.ID == 0 {
		t.Error("Save did not populate ID")
	}
	if dl.Status != "pending" {
		t.Errorf("Save left Status = %q, want %q", dl.Status, "pending")
	}

	got, err := db.GetByFileID(ctx, "test-file-id")
	if err != nil {
		t.Fatalf("GetByFileID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByFileID returned nil for a just-saved record")
	}
	if got.Title != "Placeholder Title" || got.URL != dl.URL || got.Status != "pending" {
		t.Errorf("GetByFileID = %+v, want fields matching saved record", got)
	}
}

func TestGetByFileIDUnknownReturnsNilNotError(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetByFileID(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("GetByFileID: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("GetByFileID(unknown) = %+v, want nil", got)
	}
}

func TestUpdateTitle(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	dl := &Download{FileID: "f1", URL: "https://example.com", Title: "https://example.com"}
	if err := db.Save(ctx, dl); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Mirrors HandleJobMessage: a placeholder title (the URL) is saved first
	// so the record is visible immediately, then updated once the real
	// title is fetched.
	if err := db.UpdateTitle(ctx, dl.ID, "Real Title"); err != nil {
		t.Fatalf("UpdateTitle: %v", err)
	}

	got, err := db.GetByFileID(ctx, "f1")
	if err != nil {
		t.Fatalf("GetByFileID: %v", err)
	}
	if got.Title != "Real Title" {
		t.Errorf("Title after UpdateTitle = %q, want %q", got.Title, "Real Title")
	}
}

func TestUpdateOutputPathMarksReady(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	dl := &Download{FileID: "f2", URL: "https://example.com", Title: "t"}
	if err := db.Save(ctx, dl); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := db.UpdateOutputPath(ctx, dl.ID, "/downloads/f2.mp4"); err != nil {
		t.Fatalf("UpdateOutputPath: %v", err)
	}

	got, err := db.GetByFileID(ctx, "f2")
	if err != nil {
		t.Fatalf("GetByFileID: %v", err)
	}
	if got.Status != "ready" {
		t.Errorf("Status after UpdateOutputPath = %q, want %q", got.Status, "ready")
	}
	if got.OutputPath != "/downloads/f2.mp4" {
		t.Errorf("OutputPath = %q, want %q", got.OutputPath, "/downloads/f2.mp4")
	}
}

func TestUpdateErrorMarksFailed(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	dl := &Download{FileID: "f3", URL: "https://example.com", Title: "t"}
	if err := db.Save(ctx, dl); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := db.UpdateError(ctx, dl.ID, "boom"); err != nil {
		t.Fatalf("UpdateError: %v", err)
	}

	got, err := db.GetByFileID(ctx, "f3")
	if err != nil {
		t.Fatalf("GetByFileID: %v", err)
	}
	if got.Status != "failed" || got.ErrorMsg != "boom" {
		t.Errorf("after UpdateError: Status=%q ErrorMsg=%q, want failed/boom", got.Status, got.ErrorMsg)
	}
}

func TestListOrdersMostRecentFirst(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	for _, id := range []string{"a", "b", "c"} {
		dl := &Download{FileID: id, URL: "https://example.com", Title: id}
		if err := db.Save(ctx, dl); err != nil {
			t.Fatalf("Save(%q): %v", id, err)
		}
	}

	got, err := db.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List returned %d records, want 3", len(got))
	}
	if got[0].FileID != "c" || got[2].FileID != "a" {
		t.Errorf("List order = %q, %q, %q, want most-recent-first (c, b, a)", got[0].FileID, got[1].FileID, got[2].FileID)
	}
}
