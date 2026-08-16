package organization_invitation

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/disillusioned-labs/identity/internal/handler"
	organizationinvitationservice "github.com/disillusioned-labs/identity/internal/service/organization_invitation"
)

var tracer = otel.Tracer("handler/organization_invitation")

type OrganizationInvitationHandler struct {
	service organizationinvitationservice.OrganizationInvitationService
	log     *slog.Logger
}

func NewOrganizationInvitationHandler(service organizationinvitationservice.OrganizationInvitationService, log *slog.Logger) *OrganizationInvitationHandler {
	return &OrganizationInvitationHandler{
		service: service,
		log:     log,
	}
}

func (h *OrganizationInvitationHandler) listMyInvitations(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationInvitationHandler.listMyInvitations")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		return
	}

	output, err := h.service.ListMyInvitations(ctx, organizationinvitationservice.ListMyInvitationsInput{
		UserID: userID,
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(attribute.String("user.id", userID.String()))

	handler.OK(w, http.StatusOK, toListMyInvitationsResponse(output))
}

func (h *OrganizationInvitationHandler) listInvitations(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationInvitationHandler.listInvitations")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		return
	}

	organizationID, ok := parseOrganizationID(w, r)
	if !ok {
		return
	}

	output, err := h.service.ListInvitations(ctx, organizationinvitationservice.ListInvitationsInput{
		UserID:         userID,
		OrganizationID: organizationID,
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
	)

	handler.OK(w, http.StatusOK, toListInvitationsResponse(output))
}

func (h *OrganizationInvitationHandler) createInvitation(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationInvitationHandler.createInvitation")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		return
	}

	organizationID, ok := parseOrganizationID(w, r)
	if !ok {
		return
	}

	req, ok := handler.DecodeValid[CreateInvitationRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.CreateInvitation(ctx, organizationinvitationservice.CreateInvitationInput{
		UserID:         userID,
		OrganizationID: organizationID,
		Email:          strings.TrimSpace(req.Email),
		Role:           req.Role,
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
		attribute.String("invitation.id", output.Invitation.ID.String()),
	)

	handler.OK(w, http.StatusCreated, toCreateInvitationResponse(output))
}

func (h *OrganizationInvitationHandler) getInvitation(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationInvitationHandler.getInvitation")
	defer span.End()
	r = r.WithContext(ctx)

	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		span.SetStatus(codes.Error, "missing invitation token")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid invitation token")
		return
	}

	output, err := h.service.GetInvitation(ctx, organizationinvitationservice.GetInvitationInput{
		Token: token,
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	handler.OK(w, http.StatusOK, toGetInvitationResponse(output))
}

func (h *OrganizationInvitationHandler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationInvitationHandler.acceptInvitation")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		return
	}

	invitationID, err := uuid.Parse(r.PathValue("invitation_id"))
	if err != nil {
		span.SetStatus(codes.Error, "invalid invitation id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid invitation id")
		return
	}

	output, err := h.service.AcceptInvitation(ctx, organizationinvitationservice.AcceptInvitationInput{
		UserID:       userID,
		InvitationID: invitationID,
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", output.OrganizationID.String()),
		attribute.String("user.id", userID.String()),
		attribute.String("invitation.id", invitationID.String()),
	)

	handler.OK(w, http.StatusOK, toAcceptInvitationResponse(output))
}

func (h *OrganizationInvitationHandler) acceptInvitationByToken(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationInvitationHandler.acceptInvitationByToken")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		return
	}

	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		span.SetStatus(codes.Error, "missing invitation token")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid invitation token")
		return
	}

	output, err := h.service.AcceptInvitationByToken(ctx, organizationinvitationservice.AcceptInvitationByTokenInput{
		UserID: userID,
		Token:  token,
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", output.OrganizationID.String()),
		attribute.String("user.id", userID.String()),
	)

	handler.OK(w, http.StatusOK, toAcceptInvitationResponse(output))
}

func (h *OrganizationInvitationHandler) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationInvitationHandler.revokeInvitation")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		return
	}

	organizationID, ok := parseOrganizationID(w, r)
	if !ok {
		return
	}

	invitationID, err := uuid.Parse(r.PathValue("invitation_id"))
	if err != nil {
		span.SetStatus(codes.Error, "invalid invitation id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid invitation id")
		return
	}

	output, err := h.service.RevokeInvitation(ctx, organizationinvitationservice.RevokeInvitationInput{
		UserID:         userID,
		OrganizationID: organizationID,
		InvitationID:   invitationID,
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
		attribute.String("invitation.id", invitationID.String()),
	)

	handler.OK(w, http.StatusOK, toRevokeInvitationResponse(output))
}

func userIDFromClaims(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	claims, ok := handler.ClaimsFrom(r.Context())
	if !ok {
		handler.WriteError(w, http.StatusUnauthorized, handler.CodeUnauthorized, "unauthorized")
		return uuid.Nil, false
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		handler.WriteError(w, http.StatusUnauthorized, handler.CodeUnauthorized, "unauthorized")
		return uuid.Nil, false
	}

	return userID, true
}

func parseOrganizationID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	organizationID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid organization id")
		return uuid.Nil, false
	}

	return organizationID, true
}
