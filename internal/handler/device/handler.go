package device

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/disillusioned-labs/identity/internal/handler"
	deviceservice "github.com/disillusioned-labs/identity/internal/service/device"
)

var tracer = otel.Tracer("handler/device")

type DeviceHandler struct {
	service deviceservice.DeviceService
	log     *slog.Logger
}

func NewDeviceHandler(
	service deviceservice.DeviceService,
	log *slog.Logger,
) *DeviceHandler {
	return &DeviceHandler{
		service: service,
		log:     log,
	}
}

// register stores the caller's push token. The user comes from the JWT
// claims, never from the body, so a client can only register its own device.
func (h *DeviceHandler) register(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "DeviceHandler.register")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r, span)
	if !ok {
		return
	}

	req, ok := handler.DecodeValid[RegisterRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.Register(r.Context(), userID, deviceservice.RegisterInput{
		Token:      req.Token,
		Platform:   req.Platform,
		AppVersion: req.AppVersion,
	})
	if err != nil {
		span.SetStatus(codes.Error, "register device failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	_ = output.DeviceID // not part of the wire contract; audit event carries it

	handler.OK(w, http.StatusCreated, RegisterResponse{Registered: true})
}

func (h *DeviceHandler) revoke(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "DeviceHandler.revoke")
	defer span.End()
	r = r.WithContext(ctx)

	userID, ok := userIDFromClaims(w, r, span)
	if !ok {
		return
	}

	req, ok := handler.DecodeValid[RevokeRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	if _, err := h.service.Unregister(r.Context(), userID, deviceservice.RevokeInput{
		Token: req.Token,
	}); err != nil {
		span.SetStatus(codes.Error, "revoke device failed")
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	handler.OK(w, http.StatusOK, RevokeResponse{Revoked: true})
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
