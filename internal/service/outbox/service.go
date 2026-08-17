package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disillusioned-labs/identity/internal/platform/kafka"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("service/outbox")

const (
	defaultBatchSize  = 100
	defaultRetryDelay = 5 * time.Second
)

type OutboxService interface {
	PublishPending(ctx context.Context, workerID string, batchSize int) error
}

type outboxService struct {
	repo     repository.Store
	producer kafka.Producer
	log      *slog.Logger
}

func NewOutboxService(
	repo repository.Store,
	producer kafka.Producer,
	log *slog.Logger,
) OutboxService {
	return &outboxService{
		repo:     repo,
		producer: producer,
		log:      log,
	}
}

func (s *outboxService) PublishPending(
	ctx context.Context,
	workerID string,
	batchSize int,
) error {
	ctx, span := tracer.Start(ctx, "OutboxService.PublishPending")
	defer span.End()

	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	span.SetAttributes(
		attribute.String("outbox.worker_id", workerID),
		attribute.Int("outbox.batch_size", batchSize),
	)

	events, err := s.repo.ClaimPendingOutboxEvents(
		ctx,
		repository.ClaimPendingOutboxEventsParams{
			Limit: int32(batchSize),
			LockedBy: pgtype.Text{
				String: workerID,
				Valid:  true,
			},
		},
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			"claim pending outbox events failed",
		)

		s.log.ErrorContext(
			ctx,
			"claim pending outbox events failed",
			"error", err,
			"worker_id", workerID,
		)

		return fmt.Errorf("claim pending outbox events: %w", err)
	}

	span.SetAttributes(
		attribute.Int("outbox.events_claimed", len(events)),
	)

	for _, event := range events {
		if err := s.publishEvent(ctx, event); err != nil {
			continue
		}
	}

	return nil
}

func (s *outboxService) publishEvent(
	ctx context.Context,
	event repository.OutboxEvent,
) error {
	ctx, span := tracer.Start(ctx, "OutboxService.PublishEvent")
	defer span.End()

	span.SetAttributes(
		attribute.String("outbox.event_id", event.ID.String()),
		attribute.String("outbox.event_type", event.EventType),
		attribute.String("outbox.aggregate_type", event.AggregateType),
		attribute.String("outbox.aggregate_id", event.AggregateID.String()),
		attribute.Int("outbox.attempt_count", int(event.AttemptCount)),
	)

	err := s.producer.Publish(
		ctx,
		kafka.Record{
			Topic:   event.EventType,
			Key:     []byte(event.AggregateID.String()),
			Value:   event.Payload,
			EventID: event.ID.String(),
		},
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			"publish kafka event failed",
		)

		s.log.ErrorContext(
			ctx,
			"publish outbox event failed",
			"error", err,
			"event_id", event.ID,
			"event_type", event.EventType,
		)

		nextAttemptAt := time.Now().Add(defaultRetryDelay)
		lastError := err.Error()

		markErr := s.repo.MarkOutboxEventFailed(
			ctx,
			repository.MarkOutboxEventFailedParams{
				ID: event.ID,
				NextAttemptAt: pgtype.Timestamptz{
					Time:  nextAttemptAt,
					Valid: true,
				},
				LastError: pgtype.Text{
					String: lastError,
					Valid:  true,
				},
			},
		)
		if markErr != nil {
			span.RecordError(markErr)

			s.log.ErrorContext(
				ctx,
				"mark outbox event failed",
				"error", markErr,
				"event_id", event.ID,
				"event_type", event.EventType,
			)

			return fmt.Errorf("mark outbox event failed: %w", markErr)
		}

		return fmt.Errorf("publish outbox event: %w", err)
	}

	err = s.repo.MarkOutboxEventPublished(
		ctx,
		event.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			"mark outbox event published failed",
		)

		s.log.ErrorContext(
			ctx,
			"mark outbox event published failed",
			"error", err,
			"event_id", event.ID,
			"event_type", event.EventType,
		)

		return fmt.Errorf("mark outbox event published: %w", err)
	}

	s.log.DebugContext(
		ctx,
		"outbox event published",
		"event_id", event.ID,
		"event_type", event.EventType,
	)

	return nil
}
