package organizationmember

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
)

type OrganizationMemberService interface {
	Create(ctx context.Context, querier repository.Querier, input CreateInput) error
	ListByUser(ctx context.Context, querier repository.Querier, input ListByUserInput) ([]ListByUserOutput, error)
	Get(ctx context.Context, querier repository.Querier, input GetInput) (GetOutput, error)
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

func (s *organizationMemberService) ListByUser(ctx context.Context, querier repository.Querier, input ListByUserInput) ([]ListByUserOutput, error) {
	ctx, span := s.tracer.Start(ctx, "OrganizationMemberService.ListByUser")
	defer span.End()

	rows, err := querier.ListUserMemberships(ctx, input.UserID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list user memberships failed")
		s.log.ErrorContext(ctx, "list user memberships failed", "error", err)
		return nil, service.ErrInternal
	}

	out := make([]ListByUserOutput, 0, len(rows))
	for _, row := range rows {
		out = append(out, ListByUserOutput{
			OrganizationID:   row.OrganizationID,
			OrganizationName: row.Name,
			OrganizationType: row.Type,
			Role:             row.Role,
		})
	}
	return out, nil
}

func (s *organizationMemberService) Get(ctx context.Context, querier repository.Querier, input GetInput) (GetOutput, error) {
	ctx, span := s.tracer.Start(ctx, "OrganizationMemberService.Get")
	defer span.End()

	row, err := querier.GetMembership(ctx, repository.GetMembershipParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "membership not found")
			return GetOutput{}, service.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get membership failed")
		s.log.ErrorContext(ctx, "get membership failed", "error", err)
		return GetOutput{}, service.ErrInternal
	}

	return GetOutput{
		OrganizationID:   row.OrganizationID,
		OrganizationName: row.Name,
		OrganizationType: row.Type,
		Role:             row.Role,
	}, nil
}
