package auth

import "github.com/go-chi/chi/v5"

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	return r
}
