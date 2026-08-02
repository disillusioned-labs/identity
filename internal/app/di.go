package app

import (
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/config"
	"github.com/disillusioned-labs/identity/internal/platform/cache"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/server"
	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
	jwtservice "github.com/disillusioned-labs/identity/internal/service/jwt"
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
	userservice "github.com/disillusioned-labs/identity/internal/service/user"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

func buildDeps(pool *pgxpool.Pool, rdb *goredis.Client, redisRequired bool, svcCache cache.Cache, authCfg config.AuthConfig, log *slog.Logger) server.Deps {
	repo := repository.NewStore(pool)

	masterKey, _ := authCfg.MasterKeyBytes()

	jwt := jwtservice.New(
		repo,
		masterKey,
		authCfg.AccessTokenTTL,
		authCfg.RefreshTokenTTL,
		authCfg.Issuer,
		log,
	)

	users := userservice.New(log)
	orgs := organizationservice.New(log)
	members := organizationmemberservice.New(log)

	auth := authservice.New(repo, users, orgs, members, jwt, log)

	return server.Deps{
		Auth:          auth,
		Pool:          pool,
		Redis:         rdb,
		RedisRequired: redisRequired,
	}
}
