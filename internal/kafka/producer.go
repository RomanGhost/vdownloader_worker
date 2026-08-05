// Package kafka handles the worker's two Kafka topics: it consumes job
// requests and publishes job-completed notifications.
//
// The worker publishes only the job's file_id when a download finishes;
// consumers fetch the full status (ready/failed, file_id, error) via the
// worker's REST API at GET /api/jobs/{file_id}.
package kafka

import (
	"context"
	"encoding/json"

	"github.com/segmentio/kafka-go"
)

// completedMessage is the JSON value written to the completed topic.
type completedMessage struct {
	FileID string `json:"file_id"`
}

// Producer publishes job-completed notifications to Kafka.
type Producer struct {
	writer *kafka.Writer
}

// NewProducer creates a Producer targeting topic on the given brokers.
// The underlying writer connects lazily on the first Publish call.
func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.LeastBytes{},
			AllowAutoTopicCreation: true,
		},
	}
}

// Publish writes the completed job's file_id to the topic, keyed by file_id.
func (p *Producer) Publish(ctx context.Context, fileID string) error {
	value, err := json.Marshal(completedMessage{FileID: fileID})
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(fileID),
		Value: value,
	})
}

// Close flushes and closes the underlying writer.
func (p *Producer) Close() error {
	return p.writer.Close()
}
