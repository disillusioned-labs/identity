package organization_member

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

var tracer = otel.Tracer("service/organization_member")

type OrganizationMemberService interface {
	ListOrganizationMembers(ctx context.Context, input ListOrganizationMembersInput) (ListOrganizationMembersOutput, error)
	UpdateOrganizationMemberRole(ctx context.Context, input UpdateOrganizationMemberRoleInput) (UpdateOrganizationMemberRoleOutput, error)
	RemoveOrganizationMember(ctx context.Context, input RemoveOrganizationMemberInput) (RemoveOrganizationMemberOutput, error)
	LeaveOrganization(ctx context.Context, input LeaveOrganizationInput) (LeaveOrganizationOutput, error)
}

type organizationMemberService struct {
	repo repository.Store
	log  *slog.Logger
}

func NewOrganizationMemberService(repo repository.Store, log *slog.Logger) OrganizationMemberService {
	return &organizationMemberService{
		repo: repo,
		log:  log,
	}
}

func (s *organizationMemberService) ListOrganizationMembers(ctx context.Context, input ListOrganizationMembersInput) (ListOrganizationMembersOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationMemberService.ListOrganizationMembers")
	defer span.End()

	_, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ListOrganizationMembersOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get user organization failed")
		s.log.ErrorContext(ctx, "get user organization failed", "error", err)
		return ListOrganizationMembersOutput{}, service.ErrInternal
	}

	memberRows, err := s.repo.ListOrganizationMembers(ctx, input.OrganizationID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list organization members failed")
		s.log.ErrorContext(ctx, "list organization members failed", "error", err)
		return ListOrganizationMembersOutput{}, service.ErrInternal
	}

	members := make([]OrganizationMemberOutput, 0, len(memberRows))

	for _, memberRow := range memberRows {
		members = append(members, OrganizationMemberOutput{
			UserID:   memberRow.UserID,
			Name:     memberRow.Name,
			Email:    memberRow.Email,
			Role:     memberRow.Role,
			JoinedAt: memberRow.JoinedAt,
		})
	}

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
	)

	return ListOrganizationMembersOutput{
		Members: members,
	}, nil
}

func (s *organizationMemberService) UpdateOrganizationMemberRole(ctx context.Context, input UpdateOrganizationMemberRoleInput) (UpdateOrganizationMemberRoleOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationMemberService.UpdateOrganizationMemberRole")
	defer span.End()

	if input.UserID == input.TargetUserID {
		span.SetStatus(codes.Error, "cannot modify own organization membership")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrCannotModifySelf
	}

	if input.Role != constant.RoleOwner && input.Role != constant.RoleAdmin && input.Role != constant.RoleMember {
		span.SetStatus(codes.Error, "invalid organization member role")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrInvalidRole
	}

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UpdateOrganizationMemberRoleOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)
		return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner && currentMember.Role != constant.RoleAdmin {
		span.SetStatus(codes.Error, "insufficient organization role")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrForbidden
	}

	if currentMember.Role == constant.RoleAdmin && input.Role == constant.RoleOwner {
		span.SetStatus(codes.Error, "admin cannot assign owner role")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrForbidden
	}

	targetMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.TargetUserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UpdateOrganizationMemberRoleOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get target organization member failed")
		s.log.ErrorContext(ctx, "get target organization member failed", "error", err)
		return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
	}

	if currentMember.Role == constant.RoleAdmin && targetMember.Role == constant.RoleOwner {
		span.SetStatus(codes.Error, "admin cannot modify owner")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrForbidden
	}

	if targetMember.Role == constant.RoleOwner && input.Role != constant.RoleOwner {
		ownerCount, err := s.repo.CountActiveOrganizationOwners(ctx, input.OrganizationID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "count active organization owners failed")
			s.log.ErrorContext(ctx, "count active organization owners failed", "error", err)
			return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
		}

		if ownerCount <= 1 {
			span.SetStatus(codes.Error, "last owner cannot change role")
			return UpdateOrganizationMemberRoleOutput{}, service.ErrLastOwnerCannotLeave
		}
	}

	rows, err := s.repo.UpdateOrganizationMemberRole(ctx, repository.UpdateOrganizationMemberRoleParams{
		Role:           input.Role,
		OrganizationID: input.OrganizationID,
		UserID:         input.TargetUserID,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update organization member role failed")
		s.log.ErrorContext(ctx, "update organization member role failed", "error", err)
		return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
	}

	if rows == 0 {
		return UpdateOrganizationMemberRoleOutput{}, service.ErrNotFound
	}

	updatedMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.TargetUserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UpdateOrganizationMemberRoleOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get updated organization member failed")
		s.log.ErrorContext(ctx, "get updated organization member failed", "error", err)
		return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
	}

	// insert to audit logs

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("target_user.id", input.TargetUserID.String()),
	)

	return UpdateOrganizationMemberRoleOutput{
		Member: OrganizationMemberOutput{
			UserID:   updatedMember.UserID,
			Name:     updatedMember.UserName,
			Email:    updatedMember.Email,
			Role:     updatedMember.Role,
			JoinedAt: updatedMember.JoinedAt,
		},
	}, nil
}

func (s *organizationMemberService) RemoveOrganizationMember(ctx context.Context, input RemoveOrganizationMemberInput) (RemoveOrganizationMemberOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationMemberService.RemoveOrganizationMember")
	defer span.End()

	if input.UserID == input.TargetUserID {
		span.SetStatus(codes.Error, "cannot remove own organization membership")
		return RemoveOrganizationMemberOutput{}, service.ErrCannotModifySelf
	}

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RemoveOrganizationMemberOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)
		return RemoveOrganizationMemberOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner && currentMember.Role != constant.RoleAdmin {
		return RemoveOrganizationMemberOutput{}, service.ErrForbidden
	}

	targetMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.TargetUserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RemoveOrganizationMemberOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get target organization member failed")
		s.log.ErrorContext(ctx, "get target organization member failed", "error", err)
		return RemoveOrganizationMemberOutput{}, service.ErrInternal
	}

	if currentMember.Role == constant.RoleAdmin && targetMember.Role == constant.RoleOwner {
		return RemoveOrganizationMemberOutput{}, service.ErrForbidden
	}

	if targetMember.Role == constant.RoleOwner {
		ownerCount, err := s.repo.CountActiveOrganizationOwners(ctx, input.OrganizationID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "count active organization owners failed")
			s.log.ErrorContext(ctx, "count active organization owners failed", "error", err)
			return RemoveOrganizationMemberOutput{}, service.ErrInternal
		}

		if ownerCount <= 1 {
			memberCount, err := s.repo.CountActiveOrganizationMembers(ctx, input.OrganizationID)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "count active organization members failed")
				s.log.ErrorContext(ctx, "count active organization members failed", "error", err)
				return RemoveOrganizationMemberOutput{}, service.ErrInternal
			}

			if memberCount > 1 {
				return RemoveOrganizationMemberOutput{}, service.ErrLastOwnerCannotLeave
			}
		}
	}

	rows, err := s.repo.SoftDeleteOrganizationMember(ctx, repository.SoftDeleteOrganizationMemberParams{
		OrganizationID: input.OrganizationID,
		UserID:         input.TargetUserID,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "remove organization member failed")
		s.log.ErrorContext(ctx, "remove organization member failed", "error", err)
		return RemoveOrganizationMemberOutput{}, service.ErrInternal
	}

	if rows == 0 {
		return RemoveOrganizationMemberOutput{}, service.ErrNotFound
	}

	// insert to audit logs

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("target_user.id", input.TargetUserID.String()),
	)

	return RemoveOrganizationMemberOutput{}, nil
}

func (s *organizationMemberService) LeaveOrganization(ctx context.Context, input LeaveOrganizationInput) (LeaveOrganizationOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationMemberService.LeaveOrganization")
	defer span.End()

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LeaveOrganizationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)
		return LeaveOrganizationOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("organization.type", currentMember.OrganizationType),
	)

	// Personal organization:
	// leaving it means replacing it with a new personal organization.
	if currentMember.OrganizationType == constant.OrganizationTypePersonal {
		var replacementOrganization repository.CreateOrganizationRow

		err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
			rows, err := q.SoftDeleteOrganizationMember(
				ctx,
				repository.SoftDeleteOrganizationMemberParams{
					OrganizationID: input.OrganizationID,
					UserID:         input.UserID,
				},
			)
			if err != nil {
				return err
			}

			if rows == 0 {
				return pgx.ErrNoRows
			}

			rows, err = q.SoftDeleteOrganization(
				ctx,
				input.OrganizationID,
			)
			if err != nil {
				return err
			}

			if rows == 0 {
				return pgx.ErrNoRows
			}

			replacementOrganization, err = q.CreateOrganization(
				ctx,
				repository.CreateOrganizationParams{
					Name: "Personal Organization",
					Type: constant.OrganizationTypePersonal,
				},
			)
			if err != nil {
				return err
			}

			_, err = q.CreateOrganizationMember(
				ctx,
				repository.CreateOrganizationMemberParams{
					OrganizationID: replacementOrganization.ID,
					UserID:         input.UserID,
					Role:           constant.RoleOwner,
				},
			)
			if err != nil {
				return err
			}

			// insert to audit logs

			return nil
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return LeaveOrganizationOutput{}, service.ErrNotFound
			}

			span.RecordError(err)
			span.SetStatus(codes.Error, "leave personal organization failed")
			s.log.ErrorContext(ctx, "leave personal organization failed", "error", err)
			return LeaveOrganizationOutput{}, service.ErrInternal
		}

		span.SetAttributes(
			attribute.String(
				"replacement_organization.id",
				replacementOrganization.ID.String(),
			),
		)

		return LeaveOrganizationOutput{
			OrganizationDeleted: true,
		}, nil
	}

	// Business organization.
	if currentMember.Role == constant.RoleOwner {
		ownerCount, err := s.repo.CountActiveOrganizationOwners(
			ctx,
			input.OrganizationID,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "count active organization owners failed")
			s.log.ErrorContext(
				ctx,
				"count active organization owners failed",
				"error",
				err,
			)
			return LeaveOrganizationOutput{}, service.ErrInternal
		}

		if ownerCount <= 1 {
			memberCount, err := s.repo.CountActiveOrganizationMembers(
				ctx,
				input.OrganizationID,
			)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "count active organization members failed")
				s.log.ErrorContext(
					ctx,
					"count active organization members failed",
					"error",
					err,
				)
				return LeaveOrganizationOutput{}, service.ErrInternal
			}

			// There are other members, but nobody else can own
			// the organization.
			if memberCount > 1 {
				span.SetStatus(
					codes.Error,
					"last owner cannot leave organization",
				)
				return LeaveOrganizationOutput{}, service.ErrLastOwnerCannotLeave
			}

			// Last member of a business organization:
			// leaving also deletes the organization.
			err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
				rows, err := q.SoftDeleteOrganizationMember(
					ctx,
					repository.SoftDeleteOrganizationMemberParams{
						OrganizationID: input.OrganizationID,
						UserID:         input.UserID,
					},
				)
				if err != nil {
					return err
				}

				if rows == 0 {
					return pgx.ErrNoRows
				}

				rows, err = q.SoftDeleteOrganization(
					ctx,
					input.OrganizationID,
				)
				if err != nil {
					return err
				}

				if rows == 0 {
					return pgx.ErrNoRows
				}

				// insert to audit logs

				return nil
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return LeaveOrganizationOutput{}, service.ErrNotFound
				}

				span.RecordError(err)
				span.SetStatus(codes.Error, "leave organization failed")
				s.log.ErrorContext(
					ctx,
					"leave organization failed",
					"error",
					err,
				)
				return LeaveOrganizationOutput{}, service.ErrInternal
			}

			return LeaveOrganizationOutput{
				OrganizationDeleted: true,
			}, nil
		}
	}

	rows, err := s.repo.SoftDeleteOrganizationMember(
		ctx,
		repository.SoftDeleteOrganizationMemberParams{
			OrganizationID: input.OrganizationID,
			UserID:         input.UserID,
		},
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "leave organization failed")
		s.log.ErrorContext(ctx, "leave organization failed", "error", err)
		return LeaveOrganizationOutput{}, service.ErrInternal
	}

	if rows == 0 {
		return LeaveOrganizationOutput{}, service.ErrNotFound
	}

	// insert to audit logs

	return LeaveOrganizationOutput{}, nil
}
