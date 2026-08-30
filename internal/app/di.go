package app

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/disillusioned-labs/platform/authkit"
	"github.com/disillusioned-labs/identity/internal/handler"
	jwkservice "github.com/disillusioned-labs/identity/internal/service/jwks"
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
	organizationinvitationservice "github.com/disillusioned-labs/identity/internal/service/organization_invitation"
	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/disillusioned-labs/identity/internal/config"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/server"
	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
	"github.com/disillusioned-labs/platform/cache"
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

func buildDeps(pool *pgxpool.Pool, rdb *goredis.Client, redisRequired bool, cache cache.Cache, authCfg config.AuthConfig, log *slog.Logger) (server.Deps, error) {
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

	var (
		revocationStore *authservice.RevocationStore
		verifierOptions []authkit.Option
	)
	if rdb != nil {
		revocationStore = authservice.NewRevocationStore(rdb, authCfg.AccessTokenTTL)
		verifierOptions = append(verifierOptions, authkit.WithDenylist(revocationStore))
	}

	auth := authservice.NewAuthService(
		repo,
		masterKey,
		authCfg.AccessTokenTTL,
		authCfg.RefreshTokenTTL,
		authCfg.Issuer,
		revocationStore,
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
		revocationStore,
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
		append([]authkit.Option{
			authkit.WithKeySource(jwksKeySource{
				service: jwksService,
			}),
			authkit.WithErrorHandler(authErrorHandler),
			authkit.WithLogger(log),
		}, verifierOptions...)...,
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
		Cache:                         cache,
	}, nil
}
