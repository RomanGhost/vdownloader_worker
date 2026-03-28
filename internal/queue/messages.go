package queue

// Queues consumed and published by this service.
const (
	QueueGetFormats = "video.get_formats"
	QueueDownload   = "video.download"
	QueueCompleted  = "video.completed"
)

// GetFormatsRequest is sent by the client to request available formats.
type GetFormatsRequest struct {
	URL string `json:"url"`
}

// GetFormatsResponse is the RPC reply with the video title and its formats.
type GetFormatsResponse struct {
	Title   string          `json:"title"`
	Formats []FormatMessage `json:"formats"`
	Error   string          `json:"error,omitempty"`
}

// FormatMessage is a JSON-serialisable description of one yt-dlp format.
type FormatMessage struct {
	FormatID      string  `json:"format_id"`
	Ext           string  `json:"ext"`
	Resolution    string  `json:"resolution"`
	FPS           float64 `json:"fps"`
	TBR           float64 `json:"tbr"`
	VCodec        string  `json:"vcodec"`
	AudioChannels int     `json:"audio_channels"`
	Filesize      int64   `json:"filesize"`
	FormatNote    string  `json:"format_note"`
	AudioOnly     bool    `json:"audio_only"`
	VideoOnly     bool    `json:"video_only"`
}

// DownloadRequest is sent by the client to start a download job.
// The output directory is determined by the worker (OUT_DIR env).
type DownloadRequest struct {
	URL          string `json:"url"`
	Title        string `json:"title"`
	FormatArg    string `json:"format_arg"`
	QualityLabel string `json:"quality_label"`
	AudioOnly    bool   `json:"audio_only"`
	MergeAudio   bool   `json:"merge_audio"`
}

// DownloadResponse is the synchronous RPC reply: the job identifier.
type DownloadResponse struct {
	JobID int64  `json:"job_id"`
	Error string `json:"error,omitempty"`
}

// Status values for CompletedEvent.
const (
	StatusReady  = "ready"
	StatusFailed = "failed"
)

// CompletedEvent is published to QueueCompleted when the download finishes.
type CompletedEvent struct {
	JobID  int64  `json:"job_id"`
	FileID string `json:"file_id,omitempty"`
	Status string `json:"status"` // "ready" | "failed"
	Error  string `json:"error,omitempty"`
}
