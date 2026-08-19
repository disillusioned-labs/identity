package organization_invitation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/disillusioned-labs/identity/internal/constant"
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

var tracer = otel.Tracer("service/organization_invitation")

const invitationExpiration = 7 * 24 * time.Hour

const (
	organizationInvitationAggregateType = "organization_invitation"
	organizationInvitationEventVersion  = 1
)

type OrganizationInvitationService interface {
	ListMyInvitations(ctx context.Context, input ListMyInvitationsInput) (ListMyInvitationsOutput, error)
	CreateInvitation(ctx context.Context, input CreateInvitationInput) (CreateInvitationOutput, error)
	ListInvitations(ctx context.Context, input ListInvitationsInput) (ListInvitationsOutput, error)
	GetInvitation(ctx context.Context, input GetInvitationInput) (GetInvitationOutput, error)
	AcceptInvitation(ctx context.Context, input AcceptInvitationInput) (AcceptInvitationOutput, error)
	AcceptInvitationByToken(ctx context.Context, input AcceptInvitationByTokenInput) (AcceptInvitationOutput, error)
	RevokeInvitation(ctx context.Context, input RevokeInvitationInput) (RevokeInvitationOutput, error)
}

type organizationInvitationService struct {
	repo repository.Store
	log  *slog.Logger
}

func NewOrganizationInvitationService(repo repository.Store, log *slog.Logger) OrganizationInvitationService {
	return &organizationInvitationService{
		repo: repo,
		log:  log,
	}
}

func (s *organizationInvitationService) ListMyInvitations(ctx context.Context, input ListMyInvitationsInput) (ListMyInvitationsOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationInvitationService.ListMyInvitations")
	defer span.End()

	user, err := s.repo.GetUserByID(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ListMyInvitationsOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get invitation user failed")
		s.log.ErrorContext(ctx, "get invitation user failed", "error", err)
		return ListMyInvitationsOutput{}, service.ErrInternal
	}

	rows, err := s.repo.ListMyPendingOrganizationInvitations(ctx, normalizeEmail(user.Email))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list my organization invitations failed")
		s.log.ErrorContext(ctx, "list my organization invitations failed", "error", err)
		return ListMyInvitationsOutput{}, service.ErrInternal
	}

	invitations := make([]MyInvitationOutput, 0, len(rows))

	for _, row := range rows {
		invitations = append(invitations, MyInvitationOutput{
			ID:               row.ID,
			OrganizationID:   row.OrganizationID,
			OrganizationName: row.OrganizationName,
			Role:             row.Role,
			Status:           row.Status,
			ExpiresAt:        row.ExpiresAt,
			CreatedAt:        row.CreatedAt,
		})
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	return ListMyInvitationsOutput{
		Invitations: invitations,
	}, nil
}

func (s *organizationInvitationService) CreateInvitation(ctx context.Context, input CreateInvitationInput) (CreateInvitationOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationInvitationService.CreateInvitation")
	defer span.End()

	if input.Role != constant.RoleAdmin && input.Role != constant.RoleMember {
		span.SetStatus(codes.Error, "invalid invitation role")
		return CreateInvitationOutput{}, service.ErrInvalidRole
	}

	input.Email = normalizeEmail(input.Email)

	if input.Email == "" {
		span.SetStatus(codes.Error, "invalid invitation email")
		return CreateInvitationOutput{}, service.ErrInvalidEmail
	}

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization member not found")
			return CreateInvitationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)
		return CreateInvitationOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner && currentMember.Role != constant.RoleAdmin {
		span.SetStatus(codes.Error, "insufficient organization role")
		return CreateInvitationOutput{}, service.ErrForbidden
	}

	_, err = s.repo.GetUserOrganizationByEmail(ctx, repository.GetUserOrganizationByEmailParams{
		OrganizationID: input.OrganizationID,
		Email:          input.Email,
	})
	if err == nil {
		span.SetStatus(codes.Error, "user is already an organization member")
		return CreateInvitationOutput{}, service.ErrConflict
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check existing organization member failed")
		s.log.ErrorContext(ctx, "check existing organization member failed", "error", err)
		return CreateInvitationOutput{}, service.ErrInternal
	}

	pendingInvitation, err := s.repo.GetPendingOrganizationInvitation(ctx, repository.GetPendingOrganizationInvitationParams{
		OrganizationID: input.OrganizationID,
		Email:          input.Email,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get pending organization invitation failed")
		s.log.ErrorContext(ctx, "get pending organization invitation failed", "error", err)
		return CreateInvitationOutput{}, service.ErrInternal
	}

	now := time.Now()

	if err == nil && pendingInvitation.ExpiresAt.After(now) {
		span.SetStatus(codes.Error, "pending organization invitation already exists")
		return CreateInvitationOutput{}, service.ErrConflict
	}

	rawToken, err := generateInvitationToken()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate invitation token failed")
		s.log.ErrorContext(ctx, "generate invitation token failed", "error", err)
		return CreateInvitationOutput{}, service.ErrInternal
	}

	expiresAt := now.Add(invitationExpiration)

	var invitation repository.OrganizationInvitation

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		// Keep expiration and creation in the same transaction as the
		// outbox event so they cannot partially succeed.
		if err == nil {
			_, txErr := q.ExpireInvitation(ctx, pendingInvitation.ID)
			if txErr != nil {
				return txErr
			}
		}

		createdInvitation, txErr := q.CreateInvitation(ctx, repository.CreateInvitationParams{
			OrganizationID: input.OrganizationID,
			Email:          input.Email,
			Role:           input.Role,
			TokenHash:      hashInvitationToken(rawToken),
			InvitedBy:      input.UserID,
			ExpiresAt:      expiresAt,
		})
		if txErr != nil {
			return txErr
		}

		event := OrganizationInvitationCreatedEvent{
			InvitationID:   createdInvitation.ID,
			OrganizationID: createdInvitation.OrganizationID,
			Email:          createdInvitation.Email,
			Role:           createdInvitation.Role,
			InvitedBy:      createdInvitation.InvitedBy,
			ExpiresAt:      createdInvitation.ExpiresAt.Format(time.RFC3339),
		}

		if txErr := createOrganizationInvitationOutboxEvent(
			ctx,
			q,
			createdInvitation.ID,
			EventOrganizationInvitationCreated,
			event,
		); txErr != nil {
			return txErr
		}

		invitation = createdInvitation

		return nil
	})
	if err != nil {
		if service.IsUniqueViolation(err) {
			span.SetStatus(codes.Error, "organization invitation already exists")
			return CreateInvitationOutput{}, service.ErrConflict
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "create organization invitation transaction failed")
		s.log.ErrorContext(ctx, "create organization invitation transaction failed",
			"error", err,
			"organization_id", input.OrganizationID,
			"user_id", input.UserID,
			"event_type", EventOrganizationInvitationCreated,
		)
		return CreateInvitationOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("invitation.id", invitation.ID.String()),
		attribute.String("invitation.role", invitation.Role),
		attribute.String("event.type", EventOrganizationInvitationCreated),
	)

	var acceptedAt *time.Time

	if invitation.AcceptedAt.Valid {
		acceptedAt = &invitation.AcceptedAt.Time
	}

	return CreateInvitationOutput{
		Invitation: InvitationOutput{
			ID:             invitation.ID,
			OrganizationID: invitation.OrganizationID,
			Email:          invitation.Email,
			Role:           invitation.Role,
			Status:         invitation.Status,
			ExpiresAt:      invitation.ExpiresAt,
			AcceptedAt:     acceptedAt,
			AcceptedBy:     invitation.AcceptedBy,
			InvitedBy:      invitation.InvitedBy,
			CreatedAt:      invitation.CreatedAt,
		},
		Token: rawToken,
	}, nil
}

func (s *organizationInvitationService) ListInvitations(ctx context.Context, input ListInvitationsInput) (ListInvitationsOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationInvitationService.ListInvitations")
	defer span.End()

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization member not found")
			return ListInvitationsOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)
		return ListInvitationsOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner && currentMember.Role != constant.RoleAdmin {
		span.SetStatus(codes.Error, "insufficient organization role")
		return ListInvitationsOutput{}, service.ErrForbidden
	}

	rows, err := s.repo.ListInvitations(ctx, input.OrganizationID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list organization invitations failed")
		s.log.ErrorContext(ctx, "list organization invitations failed", "error", err)
		return ListInvitationsOutput{}, service.ErrInternal
	}

	invitations := make([]InvitationOutput, 0, len(rows))

	for _, row := range rows {
		var acceptedAt *time.Time

		if row.AcceptedAt.Valid {
			acceptedAt = &row.AcceptedAt.Time
		}

		invitations = append(invitations, InvitationOutput{
			ID:             row.ID,
			OrganizationID: row.OrganizationID,
			Email:          row.Email,
			Role:           row.Role,
			Status:         row.Status,
			ExpiresAt:      row.ExpiresAt,
			AcceptedAt:     acceptedAt,
			AcceptedBy:     row.AcceptedBy,
			InvitedBy:      row.InvitedBy,
			InvitedByName:  row.InvitedByName,
			CreatedAt:      row.CreatedAt,
		})
	}

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
	)

	return ListInvitationsOutput{
		Invitations: invitations,
	}, nil
}

func (s *organizationInvitationService) GetInvitation(ctx context.Context, input GetInvitationInput) (GetInvitationOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationInvitationService.GetInvitation")
	defer span.End()

	token := strings.TrimSpace(input.Token)

	if token == "" {
		span.SetStatus(codes.Error, "invalid invitation token")
		return GetInvitationOutput{}, service.ErrNotFound
	}

	invitation, err := s.repo.GetInvitationByTokenHash(ctx, hashInvitationToken(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization invitation not found")
			return GetInvitationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get organization invitation failed")
		s.log.ErrorContext(ctx, "get organization invitation failed", "error", err)
		return GetInvitationOutput{}, service.ErrInternal
	}

	if err := s.validateInvitation(ctx, span, invitation.Status, invitation.ID, invitation.ExpiresAt); err != nil {
		return GetInvitationOutput{}, err
	}

	userExists, err := s.repo.UserExistsByEmail(ctx, invitation.Email)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check invitation user failed")
		s.log.ErrorContext(ctx, "check invitation user failed", "error", err)
		return GetInvitationOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", invitation.OrganizationID.String()),
		attribute.String("invitation.id", invitation.ID.String()),
	)

	return GetInvitationOutput{
		Invitation: InvitationDetailOutput{
			OrganizationName:     invitation.OrganizationName,
			InvitedByName:        invitation.InvitedByName,
			Role:                 invitation.Role,
			ExpiresAt:            invitation.ExpiresAt,
			RequiresRegistration: !userExists,
		},
	}, nil
}

func (s *organizationInvitationService) AcceptInvitation(ctx context.Context, input AcceptInvitationInput) (AcceptInvitationOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationInvitationService.AcceptInvitation")
	defer span.End()

	invitation, err := s.repo.GetOrganizationInvitation(ctx, input.InvitationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization invitation not found")
			return AcceptInvitationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get organization invitation failed")
		s.log.ErrorContext(ctx, "get organization invitation failed", "error", err)
		return AcceptInvitationOutput{}, service.ErrInternal
	}

	return s.acceptInvitation(
		ctx,
		span,
		input.UserID,
		invitation.ID,
		invitation.OrganizationID,
		invitation.Email,
		invitation.Role,
		invitation.Status,
		invitation.ExpiresAt,
	)
}

func (s *organizationInvitationService) AcceptInvitationByToken(ctx context.Context, input AcceptInvitationByTokenInput) (AcceptInvitationOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationInvitationService.AcceptInvitationByToken")
	defer span.End()

	token := strings.TrimSpace(input.Token)

	if token == "" {
		span.SetStatus(codes.Error, "invalid invitation token")
		return AcceptInvitationOutput{}, service.ErrNotFound
	}

	invitation, err := s.repo.GetInvitationByTokenHash(ctx, hashInvitationToken(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization invitation not found")
			return AcceptInvitationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get organization invitation failed")
		s.log.ErrorContext(ctx, "get organization invitation failed", "error", err)
		return AcceptInvitationOutput{}, service.ErrInternal
	}

	return s.acceptInvitation(
		ctx,
		span,
		input.UserID,
		invitation.ID,
		invitation.OrganizationID,
		invitation.Email,
		invitation.Role,
		invitation.Status,
		invitation.ExpiresAt,
	)
}

func (s *organizationInvitationService) acceptInvitation(
	ctx context.Context,
	span trace.Span,
	userID uuid.UUID,
	invitationID uuid.UUID,
	organizationID uuid.UUID,
	invitationEmail string,
	invitationRole string,
	invitationStatus string,
	expiresAt time.Time,
) (AcceptInvitationOutput, error) {
	if err := s.validateInvitation(ctx, span, invitationStatus, invitationID, expiresAt); err != nil {
		return AcceptInvitationOutput{}, err
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "accepting user not found")
			return AcceptInvitationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get accepting user failed")
		s.log.ErrorContext(ctx, "get accepting user failed", "error", err)
		return AcceptInvitationOutput{}, service.ErrInternal
	}

	if normalizeEmail(user.Email) != normalizeEmail(invitationEmail) {
		span.SetStatus(codes.Error, "invitation email does not match user email")
		return AcceptInvitationOutput{}, service.ErrForbidden
	}

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		_, err := q.CreateOrganizationMember(ctx, repository.CreateOrganizationMemberParams{
			OrganizationID: organizationID,
			UserID:         userID,
			Role:           invitationRole,
		})
		if err != nil {
			return err
		}

		rows, err := q.AcceptInvitation(ctx, repository.AcceptInvitationParams{
			ID:         invitationID,
			AcceptedBy: &userID,
		})
		if err != nil {
			return err
		}

		if rows == 0 {
			return service.ErrConflict
		}

		event := OrganizationInvitationAcceptedEvent{
			InvitationID:   invitationID,
			OrganizationID: organizationID,
			UserID:         userID,
			Role:           invitationRole,
		}

		if err := createOrganizationInvitationOutboxEvent(
			ctx,
			q,
			invitationID,
			EventOrganizationInvitationAccepted,
			event,
		); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, service.ErrConflict) {
			span.SetStatus(codes.Error, "invitation acceptance conflict")
			return AcceptInvitationOutput{}, service.ErrConflict
		}

		if service.IsUniqueViolation(err) {
			span.SetStatus(codes.Error, "user is already an organization member")
			return AcceptInvitationOutput{}, service.ErrConflict
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "accept organization invitation failed")
		s.log.ErrorContext(ctx, "accept organization invitation failed", "error", err)
		return AcceptInvitationOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("invitation.id", invitationID.String()),
		attribute.String("user.id", userID.String()),
		attribute.String("invitation.role", invitationRole),
		attribute.String("event.type", EventOrganizationInvitationAccepted),
	)

	return AcceptInvitationOutput{
		OrganizationID: organizationID,
		Role:           invitationRole,
	}, nil
}

func (s *organizationInvitationService) RevokeInvitation(ctx context.Context, input RevokeInvitationInput) (RevokeInvitationOutput, error) {
	ctx, span := tracer.Start(ctx, "OrganizationInvitationService.RevokeInvitation")
	defer span.End()

	currentMember, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "organization member not found")
			return RevokeInvitationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get current organization member failed")
		s.log.ErrorContext(ctx, "get current organization member failed", "error", err)
		return RevokeInvitationOutput{}, service.ErrInternal
	}

	if currentMember.Role != constant.RoleOwner && currentMember.Role != constant.RoleAdmin {
		span.SetStatus(codes.Error, "insufficient organization role")
		return RevokeInvitationOutput{}, service.ErrForbidden
	}

	err = s.repo.ExecTx(ctx, func(q repository.Querier) error {
		rows, err := q.RevokeInvitation(ctx, repository.RevokeInvitationParams{
			ID:             input.InvitationID,
			OrganizationID: input.OrganizationID,
		})
		if err != nil {
			return err
		}

		if rows == 0 {
			return service.ErrNotFound
		}

		event := OrganizationInvitationRevokedEvent{
			InvitationID:   input.InvitationID,
			OrganizationID: input.OrganizationID,
			RevokedBy:      input.UserID,
		}

		if err := createOrganizationInvitationOutboxEvent(
			ctx,
			q,
			input.InvitationID,
			EventOrganizationInvitationRevoked,
			event,
		); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			span.SetStatus(codes.Error, "organization invitation not found")
			return RevokeInvitationOutput{}, service.ErrNotFound
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke organization invitation transaction failed")
		s.log.ErrorContext(ctx, "revoke organization invitation transaction failed",
			"error", err,
			"event_type", EventOrganizationInvitationRevoked,
			"invitation_id", input.InvitationID,
			"organization_id", input.OrganizationID,
			"user_id", input.UserID,
		)
		return RevokeInvitationOutput{}, service.ErrInternal
	}

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("invitation.id", input.InvitationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("event.type", EventOrganizationInvitationRevoked),
	)

	return RevokeInvitationOutput{}, nil
}

func (s *organizationInvitationService) validateInvitation(ctx context.Context, span trace.Span, status string, invitationID uuid.UUID, expiresAt time.Time) error {
	switch status {
	case "revoked":
		span.SetStatus(codes.Error, "invitation has been revoked")
		return service.ErrInvitationRevoked

	case "accepted":
		span.SetStatus(codes.Error, "invitation has already been accepted")
		return service.ErrInvitationAlreadyAccepted

	case "expired":
		span.SetStatus(codes.Error, "invitation has expired")
		return service.ErrInvitationExpired

	case "pending":
		if expiresAt.After(time.Now()) {
			return nil
		}

		_, err := s.repo.ExpireInvitation(ctx, invitationID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "expire organization invitation failed")
			s.log.ErrorContext(ctx, "expire organization invitation failed", "error", err)
			return service.ErrInternal
		}

		span.SetStatus(codes.Error, "invitation has expired")
		return service.ErrInvitationExpired

	default:
		span.SetStatus(codes.Error, "invalid organization invitation status")
		return service.ErrInternal
	}
}

func createOrganizationInvitationOutboxEvent(
	ctx context.Context,
	q repository.Querier,
	aggregateID uuid.UUID,
	eventType string,
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
		AggregateType: organizationInvitationAggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		EventVersion:  organizationInvitationEventVersion,
		Payload:       payload,
		TraceID:       traceID,
	})
	return err
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func generateInvitationToken() (string, error) {
	buf := make([]byte, 32)

	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}

func hashInvitationToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
