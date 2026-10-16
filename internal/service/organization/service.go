package organization

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	notificationcontract "github.com/disillusioned-labs/platform/contract/notification"
)

var tracer = otel.Tracer("service/organization")

const (
	organizationAggregateType = "organization"
	organizationEventVersion  = 1
)

type OrganizationService interface {
	ListOrganizations(ctx context.Context, input ListInput) (ListOutput, error)
	CreateOrganization(ctx context.Context, input CreateInput) (CreateOutput, error)
	GetOrganization(ctx context.Context, input GetInput) (GetOutput, error)
	GetOrganizationStatus(ctx context.Context, input GetOrganizationStatusInput) (GetOrganizationStatusOutput, error)
	// HoldExpenseWriteAccess serializes an Expense mutation against Identity authorization changes.
	HoldExpenseWriteAccess(ctx context.Context, input HoldExpenseWriteAccessInput, hold func(HoldExpenseWriteAccessOutput) error) error
	// SetFrozen changes the organization freeze state for an owner or admin.
	SetFrozen(ctx context.Context, input SetFrozenInput) error
	UpdateOrganization(ctx context.Context, input UpdateInput) (UpdateOutput, error)
	DeleteOrganization(ctx context.Context, input DeleteInput) (DeleteOutput, error)
	Transfer(ctx context.Context, input TransferInput) (TransferOutput, error)
}

type organizationService struct {
	repo repository.Store
	log  *slog.Logger
}

func NewOrganizationService(repo repository.Store, log *slog.Logger) OrganizationService {
	return &organizationService{repo: repo, log: log}
}

func (s *organizationService) ListOrganizations(ctx context.Context, input ListInput) (ListOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.ListOrganizations")
	defer span.End()

	organizationRows, err := s.repo.ListUserOrganizations(ctx, input.UserID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list organizations failed")
		s.log.ErrorContext(ctx, "list organizations failed", "error", err, "user_id", input.UserID)
		return ListOutput{}, service.ErrInternal
	}

	organizations := make([]OrganizationOutput, 0, len(organizationRows))
	for _, organizationRow := range organizationRows {
		organizations = append(organizations, OrganizationOutput{
			ID:       organizationRow.OrganizationID,
			Name:     organizationRow.Name,
			Type:     organizationRow.Type,
			Role:     organizationRow.Role,
			IsFrozen: organizationRow.FrozenAt.Valid,
		})
	}

	span.SetAttributes(
		attribute.String("user.id", input.UserID.String()),
		attribute.Int("organization.count", len(organizations)),
	)

	return ListOutput{Organizations: organizations}, nil
}

func (s *organizationService) CreateOrganization(ctx context.Context, input CreateInput) (CreateOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.CreateOrganization")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", input.UserID.String()),
		attribute.String("organization.type", input.Type),
	)

	if input.Type != constant.OrganizationTypePersonal && input.Type != constant.OrganizationTypeBusiness {
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

		event := OrganizationCreatedEvent{
			OrganizationID: organization.ID,
			UserID:         input.UserID,
			Name:           organization.Name,
			Type:           organization.Type,
			Role:           constant.RoleOwner,
		}
		err = service.Emit(ctx, q, organizationAggregateType, input.UserID, EventOrganizationCreated, organizationEventVersion, constant.TopicAudit, event)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create organization failed")
		s.log.ErrorContext(ctx, "create organization failed", "error", err, "user_id", input.UserID, "organization_id", organization.ID)
		return CreateOutput{}, service.ErrInternal
	}

	span.SetAttributes(attribute.String("organization.id", organization.ID.String()))

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
			span.SetStatus(codes.Error, "organization not found")
			return GetOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get organization failed")
		s.log.ErrorContext(ctx, "get organization failed", "error", err, "user_id", input.UserID, "organization_id", input.OrganizationID)
		return GetOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", organization.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("organization.type", organization.OrganizationType),
		attribute.String("organization.role", organization.Role),
	)

	return GetOutput{
		Organization: OrganizationOutput{
			ID:       organization.OrganizationID,
			Name:     organization.OrganizationName,
			Type:     organization.OrganizationType,
			Role:     organization.Role,
			IsFrozen: organization.FrozenAt.Valid,
		},
	}, nil
}

func (s *organizationService) UpdateOrganization(ctx context.Context, input UpdateInput) (UpdateOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.UpdateOrganization")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
	)

	var (
		organizationID     uuid.UUID
		organizationName   string
		organizationType   string
		memberRole         string
		organizationFrozen bool
	)

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		currentMember, err := q.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
			UserID:         input.UserID,
			OrganizationID: input.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return service.ErrNotFound
			}
			return err
		}

		if currentMember.Role != constant.RoleOwner && currentMember.Role != constant.RoleAdmin {
			return service.ErrForbidden
		}

		organization, err := q.UpdateOrganization(ctx, repository.UpdateOrganizationParams{
			Name: input.Name,
			ID:   input.OrganizationID,
		})
		if err != nil {
			return err
		}

		organizationID = organization.ID
		organizationName = organization.Name
		organizationType = organization.Type
		memberRole = currentMember.Role
		organizationFrozen = currentMember.FrozenAt.Valid

		event := OrganizationUpdatedEvent{
			OrganizationID: organization.ID,
			UserID:         input.UserID,
			Name:           organization.Name,
			Type:           organization.Type,
			Role:           currentMember.Role,
		}
		err = service.Emit(ctx, q, organizationAggregateType, input.UserID, EventOrganizationUpdated, organizationEventVersion, constant.TopicAudit, event)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			span.SetStatus(codes.Error, "organization member not found")
			return UpdateOutput{}, service.ErrNotFound
		}

		if errors.Is(err, service.ErrForbidden) {
			span.SetStatus(codes.Error, "insufficient organization role")
			return UpdateOutput{}, service.ErrForbidden
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "update organization failed")
		s.log.ErrorContext(ctx, "update organization failed", "error", err, "user_id", input.UserID, "organization_id", input.OrganizationID)
		return UpdateOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.type", organizationType),
		attribute.String("organization.role", memberRole),
	)

	return UpdateOutput{
		Organization: OrganizationOutput{
			ID:       organizationID,
			Name:     organizationName,
			Type:     organizationType,
			Role:     memberRole,
			IsFrozen: organizationFrozen,
		},
	}, nil
}

func (s *organizationService) DeleteOrganization(ctx context.Context, input DeleteInput) (DeleteOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.DeleteOrganization")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
	)

	var (
		organizationType string
		replacementID    *uuid.UUID
	)

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		if _, err := q.GetOrganizationStatusForUpdate(ctx, input.OrganizationID); err != nil {
			return err
		}

		currentMember, err := q.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
			UserID:         input.UserID,
			OrganizationID: input.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return service.ErrNotFound
			}
			return err
		}

		if currentMember.Role != constant.RoleOwner {
			return service.ErrForbidden
		}

		organizationType = currentMember.OrganizationType

		rows, err := q.SoftDeleteOrganization(ctx, input.OrganizationID)
		if err != nil {
			return err
		}

		if rows == 0 {
			return pgx.ErrNoRows
		}

		if organizationType == constant.OrganizationTypePersonal {
			newOrganization, err := q.CreateOrganization(ctx, repository.CreateOrganizationParams{
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

			replacementID = &newOrganization.ID
		}

		event := OrganizationDeletedEvent{
			OrganizationID:            input.OrganizationID,
			UserID:                    input.UserID,
			Type:                      organizationType,
			ReplacementOrganizationID: replacementID,
		}
		err = service.Emit(ctx, q, organizationAggregateType, input.UserID, EventOrganizationDeleted, organizationEventVersion, constant.TopicAudit, event)
		if err != nil {
			return err
		}

		members, err := q.ListOrganizationMembers(ctx, input.OrganizationID)
		if err != nil {
			return err
		}

		for _, member := range members {
			notificationPayload, err := json.Marshal(map[string]string{
				"organization_name": currentMember.OrganizationName,
				"member_name":       member.Name,
			})
			if err != nil {
				return err
			}

			notificationEvent := notificationcontract.CreatedEvent{
				NotificationType: "organization_deleted",
				Category:         notificationcontract.CategoryTransactional,
				RecipientID:      member.UserID.String(),
				Targets: []notificationcontract.Target{
					{
						Channel:     notificationcontract.ChannelEmail,
						Destination: member.Email,
					},
				},
				Payload: notificationPayload,
			}

			if err := notificationEvent.Validate(); err != nil {
				return err
			}

			if err := service.Emit(
				ctx,
				q,
				organizationAggregateType,
				member.UserID,
				notificationcontract.EventTypeCreated,
				organizationEventVersion,
				constant.TopicNotificationTransactional,
				notificationEvent,
			); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization not found")
			return DeleteOutput{}, service.ErrNotFound
		}

		if errors.Is(err, service.ErrForbidden) {
			span.SetStatus(codes.Error, "only organization owner can delete organization")
			return DeleteOutput{}, service.ErrForbidden
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "delete organization failed")
		s.log.ErrorContext(ctx, "delete organization failed", "error", err, "user_id", input.UserID, "organization_id", input.OrganizationID)
		return DeleteOutput{}, service.ErrInternal
	}

	span.SetAttributes(attribute.String("organization.type", organizationType))

	if replacementID != nil {
		span.SetAttributes(attribute.String("replacement_organization.id", replacementID.String()))
	}

	return DeleteOutput{}, nil
}

func (s *organizationService) Transfer(ctx context.Context, input TransferInput) (TransferOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.Transfer")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
	)

	if input.TargetUserID == input.UserID {
		span.SetStatus(codes.Error, "cannot transfer ownership to yourself")
		return TransferOutput{}, service.ErrCannotTransferToSelf
	}

	var output TransferOutput

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		if _, err := q.GetOrganizationStatusForUpdate(ctx, input.OrganizationID); err != nil {
			return err
		}

		currentMember, err := q.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
			UserID:         input.UserID,
			OrganizationID: input.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return service.ErrNotFound
			}
			return err
		}

		if currentMember.Role != constant.RoleOwner {
			return service.ErrForbidden
		}

		targetMember, err := q.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
			UserID:         input.TargetUserID,
			OrganizationID: input.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return service.ErrNotFound
			}
			return err
		}

		rows, err := q.UpdateOrganizationMemberRole(ctx, repository.UpdateOrganizationMemberRoleParams{
			Role:           constant.RoleAdmin,
			OrganizationID: input.OrganizationID,
			UserID:         input.UserID,
		})
		if err != nil {
			return err
		}

		if rows == 0 {
			return pgx.ErrNoRows
		}

		rows, err = q.UpdateOrganizationMemberRole(ctx, repository.UpdateOrganizationMemberRoleParams{
			Role:           constant.RoleOwner,
			OrganizationID: input.OrganizationID,
			UserID:         input.TargetUserID,
		})
		if err != nil {
			return err
		}

		if rows == 0 {
			return pgx.ErrNoRows
		}

		event := OrganizationOwnershipTransferredEvent{
			OrganizationID: input.OrganizationID,
			FromUserID:     input.UserID,
			ToUserID:       input.TargetUserID,
		}
		err = service.Emit(ctx, q, organizationAggregateType, input.OrganizationID, EventOrganizationOwnershipTransferred, organizationEventVersion, constant.TopicAudit, event)
		if err != nil {
			return err
		}

		output = TransferOutput{
			Organization: OrganizationOutput{
				ID:       input.OrganizationID,
				Name:     targetMember.OrganizationName,
				Type:     targetMember.OrganizationType,
				Role:     constant.RoleOwner,
				IsFrozen: currentMember.FrozenAt.Valid,
			},
			From: TransferUserOutput{
				ID:   currentMember.UserID,
				Name: currentMember.UserName,
				Role: constant.RoleAdmin,
			},
			To: TransferUserOutput{
				ID:   targetMember.UserID,
				Name: targetMember.UserName,
				Role: constant.RoleOwner,
			},
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, service.ErrCannotTransferToSelf) {
			return TransferOutput{}, service.ErrCannotTransferToSelf
		}

		if errors.Is(err, service.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization or member not found")
			return TransferOutput{}, service.ErrNotFound
		}

		if errors.Is(err, service.ErrForbidden) {
			span.SetStatus(codes.Error, "only organization owner can transfer ownership")
			return TransferOutput{}, service.ErrForbidden
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "transfer ownership failed")
		s.log.ErrorContext(ctx, "transfer ownership failed", "error", err, "user_id", input.UserID, "target_user_id", input.TargetUserID, "organization_id", input.OrganizationID)
		return TransferOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("from_user.id", output.From.ID.String()),
		attribute.String("to_user.id", output.To.ID.String()),
	)

	return output, nil
}

func (s *organizationService) GetOrganizationStatus(
	ctx context.Context,
	input GetOrganizationStatusInput,
) (GetOrganizationStatusOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationService.GetOrganizationStatus")
	defer span.End()

	row, err := s.repo.GetOrganizationByID(ctx, input.OrganizationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization not found")
			return GetOrganizationStatusOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get organization status failed")
		s.log.ErrorContext(ctx, "get organization status failed", "error", err, "organization_id", input.OrganizationID)

		return GetOrganizationStatusOutput{}, service.ErrInternal
	}

	span.SetAttributes(attribute.String("organization.id", input.OrganizationID.String()))

	return GetOrganizationStatusOutput{
		IsFrozen:  row.FrozenAt.Valid,
		IsDeleted: row.DeletedAt.Valid,
	}, nil
}

func (s *organizationService) HoldExpenseWriteAccess(
	ctx context.Context,
	input HoldExpenseWriteAccessInput,
	hold func(HoldExpenseWriteAccessOutput) error,
) error {
	ctx, span := tracer.Start(ctx, "OrganizationService.HoldExpenseWriteAccess")
	defer span.End()

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		status, err := q.GetOrganizationStatusForShare(ctx, input.OrganizationID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		output := HoldExpenseWriteAccessOutput{IsDeleted: errors.Is(err, pgx.ErrNoRows)}
		if err == nil {
			output.IsDeleted = status.DeletedAt.Valid
			output.IsFrozen = status.FrozenAt.Valid
		}

		if !output.IsDeleted {
			member, memberErr := q.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
				UserID:         input.UserID,
				OrganizationID: input.OrganizationID,
			})
			if memberErr != nil && !errors.Is(memberErr, pgx.ErrNoRows) {
				return memberErr
			}
			if memberErr == nil {
				output.MemberActive = true
				output.ServiceAllowed = member.Role == constant.RoleOwner || member.Role == constant.RoleAdmin
				if !output.ServiceAllowed {
					output.ServiceAllowed, err = q.IsServiceAccessAllowed(ctx, repository.IsServiceAccessAllowedParams{
						OrganizationID: input.OrganizationID,
						UserID:         input.UserID,
						ServiceName:    "expense",
					})
					if err != nil {
						return err
					}
				}
			}
		}

		span.SetAttributes(attribute.String("organization.id", input.OrganizationID.String()))
		return hold(output)
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hold expense write access failed")
		s.log.ErrorContext(ctx, "hold expense write access failed", "error", err, "organization_id", input.OrganizationID)
		return service.ErrInternal
	}
	return nil
}

func (s *organizationService) SetFrozen(ctx context.Context, input SetFrozenInput) error {
	ctx, span := tracer.Start(ctx, "OrganizationService.SetFrozen")
	defer span.End()

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		// Lock organization before membership so Expense writers drain before the state change.
		status, err := q.GetOrganizationStatusForUpdate(ctx, input.OrganizationID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return service.ErrNotFound
			}
			return err
		}
		if status.DeletedAt.Valid {
			return service.ErrNotFound
		}

		member, err := q.GetUserOrganizationForUpdate(ctx, repository.GetUserOrganizationForUpdateParams{
			UserID:         input.UserID,
			OrganizationID: input.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return service.ErrNotFound
			}
			return err
		}
		if member.Role != constant.RoleOwner && member.Role != constant.RoleAdmin {
			return service.ErrForbidden
		}

		if status.FrozenAt.Valid == input.Frozen {
			return nil
		}

		rows, err := q.SetOrganizationFrozen(ctx, repository.SetOrganizationFrozenParams{
			ID:       input.OrganizationID,
			IsFrozen: input.Frozen,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return service.ErrNotFound
		}

		eventType := EventOrganizationUnfrozen
		if input.Frozen {
			eventType = EventOrganizationFrozen
		}
		return service.Emit(ctx, q, organizationAggregateType, input.OrganizationID, eventType, organizationEventVersion, constant.TopicAudit, OrganizationFreezeEvent{
			OrganizationID: input.OrganizationID,
			UserID:         input.UserID,
		})
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) || errors.Is(err, service.ErrForbidden) {
			return err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "set organization frozen status failed")
		s.log.ErrorContext(ctx, "set organization frozen status failed", "error", err, "user_id", input.UserID, "organization_id", input.OrganizationID)
		return service.ErrInternal
	}

	return nil
}
