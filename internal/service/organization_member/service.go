package organizationmember

import (
	"context"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Service interface {
	Create(ctx context.Context, querier repository.Querier, input CreateInput) error
}

type svc struct {
	log    *slog.Logger
	tracer trace.Tracer
}

var _ Service = (*svc)(nil)

func New(log *slog.Logger) Service {
	return &svc{
		log:    log,
		tracer: otel.Tracer("service/organization_member"),
	}
}

func (s *svc) Create(ctx context.Context, querier repository.Querier, input CreateInput) error {
	ctx, span := s.tracer.Start(ctx, "OrganizationMemberService.Create")
	defer span.End()

	_, err := querier.CreateOrganizationMember(ctx, repository.CreateOrganizationMemberParams{
		OrganizationID: input.OrganizationID,
		UserID:         input.UserID,
		Role:           input.Role,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create organization member failed")
		s.log.ErrorContext(ctx, "create organization member failed", "error", err)
		return service.ErrInternal
	}

	return nil
}
