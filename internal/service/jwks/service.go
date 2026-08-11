package jwks

import (
	"context"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/platform/jwks"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("service/jwks")

type JwksService interface {
	Jwks(ctx context.Context) ([]JwksKeyOutput, error)
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
