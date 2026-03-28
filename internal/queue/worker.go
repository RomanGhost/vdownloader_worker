package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"downloader/internal/downloader"
	"downloader/internal/storage"
)

// Worker consumes RabbitMQ queues and runs yt-dlp downloads.
//
// Flow:
//   - video.get_formats → FetchVideoInfo → RPC reply with formats        (sync)
//   - video.download    → save DB record → RPC reply with job_id         (sync)
//                       → yt-dlp download in goroutine                    (async)
//                       → publish CompletedEvent{file_id, status} when done (async)
type Worker struct {
	conn   *amqp.Connection
	ch     *amqp.Channel
	db     *storage.DB
	outDir string
	log    *slog.Logger
}

// NewWorker dials RabbitMQ, opens a channel, and declares all required queues.
func NewWorker(amqpURL string, db *storage.DB, outDir string, log *slog.Logger) (*Worker, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	for _, name := range []string{QueueGetFormats, QueueDownload, QueueCompleted} {
		if _, err := ch.QueueDeclare(name, true, false, false, false, nil); err != nil {
			ch.Close()
			conn.Close()
			return nil, fmt.Errorf("declare queue %q: %w", name, err)
		}
	}

	return &Worker{conn: conn, ch: ch, db: db, outDir: outDir, log: log}, nil
}

// Run blocks until ctx is cancelled, processing one message at a time per queue.
func (w *Worker) Run(ctx context.Context) error {
	// Fair dispatch: don't give a worker more than 1 unacknowledged message.
	if err := w.ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	formats, err := w.ch.Consume(QueueGetFormats, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", QueueGetFormats, err)
	}

	downloads, err := w.ch.Consume(QueueDownload, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", QueueDownload, err)
	}

	w.log.Info("worker ready", "queues", []string{QueueGetFormats, QueueDownload})

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-formats:
			if !ok {
				return fmt.Errorf("queue %s closed", QueueGetFormats)
			}
			w.handleGetFormats(ctx, msg)
		case msg, ok := <-downloads:
			if !ok {
				return fmt.Errorf("queue %s closed", QueueDownload)
			}
			w.handleDownload(ctx, msg)
		}
	}
}

// Close shuts down the AMQP channel and connection gracefully.
func (w *Worker) Close() {
	w.ch.Close()
	w.conn.Close()
}

// handleGetFormats: sync RPC — fetch formats and reply.
func (w *Worker) handleGetFormats(ctx context.Context, msg amqp.Delivery) {
	var req GetFormatsRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		w.log.Error("decode get_formats request", "err", err)
		msg.Nack(false, false)
		return
	}

	resp := GetFormatsResponse{}
	info, err := downloader.FetchVideoInfo(ctx, req.URL)
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Title = info.Title
		resp.Formats = make([]FormatMessage, len(info.Formats))
		for i, f := range info.Formats {
			resp.Formats[i] = FormatMessage{
				FormatID:      f.FormatID,
				Ext:           f.Ext,
				Resolution:    f.Resolution,
				FPS:           f.FPS,
				TBR:           f.TBR,
				VCodec:        f.VCodec,
				AudioChannels: f.AudioChannels,
				Filesize:      f.Filesize,
				FormatNote:    f.FormatNote,
				AudioOnly:     f.IsAudioOnly(),
				VideoOnly:     f.IsVideoOnly(),
			}
		}
	}

	if err := w.reply(msg, resp); err != nil {
		w.log.Error("reply get_formats", "err", err)
		msg.Nack(false, false)
		return
	}
	msg.Ack(false)
}

// handleDownload: sync part — save record with UUID file_id, reply with job_id.
// async part — run yt-dlp, update DB, publish CompletedEvent.
func (w *Worker) handleDownload(ctx context.Context, msg amqp.Delivery) {
	var req DownloadRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		w.log.Error("decode download request", "err", err)
		msg.Nack(false, false)
		return
	}

	if err := os.MkdirAll(w.outDir, 0o755); err != nil {
		w.replyErr(msg, fmt.Errorf("create out dir: %w", err))
		return
	}

	// Persist a pending record now to generate the job ID before downloading.
	record := &storage.Download{
		FileID:       uuid.NewString(),
		URL:          req.URL,
		Title:        req.Title,
		FormatArg:    req.FormatArg,
		QualityLabel: req.QualityLabel,
	}
	if err := w.db.Save(ctx, record); err != nil {
		w.replyErr(msg, fmt.Errorf("save record: %w", err))
		return
	}

	// ── Synchronous reply: job accepted ──────────────────────────────────────
	if err := w.reply(msg, DownloadResponse{JobID: record.ID}); err != nil {
		w.log.Error("reply download", "err", err)
		msg.Nack(false, false)
		return
	}
	msg.Ack(false)

	// ── Asynchronous: download → update DB → publish completion ──────────────
	go func(jobID int64, fileID string) {
		result, err := downloader.Download(context.Background(), downloader.Request{
			URL:   req.URL,
			Title: req.Title,
			Format: downloader.Format{
				Arg:        req.FormatArg,
				Label:      req.QualityLabel,
				AudioOnly:  req.AudioOnly,
				MergeAudio: req.MergeAudio,
			},
			OutDir: w.outDir,
		})

		event := CompletedEvent{JobID: jobID, FileID: fileID}
		if err != nil {
			event.Status = StatusFailed
			event.Error = err.Error()
			w.log.Error("download failed", "job_id", jobID, "err", err)
		} else {
			event.Status = StatusReady
			if dbErr := w.db.UpdateOutputPath(context.Background(), jobID, result.FilePath); dbErr != nil {
				w.log.Error("update output path", "job_id", jobID, "err", dbErr)
			}
			w.log.Info("download done", "job_id", jobID, "file_id", fileID)
		}

		if err := w.publish(QueueCompleted, event); err != nil {
			w.log.Error("publish completed", "job_id", jobID, "err", err)
		}
	}(record.ID, record.FileID)
}

// reply sends a JSON response to msg.ReplyTo preserving the correlation ID.
// It is a no-op when ReplyTo is empty (fire-and-forget calls).
func (w *Worker) reply(msg amqp.Delivery, body any) error {
	if msg.ReplyTo == "" {
		return nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return w.ch.PublishWithContext(context.Background(), "", msg.ReplyTo, false, false,
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: msg.CorrelationId,
			Body:          data,
		},
	)
}

// replyErr sends an error response and nacks the original message.
func (w *Worker) replyErr(msg amqp.Delivery, err error) {
	type errBody struct {
		Error string `json:"error"`
	}
	if rerr := w.reply(msg, errBody{Error: err.Error()}); rerr != nil {
		w.log.Error("reply error", "err", rerr)
	}
	msg.Nack(false, false)
}

// publish sends a JSON message to a named queue.
func (w *Worker) publish(queue string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return w.ch.PublishWithContext(context.Background(), "", queue, false, false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        data,
		},
	)
}
