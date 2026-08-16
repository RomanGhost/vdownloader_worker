package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"downloader/internal/storage"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	noopPublish := func(context.Context, string) error { return nil }
	return New(db, t.TempDir(), noopPublish, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestListJobsEmpty(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	rec := httptest.NewRecorder()
	s.listJobs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []jobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d jobs, want 0", len(got))
	}
}

func TestListJobsMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/jobs", nil)
	rec := httptest.NewRecorder()
	s.listJobs(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleJobUnknownFileID(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/does-not-exist", nil)
	rec := httptest.NewRecorder()
	s.handleJob(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleJobMissingFileID(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/", nil)
	rec := httptest.NewRecorder()
	s.handleJob(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleJobFoundReturnsRecord(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	dl := &storage.Download{FileID: "f1", URL: "https://example.com/v", Title: "T", QualityLabel: "1080p"}
	if err := s.db.Save(ctx, dl); err != nil {
		t.Fatalf("Save: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/f1", nil)
	rec := httptest.NewRecorder()
	s.handleJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got jobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got.Status != "pending" {
		t.Errorf("Status = %q, want %q", got.Status, "pending")
	}
	if got.DownloadURL != "" {
		t.Errorf("DownloadURL = %q, want empty until status is ready", got.DownloadURL)
	}
}

func TestHandleJobReadySetsDownloadURL(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	dl := &storage.Download{FileID: "f2", URL: "https://example.com/v", Title: "T"}
	if err := s.db.Save(ctx, dl); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.db.UpdateOutputPath(ctx, dl.ID, "/downloads/f2.mp4"); err != nil {
		t.Fatalf("UpdateOutputPath: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/f2", nil)
	rec := httptest.NewRecorder()
	s.handleJob(rec, req)

	var got jobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got.Status != "ready" {
		t.Errorf("Status = %q, want %q", got.Status, "ready")
	}
	if got.DownloadURL != "/files/f2" {
		t.Errorf("DownloadURL = %q, want %q", got.DownloadURL, "/files/f2")
	}
}

func TestListJobsOrdersMostRecentFirst(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	for _, id := range []string{"a", "b", "c"} {
		dl := &storage.Download{FileID: id, URL: "https://example.com/v", Title: id}
		if err := s.db.Save(ctx, dl); err != nil {
			t.Fatalf("Save(%q): %v", id, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	rec := httptest.NewRecorder()
	s.listJobs(rec, req)

	var got []jobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(got) != 3 || got[0].FileID != "c" || got[2].FileID != "a" {
		t.Errorf("jobs = %+v, want most-recent-first (c, b, a)", got)
	}
}
