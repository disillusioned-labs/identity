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

	"github.com/disillusioned-labs/identity/internal/platform/crypto"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type IssueInput struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      string
	UserAgent string
	IPAddress string
}

type Service interface {
	Issue(ctx context.Context, input IssueInput) (AuthTokens, error)
}

type svc struct {
	repo            repository.Store
	masterKey       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	issuer          string
	log             *slog.Logger
	tracer          trace.Tracer
}

var _ Service = (*svc)(nil)

func New(
	repo repository.Store,
	masterKey []byte,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
	issuer string,
	log *slog.Logger,
) Service {
	return &svc{
		repo:            repo,
		masterKey:       masterKey,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
		issuer:          issuer,
		log:             log,
		tracer:          otel.Tracer("service/auth"),
	}
}

type jwtClaims struct {
	OrgID string `json:"org_id"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

func (s *svc) Issue(ctx context.Context, input IssueInput) (AuthTokens, error) {
	ctx, span := s.tracer.Start(ctx, "AuthService.Issue")
	defer span.End()

	privKey, kid, err := s.loadOrCreateSigningKey(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "load signing key failed")
		s.log.ErrorContext(ctx, "load signing key failed", "error", err)
		return AuthTokens{}, err
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
		return AuthTokens{}, service.ErrInternal
	}

	rawRefresh := make([]byte, 32)
	if _, err := rand.Read(rawRefresh); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate refresh token failed")
		s.log.ErrorContext(ctx, "generate refresh token failed", "error", err)
		return AuthTokens{}, service.ErrInternal
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

	_, err = s.repo.CreateRefreshToken(ctx, repository.CreateRefreshTokenParams{
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
		return AuthTokens{}, service.ErrInternal
	}

	return AuthTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTokenTTL.Seconds()),
	}, nil
}

func (s *svc) loadOrCreateSigningKey(ctx context.Context) (*rsa.PrivateKey, string, error) {
	row, err := s.repo.GetActiveSigningKey(ctx)
	if err == nil {
		privPEM, err := crypto.DecryptPrivateKey(row.PrivateKeyEncrypted, s.masterKey)
		if err != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, "decrypt signing key failed")
			s.log.ErrorContext(ctx, "decrypt signing key failed", "error", err)
			return nil, "", service.ErrInternal
		}
		privKey, err := parseRSAPrivateKey(privPEM)
		if err != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, "parse signing key failed")
			s.log.ErrorContext(ctx, "parse signing key failed", "error", err)
			return nil, "", service.ErrInternal
		}
		return privKey, row.Kid, nil
	}

	privPEM, pubPEM, err := crypto.GenerateKeyPair()
	if err != nil {
		span := trace.SpanFromContext(ctx)
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate key pair failed")
		s.log.ErrorContext(ctx, "generate key pair failed", "error", err)
		return nil, "", service.ErrInternal
	}

	encrypted, err := crypto.EncryptPrivateKey(privPEM, s.masterKey)
	if err != nil {
		span := trace.SpanFromContext(ctx)
		span.RecordError(err)
		span.SetStatus(codes.Error, "encrypt private key failed")
		s.log.ErrorContext(ctx, "encrypt private key failed", "error", err)
		return nil, "", service.ErrInternal
	}

	kid := uuid.New().String()
	if err := s.repo.InsertSigningKey(ctx, repository.InsertSigningKeyParams{
		Kid:                 kid,
		PrivateKeyEncrypted: encrypted,
		PublicKey:           string(pubPEM),
		Algorithm:           "RS256",
		IsActive:            true,
	}); err != nil {
		span := trace.SpanFromContext(ctx)
		span.RecordError(err)
		span.SetStatus(codes.Error, "persist signing key failed")
		s.log.ErrorContext(ctx, "persist signing key failed", "error", err)
		return nil, "", service.ErrInternal
	}

	privKey, err := parseRSAPrivateKey(privPEM)
	if err != nil {
		span := trace.SpanFromContext(ctx)
		span.RecordError(err)
		span.SetStatus(codes.Error, "parse signing key failed")
		s.log.ErrorContext(ctx, "parse signing key failed", "error", err)
		return nil, "", service.ErrInternal
	}
	return privKey, kid, nil
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
