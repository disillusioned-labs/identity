package app

import (
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/disillusioned-labs/identity/internal/config"
	"github.com/disillusioned-labs/identity/internal/platform/cache"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/server"
	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
)

func buildDeps(pool *pgxpool.Pool, rdb *goredis.Client, redisRequired bool, _ cache.Cache, authCfg config.AuthConfig, log *slog.Logger) (server.Deps, error) {
	repo := repository.NewStore(pool)

	masterKey, err := authCfg.MasterKeyBytes()
	if err != nil {
		return server.Deps{}, fmt.Errorf("decode auth master key: %w", err)
	}

	auth := authservice.NewAuthService(
		repo,
		masterKey,
		authCfg.AccessTokenTTL,
		authCfg.RefreshTokenTTL,
		authCfg.Issuer,
		log,
	)

	return server.Deps{
		Auth:          auth,
		Pool:          pool,
		Redis:         rdb,
		RedisRequired: redisRequired,
	}, nil
}
