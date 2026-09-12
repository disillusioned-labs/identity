package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/disillusioned-labs/identity/internal/service/signingkey"
)

const (
	defaultRotationInterval = 720 * time.Hour // 30 days
	defaultRetirementDelay  = 24 * time.Hour
	defaultRotationCheck    = time.Hour
)

// RotationWorker runs the scheduled half of signing-key lifecycle: rotate the
// active key when it ages past its interval, and retire deactivated keys once
// their grace period has passed. Both halves must be off by default - a
// deployment that has not deliberately scheduled rotation gets exactly the
// manual `generate-signing-key --rotate` behaviour it always had.
type RotationWorker struct {
	service          signingkey.SigningKeyService
	enabled          bool
	rotationInterval time.Duration
	retirementDelay  time.Duration
	checkInterval    time.Duration
	log              *slog.Logger
}

// RotationOption customises a RotationWorker; zero-value options keep the
// defaults.
type RotationOption func(*RotationWorker)

// WithEnabled turns the scheduled rotation on. The default is off.
func WithEnabled(enabled bool) RotationOption {
	return func(w *RotationWorker) {
		w.enabled = enabled
	}
}

// WithRotationInterval sets the age at which the active key is replaced.
func WithRotationInterval(interval time.Duration) RotationOption {
	return func(w *RotationWorker) {
		if interval > 0 {
			w.rotationInterval = interval
		}
	}
}

// WithRetirementDelay sets how long a deactivated key stays published in
// JWKS. Must be at least the access-token TTL so no valid token outlives its
// key's publication.
func WithRetirementDelay(delay time.Duration) RotationOption {
	return func(w *RotationWorker) {
		if delay > 0 {
			w.retirementDelay = delay
		}
	}
}

// WithCheckInterval sets how often the worker evaluates both halves.
func WithCheckInterval(interval time.Duration) RotationOption {
	return func(w *RotationWorker) {
		if interval > 0 {
			w.checkInterval = interval
		}
	}
}

// NewRotationWorker builds the worker; disabled by default.
func NewRotationWorker(
	service signingkey.SigningKeyService,
	log *slog.Logger,
	opts ...RotationOption,
) *RotationWorker {
	w := &RotationWorker{
		service:          service,
		rotationInterval: defaultRotationInterval,
		retirementDelay:  defaultRetirementDelay,
		checkInterval:    defaultRotationCheck,
		log:              log,
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Run blocks until ctx is cancelled, checking rotation and retirement on one
// ticker. A failing tick is logged and retried - a transient database error
// must not take the worker down.
func (w *RotationWorker) Run(ctx context.Context) error {
	if !w.enabled {
		w.log.Info("rotation worker disabled")

		<-ctx.Done()

		return nil
	}

	w.log.Info(
		"rotation worker started",
		"rotation_interval", w.rotationInterval,
		"retirement_delay", w.retirementDelay,
		"check_interval", w.checkInterval,
	)

	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("rotation worker stopped")

			return nil

		case <-ticker.C:
			_, err := w.service.RotateIfDue(ctx, w.rotationInterval)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}

				w.log.ErrorContext(ctx, "signing key rotation failed", "error", err)
			}

			// Retirement runs every tick, not only on rotation ticks: a
			// rotated key must be retired even if the process restarted in
			// between.
			if _, err := w.service.RetireExpired(ctx, w.retirementDelay); err != nil {
				if ctx.Err() != nil {
					return nil
				}

				w.log.ErrorContext(ctx, "signing key retirement failed", "error", err)
			}
		}
	}
}
