package app

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/disillusioned-labs/authkit"
	"github.com/disillusioned-labs/identity/internal/handler"
	jwkservice "github.com/disillusioned-labs/identity/internal/service/jwks"
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
	organizationinvitationservice "github.com/disillusioned-labs/identity/internal/service/organization_invitation"
	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/disillusioned-labs/identity/internal/config"
	"github.com/disillusioned-labs/identity/internal/platform/cache"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/server"
	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
)

type jwksKeySource struct {
	service jwkservice.JwksService
}

func (s jwksKeySource) Fetch(ctx context.Context) (map[string]*rsa.PublicKey, []byte, error) {
	keys, err := s.service.PublicKeys(ctx)
	if err != nil {
		return nil, nil, err
	}

	return keys, nil, nil
}

func buildDeps(pool *pgxpool.Pool, rdb *goredis.Client, redisRequired bool, _ cache.Cache, authCfg config.AuthConfig, log *slog.Logger) (server.Deps, error) {
	repo := repository.NewStore(pool)

	masterKey, err := authCfg.MasterKeyBytes()
	if err != nil {
		return server.Deps{}, fmt.Errorf("decode auth master key: %w", err)
	}

	authErrorHandler := func(
		w http.ResponseWriter,
		_ *http.Request,
		_ error,
	) {
		handler.WriteError(
			w,
			http.StatusUnauthorized,
			handler.CodeUnauthorized,
			"unauthorized",
		)
	}

	auth := authservice.NewAuthService(
		repo,
		masterKey,
		authCfg.AccessTokenTTL,
		authCfg.RefreshTokenTTL,
		authCfg.Issuer,
		log,
	)

	jwksService := jwkservice.NewJwksService(
		repo,
		log,
	)

	organizationService := organizationservice.NewOrganizationService(
		repo,
		log,
	)

	organizationMemberService := organizationmemberservice.NewOrganizationMemberService(
		repo,
		log,
	)

	organizationInvitationService := organizationinvitationservice.NewOrganizationInvitationService(
		repo,
		log,
	)

	verifier := authkit.New(
		authkit.Config{
			Issuer: authCfg.Issuer,
		},
		authkit.WithKeySource(jwksKeySource{
			service: jwksService,
		}),
		authkit.WithErrorHandler(authErrorHandler),
		authkit.WithLogger(log),
	)

	return server.Deps{
		AuthService:                   auth,
		JwksService:                   jwksService,
		OrganizationService:           organizationService,
		OrganizationMemberService:     organizationMemberService,
		OrganizationInvitationService: organizationInvitationService,
		Verifier:                      verifier,
		Pool:                          pool,
		Redis:                         rdb,
		RedisRequired:                 redisRequired,
	}, nil
}
