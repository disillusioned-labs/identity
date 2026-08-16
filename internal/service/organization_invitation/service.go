package organization_invitation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("service/organization_invitation")

const invitationExpiration = 7 * 24 * time.Hour

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

	if err == nil {
		if pendingInvitation.ExpiresAt.After(now) {
			span.SetStatus(codes.Error, "pending organization invitation already exists")
			return CreateInvitationOutput{}, service.ErrConflict
		}

		_, err = s.repo.ExpireInvitation(ctx, pendingInvitation.ID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "expire previous organization invitation failed")
			s.log.ErrorContext(ctx, "expire previous organization invitation failed", "error", err)
			return CreateInvitationOutput{}, service.ErrInternal
		}
	}

	rawToken, err := generateInvitationToken()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate invitation token failed")
		s.log.ErrorContext(ctx, "generate invitation token failed", "error", err)
		return CreateInvitationOutput{}, service.ErrInternal
	}

	invitation, err := s.repo.CreateInvitation(ctx, repository.CreateInvitationParams{
		OrganizationID: input.OrganizationID,
		Email:          input.Email,
		Role:           input.Role,
		TokenHash:      hashInvitationToken(rawToken),
		InvitedBy:      input.UserID,
		ExpiresAt:      now.Add(invitationExpiration),
	})
	if err != nil {
		if service.IsUniqueViolation(err) {
			span.SetStatus(codes.Error, "organization invitation already exists")
			return CreateInvitationOutput{}, service.ErrConflict
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "create organization invitation failed")
		s.log.ErrorContext(ctx, "create organization invitation failed", "error", err)
		return CreateInvitationOutput{}, service.ErrInternal
	}

	// insert to audit logs
	// publish outbox event

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("user.id", input.UserID.String()),
		attribute.String("invitation.id", invitation.ID.String()),
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

		// insert to audit logs
		// publish outbox event

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

	rows, err := s.repo.RevokeInvitation(ctx, repository.RevokeInvitationParams{
		ID:             input.InvitationID,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke organization invitation failed")
		s.log.ErrorContext(ctx, "revoke organization invitation failed", "error", err)
		return RevokeInvitationOutput{}, service.ErrInternal
	}

	if rows == 0 {
		span.SetStatus(codes.Error, "organization invitation not found")
		return RevokeInvitationOutput{}, service.ErrNotFound
	}

	// insert to audit logs
	// publish outbox event

	span.SetAttributes(
		attribute.String("organization.id", input.OrganizationID.String()),
		attribute.String("invitation.id", input.InvitationID.String()),
		attribute.String("user.id", input.UserID.String()),
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
