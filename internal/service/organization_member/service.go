package organization_member

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/constant"
	notificationcontract "github.com/disillusioned-labs/platform/contract/notification"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("service/organization_member")

const (
	organizationMemberAggregateType = "organization_member"
	organizationAggregateType       = "organization"
)

type OrganizationMemberService interface {
	ListOrganizationMembers(ctx context.Context, input ListOrganizationMembersInput) (ListOrganizationMembersOutput, error)
	UpdateOrganizationMemberRole(ctx context.Context, input UpdateOrganizationMemberRoleInput) (UpdateOrganizationMemberRoleOutput, error)
	RemoveOrganizationMember(ctx context.Context, input RemoveOrganizationMemberInput) (RemoveOrganizationMemberOutput, error)
	LeaveOrganization(ctx context.Context, input LeaveOrganizationInput) (LeaveOrganizationOutput, error)
}

// RevocationWriter records that a user's access to one organization has been
// revoked, so their still-valid access tokens carrying that org_id are
// rejected before expiry. It is infrastructure, not another service; a nil
// writer disables the denylist write.
type RevocationWriter interface {
	RevokeMember(ctx context.Context, organizationID, userID string) error
}

type organizationMemberService struct {
	repo        repository.Store
	revocations RevocationWriter
	log         *slog.Logger
}

func NewOrganizationMemberService(repo repository.Store, revocations RevocationWriter, log *slog.Logger) OrganizationMemberService {
	return &organizationMemberService{
		repo:        repo,
		revocations: revocations,
		log:         log,
	}
}

func (s *organizationMemberService) revokeMemberAccess(ctx context.Context, organizationID, userID uuid.UUID) {
	if s.revocations == nil {
		return
	}

	if err := s.revocations.RevokeMember(ctx, organizationID.String(), userID.String()); err != nil {
		s.log.ErrorContext(ctx, "write member revocation failed", "error", err, "organization_id", organizationID, "user_id", userID)
	}
}

func (s *organizationMemberService) ListOrganizationMembers(
	ctx context.Context,
	input ListOrganizationMembersInput,
) (ListOrganizationMembersOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationMemberService.ListOrganizationMembers")
	defer span.End()

	_, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "user is not an organization member")
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

func (s *organizationMemberService) UpdateOrganizationMemberRole(
	ctx context.Context,
	input UpdateOrganizationMemberRoleInput,
) (UpdateOrganizationMemberRoleOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationMemberService.UpdateOrganizationMemberRole")
	defer span.End()

	if input.UserID == input.TargetUserID {
		span.SetStatus(codes.Error, "cannot modify own organization membership")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrCannotModifySelf
	}

	if input.Role != constant.RoleOwner &&
		input.Role != constant.RoleAdmin &&
		input.Role != constant.RoleMember {
		span.SetStatus(codes.Error, "invalid organization member role")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrInvalidRole
	}

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "current user is not an organization member")
			return UpdateOrganizationMemberRoleOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)

		return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner &&
		currentMember.Role != constant.RoleAdmin {
		span.SetStatus(codes.Error, "insufficient organization role")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrForbidden
	}

	if currentMember.Role == constant.RoleAdmin &&
		input.Role == constant.RoleOwner {
		span.SetStatus(codes.Error, "admin cannot assign owner role")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrForbidden
	}

	targetMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.TargetUserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "target user is not an organization member")
			return UpdateOrganizationMemberRoleOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get target organization member failed")
		s.log.ErrorContext(ctx, "get target organization member failed", "error", err)

		return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
	}

	if currentMember.Role == constant.RoleAdmin &&
		targetMember.Role == constant.RoleOwner {
		span.SetStatus(codes.Error, "admin cannot modify owner")
		return UpdateOrganizationMemberRoleOutput{}, service.ErrForbidden
	}

	if targetMember.Role == constant.RoleOwner &&
		input.Role != constant.RoleOwner {
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

			return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
		}

		if ownerCount <= 1 {
			span.SetStatus(codes.Error, "last owner cannot change role")
			return UpdateOrganizationMemberRoleOutput{}, service.ErrLastOwnerCannotLeave
		}
	}

	var updatedMember repository.GetUserOrganizationRow

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		rows, err := q.UpdateOrganizationMemberRole(
			ctx,
			repository.UpdateOrganizationMemberRoleParams{
				Role:           input.Role,
				OrganizationID: input.OrganizationID,
				UserID:         input.TargetUserID,
			},
		)
		if err != nil {
			return err
		}

		if rows == 0 {
			return pgx.ErrNoRows
		}

		updatedMember, err = q.GetUserOrganization(
			ctx,
			repository.GetUserOrganizationParams{
				UserID:         input.TargetUserID,
				OrganizationID: input.OrganizationID,
			},
		)
		if err != nil {
			return err
		}

		event := OrganizationMemberRoleUpdatedEvent{
			OrganizationID: input.OrganizationID,
			UserID:         input.TargetUserID,
			UpdatedBy:      input.UserID,
			Role:           updatedMember.Role,
		}

		if err := createOutboxEvent(
			ctx,
			q,
			organizationMemberAggregateType,
			input.TargetUserID,
			EventOrganizationMemberRoleUpdated,
			1,
			constant.TopicAudit,
			event,
		); err != nil {
			return err
		}

		org, err := q.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
			UserID:         input.TargetUserID,
			OrganizationID: input.OrganizationID,
		})
		if err != nil {
			return err
		}

		targetUser, err := q.GetUserByID(ctx, input.TargetUserID)
		if err != nil {
			return err
		}

		notificationPayload, err := json.Marshal(map[string]string{
			"organization_name": org.OrganizationName,
			"member_name":       targetUser.Name,
			"old_role":          targetMember.Role,
			"new_role":          updatedMember.Role,
		})
		if err != nil {
			return err
		}

		notificationEvent := notificationcontract.CreatedEvent{
			NotificationType: "organization_member_role_updated",
			Category:         notificationcontract.CategoryTransactional,
			RecipientID:      targetUser.ID.String(),
			Targets: []notificationcontract.Target{
				{
					Channel:     notificationcontract.ChannelEmail,
					Destination: targetUser.Email,
				},
			},
			Payload: notificationPayload,
		}

		if err := notificationEvent.Validate(); err != nil {
			return err
		}

		if err := createOutboxEvent(
			ctx,
			q,
			organizationMemberAggregateType,
			input.TargetUserID,
			notificationcontract.EventTypeCreated,
			1,
			constant.TopicNotificationTransactional,
			notificationEvent,
		); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(
				codes.Error,
				"organization member role update affected no rows",
			)

			return UpdateOrganizationMemberRoleOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "update organization member role failed")
		s.log.ErrorContext(
			ctx,
			"update organization member role failed",
			"error",
			err,
		)

		return UpdateOrganizationMemberRoleOutput{}, service.ErrInternal
	}

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

func (s *organizationMemberService) RemoveOrganizationMember(
	ctx context.Context,
	input RemoveOrganizationMemberInput,
) (RemoveOrganizationMemberOutput, error) {
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
			span.SetStatus(codes.Error, "current user is not an organization member")
			return RemoveOrganizationMemberOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)

		return RemoveOrganizationMemberOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner &&
		currentMember.Role != constant.RoleAdmin {
		span.SetStatus(codes.Error, "insufficient organization role")
		return RemoveOrganizationMemberOutput{}, service.ErrForbidden
	}

	targetMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.TargetUserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "target user is not an organization member")
			return RemoveOrganizationMemberOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get target organization member failed")
		s.log.ErrorContext(ctx, "get target organization member failed", "error", err)

		return RemoveOrganizationMemberOutput{}, service.ErrInternal
	}

	if currentMember.Role == constant.RoleAdmin &&
		targetMember.Role == constant.RoleOwner {
		span.SetStatus(codes.Error, "admin cannot remove owner")
		return RemoveOrganizationMemberOutput{}, service.ErrForbidden
	}

	if targetMember.Role == constant.RoleOwner {
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

			return RemoveOrganizationMemberOutput{}, service.ErrInternal
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

				return RemoveOrganizationMemberOutput{}, service.ErrInternal
			}

			if memberCount > 1 {
				span.SetStatus(
					codes.Error,
					"last owner cannot leave organization",
				)

				return RemoveOrganizationMemberOutput{}, service.ErrLastOwnerCannotLeave
			}
		}
	}

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		rows, err := q.SoftDeleteOrganizationMember(
			ctx,
			repository.SoftDeleteOrganizationMemberParams{
				OrganizationID: input.OrganizationID,
				UserID:         input.TargetUserID,
			},
		)
		if err != nil {
			return err
		}

		if rows == 0 {
			return pgx.ErrNoRows
		}

		event := OrganizationMemberRemovedEvent{
			OrganizationID: input.OrganizationID,
			UserID:         input.TargetUserID,
			RemovedBy:      input.UserID,
		}

		if err := createOutboxEvent(
			ctx,
			q,
			organizationMemberAggregateType,
			input.TargetUserID,
			EventOrganizationMemberRemoved,
			1,
			constant.TopicAudit,
			event,
		); err != nil {
			return err
		}

		org, err := q.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
			UserID:         input.TargetUserID,
			OrganizationID: input.OrganizationID,
		})
		if err != nil {
			return err
		}

		targetUser, err := q.GetUserByID(ctx, input.TargetUserID)
		if err != nil {
			return err
		}

		remover, err := q.GetUserByID(ctx, input.UserID)
		if err != nil {
			return err
		}

		notificationPayload, err := json.Marshal(map[string]string{
			"organization_name": org.OrganizationName,
			"member_name":       targetUser.Name,
			"remover_name":      remover.Name,
		})
		if err != nil {
			return err
		}

		notificationEvent := notificationcontract.CreatedEvent{
			NotificationType: "organization_member_removed",
			Category:         notificationcontract.CategoryTransactional,
			RecipientID:      targetUser.ID.String(),
			Targets: []notificationcontract.Target{
				{
					Channel:     notificationcontract.ChannelEmail,
					Destination: targetUser.Email,
				},
			},
			Payload: notificationPayload,
		}

		if err := notificationEvent.Validate(); err != nil {
			return err
		}

		if err := createOutboxEvent(
			ctx,
			q,
			organizationMemberAggregateType,
			input.TargetUserID,
			notificationcontract.EventTypeCreated,
			1,
			constant.TopicNotificationTransactional,
			notificationEvent,
		); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(
				codes.Error,
				"organization member removal affected no rows",
			)

			return RemoveOrganizationMemberOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "remove organization member failed")
		s.log.ErrorContext(
			ctx,
			"remove organization member failed",
			"error",
			err,
		)

		return RemoveOrganizationMemberOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("target_user.id", input.TargetUserID.String()),
	)

	s.revokeMemberAccess(ctx, input.OrganizationID, input.TargetUserID)

	return RemoveOrganizationMemberOutput{}, nil
}

func (s *organizationMemberService) LeaveOrganization(
	ctx context.Context,
	input LeaveOrganizationInput,
) (LeaveOrganizationOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationMemberService.LeaveOrganization")
	defer span.End()

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "current user is not an organization member")
			return LeaveOrganizationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(
			ctx,
			"get current organization member failed",
			"error",
			err,
		)

		return LeaveOrganizationOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("organization.type", currentMember.OrganizationType),
	)

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

			leaveEvent := OrganizationMemberLeftEvent{
				OrganizationID: input.OrganizationID,
				UserID:         input.UserID,
			}

			if err := createOutboxEvent(
				ctx,
				q,
				organizationMemberAggregateType,
				input.UserID,
				EventOrganizationMemberLeft,
				1,
				constant.TopicAudit,
				leaveEvent,
			); err != nil {
				return err
			}

			deleteEvent := OrganizationDeletedEvent{
				OrganizationID: input.OrganizationID,
				DeletedBy:      input.UserID,
			}

			if err := createOutboxEvent(
				ctx,
				q,
				organizationAggregateType,
				input.UserID,
				EventOrganizationDeleted,
				1,
				constant.TopicAudit,
				deleteEvent,
			); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				span.SetStatus(
					codes.Error,
					"organization membership or organization not found",
				)

				return LeaveOrganizationOutput{}, service.ErrNotFound
			}

			span.RecordError(err)
			span.SetStatus(
				codes.Error,
				"leave personal organization failed",
			)
			s.log.ErrorContext(
				ctx,
				"leave personal organization failed",
				"error",
				err,
			)

			return LeaveOrganizationOutput{}, service.ErrInternal
		}

		span.SetAttributes(
			attribute.String(
				"replacement_organization.id",
				replacementOrganization.ID.String(),
			),
		)

		s.revokeMemberAccess(ctx, input.OrganizationID, input.UserID)

		return LeaveOrganizationOutput{
			OrganizationDeleted: true,
		}, nil
	}

	if currentMember.Role == constant.RoleOwner {
		ownerCount, err := s.repo.CountActiveOrganizationOwners(
			ctx,
			input.OrganizationID,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(
				codes.Error,
				"count active organization owners failed",
			)
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
				span.SetStatus(
					codes.Error,
					"count active organization members failed",
				)
				s.log.ErrorContext(
					ctx,
					"count active organization members failed",
					"error",
					err,
				)

				return LeaveOrganizationOutput{}, service.ErrInternal
			}

			if memberCount > 1 {
				span.SetStatus(
					codes.Error,
					"last owner cannot leave organization",
				)

				return LeaveOrganizationOutput{}, service.ErrLastOwnerCannotLeave
			}

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

				leaveEvent := OrganizationMemberLeftEvent{
					OrganizationID: input.OrganizationID,
					UserID:         input.UserID,
				}

				if err := createOutboxEvent(
					ctx,
					q,
					organizationMemberAggregateType,
					input.UserID,
					EventOrganizationMemberLeft,
					1,
					constant.TopicAudit,
					leaveEvent,
				); err != nil {
					return err
				}

				deleteEvent := OrganizationDeletedEvent{
					OrganizationID: input.OrganizationID,
					DeletedBy:      input.UserID,
				}

				if err := createOutboxEvent(
					ctx,
					q,
					organizationAggregateType,
					input.UserID,
					EventOrganizationDeleted,
					1,
					constant.TopicAudit,
					deleteEvent,
				); err != nil {
					return err
				}

				return nil
			})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				span.SetStatus(
					codes.Error,
					"organization membership or organization not found",
				)

				return LeaveOrganizationOutput{}, service.ErrNotFound
			}

			span.RecordError(err)
			span.SetStatus(
				codes.Error,
				"leave organization failed",
			)
			s.log.ErrorContext(
				ctx,
				"leave organization failed",
				"error",
				err,
			)

			return LeaveOrganizationOutput{}, service.ErrInternal
		}

			s.revokeMemberAccess(ctx, input.OrganizationID, input.UserID)

			return LeaveOrganizationOutput{
				OrganizationDeleted: true,
			}, nil
		}
	}

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

		event := OrganizationMemberLeftEvent{
			OrganizationID: input.OrganizationID,
			UserID:         input.UserID,
		}

		if err := createOutboxEvent(
			ctx,
			q,
			organizationMemberAggregateType,
			input.UserID,
			EventOrganizationMemberLeft,
			1,
			constant.TopicAudit,
			event,
		); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(
				codes.Error,
				"organization member leave affected no rows",
			)

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

	s.revokeMemberAccess(ctx, input.OrganizationID, input.UserID)

	return LeaveOrganizationOutput{}, nil
}

func createOutboxEvent(ctx context.Context, q repository.Querier, aggregateType string, aggregateID uuid.UUID, eventType string, eventVersion int32, topic string, payload any) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	spanCtx := trace.SpanContextFromContext(ctx)
	var traceID pgtype.Text
	if spanCtx.IsValid() {
		traceID = pgtype.Text{
			String: spanCtx.TraceID().String(),
			Valid:  true,
		}
	}

	_, err = q.CreateOutboxEvent(
		ctx,
		repository.CreateOutboxEventParams{
			AggregateType: aggregateType,
			AggregateID:   aggregateID,
			EventType:     eventType,
			EventVersion:  eventVersion,
			Topic:         topic,
			Payload:       payloadJSON,
			TraceID:       traceID,
		},
	)

	return err
}
