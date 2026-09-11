package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/config"
	"github.com/disillusioned-labs/identity/internal/contract"
	platformgrpc "github.com/disillusioned-labs/platform/grpc"
)

// newExpenseClient creates the gRPC client to expense's internal
// MemberService surface (decision D2). Returns (client, cleanup, error).
// Caller must defer cleanup.
func newExpenseClient(ctx context.Context, cfg *config.Config, log *slog.Logger) (contract.ExpenseClient, func(), error) {
	opts := []platformgrpc.Option{
		platformgrpc.WithUnaryTimeout(cfg.GRPCClient.Timeout),
		platformgrpc.WithMaxRecvMsgSize(cfg.GRPCClient.MaxRecvMsgSize),
		platformgrpc.WithMaxSendMsgSize(cfg.GRPCClient.MaxSendMsgSize),
		platformgrpc.WithLogger(log),
		// Propagate x-request-id so an expense-side trace/log for the
		// removal check correlates with the identity request.
		platformgrpc.WithUnaryClientInterceptor(
			platformgrpc.UnaryForwardMetadataClient([]string{"x-request-id"}),
		),
	}

	if cfg.GRPCClient.TLS.Enabled {
		// TODO: build *tls.Config from GRPCTLSConfig fields when TLS is enabled.
		// opts = append(opts, platformgrpc.WithTLS(tlsConfig))
	}

	client, err := platformgrpc.NewClient(cfg.GRPCClient.Target, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create expense grpc client: %w", err)
	}

	expenseClient := contract.NewGRPCExpenseClient(client, log)
	cleanup := func() { _ = client.Close() }

	return expenseClient, cleanup, nil
}
