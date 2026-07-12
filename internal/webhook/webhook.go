// Package webhook fires an HTTP POST notification when a download completes.
// If no URL is configured the Caller is a no-op.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Payload is the JSON body sent to the webhook endpoint on every completion event.
type Payload struct {
	JobID  int64  `json:"job_id"`
	FileID string `json:"file_id,omitempty"`
	Status string `json:"status"` // "ready" | "failed"
	Error  string `json:"error,omitempty"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
}

// Caller posts completion payloads to a configured HTTP endpoint.
type Caller struct {
	url    string
	client *http.Client
	log    *slog.Logger
}

// New creates a Caller. When url is empty all Send calls are silent no-ops.
func New(url string, log *slog.Logger) *Caller {
	return &Caller{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		log:    log,
	}
}

// Send POSTs p as JSON to the configured URL.
// Returns nil immediately when no URL is set.
func (c *Caller) Send(ctx context.Context, p Payload) error {
	if c.url == "" {
		return nil
	}

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}

	c.log.Info("webhook sent", "job_id", p.JobID, "status", p.Status)
	return nil
}
