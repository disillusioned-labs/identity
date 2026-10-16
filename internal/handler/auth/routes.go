package auth

import "github.com/go-chi/chi/v5"

func (h *AuthHandler) PublicRoutes(r chi.Router) {
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", h.logout)
}

func (h *AuthHandler) RecoveryRoutes(r chi.Router) {
	r.Post("/password-recovery", h.requestPasswordRecovery)
	r.Post("/password-reset", h.resetPassword)
}

func (h *AuthHandler) ProtectedRoutes(r chi.Router) {
	r.Get("/me", h.me)
	r.Post("/switch-org", h.switchOrg)
}
