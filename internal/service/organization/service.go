package organization

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

var tracer = otel.Tracer("service/organization")

const (
	organizationAggregateType = "organization"
	organizationEventVersion  = 1
)

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
			ID:   organizationRow.OrganizationID,
			Name: organizationRow.Name,
			Type: organizationRow.Type,
			Role: organizationRow.Role,
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
		err = createOrganizationOutboxEvent(ctx, q, input.UserID, EventOrganizationCreated, constant.TopicAudit, event)
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

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
	)

	var (
		organizationID   uuid.UUID
		organizationName string
		organizationType string
		memberRole       string
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

		event := OrganizationUpdatedEvent{
			OrganizationID: organization.ID,
			UserID:         input.UserID,
			Name:           organization.Name,
			Type:           organization.Type,
			Role:           currentMember.Role,
		}
		err = createOrganizationOutboxEvent(ctx, q, input.UserID, EventOrganizationUpdated, constant.TopicAudit, event)
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
			ID:   organizationID,
			Name: organizationName,
			Type: organizationType,
			Role: memberRole,
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
		err = createOrganizationOutboxEvent(ctx, q, input.UserID, EventOrganizationDeleted, constant.TopicAudit, event)
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

			if err := createOrganizationOutboxEvent(
				ctx,
				q,
				member.UserID,
				notificationcontract.EventTypeCreated,
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

	span.SetAttributes(
		attribute.String("organization.type", organizationType),
	)

	if replacementID != nil {
		span.SetAttributes(attribute.String("replacement_organization.id", replacementID.String()))
	}

	return DeleteOutput{}, nil
}

func createOrganizationOutboxEvent(
	ctx context.Context,
	q repository.Querier,
	aggregateID uuid.UUID,
	eventType string,
	topic string,
	event any,
) error {
	payload, err := json.Marshal(event)
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

	_, err = q.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
		AggregateType: organizationAggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		EventVersion:  organizationEventVersion,
		Topic:         topic,
		Payload:       payload,
		TraceID:       traceID,
	})
	return err
}
