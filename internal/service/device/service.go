package device

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/disillusioned-labs/platform/pgutil"
)

var tracer = otel.Tracer("service/device")

const eventVersion = 1

// DeviceService registers and revokes the push tokens mobile/web clients
// present on login. The notification service resolves deliveries against
// these rows via gRPC; identity only ever stores the token opaquely and
// never logs or emits it.
type DeviceService interface {
	Register(ctx context.Context, userID uuid.UUID, input RegisterInput) (RegisterOutput, error)
	Unregister(ctx context.Context, userID uuid.UUID, input RevokeInput) (UnregisterOutput, error)
	// ListActiveTokens backs the gRPC GetDeviceTokens surface consumed by the
	// notification service before fanning a push delivery out per device.
	ListActiveTokens(ctx context.Context, userID uuid.UUID) ([]string, error)
}

type deviceService struct {
	repo repository.Store
	log  *slog.Logger
}

// NewDeviceService takes the store because Register and Unregister each write
// the device row and its audit event in one transaction.
func NewDeviceService(repo repository.Store, log *slog.Logger) DeviceService {
	return &deviceService{repo: repo, log: log}
}

func (s *deviceService) Register(ctx context.Context, userID uuid.UUID, input RegisterInput) (RegisterOutput, error) {
	ctx, span := tracer.Start(ctx, "DeviceService.Register")
	defer span.End()

	var deviceID uuid.UUID

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		row, err := q.UpsertUserDevice(ctx, repository.UpsertUserDeviceParams{
			UserID:     userID,
			Token:      input.Token,
			Platform:   input.Platform,
			AppVersion: pgutil.TextFromString(input.AppVersion),
		})
		if err != nil {
			return err
		}
		deviceID = row.ID
		return service.Emit(ctx, q, "user_device", row.ID,
			EventDeviceRegistered, eventVersion, constant.TopicAudit, DeviceRegisteredEvent{
				UserID:     userID,
				DeviceID:   row.ID,
				Platform:   input.Platform,
				AppVersion: input.AppVersion,
			})
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "register device failed")
		s.log.ErrorContext(ctx, "register device failed", "error", err, "user_id", userID)
		return RegisterOutput{}, service.ErrInternal
	}

	return RegisterOutput{DeviceID: deviceID}, nil
}

func (s *deviceService) Unregister(ctx context.Context, userID uuid.UUID, input RevokeInput) (UnregisterOutput, error) {
	ctx, span := tracer.Start(ctx, "DeviceService.Unregister")
	defer span.End()

	var revoked repository.RevokeUserDeviceRow

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		row, err := q.RevokeUserDevice(ctx, repository.RevokeUserDeviceParams{
			UserID: userID,
			Token:  input.Token,
		})
		if err != nil {
			return err
		}
		revoked = row
		return service.Emit(ctx, q, "user_device", row.ID,
			EventDeviceRevoked, eventVersion, constant.TopicAudit, DeviceRevokedEvent{
				UserID:   userID,
				DeviceID: row.ID,
				Platform: row.Platform,
			})
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The token was never registered by this user, or is already
			// revoked - either way there is nothing to undo.
			return UnregisterOutput{}, service.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "unregister device failed")
		s.log.ErrorContext(ctx, "unregister device failed", "error", err, "user_id", userID)
		return UnregisterOutput{}, service.ErrInternal
	}

	return UnregisterOutput{DeviceID: revoked.ID, Platform: revoked.Platform}, nil
}

func (s *deviceService) ListActiveTokens(ctx context.Context, userID uuid.UUID) ([]string, error) {
	ctx, span := tracer.Start(ctx, "DeviceService.ListActiveTokens")
	defer span.End()

	tokens, err := s.repo.ListActiveDeviceTokens(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list device tokens failed")
		s.log.ErrorContext(ctx, "list device tokens failed", "error", err, "user_id", userID)
		return nil, service.ErrInternal
	}

	return tokens, nil
}
