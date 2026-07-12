// Package api exposes a REST interface for frontend clients.
//
// Routes:
//
//	GET  /api/formats?url=<url>  – fetch available formats for a video URL
//	POST /api/jobs               – submit a download job
//	GET  /api/jobs               – list all jobs
//	GET  /api/jobs/{id}          – get a single job by ID
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"downloader/internal/downloader"
	"downloader/internal/queue"
	"downloader/internal/storage"
	"downloader/internal/webhook"
)

// Server handles REST requests for the frontend.
type Server struct {
	db        *storage.DB
	outDir    string
	hook      *webhook.Caller
	publishFn func(queue.CompletedEvent) error // optional; publishes to RabbitMQ
	log       *slog.Logger
}

// New creates a Server. publishFn may be nil when RabbitMQ is not used.
func New(
	db *storage.DB,
	outDir string,
	hook *webhook.Caller,
	publishFn func(queue.CompletedEvent) error,
	log *slog.Logger,
) *Server {
	return &Server{
		db:        db,
		outDir:    outDir,
		hook:      hook,
		publishFn: publishFn,
		log:       log,
	}
}

// RegisterRoutes mounts all API handlers on mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/formats", s.handleFormats)
	mux.HandleFunc("/api/jobs", s.handleJobs)
	mux.HandleFunc("/api/jobs/", s.handleJob)
}

// ── GET /api/formats?url=<url> ────────────────────────────────────────────────

type formatsResponse struct {
	Title   string       `json:"title"`
	Formats []formatItem `json:"formats"`
}

type formatItem struct {
	FormatID      string  `json:"format_id"`
	Ext           string  `json:"ext"`
	Resolution    string  `json:"resolution"`
	FPS           float64 `json:"fps"`
	TBR           float64 `json:"tbr"`
	VCodec        string  `json:"vcodec"`
	AudioChannels int     `json:"audio_channels"`
	Filesize      int64   `json:"filesize"`
	FormatNote    string  `json:"format_note"`
	HaveAudio     bool    `json:"have_audio"`
	HaveVideo     bool    `json:"have_video"`
}

func (s *Server) handleFormats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := r.URL.Query().Get("url")
	if url == "" {
		http.Error(w, "url query parameter is required", http.StatusBadRequest)
		return
	}

	info, err := downloader.FetchVideoInfo(r.Context(), url)
	if err != nil {
		s.log.Warn("api: fetch formats failed", "url", url, "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	resp := formatsResponse{
		Title:   info.Title,
		Formats: make([]formatItem, len(info.Formats)),
	}
	for i, f := range info.Formats {
		resp.Formats[i] = formatItem{
			FormatID:      f.FormatID,
			Ext:           f.Ext,
			Resolution:    f.Resolution,
			FPS:           f.FPS,
			TBR:           f.TBR,
			VCodec:        f.VCodec,
			AudioChannels: f.AudioChannels,
			Filesize:      f.Filesize,
			FormatNote:    f.FormatNote,
			HaveAudio:     f.HasAudio(),
			HaveVideo:     f.HasVideo(),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ── POST /api/jobs  /  GET /api/jobs ─────────────────────────────────────────

type createJobRequest struct {
	URL          string `json:"url"`
	Title        string `json:"title,omitempty"`
	FormatArg    string `json:"format_arg,omitempty"`
	QualityLabel string `json:"quality_label,omitempty"`
	AudioOnly    bool   `json:"audio_only"`
	MergeAudio   bool   `json:"merge_audio"`
	OutputFormat string `json:"output_format,omitempty"`
}

type createJobResponse struct {
	JobID  int64  `json:"job_id"`
	FileID string `json:"file_id"`
}

type jobResponse struct {
	ID           int64  `json:"id"`
	FileID       string `json:"file_id"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	QualityLabel string `json:"quality_label"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	CreatedAt    string `json:"created_at"`
	DownloadURL  string `json:"download_url,omitempty"`
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listJobs(w, r)
	case http.MethodPost:
		s.createJob(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	if req.FormatArg == "" {
		req.FormatArg = "bestvideo+bestaudio/best"
	}

	// Fetch title synchronously if the caller did not supply one.
	if req.Title == "" {
		info, err := downloader.FetchVideoInfo(r.Context(), req.URL)
		if err != nil {
			s.log.Warn("api: fetch title failed", "url", req.URL, "err", err)
			req.Title = req.URL // fall back to URL as placeholder title
		} else {
			req.Title = info.Title
		}
	}

	if err := os.MkdirAll(s.outDir, 0o755); err != nil {
		s.log.Error("api: create out dir", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	record := &storage.Download{
		FileID:       uuid.NewString(),
		URL:          req.URL,
		Title:        req.Title,
		FormatArg:    req.FormatArg,
		QualityLabel: req.QualityLabel,
	}
	if err := s.db.Save(r.Context(), record); err != nil {
		s.log.Error("api: save record", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, createJobResponse{JobID: record.ID, FileID: record.FileID})

	go s.runDownload(context.Background(), record, downloader.Request{
		URL:   req.URL,
		Title: req.Title,
		Format: downloader.Format{
			Arg:        req.FormatArg,
			Label:      req.QualityLabel,
			AudioOnly:  req.AudioOnly,
			MergeAudio: req.MergeAudio,
		},
		OutDir:       s.outDir,
		OutputFormat: req.OutputFormat,
	})
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	records, err := s.db.List(r.Context())
	if err != nil {
		s.log.Error("api: list jobs", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	resp := make([]jobResponse, len(records))
	for i, dl := range records {
		resp[i] = toJobResponse(dl)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ── GET /api/jobs/{id} ────────────────────────────────────────────────────────

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if rawID == "" {
		http.Error(w, "missing job id", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		http.Error(w, "job id must be an integer", http.StatusBadRequest)
		return
	}

	dl, err := s.db.GetByID(r.Context(), id)
	if err != nil {
		s.log.Error("api: get job", "id", id, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if dl == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, toJobResponse(*dl))
}

// ── helpers ───────────────────────────────────────────────────────────────────

// runDownload executes the download in a goroutine, updates the DB, fires the
// webhook, and optionally publishes to RabbitMQ.
func (s *Server) runDownload(ctx context.Context, record *storage.Download, req downloader.Request) {
	result, err := downloader.Download(ctx, req)

	hookPayload := webhook.Payload{
		JobID:  record.ID,
		FileID: record.FileID,
		Title:  record.Title,
		URL:    record.URL,
	}
	event := queue.CompletedEvent{JobID: record.ID, FileID: record.FileID}

	if err != nil {
		hookPayload.Status = queue.StatusFailed
		hookPayload.Error = err.Error()
		event.Status = queue.StatusFailed
		event.Error = err.Error()
		s.log.Error("api: download failed", "job_id", record.ID, "err", err)
		if dbErr := s.db.UpdateError(ctx, record.ID, err.Error()); dbErr != nil {
			s.log.Error("api: update error status", "job_id", record.ID, "err", dbErr)
		}
	} else {
		hookPayload.Status = queue.StatusReady
		event.Status = queue.StatusReady
		if dbErr := s.db.UpdateOutputPath(ctx, record.ID, result.FilePath); dbErr != nil {
			s.log.Error("api: update output path", "job_id", record.ID, "err", dbErr)
		}
		s.log.Info("api: download done", "job_id", record.ID, "file_id", record.FileID)
	}

	if err := s.hook.Send(ctx, hookPayload); err != nil {
		s.log.Warn("api: webhook send failed", "job_id", record.ID, "err", err)
	}

	if s.publishFn != nil {
		if err := s.publishFn(event); err != nil {
			s.log.Warn("api: publish completed failed", "job_id", record.ID, "err", err)
		}
	}
}

func toJobResponse(dl storage.Download) jobResponse {
	r := jobResponse{
		ID:           dl.ID,
		FileID:       dl.FileID,
		URL:          dl.URL,
		Title:        dl.Title,
		QualityLabel: dl.QualityLabel,
		Status:       dl.Status,
		Error:        dl.ErrorMsg,
		CreatedAt:    dl.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if dl.Status == "ready" {
		r.DownloadURL = "/files/" + dl.FileID
	}
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
