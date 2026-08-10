package organization

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
)

type OrganizationService interface {
	Create(ctx context.Context, querier repository.Querier, input CreateInput) (CreateOutput, error)
}

type organizationService struct {
	log    *slog.Logger
	tracer trace.Tracer
}

func NewOrganizationService(log *slog.Logger) OrganizationService {
	return &organizationService{
		log:    log,
		tracer: otel.Tracer("service/organization"),
	}
}

func (s *organizationService) Create(ctx context.Context, querier repository.Querier, input CreateInput) (CreateOutput, error) {
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
		return CreateOutput{}, service.ErrInternal
	}

	return CreateOutput{ID: row.ID, Name: row.Name, Type: row.Type}, nil
}
