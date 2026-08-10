package auth

import (
	"context"
	"crypto/rsa"
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
	"github.com/disillusioned-labs/identity/internal/platform/crypto"
	platformjwt "github.com/disillusioned-labs/identity/internal/platform/jwt"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
)

type AuthService interface {
	Register(ctx context.Context, input RegisterInput) (RegisterOutput, error)
	Login(ctx context.Context, input LoginInput) (LoginOutput, error)
	Refresh(ctx context.Context, input RefreshInput) (RefreshOutput, error)
}

type authService struct {
	repo            repository.Store
	masterKey       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	issuer          string
	log             *slog.Logger
	tracer          trace.Tracer
}

func NewAuthService(
	repo repository.Store,
	masterKey []byte,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
	issuer string,
	log *slog.Logger,
) AuthService {
	return &authService{
		repo:            repo,
		masterKey:       masterKey,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
		issuer:          issuer,
		log:             log,
		tracer:          otel.Tracer("service/auth"),
	}
}

func (s *authService) Register(ctx context.Context, input RegisterInput) (RegisterOutput, error) {
	ctx, span := s.tracer.Start(ctx, "AuthService.Register")
	defer span.End()

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

	err = s.repo.ExecTx(ctx, func(querier repository.Querier) error {
		user, err = querier.CreateUser(ctx, repository.CreateUserParams{
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

		organization, err = querier.CreateOrganization(ctx, repository.CreateOrganizationParams{
			Name: "Personal " + input.Name,
			Type: constant.OrganizationTypePersonal,
		})
		if err != nil {
			return err
		}

		if _, err := querier.CreateOrganizationMember(ctx, repository.CreateOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         user.ID,
			Role:           constant.RoleOwner,
		}); err != nil {
			return err
		}

		if _, err := querier.SetLastActiveOrganization(ctx, repository.SetLastActiveOrganizationParams{
			ID:                       user.ID,
			LastActiveOrganizationID: &organization.ID,
		}); err != nil {
			return err
		}

		tokens, err = s.issueTokens(ctx, querier, issueParams{
			UserID:         user.ID,
			OrganizationID: organization.ID,
			Role:           constant.RoleOwner,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		})
		return err
	})
	if err != nil {
		return RegisterOutput{}, s.txError(ctx, span, err, "register transaction failed")
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
	ctx, span := s.tracer.Start(ctx, "AuthService.Login")
	defer span.End()

	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "no active user with that email")
			return LoginOutput{}, service.ErrUnauthenticated
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "query user by email failed")
		s.log.ErrorContext(ctx, "query user by email failed", "error", err)
		return LoginOutput{}, service.ErrInternal
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		span.SetStatus(codes.Error, "password mismatch")
		return LoginOutput{}, service.ErrUnauthenticated
	}

	memberships, err := s.repo.ListUserMemberships(ctx, user.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query user memberships failed")
		s.log.ErrorContext(ctx, "query user memberships failed", "error", err)
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

		var err error
		tokens, err = s.issueTokens(ctx, querier, issueParams{
			UserID:         user.ID,
			OrganizationID: active.OrganizationID,
			Role:           active.Role,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		})
		return err
	})
	if err != nil {
		return LoginOutput{}, s.txError(ctx, span, err, "login transaction failed")
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

func (s *authService) Refresh(ctx context.Context, input RefreshInput) (RefreshOutput, error) {
	ctx, span := s.tracer.Start(ctx, "AuthService.Refresh")
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
		s.log.WarnContext(ctx, "revoked refresh token replayed, revoking every session",
			"user_id", stored.UserID)
		if _, err := s.repo.RevokeAllUserRefreshTokens(ctx, stored.UserID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "revoke all user refresh tokens failed")
			s.log.ErrorContext(ctx, "revoke all user refresh tokens failed", "error", err)
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
			return err
		}
		if user.LastActiveOrganizationID == nil {
			span.SetStatus(codes.Error, "user has no active organization")
			return service.ErrUnauthenticated
		}

		membership, err := querier.GetMembership(ctx, repository.GetMembershipParams{
			UserID:         user.ID,
			OrganizationID: *user.LastActiveOrganizationID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				span.SetStatus(codes.Error, "membership no longer active")
				return service.ErrUnauthenticated
			}
			return err
		}

		revoked, err := querier.RevokeRefreshToken(ctx, stored.ID)
		if err != nil {
			return err
		}
		if revoked == 0 {
			span.SetStatus(codes.Error, "refresh token revoked concurrently")
			return service.ErrUnauthenticated
		}

		span.SetAttributes(
			attribute.String("user.id", user.ID.String()),
			attribute.String("organization.id", membership.OrganizationID.String()),
		)

		tokens, err = s.issueTokens(ctx, querier, issueParams{
			UserID:         user.ID,
			OrganizationID: membership.OrganizationID,
			Role:           membership.Role,
			UserAgent:      input.UserAgent,
			IPAddress:      input.IPAddress,
		})
		return err
	})
	if err != nil {
		return RefreshOutput{}, s.txError(ctx, span, err, "refresh transaction failed")
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
		s.log.ErrorContext(ctx, "sign access token failed", "error", err)
		return tokens{}, service.ErrInternal
	}

	refreshToken, err := platformjwt.GenerateRefreshToken()
	if err != nil {
		s.log.ErrorContext(ctx, "generate refresh token failed", "error", err)
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
		userAgent = pgtype.Text{String: params.UserAgent, Valid: true}
	}

	if _, err := querier.CreateRefreshToken(ctx, repository.CreateRefreshTokenParams{
		UserID:    params.UserID,
		TokenHash: platformjwt.HashRefreshToken(refreshToken),
		UserAgent: userAgent,
		IpAddress: ipAddress,
		ExpiresAt: now.Add(s.refreshTokenTTL),
	}); err != nil {
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
		s.log.ErrorContext(ctx, "decrypt signing key failed", "error", err)
		return nil, "", service.ErrInternal
	}

	privateKey, err := platformjwt.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		s.log.ErrorContext(ctx, "parse signing key failed", "error", err)
		return nil, "", service.ErrInternal
	}

	return privateKey, row.Kid, nil
}

func (s *authService) txError(ctx context.Context, span trace.Span, err error, msg string) error {
	span.RecordError(err)
	if !service.IsError(err) {
		span.SetStatus(codes.Error, msg)
		s.log.ErrorContext(ctx, msg, "error", err)
		return service.ErrInternal
	}
	return err
}

func selectActiveOrganization(
	memberships []repository.ListUserMembershipsRow,
	preferred *uuid.UUID,
) repository.ListUserMembershipsRow {
	if preferred != nil {
		for _, m := range memberships {
			if m.OrganizationID == *preferred {
				return m
			}
		}
	}
	return memberships[0]
}
