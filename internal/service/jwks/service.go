package jwks

import (
	"context"
	"crypto/rsa"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/disillusioned-labs/platform/jwks"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("service/jwks")

type JwksService interface {
	Jwks(ctx context.Context) ([]JwksKeyOutput, error)
	PublicKeys(ctx context.Context) (map[string]*rsa.PublicKey, error)
}

type jwksService struct {
	repo repository.Store
	log  *slog.Logger
}

func NewJwksService(
	repo repository.Store,
	log *slog.Logger) JwksService {
	return &jwksService{
		repo: repo,
		log:  log,
	}
}

func (s *jwksService) Jwks(ctx context.Context) ([]JwksKeyOutput, error) {
	ctx, span := tracer.Start(ctx, "JwksService.JwksService")
	defer span.End()

	listActiveSigningKeys, err := s.repo.ListActiveSigningKeys(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query list active signing keys failed")
		s.log.ErrorContext(ctx, "query list active signing keys failed", "error", err)
		return nil, service.ErrInternal
	}

	var jwksOutput []JwksKeyOutput

	for _, signingKey := range listActiveSigningKeys {

		jwkKey, err := jwks.PublicKeyToJWKS(signingKey.PublicKey, signingKey.Kid)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "public key to jwks failed")
			s.log.ErrorContext(ctx, "public key to jwks failed", "error", err)
			return nil, service.ErrInternal
		}

		jwksDetailOutput := JwksKeyOutput{
			Kid: jwkKey.Kid,
			Kty: jwkKey.Kty,
			Alg: jwkKey.Alg,
			Use: jwkKey.Use,
			N:   jwkKey.N,
			E:   jwkKey.E,
		}
		jwksOutput = append(jwksOutput, jwksDetailOutput)
	}

	return jwksOutput, nil
}

func (s *jwksService) PublicKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	ctx, span := tracer.Start(ctx, "JwksService.PublicKeys")
	defer span.End()

	signingKeys, err := s.repo.ListActiveSigningKeys(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query list active signing keys failed")

		s.log.ErrorContext(
			ctx,
			"query list active signing keys failed",
			"error",
			err,
		)

		return nil, service.ErrInternal
	}

	keys := make(map[string]*rsa.PublicKey, len(signingKeys))

	for _, signingKey := range signingKeys {
		publicKey, err := jwks.ParseRSAPublicKey(signingKey.PublicKey)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "parse RSA public key failed")

			s.log.ErrorContext(
				ctx,
				"parse RSA public key failed",
				"kid", signingKey.Kid,
				"error", err,
			)

			return nil, service.ErrInternal
		}

		keys[signingKey.Kid] = publicKey
	}

	return keys, nil
}
