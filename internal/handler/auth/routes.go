package auth

import "github.com/go-chi/chi/v5"

func (h *AuthHandler) PublicRoutes(r chi.Router) {
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
}

func (h *AuthHandler) ProtectedRoutes(r chi.Router) {
	r.Get("/me", h.me)
}
