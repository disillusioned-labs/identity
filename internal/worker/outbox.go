package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/disillusioned-labs/identity/internal/service/outbox"
	"github.com/google/uuid"
)

const (
	defaultOutboxInterval = time.Second
	defaultOutboxBatch    = 100
)

type OutboxWorker struct {
	service   outbox.OutboxService
	log       *slog.Logger
	interval  time.Duration
	batchSize int
	workerID  string
}

type Option func(*OutboxWorker)

func WithInterval(interval time.Duration) Option {
	return func(worker *OutboxWorker) {
		if interval > 0 {
			worker.interval = interval
		}
	}
}

func WithBatchSize(batchSize int) Option {
	return func(worker *OutboxWorker) {
		if batchSize > 0 {
			worker.batchSize = batchSize
		}
	}
}

func NewOutboxWorker(
	service outbox.OutboxService,
	log *slog.Logger,
	opts ...Option,
) *OutboxWorker {
	worker := &OutboxWorker{
		service:   service,
		log:       log,
		interval:  defaultOutboxInterval,
		batchSize: defaultOutboxBatch,
		workerID:  uuid.NewString(),
	}

	for _, opt := range opts {
		opt(worker)
	}

	return worker
}

func (w *OutboxWorker) Run(ctx context.Context) error {
	w.log.Info(
		"outbox worker started",
		"worker_id", w.workerID,
		"interval", w.interval,
		"batch_size", w.batchSize,
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info(
				"outbox worker stopped",
				"worker_id", w.workerID,
			)
			return nil

		case <-ticker.C:
			err := w.service.PublishPending(
				ctx,
				w.workerID,
				w.batchSize,
			)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}

				w.log.ErrorContext(
					ctx,
					"outbox publish failed",
					"error", err,
					"worker_id", w.workerID,
				)
			}
		}
	}
}
