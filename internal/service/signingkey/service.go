// Package signingkey owns the signing-key lifecycle: scheduled rotation and
// retirement. Rotation publishes a new active key and deactivates the old one;
// retirement removes the deactivated key from JWKS after a grace period, so
// tokens signed just before a rotation keep verifying until they expire.
package signingkey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/disillusioned-labs/platform/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("service/signingkey")

type SigningKeyService interface {
	// RotateIfDue inserts a new active signing key when the current one is
	// older than rotationInterval and reports whether it rotated. With no
	// active key at all it seeds the first one (the boot-without-key case the
	// generate-signing-key tool normally covers).
	RotateIfDue(ctx context.Context, rotationInterval time.Duration) (bool, error)

	// RetireExpired marks deactivated keys older than the retirement delay as
	// retired, dropping them from JWKS. The delay must be at least the
	// access-token TTL: anything signed before rotation has expired by then.
	RetireExpired(ctx context.Context, retirementDelay time.Duration) (int64, error)
}

type signingKeyService struct {
	repo      repository.Store
	masterKey []byte
	log       *slog.Logger
}

func NewSigningKeyService(repo repository.Store, masterKey []byte, log *slog.Logger) SigningKeyService {
	return &signingKeyService{
		repo:      repo,
		masterKey: masterKey,
		log:       log,
	}
}

func (s *signingKeyService) RotateIfDue(
	ctx context.Context,
	rotationInterval time.Duration,
) (bool, error) {
	ctx, span := tracer.Start(ctx, "SigningKeyService.RotateIfDue")
	defer span.End()

	active, err := s.repo.GetActiveSigningKeyAge(ctx)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			span.RecordError(err)
			span.SetStatus(codes.Error, "query active signing key age failed")
			s.log.ErrorContext(ctx, "query active signing key age failed", "error", err)
			return false, fmt.Errorf("get active signing key age: %w", err)
		}

		// No active key: seed the first one. Signing fails without a key, so
		// this is a repair, not a routine rotation.
		if err := s.rotate(ctx); err != nil {
			return false, err
		}

		s.log.InfoContext(ctx, "seeded initial signing key")
		return true, nil
	}

	if time.Since(active.CreatedAt) < rotationInterval {
		return false, nil
	}

	if err := s.rotate(ctx); err != nil {
		return false, err
	}

	s.log.InfoContext(
		ctx,
		"signing key rotated",
		"old_kid", active.Kid,
		"age", time.Since(active.CreatedAt),
	)

	return true, nil
}

// rotate generates a fresh keypair and swaps it in as the single active key,
// mirroring the generate-signing-key tool: deactivate current, insert new,
// one transaction, protected by ux_signing_keys_active.
func (s *signingKeyService) rotate(ctx context.Context) error {
	privPEM, pubPEM, err := crypto.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate keypair: %w", err)
	}

	encrypted, err := crypto.EncryptPrivateKey(privPEM, s.masterKey)
	if err != nil {
		return fmt.Errorf("encrypt private key: %w", err)
	}

	kid := uuid.New().String()

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		if err := q.RotateSigningKey(ctx); err != nil {
			return fmt.Errorf("deactivate current key: %w", err)
		}

		if err := q.InsertSigningKey(ctx, repository.InsertSigningKeyParams{
			Kid:                 kid,
			PrivateKeyEncrypted: encrypted,
			PublicKey:           string(pubPEM),
			Algorithm:           "RS256",
			IsActive:            true,
		}); err != nil {
			return err
		}

		// A new signing key changes what the platform will trust - a
		// security-configuration change (PCI 10.2), audited with the key id
		// only; the material itself never enters the event.
		kidUUID, parseErr := uuid.Parse(kid)
		if parseErr != nil {
			return fmt.Errorf("parse kid: %w", parseErr)
		}
		return service.Emit(ctx, q, "signing_key", kidUUID,
			EventKeyRotated, eventVersion, constant.TopicAudit, KeyRotatedEvent{
				Kid:       kid,
				Algorithm: "RS256",
				Actor:     "system:rotation-worker",
			})
	})
	if err != nil {
		return fmt.Errorf("insert signing key: %w", err)
	}

	return nil
}

func (s *signingKeyService) RetireExpired(
	ctx context.Context,
	retirementDelay time.Duration,
) (int64, error) {
	ctx, span := tracer.Start(ctx, "SigningKeyService.RetireExpired")
	defer span.End()

	cutoff := pgtype.Timestamptz{
		Time:  time.Now().Add(-retirementDelay),
		Valid: true,
	}

	retired, err := s.repo.RetireExpiredSigningKeys(ctx, cutoff)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "retire expired signing keys failed")
		s.log.ErrorContext(ctx, "retire expired signing keys failed", "error", err)
		return 0, fmt.Errorf("retire expired signing keys: %w", err)
	}

	if retired > 0 {
		s.log.InfoContext(
			ctx,
			"signing keys retired",
			"count", retired,
			"retirement_delay", retirementDelay,
		)
	}

	return retired, nil
}

const (
	eventVersion    = 1
	EventKeyRotated = "signingkey.rotated"
)

type KeyRotatedEvent struct {
	Kid       string `json:"kid"`
	Algorithm string `json:"algorithm"`
	Actor     string `json:"actor"`
}
