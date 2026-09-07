package service_access

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/disillusioned-labs/identity/internal/handler"
	serviceservice "github.com/disillusioned-labs/identity/internal/service/service_access"
)

var tracer = otel.Tracer("handler/service_access")

type ServiceAccessHandler struct {
	service serviceservice.ServiceAccessService
	log     *slog.Logger
}

func NewServiceAccessHandler(
	service serviceservice.ServiceAccessService,
	log *slog.Logger,
) *ServiceAccessHandler {
	return &ServiceAccessHandler{
		service: service,
		log:     log,
	}
}

func (h *ServiceAccessHandler) grantAccess(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "ServiceAccessHandler.grantAccess")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r, span)
	if !ok {
		return
	}

	organizationID, ok := parseOrganizationID(w, r, span)
	if !ok {
		return
	}

	targetUserID, ok := parseTargetUserID(w, r, span)
	if !ok {
		return
	}

	req, ok := handler.DecodeValid[GrantAccessRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	if err := h.service.GrantAccess(r.Context(), serviceservice.GrantAccessInput{
		OrganizationID: organizationID,
		UserID:         targetUserID,
		ServiceName:    req.ServiceName,
		GrantedBy:      userID,
	}); err != nil {
		span.SetStatus(codes.Error, "grant access failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	handler.OK(w, http.StatusOK, GrantAccessResponse{Granted: true})
}

func (h *ServiceAccessHandler) revokeAccess(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "ServiceAccessHandler.revokeAccess")
	defer span.End()
	r = r.WithContext(ctx)

	organizationID, ok := parseOrganizationID(w, r, span)
	if !ok {
		return
	}

	targetUserID, ok := parseTargetUserID(w, r, span)
	if !ok {
		return
	}

	req, ok := handler.DecodeValid[RevokeAccessRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	if err := h.service.RevokeAccess(r.Context(), serviceservice.RevokeAccessInput{
		OrganizationID: organizationID,
		UserID:         targetUserID,
		ServiceName:    req.ServiceName,
	}); err != nil {
		span.SetStatus(codes.Error, "revoke access failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	handler.OK(w, http.StatusOK, RevokeAccessResponse{Revoked: true})
}

func (h *ServiceAccessHandler) listAccessByOrgUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "ServiceAccessHandler.listAccessByOrgUser")
	defer span.End()
	r = r.WithContext(ctx)

	organizationID, ok := parseOrganizationID(w, r, span)
	if !ok {
		return
	}

	targetUserID, ok := parseTargetUserID(w, r, span)
	if !ok {
		return
	}

	output, err := h.service.ListAccessByOrgUser(r.Context(), serviceservice.ListAccessByOrgUserInput{
		OrganizationID: organizationID,
		UserID:         targetUserID,
	})
	if err != nil {
		span.SetStatus(codes.Error, "list access failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	handler.OK(w, http.StatusOK, toListAccessResponse(output))
}

func (h *ServiceAccessHandler) listAccessByOrg(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "ServiceAccessHandler.listAccessByOrg")
	defer span.End()
	r = r.WithContext(ctx)

	organizationID, ok := parseOrganizationID(w, r, span)
	if !ok {
		return
	}

	output, err := h.service.ListAccessByOrg(r.Context(), serviceservice.ListAccessByOrgInput{
		OrganizationID: organizationID,
	})
	if err != nil {
		span.SetStatus(codes.Error, "list access failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	handler.OK(w, http.StatusOK, toListAccessResponse(output))
}

func userIDFromClaims(
	w http.ResponseWriter,
	r *http.Request,
	span trace.Span,
) (uuid.UUID, bool) {
	claims, ok := handler.ClaimsFrom(r.Context())
	if !ok {
		span.SetStatus(codes.Error, "get claims failed")
		handler.WriteError(w, http.StatusUnauthorized, handler.CodeUnauthorized, "unauthorized")
		return uuid.Nil, false
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid user id in claims")
		handler.WriteError(w, http.StatusUnauthorized, handler.CodeUnauthorized, "unauthorized")
		return uuid.Nil, false
	}

	return userID, true
}

func parseOrganizationID(
	w http.ResponseWriter,
	r *http.Request,
	span trace.Span,
) (uuid.UUID, bool) {
	organizationID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid organization id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid organization id")
		return uuid.Nil, false
	}

	return organizationID, true
}

func parseTargetUserID(
	w http.ResponseWriter,
	r *http.Request,
	span trace.Span,
) (uuid.UUID, bool) {
	targetUserID, err := uuid.Parse(r.PathValue("user_id"))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid target user id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid user id")
		return uuid.Nil, false
	}

	return targetUserID, true
}
