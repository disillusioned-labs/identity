package auth

import (
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/disillusioned-labs/identity/internal/handler"
	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
)

var tracer = otel.Tracer("handler/auth")

type AuthHandler struct {
	service authservice.AuthService
	log     *slog.Logger
}

func NewAuthHandler(service authservice.AuthService, log *slog.Logger) *AuthHandler {
	return &AuthHandler{service: service, log: log}
}

func (h *AuthHandler) register(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "AuthHandler.register")
	defer span.End()
	r = r.WithContext(ctx)

	req, ok := handler.DecodeValid[RegisterRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.Register(ctx, authservice.RegisterInput{
		Name:      strings.TrimSpace(req.Name),
		Email:     strings.TrimSpace(req.Email),
		Password:  req.Password,
		UserAgent: r.UserAgent(),
		IPAddress: clientIP(r),
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}
	span.SetAttributes(attribute.String("user.id", output.User.ID.String()))
	handler.OK(w, http.StatusCreated, toRegisterResponse(output))
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "AuthHandler.login")
	defer span.End()
	r = r.WithContext(ctx)

	req, ok := handler.DecodeValid[LoginRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.Login(ctx, authservice.LoginInput{
		Email:     strings.TrimSpace(req.Email),
		Password:  req.Password,
		UserAgent: r.UserAgent(),
		IPAddress: clientIP(r),
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}
	span.SetAttributes(attribute.String("user.id", output.User.ID.String()))
	handler.OK(w, http.StatusOK, toLoginResponse(output))
}

func (h *AuthHandler) refresh(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "AuthHandler.refresh")
	defer span.End()
	r = r.WithContext(ctx)

	req, ok := handler.DecodeValid[RefreshRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.Refresh(ctx, authservice.RefreshInput{
		RefreshToken: strings.TrimSpace(req.RefreshToken),
		UserAgent:    r.UserAgent(),
		IPAddress:    clientIP(r),
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}
	handler.OK(w, http.StatusOK, toRefreshResponse(output))
}

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "AuthHandler.logout")
	defer span.End()
	r = r.WithContext(ctx)

	req, ok := handler.DecodeValid[LogoutRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	output, err := h.service.Logout(ctx, authservice.LogoutInput{
		RefreshToken: strings.TrimSpace(req.RefreshToken),
		UserAgent:    r.UserAgent(),
		IPAddress:    clientIP(r),
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}
	handler.OK(w, http.StatusOK, toLogoutResponse(output))
}

func (h *AuthHandler) switchOrg(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "AuthHandler.switchOrg")
	defer span.End()
	r = r.WithContext(ctx)

	claims, ok := handler.ClaimsFrom(r.Context())
	if !ok {
		handler.WriteError(
			w,
			http.StatusUnauthorized,
			handler.CodeUnauthorized,
			"unauthorized",
		)
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		span.SetStatus(codes.Error, "invalid claim subject")
		handler.WriteError(
			w,
			http.StatusUnauthorized,
			handler.CodeUnauthorized,
			"unauthorized",
		)
		return
	}

	req, ok := handler.DecodeValid[SwitchOrgRequest](w, r)
	if !ok {
		span.SetStatus(codes.Error, "decode/validate failed")
		return
	}

	organizationID, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil {
		span.SetStatus(codes.Error, "invalid organization id")
		handler.WriteError(w, http.StatusBadRequest, handler.CodeBadRequest, "invalid organization id")
		return
	}

	output, err := h.service.SwitchOrg(ctx, authservice.SwitchOrgInput{
		UserID:         userID,
		OrganizationID: organizationID,
		RefreshToken:   strings.TrimSpace(req.RefreshToken),
		UserAgent:      r.UserAgent(),
		IPAddress:      clientIP(r),
	})
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}
	handler.OK(w, http.StatusOK, toSwitchOrgResponse(output))
}

func (h *AuthHandler) me(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "AuthHandler.me")
	defer span.End()
	r = r.WithContext(ctx)

	claims, ok := handler.ClaimsFrom(r.Context())
	if !ok {
		handler.WriteError(
			w,
			http.StatusUnauthorized,
			handler.CodeUnauthorized,
			"unauthorized",
		)
		return
	}

	userId, err := uuid.Parse(claims.Subject)
	if err != nil {
		span.SetStatus(codes.Error, "invalid claim subject")
		handler.WriteError(
			w,
			http.StatusUnauthorized,
			handler.CodeUnauthorized,
			"unauthorized",
		)
		return
	}

	meInput := authservice.MeInput{
		UserID: userId,
	}

	meOutput, err := h.service.Me(ctx, meInput)
	if err != nil {
		handler.WriteServiceError(w, r, h.log, err)
		return
	}

	handler.WriteJSON(w, http.StatusOK, toMeResponse(meOutput))
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return host
}
