package kafka

import (
	"context"
	"errors"
	"log/slog"

	"github.com/segmentio/kafka-go"
)

// Consumer reads job requests from Kafka.
type Consumer struct {
	reader *kafka.Reader
}

// NewConsumer creates a Consumer reading topic on the given brokers as groupID.
func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: groupID,
		}),
	}
}

// Close closes the underlying reader, unblocking any in-flight Consume call.
func (c *Consumer) Close() error {
	return c.reader.Close()
}

// Consume blocks reading messages from the topic and invokes handle with each
// message's raw JSON value until ctx is cancelled or the reader is closed.
func (c *Consumer) Consume(ctx context.Context, log *slog.Logger, handle func(context.Context, []byte)) {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, kafka.ErrGroupClosed) {
				return
			}
			log.Error("kafka read job request", "err", err)
			continue
		}
		handle(ctx, msg.Value)
	}
}
