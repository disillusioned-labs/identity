package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/disillusioned-labs/identity/internal/config"
	"github.com/disillusioned-labs/identity/internal/server"
	"github.com/disillusioned-labs/platform/cache"
	platformconfig "github.com/disillusioned-labs/platform/config"
	"github.com/disillusioned-labs/platform/postgres"
	"github.com/disillusioned-labs/platform/redis"
	"github.com/disillusioned-labs/platform/telemetry"

	migrations "github.com/disillusioned-labs/identity/db/migrations"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

const grpcOtelFlushTimeout = 5 * time.Second

// RunGRPC boots the gRPC server with the given configuration and blocks until
// the process is told to stop. The caller owns loading and validating cfg
// (see cmd/grpc).
func RunGRPC(cfg *config.Config) error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	log := telemetry.NewLogger(
		cfg.Log.Level,
		telemetry.Format(cfg.Log.Format),
		telemetry.Env(cfg.Service.Env),
		telemetry.Service(cfg.Service.Name),
	)
	slog.SetDefault(log)

	log.Info(
		"starting",
		"service", cfg.Service.Name,
		"build", buildInfo(),
		"transport", "grpc",
	)

	// -------------------------------------------------------------------------
	// Telemetry
	// -------------------------------------------------------------------------
	otelOpts := []telemetry.Option{
		telemetry.WithBuild(version, commit),
	}

	if cfg.OTel.TracesEnabled() {
		sampler, err := telemetry.NewSampler(
			cfg.OTel.TracesSampler,
			cfg.OTel.TracesSamplerArg,
		)
		if err != nil {
			return fmt.Errorf("configure trace sampler: %w", err)
		}

		otelOpts = append(
			otelOpts,
			telemetry.WithTracing(
				cfg.OTel.TraceEndpoint(),
				sampler,
			),
		)
	}

	if cfg.OTel.MetricsEnabled() {
		otelOpts = append(
			otelOpts,
			telemetry.WithMetrics(
				cfg.OTel.MetricEndpoint(),
				cfg.OTel.MetricExportInterval(),
			),
		)
	}

	shutdownOtel, err := telemetry.Setup(
		ctx,
		cfg.Service.Name,
		cfg.Service.Env,
		otelOpts...,
	)
	if err != nil {
		return fmt.Errorf("setup telemetry: %w", err)
	}

	log.Info(
		"telemetry configured",
		"traces", grpcExportTarget(
			cfg.OTel.TracesEnabled(),
			cfg.OTel.TraceEndpoint(),
		),
		"metrics", grpcExportTarget(
			cfg.OTel.MetricsEnabled(),
			cfg.OTel.MetricEndpoint(),
		),
		"metric_export_interval", cfg.OTel.MetricExportInterval(),
	)

	defer func() {
		flushCtx, cancel := context.WithTimeout(
			context.Background(),
			grpcOtelFlushTimeout,
		)
		defer cancel()

		if err := shutdownOtel(flushCtx); err != nil {
			log.Error("otel shutdown failed", "error", err)
		}
	}()

	// -------------------------------------------------------------------------
	// PostgreSQL
	// -------------------------------------------------------------------------
	pool, err := postgres.NewPool(
		ctx,
		cfg.Postgres.DSN,
		postgres.MaxConns(cfg.Postgres.MaxConns),
		postgres.MinConns(cfg.Postgres.MinConns),
		postgres.MaxConnLifetime(cfg.Postgres.MaxConnLifetime),
		postgres.QueryExecMode(cfg.Postgres.QueryExecMode),
	)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	log.Info("connected to postgres", "postgres", cfg.Postgres)

	if cfg.Postgres.Migrate {
		if err := postgres.Migrate(
			ctx,
			pool,
			migrations.FS,
			log,
		); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	// -------------------------------------------------------------------------
	// Redis
	// -------------------------------------------------------------------------
	rdb, svcCache, closeRedis, err := grpcSetupRedis(
		ctx,
		cfg,
		log,
	)
	if err != nil {
		return err
	}
	defer closeRedis()

	redisRequired := cfg.Redis.Mode == platformconfig.RedisModeRequired

	// -------------------------------------------------------------------------
	// Dependencies
	// -------------------------------------------------------------------------
	// buildDeps wires the member service, whose removal flow calls expense
	// over gRPC (decision D2). Unused on this binary's own RPCs but kept
	// wired so both entry points build the same dependency graph.
	expenseClient, closeExpense, err := newExpenseClient(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer closeExpense()

	deps, err := buildDeps(
		pool,
		rdb,
		redisRequired,
		svcCache,
		cfg.Auth,
		expenseClient,
		log,
	)
	if err != nil {
		return fmt.Errorf("build dependencies: %w", err)
	}

	// -------------------------------------------------------------------------
	// gRPC server
	// -------------------------------------------------------------------------
	grpcSrv, err := server.NewGRPC(
		cfg,
		log,
		deps,
	)
	if err != nil {
		return fmt.Errorf("create grpc server: %w", err)
	}

	// -------------------------------------------------------------------------
	// pprof
	// -------------------------------------------------------------------------
	//
	// pprof is an independent HTTP debug listener. It does not share the gRPC
	// listener or port. Keep it disabled by default and expose it only through
	// a private/local interface.
	pprofSrv := server.NewPprofServer(
		cfg.Pprof.Enabled,
		cfg.Pprof.Port,
		log,
	)

	// -------------------------------------------------------------------------
	// Serve
	// -------------------------------------------------------------------------
	g, runCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := grpcSrv.Start(); err != nil {
			return fmt.Errorf("grpc listener: %w", err)
		}

		return nil
	})

	if pprofSrv != nil {
		g.Go(func() error {
			if err := pprofSrv.ListenAndServe(); err != nil &&
				!errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("pprof listener: %w", err)
			}

			return nil
		})
	}

	<-runCtx.Done()

	signalled := ctx.Err() != nil

	log.Info(
		"shutdown initiated",
		"cause", grpcShutdownCause(signalled),
	)

	// Restore default signal handling now that shutdown has begun.
	stop()

	// -------------------------------------------------------------------------
	// Graceful shutdown
	// -------------------------------------------------------------------------
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.Server.ShutdownTimeout,
	)
	defer cancel()

	// Mark gRPC as NOT_SERVING before draining active RPCs so service
	// discovery/load balancers can stop sending new traffic.
	grpcSrv.BeginDrain()

	// Drain active gRPC RPCs until the shutdown deadline expires.
	if err := grpcSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("grpc graceful shutdown: %w", err)
	}

	// pprof is only a diagnostic listener, so it does not need an application-
	// level drain. It should simply stop accepting new requests.
	if pprofSrv != nil {
		if err := pprofSrv.Shutdown(shutdownCtx); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("pprof shutdown: %w", err)
		}
	}

	// Surface listener failures such as "address already in use".
	if err := g.Wait(); err != nil {
		return err
	}

	log.Info("shutdown complete")

	return nil
}

func grpcExportTarget(enabled bool, endpoint string) string {
	if !enabled {
		return "disabled"
	}

	return endpoint
}

func grpcShutdownCause(signalled bool) string {
	if signalled {
		return "signal"
	}

	return "listener failure"
}

// grpcSetupRedis honors redis.mode: absent in disabled mode, fatal in required
// mode, best-effort in optional mode. The returned cache stays a nil interface
// (not a typed-nil *cache.Cache) when Redis is unavailable, so the services'
// nil checks keep working.
func grpcSetupRedis(
	ctx context.Context,
	cfg *config.Config,
	log *slog.Logger,
) (
	*goredis.Client,
	cache.Cache,
	func(),
	error,
) {
	noop := func() {}

	if cfg.Redis.Mode == platformconfig.RedisModeDisabled {
		log.Info("redis disabled, running without cache")
		return nil, nil, noop, nil
	}

	client, err := redis.New(
		ctx,
		cfg.Redis.Addr,
		redis.Password(cfg.Redis.Password),
		redis.DB(cfg.Redis.DB),
	)
	if err != nil {
		if cfg.Redis.Mode == platformconfig.RedisModeRequired {
			return nil, nil, noop, fmt.Errorf(
				"connect redis (required): %w",
				err,
			)
		}

		log.Warn(
			"redis unreachable, running without cache",
			"error", err,
		)

		return nil, nil, noop, nil
	}

	log.Info("connected to redis", "redis", cfg.Redis)

	return client,
		cache.New(client, cfg.Cache.DefaultTTL),
		func() { _ = client.Close() },
		nil
}
