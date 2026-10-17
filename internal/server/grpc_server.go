package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	identitypb "github.com/disillusioned-labs/platform/contract/identity"
	platformgrpc "github.com/disillusioned-labs/platform/grpc"

	"github.com/disillusioned-labs/identity/internal/config"
	identityhandler "github.com/disillusioned-labs/identity/internal/handler/grpc"
)

// GRPCServer wraps the gRPC server with the Identity service implementation
// and configuration.
type GRPCServer struct {
	grpc *platformgrpc.Server
	log  *slog.Logger
	cfg  *config.Config
}

// NewGRPC assembles the gRPC server and Identity service implementation into
// a ready-to-start GRPCServer.
func NewGRPC(cfg *config.Config, log *slog.Logger, deps Deps) (*GRPCServer, error) {
	opts := []platformgrpc.Option{
		platformgrpc.WithMaxRecvMsgSize(cfg.GRPC.MaxRecvMsgSize),
		platformgrpc.WithMaxSendMsgSize(cfg.GRPC.MaxSendMsgSize),
		platformgrpc.WithMaxHeaderSize(cfg.GRPC.MaxHeaderSize),
		platformgrpc.WithLogger(log),

		// Request ID interceptor — extracts x-request-id from incoming
		// metadata, generates one if absent, logs duration.
		platformgrpc.WithUnaryServerInterceptor(
			platformgrpc.UnaryRequestIDServer(log),
		),
		platformgrpc.WithStreamServerInterceptor(
			platformgrpc.StreamRequestIDServer(log),
		),
	}
	if cfg.Service.Env != "development" || cfg.GRPCAuth.ExpenseServiceTokens != "" || cfg.GRPCAuth.NotificationServiceTokens != "" {
		tokensByCaller := make(map[string][]string, 2)
		callersByMethod := make(map[string][]string)
		if cfg.GRPCAuth.ExpenseServiceTokens != "" {
			expenseTokens, err := platformgrpc.ParseServiceTokens(cfg.GRPCAuth.ExpenseServiceTokens)
			if err != nil {
				return nil, fmt.Errorf("parse expense gRPC service tokens: %w", err)
			}
			tokensByCaller["expense"] = expenseTokens
			for _, method := range []string{
				identitypb.IdentityService_IsMemberActive_FullMethodName,
				identitypb.IdentityService_GetMemberRole_FullMethodName,
				identitypb.IdentityService_GetUsersInfo_FullMethodName,
				identitypb.IdentityService_GetOrgStatus_FullMethodName,
				identitypb.IdentityService_IsServiceAccessAllowed_FullMethodName,
				identitypb.IdentityService_HoldExpenseWriteAccess_FullMethodName,
			} {
				callersByMethod[method] = []string{"expense"}
			}
		}
		if cfg.GRPCAuth.NotificationServiceTokens != "" {
			notificationTokens, err := platformgrpc.ParseServiceTokens(cfg.GRPCAuth.NotificationServiceTokens)
			if err != nil {
				return nil, fmt.Errorf("parse notification gRPC service tokens: %w", err)
			}
			tokensByCaller["notification"] = notificationTokens
			callersByMethod[identitypb.IdentityService_GetDeviceTokens_FullMethodName] = []string{"notification"}
		}
		unaryAuth, streamAuth, err := platformgrpc.NewServiceAuthInterceptors(tokensByCaller, callersByMethod)
		if err != nil {
			return nil, fmt.Errorf("configure identity gRPC service auth: %w", err)
		}
		opts = append(opts,
			platformgrpc.WithUnaryServerInterceptor(unaryAuth),
			platformgrpc.WithStreamServerInterceptor(streamAuth),
		)
	}
	if cfg.GRPC.TLS.Enabled {
		tlsConfig, err := platformgrpc.NewTLSConfig(
			cfg.GRPC.TLS.CAFile,
			cfg.GRPC.TLS.CertFile,
			cfg.GRPC.TLS.KeyFile,
			cfg.GRPC.TLS.ServerName,
			cfg.GRPC.TLS.MutualTLS,
		)
		if err != nil {
			return nil, fmt.Errorf("build grpc server TLS config: %w", err)
		}
		opts = append(opts, platformgrpc.WithTLS(tlsConfig))
	}

	grpcServer, err := platformgrpc.NewServer(opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc server: %w", err)
	}

	identityServer := identityhandler.NewIdentityServer(
		deps.AuthService,
		deps.OrganizationService,
		deps.OrganizationMemberService,
		deps.ServiceAccessService,
		deps.DeviceService,
		log,
	)

	identitypb.RegisterIdentityServiceServer(
		grpcServer.GRPC(),
		identityServer,
	)

	return &GRPCServer{
		grpc: grpcServer,
		log:  log,
		cfg:  cfg,
	}, nil
}

// BeginDrain flips gRPC health to NOT_SERVING while continuing to serve
// existing traffic, so an orchestrator can remove this instance from
// rotation before Shutdown starts.
func (s *GRPCServer) BeginDrain() {
	s.grpc.Health().SetNotServing("")
}

// Start blocks until the listener fails or Shutdown is called.
func (s *GRPCServer) Start() error {
	addr := fmt.Sprintf(":%d", s.cfg.GRPC.ServerPort)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	s.log.Info("grpc server listening", "addr", addr)

	if err := s.grpc.Serve(listener); err != nil {
		return fmt.Errorf("grpc server: %w", err)
	}

	return nil
}

// Shutdown drains in-flight RPCs until ctx expires, then closes.
func (s *GRPCServer) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down grpc server")
	return s.grpc.Stop(ctx)
}
