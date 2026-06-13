// Package fileserver exposes downloaded files over HTTP.
// GET /files/{file_id} looks up the file path in the database and streams it.
// GET /formats        returns the list of containers available for remuxing.
package fileserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"downloader/internal/downloader"
	"downloader/internal/storage"
)

// invalidChars matches characters forbidden in filenames on Windows and Unix.
// Windows forbids: \ / : * ? " < > |  and control characters (0x00–0x1F).
// Unix forbids: / and null byte.
// Covering the superset keeps names portable across both.
var invalidChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1F]`)

// sanitizeFilename replaces forbidden characters with "_" and trims surrounding spaces.
func sanitizeFilename(s string) string {
	s = invalidChars.ReplaceAllString(s, "_")
	return strings.TrimSpace(s)
}

// Server is an HTTP file server backed by the downloads database.
type Server struct {
	db   *storage.DB
	log  *slog.Logger
	http *http.Server
}

// New creates a Server listening on addr (e.g. ":8080").
func New(addr string, db *storage.DB, log *slog.Logger) *Server {
	s := &Server{db: db, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("/files/", s.handleFile)
	mux.HandleFunc("/formats", s.handleFormats)
	s.http = &http.Server{Addr: addr, Handler: mux}
	return s
}

// Start begins serving in a background goroutine.
func (s *Server) Start() {
	go func() {
		s.log.Info("file server listening", "addr", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("file server error", "err", err)
		}
	}()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) handleFormats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(downloader.OutputFormats())
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	fileID := strings.TrimPrefix(r.URL.Path, "/files/")
	if fileID == "" {
		http.Error(w, "missing file_id", http.StatusBadRequest)
		return
	}

	dl, err := s.db.GetByFileID(r.Context(), fileID)
	if err != nil {
		s.log.Error("db lookup", "file_id", fileID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if dl == nil || dl.OutputPath == "" {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	ext := filepath.Ext(dl.OutputPath)
	title := strings.TrimSuffix(dl.Title, filepath.Ext(dl.Title))
	name := sanitizeFilename(title) + " [" + sanitizeFilename(dl.QualityLabel) + "]" + ext
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	http.ServeFile(w, r, dl.OutputPath)
}
