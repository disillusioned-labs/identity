package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/bcrypt"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	notificationcontract "github.com/disillusioned-labs/platform/contract/notification"
)

const (
	passwordRecoveryTokenBytes = 32
	// This floor narrows the timing difference between missing and existing accounts.
	passwordRecoveryResponseFloor = 500 * time.Millisecond
)

var (
	ErrInvalidRecoveryToken = service.NewError("RECOVERY_LINK_INVALID", 410, "recovery link is invalid or expired")
	ErrInvalidPassword      = service.NewError("INVALID_PASSWORD", 422, "password must not exceed 72 bytes")
	ErrRecoveryUnavailable  = service.NewError("RECOVERY_UNAVAILABLE", 503, "password reset is temporarily unavailable")
)

type passwordRecoveryEmail struct {
	Name        string    `json:"name"`
	RecoveryURL string    `json:"recovery_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type passwordChangedEmail struct {
	Name string `json:"name"`
}

func (s *authService) RequestPasswordRecovery(
	ctx context.Context,
	input RequestPasswordRecoveryInput,
) (RequestPasswordRecoveryOutput, error) {
	startedAt := time.Now()
	defer waitForRecoveryResponseFloor(ctx, startedAt)

	ctx, span := tracer.Start(ctx, "AuthService.RequestPasswordRecovery")
	defer span.End()

	token, tokenHash, err := newPasswordRecoveryToken()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate recovery token")
		s.log.ErrorContext(ctx, "generate password recovery token failed", "error", err)
		return RequestPasswordRecoveryOutput{}, service.ErrInternal
	}

	link, err := passwordRecoveryLink(s.recoveryURL, token)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "build recovery link")
		s.log.ErrorContext(ctx, "build password recovery link failed", "error", err)
		return RequestPasswordRecoveryOutput{}, service.ErrInternal
	}

	user, err := s.repo.GetRecoveryUserByEmail(ctx, input.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return RequestPasswordRecoveryOutput{}, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "find recovery account")
		s.log.ErrorContext(ctx, "find account for password recovery failed", "error", err)
		return RequestPasswordRecoveryOutput{}, service.ErrInternal
	}

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		tokenRow, err := q.UpsertPasswordRecoveryToken(ctx, repository.UpsertPasswordRecoveryTokenParams{
			UserID:    user.ID,
			TokenHash: tokenHash,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// A prior request is inside the database-enforced resend cooldown.
			return nil
		}
		if err != nil {
			return fmt.Errorf("upsert password recovery token: %w", err)
		}

		payload, err := json.Marshal(passwordRecoveryEmail{
			Name:        user.Name,
			RecoveryURL: link,
			ExpiresAt:   tokenRow.ExpiresAt,
		})
		if err != nil {
			return fmt.Errorf("marshal password recovery notification: %w", err)
		}

		event := notificationcontract.CreatedEvent{
			NotificationType: "password_recovery",
			Category:         notificationcontract.CategoryTransactional,
			RecipientID:      user.ID.String(),
			Targets: []notificationcontract.Target{{
				Channel:     notificationcontract.ChannelEmail,
				Destination: user.Email,
			}},
			Payload: payload,
		}
		if err := event.Validate(); err != nil {
			return fmt.Errorf("validate password recovery notification: %w", err)
		}

		return service.Emit(ctx, q, authAggregateType, user.ID,
			notificationcontract.EventTypeCreated,
			authEventVersion,
			constant.TopicNotificationTransactional,
			event,
		)
	})
	if err != nil {
		// Keep the public response identical for known and unknown addresses.
		span.RecordError(err)
		span.SetStatus(codes.Error, "queue recovery notification")
		s.log.ErrorContext(ctx, "queue password recovery notification failed", "error", err, "user_id", user.ID)
	}

	return RequestPasswordRecoveryOutput{}, nil
}

func (s *authService) ResetPassword(
	ctx context.Context,
	input ResetPasswordInput,
) (ResetPasswordOutput, error) {
	ctx, span := tracer.Start(ctx, "AuthService.ResetPassword")
	defer span.End()

	if s.revocations == nil {
		return ResetPasswordOutput{}, ErrRecoveryUnavailable
	}
	if len([]byte(input.Password)) > 72 {
		return ResetPasswordOutput{}, ErrInvalidPassword
	}

	decoded, err := base64.RawURLEncoding.DecodeString(input.Token)
	if err != nil || len(decoded) != passwordRecoveryTokenBytes {
		return ResetPasswordOutput{}, ErrInvalidRecoveryToken
	}
	tokenHash := hashPasswordRecoveryToken(input.Token)

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash reset password")
		s.log.ErrorContext(ctx, "hash password recovery password failed", "error", err)
		return ResetPasswordOutput{}, service.ErrInternal
	}

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		token, err := q.GetPasswordRecoveryTokenByHash(ctx, tokenHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidRecoveryToken
		}
		if err != nil {
			return fmt.Errorf("get password recovery token: %w", err)
		}

		// Redis is updated before commit while the user row is locked. Login
		// rechecks the password hash under that same lock before issuing tokens.
		if err := s.revocations.RevokeUser(ctx, token.UserID.String()); err != nil {
			span.RecordError(err)
			return ErrRecoveryUnavailable
		}

		rows, err := q.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
			ID:       token.UserID,
			Password: string(passwordHash),
		})
		if err != nil {
			return fmt.Errorf("update recovered password: %w", err)
		}
		if rows != 1 {
			return ErrInvalidRecoveryToken
		}

		rows, err = q.ConsumePasswordRecoveryToken(ctx, token.UserID)
		if err != nil {
			return fmt.Errorf("consume password recovery token: %w", err)
		}
		if rows != 1 {
			return ErrInvalidRecoveryToken
		}

		if _, err := q.RevokeAllUserRefreshTokens(ctx, token.UserID); err != nil {
			return fmt.Errorf("revoke refresh tokens after password recovery: %w", err)
		}

		payload, err := json.Marshal(passwordChangedEmail{Name: token.Name})
		if err != nil {
			return fmt.Errorf("marshal password changed notification: %w", err)
		}
		event := notificationcontract.CreatedEvent{
			NotificationType: "password_changed",
			Category:         notificationcontract.CategoryTransactional,
			RecipientID:      token.UserID.String(),
			Targets: []notificationcontract.Target{{
				Channel:     notificationcontract.ChannelEmail,
				Destination: token.Email,
			}},
			Payload: payload,
		}
		if err := event.Validate(); err != nil {
			return fmt.Errorf("validate password changed notification: %w", err)
		}
		return service.Emit(ctx, q, authAggregateType, token.UserID,
			notificationcontract.EventTypeCreated,
			authEventVersion,
			constant.TopicNotificationTransactional,
			event,
		)
	})
	if err != nil {
		if errors.Is(err, ErrInvalidRecoveryToken) || errors.Is(err, ErrRecoveryUnavailable) {
			return ResetPasswordOutput{}, err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "reset password transaction failed")
		s.log.ErrorContext(ctx, "password recovery transaction failed", "error", err)
		return ResetPasswordOutput{}, service.ErrInternal
	}

	return ResetPasswordOutput{}, nil
}

func newPasswordRecoveryToken() (string, string, error) {
	value := make([]byte, passwordRecoveryTokenBytes)
	if _, err := rand.Read(value); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(value)
	return token, hashPasswordRecoveryToken(token), nil
}

func hashPasswordRecoveryToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func passwordRecoveryLink(baseURL, token string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("invalid password recovery URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + token
	return u.String(), nil
}

func waitForRecoveryResponseFloor(ctx context.Context, startedAt time.Time) {
	remaining := passwordRecoveryResponseFloor - time.Since(startedAt)
	if remaining <= 0 {
		return
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
