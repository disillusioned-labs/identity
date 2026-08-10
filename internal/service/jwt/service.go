package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"log/slog"
	"net/netip"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/disillusioned-labs/identity/internal/platform/crypto"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
)

type JWTService interface {
	Issue(ctx context.Context, querier repository.Querier, input IssueInput) (IssueOutput, error)
	LookupRefreshToken(ctx context.Context, querier repository.Querier, input LookupRefreshTokenInput) (LookupRefreshTokenOutput, error)
	RevokeRefreshToken(ctx context.Context, querier repository.Querier, input RevokeRefreshTokenInput) error
	RevokeAllUserRefreshTokens(ctx context.Context, querier repository.Querier, input RevokeAllUserRefreshTokensInput) error
}

type jwtService struct {
	masterKey       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	issuer          string
	log             *slog.Logger
	tracer          trace.Tracer
}

func NewJWTService(
	masterKey []byte,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
	issuer string,
	log *slog.Logger,
) JWTService {
	return &jwtService{
		masterKey:       masterKey,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
		issuer:          issuer,
		log:             log,
		tracer:          otel.Tracer("service/jwt"),
	}
}

type jwtClaims struct {
	OrgID string `json:"org_id"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

func (s *jwtService) Issue(ctx context.Context, querier repository.Querier, input IssueInput) (IssueOutput, error) {
	ctx, span := s.tracer.Start(ctx, "JWTService.Issue")
	defer span.End()

	privKey, kid, err := s.loadSigningKey(ctx, querier)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "load signing key failed")
		s.log.ErrorContext(ctx, "load signing key failed", "error", err)
		return IssueOutput{}, err
	}

	now := time.Now()
	expiresAt := now.Add(s.accessTokenTTL)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtClaims{
		OrgID: input.OrgID.String(),
		Role:  input.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   input.UserID.String(),
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})
	token.Header["kid"] = kid

	accessToken, err := token.SignedString(privKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "sign token failed")
		s.log.ErrorContext(ctx, "sign token failed", "error", err)
		return IssueOutput{}, service.ErrInternal
	}

	rawRefresh := make([]byte, 32)
	if _, err := rand.Read(rawRefresh); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate refresh token failed")
		s.log.ErrorContext(ctx, "generate refresh token failed", "error", err)
		return IssueOutput{}, service.ErrInternal
	}
	refreshToken := hex.EncodeToString(rawRefresh)

	var ipAddr *netip.Addr
	if input.IPAddress != "" {
		if parsed, err := netip.ParseAddr(input.IPAddress); err == nil {
			ipAddr = &parsed
		}
	}

	var ua pgtype.Text
	if input.UserAgent != "" {
		ua = pgtype.Text{String: input.UserAgent, Valid: true}
	}

	_, err = querier.CreateRefreshToken(ctx, repository.CreateRefreshTokenParams{
		UserID:    input.UserID,
		TokenHash: hashToken(refreshToken),
		UserAgent: ua,
		IpAddress: ipAddr,
		ExpiresAt: now.Add(s.refreshTokenTTL),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "store refresh token failed")
		s.log.ErrorContext(ctx, "store refresh token failed", "error", err)
		return IssueOutput{}, service.ErrInternal
	}

	return IssueOutput{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTokenTTL.Seconds()),
	}, nil
}

func (s *jwtService) LookupRefreshToken(ctx context.Context, querier repository.Querier, input LookupRefreshTokenInput) (LookupRefreshTokenOutput, error) {
	ctx, span := s.tracer.Start(ctx, "JWTService.LookupRefreshToken")
	defer span.End()

	row, err := querier.GetRefreshTokenByHash(ctx, hashToken(input.RefreshToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "refresh token not found")
			return LookupRefreshTokenOutput{}, service.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "lookup refresh token failed")
		s.log.ErrorContext(ctx, "lookup refresh token failed", "error", err)
		return LookupRefreshTokenOutput{}, service.ErrInternal
	}

	out := LookupRefreshTokenOutput{
		ID:        row.ID,
		UserID:    row.UserID,
		ExpiresAt: row.ExpiresAt,
	}
	if row.RevokedAt.Valid {
		revokedAt := row.RevokedAt.Time
		out.RevokedAt = &revokedAt
	}
	return out, nil
}

func (s *jwtService) RevokeRefreshToken(ctx context.Context, querier repository.Querier, input RevokeRefreshTokenInput) error {
	ctx, span := s.tracer.Start(ctx, "JWTService.RevokeRefreshToken")
	defer span.End()

	rows, err := querier.RevokeRefreshToken(ctx, input.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke refresh token failed")
		s.log.ErrorContext(ctx, "revoke refresh token failed", "error", err)
		return service.ErrInternal
	}
	if rows == 0 {
		span.SetStatus(codes.Error, "refresh token already revoked")
		return service.ErrNotFound
	}
	return nil
}

func (s *jwtService) RevokeAllUserRefreshTokens(ctx context.Context, querier repository.Querier, input RevokeAllUserRefreshTokensInput) error {
	ctx, span := s.tracer.Start(ctx, "JWTService.RevokeAllUserRefreshTokens")
	defer span.End()

	if _, err := querier.RevokeAllUserRefreshTokens(ctx, input.UserID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke all user refresh tokens failed")
		s.log.ErrorContext(ctx, "revoke all user refresh tokens failed", "error", err)
		return service.ErrInternal
	}
	return nil
}

func (s *jwtService) loadSigningKey(ctx context.Context, querier repository.Querier) (*rsa.PrivateKey, string, error) {
	span := trace.SpanFromContext(ctx)

	row, err := querier.GetActiveSigningKey(ctx)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "no active signing key")
			s.log.ErrorContext(ctx, "no active signing key: seed one with cmd/generate-signing-key")
			return nil, "", service.ErrInternal
		}
		span.SetStatus(codes.Error, "load signing key failed")
		s.log.ErrorContext(ctx, "load signing key failed", "error", err)
		return nil, "", service.ErrInternal
	}

	privPEM, err := crypto.DecryptPrivateKey(row.PrivateKeyEncrypted, s.masterKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "decrypt signing key failed")
		s.log.ErrorContext(ctx, "decrypt signing key failed", "error", err)
		return nil, "", service.ErrInternal
	}

	privKey, err := parseRSAPrivateKey(privPEM)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "parse signing key failed")
		s.log.ErrorContext(ctx, "parse signing key failed", "error", err)
		return nil, "", service.ErrInternal
	}

	return privKey, row.Kid, nil
}

func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
