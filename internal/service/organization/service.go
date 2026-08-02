package organization

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
	Create(ctx context.Context, querier repository.Querier, input CreateInput) (Organization, error)
}

type svc struct {
	log    *slog.Logger
	tracer trace.Tracer
}

var _ Service = (*svc)(nil)

func New(log *slog.Logger) Service {
	return &svc{
		log:    log,
		tracer: otel.Tracer("service/organization"),
	}
}

func (s *svc) Create(ctx context.Context, querier repository.Querier, input CreateInput) (Organization, error) {
	ctx, span := s.tracer.Start(ctx, "OrganizationService.Create")
	defer span.End()

	row, err := querier.CreateOrganization(ctx, repository.CreateOrganizationParams{
		Name: input.Name,
		Type: input.Type,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create organization failed")
		s.log.ErrorContext(ctx, "create organization failed", "error", err)
		return Organization{}, service.ErrInternal
	}

	return Organization{ID: row.ID, Name: row.Name, Type: row.Type}, nil
}
