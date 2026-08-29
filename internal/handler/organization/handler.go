package organization

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/disillusioned-labs/identity/internal/handler"
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
)

var tracer = otel.Tracer("handler/organization")

type OrganizationHandler struct {
	service organizationservice.OrganizationService
	log     *slog.Logger
}

func NewOrganizationHandler(service organizationservice.OrganizationService, log *slog.Logger) *OrganizationHandler {
	return &OrganizationHandler{
		service: service,
		log:     log,
	}
}

func (h *OrganizationHandler) listOrganizations(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationHandler.listOrganizations")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		span.SetStatus(codes.Error, "get user id from claims failed")
		return
	}

	output, err := h.service.ListOrganizations(ctx, organizationservice.ListInput{
		UserID: userID,
	})
	if err != nil {
		span.SetStatus(codes.Error, "list organizations failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(attribute.String("user.id", userID.String()))

	handler.OK(w, http.StatusOK, toListResponse(output))
}

func (h *OrganizationHandler) createOrganization(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationHandler.createOrganization")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		span.SetStatus(codes.Error, "get user id from claims failed")
		return
	}

	req, ok := handler.DecodeValid[CreateRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.CreateOrganization(ctx, organizationservice.CreateInput{
		UserID: userID,
		Name:   strings.TrimSpace(req.Name),
	})
	if err != nil {
		span.SetStatus(codes.Error, "create organization failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(attribute.String("organization.id", output.Organization.ID.String()), attribute.String("user.id", userID.String()))

	handler.OK(w, http.StatusCreated, toCreateResponse(output))
}

func (h *OrganizationHandler) getOrganization(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationHandler.getOrganization")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		span.SetStatus(codes.Error, "get user id from claims failed")
		return
	}

	organizationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid organization id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid organization id")
		return
	}

	output, err := h.service.GetOrganization(ctx, organizationservice.GetInput{
		UserID:         userID,
		OrganizationID: organizationID,
	})
	if err != nil {
		span.SetStatus(codes.Error, "get organization failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(attribute.String("organization.id", organizationID.String()), attribute.String("user.id", userID.String()))

	handler.OK(w, http.StatusOK, toGetResponse(output))
}

func (h *OrganizationHandler) updateOrganization(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationHandler.updateOrganization")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		span.SetStatus(codes.Error, "get user id from claims failed")
		return
	}

	organizationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid organization id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid organization id")
		return
	}

	req, ok := handler.DecodeValid[UpdateRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.UpdateOrganization(ctx, organizationservice.UpdateInput{
		UserID:         userID,
		OrganizationID: organizationID,
		Name:           strings.TrimSpace(req.Name),
	})
	if err != nil {
		span.SetStatus(codes.Error, "update organization failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(attribute.String("organization.id", organizationID.String()), attribute.String("user.id", userID.String()))

	handler.OK(w, http.StatusOK, toUpdateResponse(output))
}

func (h *OrganizationHandler) deleteOrganization(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationHandler.deleteOrganization")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		span.SetStatus(codes.Error, "get user id from claims failed")
		return
	}

	organizationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid organization id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid organization id")
		return
	}

	output, err := h.service.DeleteOrganization(ctx, organizationservice.DeleteInput{
		UserID:         userID,
		OrganizationID: organizationID,
	})
	if err != nil {
		span.SetStatus(codes.Error, "delete organization failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(attribute.String("organization.id", organizationID.String()), attribute.String("user.id", userID.String()))

	handler.OK(w, http.StatusOK, toDeleteResponse(output))
}

func (h *OrganizationHandler) transferOrganization(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "OrganizationHandler.transferOrganization")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r)
	if !ok {
		span.SetStatus(codes.Error, "get user id from claims failed")
		return
	}

	organizationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid organization id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid organization id")
		return
	}

	req, ok := handler.DecodeValid[TransferRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	targetUserID, err := uuid.Parse(strings.TrimSpace(req.UserID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid target user id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid user id")
		return
	}

	output, err := h.service.Transfer(ctx, organizationservice.TransferInput{
		UserID:         userID,
		OrganizationID: organizationID,
		TargetUserID:   targetUserID,
	})
	if err != nil {
		span.SetStatus(codes.Error, "transfer ownership failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	span.SetAttributes(attribute.String("organization.id", organizationID.String()), attribute.String("target_user.id", targetUserID.String()))

	handler.OK(w, http.StatusOK, toTransferResponse(output))
}

func userIDFromClaims(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	claims, ok := handler.ClaimsFrom(r.Context())
	if !ok {
		handler.WriteError(w, http.StatusUnauthorized, handler.CodeUnauthorized, "unauthorized")
		return uuid.Nil, false
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, false
	}

	return userID, true
}
