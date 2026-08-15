package organization_member

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/disillusioned-labs/identity/internal/handler"
	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
)

var tracer = otel.Tracer("handler/organization_member")

type OrganizationMemberHandler struct {
	service organizationmemberservice.OrganizationMemberService
	log     *slog.Logger
}

func NewOrganizationMemberHandler(
	service organizationmemberservice.OrganizationMemberService,
	log *slog.Logger,
) *OrganizationMemberHandler {
	return &OrganizationMemberHandler{
		service: service,
		log:     log,
	}
}

func (h *OrganizationMemberHandler) listOrganizationMembers(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx, span := tracer.Start(r.Context(), "OrganizationMemberHandler.listOrganizationMembers")
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

	output, err := h.service.ListOrganizationMembers(
		ctx,
		organizationmemberservice.ListOrganizationMembersInput{
			UserID:         userID,
			OrganizationID: organizationID,
		},
	)
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
	)

	handler.OK(
		w,
		http.StatusOK,
		toListOrganizationMembersResponse(output),
	)
}

func (h *OrganizationMemberHandler) updateOrganizationMemberRole(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx, span := tracer.Start(r.Context(), "OrganizationMemberHandler.updateOrganizationMemberRole")
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

	targetUserID, ok := parseTargetUserID(w, r)
	if !ok {
		return
	}

	req, ok := handler.DecodeValid[UpdateOrganizationMemberRoleRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.UpdateOrganizationMemberRole(
		ctx,
		organizationmemberservice.UpdateOrganizationMemberRoleInput{
			UserID:         userID,
			OrganizationID: organizationID,
			TargetUserID:   targetUserID,
			Role:           req.Role,
		},
	)
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
		attribute.String("target_user.id", targetUserID.String()),
	)

	handler.OK(
		w,
		http.StatusOK,
		toUpdateOrganizationMemberRoleResponse(output),
	)
}

func (h *OrganizationMemberHandler) removeOrganizationMember(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx, span := tracer.Start(r.Context(), "OrganizationMemberHandler.removeOrganizationMember")
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

	targetUserID, ok := parseTargetUserID(w, r)
	if !ok {
		return
	}

	output, err := h.service.RemoveOrganizationMember(
		ctx,
		organizationmemberservice.RemoveOrganizationMemberInput{
			UserID:         userID,
			OrganizationID: organizationID,
			TargetUserID:   targetUserID,
		},
	)
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
		attribute.String("target_user.id", targetUserID.String()),
	)

	handler.OK(
		w,
		http.StatusOK,
		toRemoveOrganizationMemberResponse(output),
	)
}

func (h *OrganizationMemberHandler) leaveOrganization(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx, span := tracer.Start(r.Context(), "OrganizationMemberHandler.leaveOrganization")
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

	output, err := h.service.LeaveOrganization(
		ctx,
		organizationmemberservice.LeaveOrganizationInput{
			UserID:         userID,
			OrganizationID: organizationID,
		},
	)
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
	)

	handler.OK(
		w,
		http.StatusOK,
		toLeaveOrganizationResponse(output),
	)
}

func userIDFromClaims(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	claims, ok := handler.ClaimsFrom(r.Context())
	if !ok {
		handler.WriteError(
			w,
			http.StatusUnauthorized,
			handler.CodeUnauthorized,
			"unauthorized",
		)
		return uuid.Nil, false
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		handler.WriteError(
			w,
			http.StatusUnauthorized,
			handler.CodeUnauthorized,
			"unauthorized",
		)
		return uuid.Nil, false
	}

	return userID, true
}

func parseOrganizationID(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	organizationID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		handler.WriteError(
			w,
			http.StatusBadRequest,
			handler.CodeBadRequest,
			"invalid organization id",
		)
		return uuid.Nil, false
	}

	return organizationID, true
}

func parseTargetUserID(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	targetUserID, err := uuid.Parse(r.PathValue("user_id"))
	if err != nil {
		handler.WriteError(
			w,
			http.StatusBadRequest,
			handler.CodeBadRequest,
			"invalid user id",
		)
		return uuid.Nil, false
	}

	return targetUserID, true
}
