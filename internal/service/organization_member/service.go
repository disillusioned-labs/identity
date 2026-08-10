package organizationmember

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
)

type OrganizationMemberService interface {
	Create(ctx context.Context, querier repository.Querier, input CreateInput) error
}

type organizationMemberService struct {
	log    *slog.Logger
	tracer trace.Tracer
}

func NewOrganizationMemberService(log *slog.Logger) OrganizationMemberService {
	return &organizationMemberService{
		log:    log,
		tracer: otel.Tracer("service/organization_member"),
	}
}

func (s *organizationMemberService) Create(ctx context.Context, querier repository.Querier, input CreateInput) error {
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
