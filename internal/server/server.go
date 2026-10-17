// Package server assembles the chi router, middleware chain, and http.Server.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/disillusioned-labs/identity/internal/config"
	"github.com/disillusioned-labs/identity/internal/handler"
	authhandler "github.com/disillusioned-labs/identity/internal/handler/auth"
	devicehandler "github.com/disillusioned-labs/identity/internal/handler/device"
	"github.com/disillusioned-labs/identity/internal/handler/health"
	jwkshandler "github.com/disillusioned-labs/identity/internal/handler/jwks"
	organizationhandler "github.com/disillusioned-labs/identity/internal/handler/organization"
	"github.com/disillusioned-labs/identity/internal/handler/organization_invitation"
	organizationmemberhandler "github.com/disillusioned-labs/identity/internal/handler/organization_member"
	serviceaccesshandler "github.com/disillusioned-labs/identity/internal/handler/service_access"
	deviceservice "github.com/disillusioned-labs/identity/internal/service/device"
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
	organizationinvitationservice "github.com/disillusioned-labs/identity/internal/service/organization_invitation"
	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
	serviceservice "github.com/disillusioned-labs/identity/internal/service/service_access"
	"github.com/disillusioned-labs/platform/authkit"
	"github.com/disillusioned-labs/platform/cache"
	"github.com/disillusioned-labs/platform/httpserver"
	"github.com/disillusioned-labs/platform/ratelimit"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
	jwksservice "github.com/disillusioned-labs/identity/internal/service/jwks"
)

// Server wraps the http.Server with the assembled router and logger.
type Server struct {
	http   *httpserver.Server
	health *health.Handler
}

// keyByRemoteIP buckets rate-limit counters by r.RemoteAddr, the unspoofable
// TCP peer (see the RealIP note in New). On the rare parse failure we fall
// back to the raw value so limiting still applies.
func keyByRemoteIP(r *http.Request) (string, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr, nil
	}
	return host, nil
}

// redisPinger adapts *goredis.Client to health.Pinger.
type redisPinger struct{ rdb *goredis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.rdb.Ping(ctx).Err() }

// Deps carries everything the router needs. Adding a resource adds a field
// here; New's signature never changes.
type Deps struct {
	AuthService                   authservice.AuthService
	JwksService                   jwksservice.JwksService
	OrganizationService           organizationservice.OrganizationService
	OrganizationMemberService     organizationmemberservice.OrganizationMemberService
	OrganizationInvitationService organizationinvitationservice.OrganizationInvitationService
	ServiceAccessService          serviceservice.ServiceAccessService
	DeviceService                 deviceservice.DeviceService

	Verifier       *authkit.Verifier
	Pool           *pgxpool.Pool
	Redis          *goredis.Client
	RateLimitRedis *goredis.Client
	RedisRequired  bool
	Cache          cache.Cache
}

func rateLimitOptions(client *goredis.Client, prefix string, log *slog.Logger) []httprate.Option {
	if client == nil {
		return nil
	}
	return []httprate.Option{
		httprate.WithLimitCounter(ratelimit.RedisCounter(client, prefix)),
		httprate.WithErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
			log.ErrorContext(r.Context(), "distributed rate limit failed closed", "prefix", prefix, "error", err)
			handler.WriteError(w, http.StatusServiceUnavailable, handler.CodeRateLimitUnavailable, "rate limiting is temporarily unavailable")
		}),
	}
}

// New assembles the router - middleware chain, probes, /metrics, and every
// /api/v1 resource - into a ready-to-start Server.
func New(cfg *config.Config, log *slog.Logger, deps Deps) (*Server, error) {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	// No RealIP middleware on purpose: it trusts X-Forwarded-For / X-Real-IP
	// unconditionally, letting any client spoof its address (and evade the
	// rate limiter). Deployments behind a trusted proxy should add their own
	// middleware that only honors forwarded headers from that proxy.
	r.Use(requestIDResponse)
	r.Use(routePattern)
	r.Use(requestLogger(log))
	r.Use(recoverPanics(log))
	// Per-request deadline: downstream ctx-aware calls (pgx, redis) abort
	// when it fires and the client gets a 504.
	r.Use(chimw.Timeout(cfg.Server.RequestTimeout))

	// Unknown routes and wrong methods use the same error envelope as the
	// API. Registered before Mount so sub-routers inherit both handlers.
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		handler.WriteError(w, http.StatusNotFound, handler.CodeNotFound, "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		handler.WriteError(w, http.StatusMethodNotAllowed, handler.CodeMethodNotAllowed, "method not allowed")
	})

	required := map[string]health.Pinger{"postgres": deps.Pool}
	optional := map[string]health.Pinger{}
	if deps.Redis != nil {
		if deps.RedisRequired {
			required["redis"] = redisPinger{rdb: deps.Redis}
		} else {
			optional["redis"] = redisPinger{rdb: deps.Redis}
		}
	}
	healthHandler := health.NewHandler(required, optional)
	r.Get("/healthz", healthHandler.Liveness)
	r.Get("/readyz", healthHandler.Readiness)
	// No /metrics on purpose: metrics are pushed over OTLP (see
	// platform/telemetry). A scrape endpoint would put route patterns, latency
	// distributions and pool sizes on an unauthenticated public port.

	authErrorHandler := func(
		w http.ResponseWriter,
		r *http.Request,
		err error,
	) {
		if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			handler.WriteError(w, http.StatusGatewayTimeout, handler.CodeTimeout, "request timed out")
			return
		}
		if errors.Is(err, authkit.ErrDenylistUnavailable) {
			handler.WriteError(w, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE", "authorization could not be verified")
			return
		}
		handler.WriteError(
			w,
			http.StatusUnauthorized,
			handler.CodeUnauthorized,
			"unauthorized",
		)
	}

	authMiddleware := func(next http.Handler) http.Handler {
		return deps.Verifier.Middleware(
			next,
			authErrorHandler,
		)
	}

	authHandler := authhandler.NewAuthHandler(
		deps.AuthService,
		log,
	)

	jwksHandler := jwkshandler.NewJwksHandler(
		deps.JwksService,
		log,
	)

	organizationHandler := organizationhandler.NewOrganizationHandler(
		deps.OrganizationService,
		log,
	)

	organizationMemberHandler := organizationmemberhandler.NewOrganizationMemberHandler(
		deps.OrganizationMemberService,
		log,
	)

	organizationInvitationHandler := organization_invitation.NewOrganizationInvitationHandler(
		deps.OrganizationInvitationService,
		log,
	)

	serviceAccessHandler := serviceaccesshandler.NewServiceAccessHandler(
		deps.ServiceAccessService,
		log,
	)

	deviceHandler := devicehandler.NewDeviceHandler(
		deps.DeviceService,
		log,
	)

	r.Route("/", func(r chi.Router) {
		jwksHandler.Routes(r)
	})

	r.Route("/api/v1", func(r chi.Router) {
		// Rate limit only the API subtree so health probes - hit constantly by
		// orchestrators - are never throttled. Requests is the per-IP count
		// allowed in each window; over it returns 429 through the standard
		// envelope.
		if cfg.RateLimit.Enabled {
			// Not httprate's KeyByRealIP: it trusts forwarded headers
			// (GHSA-9g5q-2w5x-hmxf).
			options := []httprate.Option{
				httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
					handler.WriteError(w, http.StatusTooManyRequests, handler.CodeRateLimited, "rate limit exceeded")
				}),
			}
			options = append(options, rateLimitOptions(deps.RateLimitRedis, "identity:api", log)...)
			r.Use(httprate.LimitBy(
				cfg.RateLimit.Requests,
				cfg.RateLimit.Window,
				keyByRemoteIP,
				options...,
			))
		}
		r.Route("/auth", func(r chi.Router) {
			authHandler.PublicRoutes(r)
			r.Group(func(r chi.Router) {
				options := []httprate.Option{
					httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
						handler.WriteError(w, http.StatusTooManyRequests, handler.CodeRateLimited, "rate limit exceeded")
					}),
				}
				options = append(options, rateLimitOptions(deps.RateLimitRedis, "identity:recovery", log)...)
				r.Use(httprate.LimitBy(
					5,
					15*time.Minute,
					keyByRemoteIP,
					options...,
				))
				authHandler.RecoveryRoutes(r)
			})

			r.Group(func(r chi.Router) {
				r.Use(authMiddleware)
				authHandler.ProtectedRoutes(r)
			})
		})

		r.Route("/organizations", func(r chi.Router) {
			r.Use(authMiddleware)
			organizationHandler.ProtectedRoutes(r)
			organizationMemberHandler.ProtectedRoutes(r)
			organizationInvitationHandler.ProtectedRoutes(r)
			serviceAccessHandler.ProtectedRoutes(r)
		})

		organizationInvitationHandler.PublicRoutes(r)

		// Devices are user-scoped, not organization-scoped: one flat protected
		// group, same middleware chain as the /organizations subtree.
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware)
			deviceHandler.ProtectedRoutes(r)
		})
	})

	// otelhttp wraps the whole router: creates the server span, extracts
	// incoming trace context, and records HTTP metrics. The filter drops probes
	// and token-bearing invitation paths: probes skew latency, and otelhttp
	// records URL paths verbatim.
	root := otelhttp.NewHandler(r, "http.server",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return !isProbe(r.URL.Path) && !hasInvitationTokenPath(r.URL.Path)
		}),
	)

	httpServer, err := httpserver.New(
		root,
		httpserver.WithAddress(fmt.Sprintf(":%d", cfg.Server.Port)),
		httpserver.WithReadTimeout(cfg.Server.ReadTimeout),
		httpserver.WithWriteTimeout(cfg.Server.WriteTimeout),
		httpserver.WithIdleTimeout(cfg.Server.IdleTimeout),
		httpserver.WithLogger(log),
	)
	if err != nil {
		return nil, fmt.Errorf("build HTTP server: %w", err)
	}

	return &Server{
		http:   httpServer,
		health: healthHandler,
	}, nil
}

// BeginDrain flips /readyz to 503 while continuing to serve traffic, so an
// orchestrator can remove this instance from rotation before Shutdown starts.
func (s *Server) BeginDrain() { s.health.BeginDrain() }

// Start blocks until the listener fails or Shutdown is called.
func (s *Server) Start() error {
	return s.http.Start()
}

// Shutdown drains in-flight requests until ctx expires, then closes.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
