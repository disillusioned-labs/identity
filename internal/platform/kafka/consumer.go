package kafka

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// ConsumerHandler handles one Kafka record.
//
// Returning nil means the record can be considered successfully processed.
// Returning an error leaves the decision about retry/DLQ/reprocessing to the
// consumer runner.
type ConsumerHandler func(ctx context.Context, record *kgo.Record) error

// Consumer wraps franz-go's polling consumer.
type Consumer struct {
	client *kgo.Client
}

// NewConsumer creates a consumer using an existing Kafka client.
//
// Consumer-group configuration should normally be supplied when constructing
// the client:
//
//	kafka.New(ctx, cfg.Kafka,
//	    kafka.ConsumerGroup("identity-worker", "user.created"),
//	)
func NewConsumer(client *kgo.Client) *Consumer {
	return &Consumer{
		client: client,
	}
}

// Poll fetches the next batch of Kafka records.
//
// This is intentionally a low-level helper. Business processing, database
// transactions, event-processing idempotency, DLQ policy, and commit/ack
// decisions belong to the worker/application layer.
func (c *Consumer) Poll(ctx context.Context) kgo.Fetches {
	if c == nil || c.client == nil {
		return nil
	}

	return c.client.PollFetches(ctx)
}

// Each iterates through fetched records.
//
// The handler is responsible for processing the record. This helper does not
// commit offsets because committing before durable processing would risk data
// loss.
func (c *Consumer) Each(
	ctx context.Context,
	fetches kgo.Fetches,
	handler ConsumerHandler,
) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("kafka consumer is not initialized")
	}

	if handler == nil {
		return fmt.Errorf("kafka consumer handler must not be nil")
	}

	var firstErr error

	fetches.EachRecord(func(record *kgo.Record) {
		if firstErr != nil {
			return
		}

		if err := handler(ctx, record); err != nil {
			firstErr = fmt.Errorf(
				"process kafka record topic=%s partition=%d offset=%d: %w",
				record.Topic,
				record.Partition,
				record.Offset,
				err,
			)
		}
	})

	return firstErr
}

// Commit commits offsets after successful processing.
//
// The application should call this only after the durable event-processing
// transaction has succeeded.
func (c *Consumer) CommitRecords(
	ctx context.Context,
	records ...*kgo.Record,
) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("kafka consumer is not initialized")
	}

	if len(records) == 0 {
		return nil
	}

	if err := c.client.CommitRecords(ctx, records...); err != nil {
		return fmt.Errorf("commit kafka offsets: %w", err)
	}

	return nil
}

// Close releases the underlying Kafka client.
func (c *Consumer) Close() {
	if c == nil || c.client == nil {
		return
	}

	c.client.Close()
}
