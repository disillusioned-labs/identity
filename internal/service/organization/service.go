package organization

import (
	"context"
	"errors"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("service/organization")

type OrganizationService interface {
	ListOrganizations(ctx context.Context, input ListInput) (ListOutput, error)
	CreateOrganization(ctx context.Context, input CreateInput) (CreateOutput, error)
	GetOrganization(ctx context.Context, input GetInput) (GetOutput, error)
	UpdateOrganization(ctx context.Context, input UpdateInput) (UpdateOutput, error)
	DeleteOrganization(ctx context.Context, input DeleteInput) (DeleteOutput, error)
}

type organizationService struct {
	repo repository.Store
	log  *slog.Logger
}

func NewOrganizationService(repo repository.Store, log *slog.Logger) OrganizationService {
	return &organizationService{
		repo: repo,
		log:  log,
	}
}

func (s *organizationService) ListOrganizations(ctx context.Context, input ListInput) (ListOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.ListOrganizations")
	defer span.End()

	organizationRows, err := s.repo.ListUserOrganizations(ctx, input.UserID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list organizations failed")
		s.log.ErrorContext(ctx, "list organizations failed", "error", err)
		return ListOutput{}, service.ErrInternal
	}

	organizations := make([]OrganizationOutput, 0, len(organizationRows))

	for _, organizationRow := range organizationRows {
		organizations = append(organizations, OrganizationOutput{
			ID:   organizationRow.OrganizationID,
			Name: organizationRow.Name,
			Type: organizationRow.Type,
			Role: organizationRow.Role,
		})
	}

	span.SetAttributes(
		attribute.String("user.id", input.UserID.String()),
	)

	return ListOutput{
		Organizations: organizations,
	}, nil
}

func (s *organizationService) CreateOrganization(ctx context.Context, input CreateInput) (CreateOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.CreateOrganization")
	defer span.End()

	if input.Type != constant.OrganizationTypePersonal &&
		input.Type != constant.OrganizationTypeBusiness {
		span.SetStatus(codes.Error, "invalid organization type")
		return CreateOutput{}, service.ErrInvalidOrganizationType
	}

	var organization repository.CreateOrganizationRow

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		var err error

		organization, err = q.CreateOrganization(ctx, repository.CreateOrganizationParams{
			Name: input.Name,
			Type: input.Type,
		})
		if err != nil {
			return err
		}

		_, err = q.CreateOrganizationMember(ctx, repository.CreateOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         input.UserID,
			Role:           constant.RoleOwner,
		})
		if err != nil {
			return err
		}

		// insert to audit logs

		return nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create organization failed")
		s.log.ErrorContext(ctx, "create organization failed", "error", err)
		return CreateOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", organization.ID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("organization.type", organization.Type),
	)

	return CreateOutput{
		Organization: OrganizationOutput{
			ID:   organization.ID,
			Name: organization.Name,
			Type: organization.Type,
			Role: constant.RoleOwner,
		},
	}, nil
}

func (s *organizationService) GetOrganization(ctx context.Context, input GetInput) (GetOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.GetOrganization")
	defer span.End()

	organization, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GetOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get organization failed")
		s.log.ErrorContext(ctx, "get organization failed", "error", err)
		return GetOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
	)

	return GetOutput{
		Organization: OrganizationOutput{
			ID:   organization.OrganizationID,
			Name: organization.OrganizationName,
			Type: organization.OrganizationType,
			Role: organization.Role,
		},
	}, nil
}

func (s *organizationService) UpdateOrganization(ctx context.Context, input UpdateInput) (UpdateOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.UpdateOrganization")
	defer span.End()

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UpdateOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)
		return UpdateOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner && currentMember.Role != constant.RoleAdmin {
		return UpdateOutput{}, service.ErrForbidden
	}

	organization, err := s.repo.UpdateOrganization(ctx, repository.UpdateOrganizationParams{
		Name: input.Name,
		ID:   input.OrganizationID,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update organization failed")
		s.log.ErrorContext(ctx, "update organization failed", "error", err)
		return UpdateOutput{}, service.ErrInternal
	}

	// insert to audit logs

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
	)

	return UpdateOutput{
		Organization: OrganizationOutput{
			ID:   organization.ID,
			Name: organization.Name,
			Type: organization.Type,
			Role: currentMember.Role,
		},
	}, nil
}

func (s *organizationService) DeleteOrganization(ctx context.Context, input DeleteInput) (DeleteOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.DeleteOrganization")
	defer span.End()

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DeleteOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)
		return DeleteOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner {
		span.SetStatus(codes.Error, "only organization owner can delete organization")
		return DeleteOutput{}, service.ErrForbidden
	}

	if currentMember.OrganizationType == constant.OrganizationTypePersonal {
		var newOrganization repository.CreateOrganizationRow

		err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
			rows, err := q.SoftDeleteOrganization(ctx, input.OrganizationID)
			if err != nil {
				return err
			}

			if rows == 0 {
				return pgx.ErrNoRows
			}

			newOrganization, err = q.CreateOrganization(ctx, repository.CreateOrganizationParams{
				Name: "Personal Organization",
				Type: constant.OrganizationTypePersonal,
			})
			if err != nil {
				return err
			}

			_, err = q.CreateOrganizationMember(ctx, repository.CreateOrganizationMemberParams{
				OrganizationID: newOrganization.ID,
				UserID:         input.UserID,
				Role:           constant.RoleOwner,
			})
			if err != nil {
				return err
			}

			// insert to audit logs

			return nil
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return DeleteOutput{}, service.ErrNotFound
			}

			span.RecordError(err)
			span.SetStatus(codes.Error, "delete personal organization failed")
			s.log.ErrorContext(ctx, "delete personal organization failed", "error", err)
			return DeleteOutput{}, service.ErrInternal
		}

		span.SetAttributes(
			attribute.String("organization.id", input.OrganizationID.String()),
			attribute.String("user.id", input.UserID.String()),
			attribute.String("organization.type", constant.OrganizationTypePersonal),
			attribute.String("replacement_organization.id", newOrganization.ID.String()),
		)

		return DeleteOutput{}, nil
	}

	rows, err := s.repo.SoftDeleteOrganization(ctx, input.OrganizationID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete organization failed")
		s.log.ErrorContext(ctx, "delete organization failed", "error", err)
		return DeleteOutput{}, service.ErrInternal
	}

	if rows == 0 {
		return DeleteOutput{}, service.ErrNotFound
	}

	// insert to audit logs

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("organization.type", currentMember.OrganizationType),
	)

	return DeleteOutput{}, nil
}
