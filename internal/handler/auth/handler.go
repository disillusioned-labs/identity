package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/disillusioned-labs/identity/internal/handler"
	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("handler/auth")

type Handler struct {
	svc authservice.Service
	log *slog.Logger
}

func NewHandler(svc authservice.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "AuthHandler.register")
	defer span.End()
	r = r.WithContext(ctx)

	req, ok := handler.DecodeValid[RegisterRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.svc.Register(ctx, authservice.RegisterInput{
		Name:     strings.TrimSpace(req.Name),
		Email:    strings.TrimSpace(req.Email),
		Password: req.Password,
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}
	span.SetAttributes(attribute.String("user.id", output.User.ID.String()))
	handler.OK(w, http.StatusCreated, toRegisterResponse(output))
}
