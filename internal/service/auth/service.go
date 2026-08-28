package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"log/slog"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"

	"github.com/disillusioned-labs/identity/internal/constant"
	notificationcontract "github.com/disillusioned-labs/platform/contract/notification"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/disillusioned-labs/platform/crypto"
	platformjwt "github.com/disillusioned-labs/platform/jwt"
)

var tracer = otel.Tracer("service/auth")

const (
	authAggregateType = "user"
	authEventVersion  = 1
)

type AuthService interface {
	Register(ctx context.Context, input RegisterInput) (RegisterOutput, error)
	Login(ctx context.Context, input LoginInput) (LoginOutput, error)
	Me(ctx context.Context, input MeInput) (MeOutput, error)
	Refresh(ctx context.Context, input RefreshInput) (RefreshOutput, error)
}

type authService struct {
	repo            repository.Store
	masterKey       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	issuer          string
	log             *slog.Logger
}

func NewAuthService(repo repository.Store, masterKey []byte, accessTokenTTL time.Duration, refreshTokenTTL time.Duration, issuer string, log *slog.Logger) AuthService {
	return &authService{
		repo:            repo,
		masterKey:       masterKey,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
		issuer:          issuer,
		log:             log,
	}
}

func (s *authService) Register(ctx context.Context, input RegisterInput) (RegisterOutput, error) {
	ctx, span := tracer.Start(ctx, "AuthService.Register")
	defer span.End()

	_, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err == nil {
		span.SetStatus(codes.Error, "email already taken")
		return RegisterOutput{}, service.ErrEmailTaken
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query user by email failed")
		s.log.ErrorContext(ctx, "query user by email failed", "error", err)
		return RegisterOutput{}, service.ErrInternal
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash password failed")
		s.log.ErrorContext(ctx, "hash password failed", "error", err)
		return RegisterOutput{}, service.ErrInternal
	}

	var (
		user         repository.CreateUserRow
		organization repository.CreateOrganizationRow
		tokens       tokens
	)

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		user, err = q.CreateUser(ctx, repository.CreateUserParams{
			Email:    input.Email,
			Password: string(hash),
			Name:     input.Name,
		})
		if err != nil {
			if service.IsUniqueViolation(err) {
				span.SetStatus(codes.Error, "email already taken")
				return service.ErrEmailTaken
			}
			return err
		}

		organization, err = q.CreateOrganization(ctx, repository.CreateOrganizationParams{
			Name: "Personal " + input.Name,
			Type: constant.OrganizationTypePersonal,
		})
		if err != nil {
			return err
		}

		_, err = q.CreateOrganizationMember(ctx, repository.CreateOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         user.ID,
			Role:           constant.RoleOwner,
		})
		if err != nil {
			return err
		}

		_, err = q.SetLastActiveOrganization(ctx, repository.SetLastActiveOrganizationParams{
			ID:                       user.ID,
			LastActiveOrganizationID: &organization.ID,
		})
		if err != nil {
			return err
		}

		event := UserRegisteredEvent{
			UserID:         user.ID,
			OrganizationID: organization.ID,
			Email:          user.Email,
			Name:           user.Name,
			Role:           constant.RoleOwner,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		}

		err = createOutboxEvent(
			ctx,
			q,
			user.ID,
			EventUserRegistered,
			constant.TopicAudit,
			event,
		)
		if err != nil {
			return err
		}

		notificationPayload, err := json.Marshal(map[string]string{
			"name": user.Name,
		})
		if err != nil {
			return err
		}

		notificationEvent := notificationcontract.CreatedEvent{
			NotificationType: "user_registered",
			Category:         "transactional",
			RecipientID:      user.ID.String(),
			Targets: []notificationcontract.Target{
				{
					Channel:     notificationcontract.ChannelEmail,
					Destination: user.Email,
				},
			},
			Payload: notificationPayload,
		}

		if err := notificationEvent.Validate(); err != nil {
			return err
		}

		err = createOutboxEvent(
			ctx,
			q,
			user.ID,
			notificationcontract.EventTypeCreated,
			constant.TopicNotificationTransactional,
			notificationEvent,
		)
		if err != nil {
			return err
		}

		tokens, err = s.issueTokens(ctx, q, issueParams{
			UserID:         user.ID,
			OrganizationID: organization.ID,
			Role:           constant.RoleOwner,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		})
		return err
	})
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			return RegisterOutput{}, service.ErrEmailTaken
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "register transaction failed")
		s.log.ErrorContext(ctx, "register transaction failed", "error", err, "user_id", user.ID, "organization_id", organization.ID)
		return RegisterOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("user.id", user.ID.String()),
		attribute.String("organization.id", organization.ID.String()),
	)

	return RegisterOutput{
		User: UserOutput{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
		},
		Organization: OrganizationOutput{
			ID:   organization.ID,
			Name: organization.Name,
			Type: organization.Type,
			Role: constant.RoleOwner,
		},
		Tokens: TokensOutput(tokens),
	}, nil
}

func (s *authService) Login(ctx context.Context, input LoginInput) (LoginOutput, error) {
	ctx, span := tracer.Start(ctx, "AuthService.Login")
	defer span.End()

	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "invalid credentials")
			return LoginOutput{}, service.ErrUnauthenticated
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "query user by email failed")
		s.log.ErrorContext(ctx, "query user by email failed", "error", err)
		return LoginOutput{}, service.ErrInternal
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		span.SetStatus(codes.Error, "invalid credentials")
		return LoginOutput{}, service.ErrUnauthenticated
	}

	memberships, err := s.repo.ListUserOrganizations(ctx, user.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query user memberships failed")
		s.log.ErrorContext(ctx, "query user memberships failed", "error", err, "user_id", user.ID)
		return LoginOutput{}, service.ErrInternal
	}

	if len(memberships) == 0 {
		span.SetStatus(codes.Error, "user has no active organization")
		s.log.ErrorContext(ctx, "user has no active organization", "user_id", user.ID)
		return LoginOutput{}, service.ErrInternal
	}

	active := selectActiveOrganization(memberships, user.LastActiveOrganizationID)

	span.SetAttributes(
		attribute.String("user.id", user.ID.String()),
		attribute.String("organization.id", active.OrganizationID.String()),
	)

	var tokens tokens

	err = s.repo.ExecTx(ctx, func(querier repository.Querier) error {
		if user.LastActiveOrganizationID == nil || *user.LastActiveOrganizationID != active.OrganizationID {
			if _, err := querier.SetLastActiveOrganization(ctx, repository.SetLastActiveOrganizationParams{
				ID:                       user.ID,
				LastActiveOrganizationID: &active.OrganizationID,
			}); err != nil {
				return err
			}
		}

		tokens, err = s.issueTokens(ctx, querier, issueParams{
			UserID:         user.ID,
			OrganizationID: active.OrganizationID,
			Role:           active.Role,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		})
		if err != nil {
			return err
		}

		event := UserLoggedInEvent{
			UserID:         user.ID,
			OrganizationID: active.OrganizationID,
			Email:          user.Email,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		}

		err = createOutboxEvent(
			ctx,
			querier,
			user.ID,
			EventUserLoggedIn,
			constant.TopicAudit,
			event,
		)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "login transaction failed")
		s.log.ErrorContext(ctx, "login transaction failed", "error", err, "user_id", user.ID, "organization_id", active.OrganizationID)
		return LoginOutput{}, service.ErrInternal
	}

	return LoginOutput{
		User: UserOutput{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
		},
		Organization: OrganizationOutput{
			ID:   active.OrganizationID,
			Name: active.Name,
			Type: active.Type,
			Role: active.Role,
		},
		Tokens: TokensOutput(tokens),
	}, nil
}

func (s *authService) Me(ctx context.Context, input MeInput) (MeOutput, error) {
	ctx, span := tracer.Start(ctx, "AuthService.Me")
	defer span.End()

	user, err := s.repo.GetUserByID(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "user not found")
			return MeOutput{}, service.ErrUnauthenticated
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "query get user by id failed")
		s.log.ErrorContext(ctx, "query get user by id failed", "error", err, "user_id", input.UserID)
		return MeOutput{}, service.ErrInternal
	}

	listUserOrganization, err := s.repo.ListUserOrganizations(ctx, input.UserID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query list user organization failed")
		s.log.ErrorContext(ctx, "query list user organization failed", "error", err, "user_id", user.ID)
		return MeOutput{}, service.ErrInternal
	}

	span.SetAttributes(attribute.String("user.id", user.ID.String()))

	var organizationOutput []MeOrganizationOutput
	for _, organizationRow := range listUserOrganization {
		organizationOutput = append(organizationOutput, MeOrganizationOutput{
			ID:   organizationRow.OrganizationID,
			Name: organizationRow.Name,
			Type: organizationRow.Type,
			Role: organizationRow.Role,
		})
	}

	if user.LastActiveOrganizationID != nil {
		span.SetAttributes(attribute.String("organization.id", user.LastActiveOrganizationID.String()))
	}

	return MeOutput{
		User: MeUserOutput{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
		},
		ActiveOrganizationId: user.LastActiveOrganizationID,
		Organizations:        organizationOutput,
	}, nil
}

func (s *authService) Refresh(ctx context.Context, input RefreshInput) (RefreshOutput, error) {
	ctx, span := tracer.Start(ctx, "AuthService.Refresh")
	defer span.End()

	stored, err := s.repo.GetRefreshTokenByHash(ctx, platformjwt.HashRefreshToken(input.RefreshToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "refresh token not found")
			return RefreshOutput{}, service.ErrUnauthenticated
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "query refresh token failed")
		s.log.ErrorContext(ctx, "query refresh token failed", "error", err)
		return RefreshOutput{}, service.ErrInternal
	}

	if stored.RevokedAt.Valid {
		s.log.WarnContext(ctx, "revoked refresh token replayed, revoking every session", "user_id", stored.UserID)

		if _, err := s.repo.RevokeAllUserRefreshTokens(ctx, stored.UserID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "revoke all user refresh tokens failed")
			s.log.ErrorContext(ctx, "revoke all user refresh tokens failed", "error", err, "user_id", stored.UserID)
			return RefreshOutput{}, service.ErrInternal
		}

		span.SetStatus(codes.Error, "refresh token reuse detected")
		return RefreshOutput{}, service.ErrUnauthenticated
	}

	if !stored.ExpiresAt.After(time.Now()) {
		span.SetStatus(codes.Error, "refresh token expired")
		return RefreshOutput{}, service.ErrUnauthenticated
	}

	var tokens tokens

	err = s.repo.ExecTx(ctx, func(querier repository.Querier) error {
		user, err := querier.GetUserByID(ctx, stored.UserID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				span.SetStatus(codes.Error, "user no longer active")
				return service.ErrUnauthenticated
			}

			span.RecordError(err)
			span.SetStatus(codes.Error, "query user by id failed")
			s.log.ErrorContext(ctx, "query user by id failed", "error", err, "user_id", stored.UserID)
			return err
		}

		if user.LastActiveOrganizationID == nil {
			span.SetStatus(codes.Error, "user has no active organization")
			return service.ErrUnauthenticated
		}

		membership, err := querier.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
			UserID:         user.ID,
			OrganizationID: *user.LastActiveOrganizationID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				span.SetStatus(codes.Error, "membership no longer active")
				return service.ErrUnauthenticated
			}

			span.RecordError(err)
			span.SetStatus(codes.Error, "query user organization failed")
			s.log.ErrorContext(ctx, "query user organization failed", "error", err, "user_id", user.ID, "organization_id", *user.LastActiveOrganizationID)
			return err
		}

		span.SetAttributes(
			attribute.String("user.id", user.ID.String()),
			attribute.String("organization.id", membership.OrganizationID.String()),
		)

		revoked, err := querier.RevokeRefreshToken(ctx, stored.ID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "revoke refresh token failed")
			s.log.ErrorContext(ctx, "revoke refresh token failed", "error", err, "user_id", user.ID, "refresh_token_id", stored.ID)
			return err
		}

		if revoked == 0 {
			span.SetStatus(codes.Error, "refresh token revoked concurrently")
			return service.ErrUnauthenticated
		}

		tokens, err = s.issueTokens(ctx, querier, issueParams{
			UserID:         user.ID,
			OrganizationID: membership.OrganizationID,
			Role:           membership.Role,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "issue tokens failed")
			s.log.ErrorContext(ctx, "issue tokens failed", "error", err, "user_id", user.ID, "organization_id", membership.OrganizationID)
			return err
		}

		event := TokenRefreshedEvent{
			UserID:         user.ID,
			OrganizationID: membership.OrganizationID,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		}

		err = createOutboxEvent(
			ctx,
			querier,
			user.ID,
			EventTokenRefreshed,
			constant.TopicAudit,
			event,
		)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, service.ErrUnauthenticated) {
			return RefreshOutput{}, service.ErrUnauthenticated
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "refresh transaction failed")
		s.log.ErrorContext(ctx, "refresh transaction failed", "error", err, "user_id", stored.UserID)
		return RefreshOutput{}, service.ErrInternal
	}

	return RefreshOutput{Tokens: TokensOutput(tokens)}, nil
}

func (s *authService) issueTokens(ctx context.Context, querier repository.Querier, params issueParams) (tokens, error) {
	privateKey, kid, err := s.loadSigningKey(ctx, querier)
	if err != nil {
		return tokens{}, err
	}

	now := time.Now()

	accessToken, err := platformjwt.Sign(privateKey, kid, platformjwt.Claims{
		Subject:        params.UserID.String(),
		OrganizationID: params.OrganizationID.String(),
		Role:           params.Role,
		Issuer:         s.issuer,
		IssuedAt:       now,
		ExpiresAt:      now.Add(s.accessTokenTTL),
	})
	if err != nil {
		s.log.ErrorContext(ctx, "sign access token failed", "error", err, "user_id", params.UserID, "organization_id", params.OrganizationID)
		return tokens{}, service.ErrInternal
	}

	refreshToken, err := platformjwt.GenerateRefreshToken()
	if err != nil {
		s.log.ErrorContext(ctx, "generate refresh token failed", "error", err, "user_id", params.UserID)
		return tokens{}, service.ErrInternal
	}

	var ipAddress *netip.Addr
	if params.IPAddress != "" {
		if parsed, err := netip.ParseAddr(params.IPAddress); err == nil {
			ipAddress = &parsed
		}
	}

	var userAgent pgtype.Text
	if params.UserAgent != "" {
		userAgent = pgtype.Text{
			String: params.UserAgent,
			Valid:  true,
		}
	}

	if _, err := querier.CreateRefreshToken(ctx, repository.CreateRefreshTokenParams{
		UserID:    params.UserID,
		TokenHash: platformjwt.HashRefreshToken(refreshToken),
		UserAgent: userAgent,
		IpAddress: ipAddress,
		ExpiresAt: now.Add(s.refreshTokenTTL),
	}); err != nil {
		s.log.ErrorContext(ctx, "create refresh token failed", "error", err, "user_id", params.UserID, "organization_id", params.OrganizationID)
		return tokens{}, err
	}

	return tokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTokenTTL.Seconds()),
	}, nil
}

func (s *authService) loadSigningKey(ctx context.Context, querier repository.Querier) (*rsa.PrivateKey, string, error) {
	row, err := querier.GetActiveSigningKey(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.log.ErrorContext(ctx, "no active signing key: seed one with cmd/generate-signing-key")
			return nil, "", service.ErrInternal
		}

		s.log.ErrorContext(ctx, "load signing key failed", "error", err)
		return nil, "", service.ErrInternal
	}

	privateKeyPEM, err := crypto.DecryptPrivateKey(row.PrivateKeyEncrypted, s.masterKey)
	if err != nil {
		s.log.ErrorContext(ctx, "decrypt signing key failed", "error", err, "kid", row.Kid)
		return nil, "", service.ErrInternal
	}

	privateKey, err := platformjwt.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		s.log.ErrorContext(ctx, "parse signing key failed", "error", err, "kid", row.Kid)
		return nil, "", service.ErrInternal
	}

	return privateKey, row.Kid, nil
}

func selectActiveOrganization(memberships []repository.ListUserOrganizationsRow, preferred *uuid.UUID) repository.ListUserOrganizationsRow {
	if preferred != nil {
		for _, m := range memberships {
			if m.OrganizationID == *preferred {
				return m
			}
		}
	}
	return memberships[0]
}

func createOutboxEvent(
	ctx context.Context,
	q repository.Querier,
	aggregateID uuid.UUID,
	eventType string,
	topic string,
	event any,
) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	spanCtx := trace.SpanContextFromContext(ctx)
	var traceID pgtype.Text
	if spanCtx.IsValid() {
		traceID = pgtype.Text{
			String: spanCtx.TraceID().String(),
			Valid:  true,
		}
	}

	_, err = q.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
		AggregateType: authAggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		EventVersion:  authEventVersion,
		Topic:         topic,
		Payload:       payload,
		TraceID:       traceID,
	})
	return err
}
