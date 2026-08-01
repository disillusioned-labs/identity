package app

import (
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/platform/cache"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/server"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	usersvc "github.com/disillusioned-labs/identity/internal/service/user"
)

// buildDeps wires repositories → services into server dependencies. Adding a
// resource means adding its construction here and a field in server.Deps.
func buildDeps(pool *pgxpool.Pool, rdb *goredis.Client, redisRequired bool, svcCache cache.Cache, log *slog.Logger) server.Deps {
	repo := repository.NewStore(pool)

	return server.Deps{
		Users:         usersvc.New(repo, svcCache, log),
		Pool:          pool,
		Redis:         rdb,
		RedisRequired: redisRequired,
	}
}
