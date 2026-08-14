package jwks

import "github.com/go-chi/chi/v5"

func (h *JwksHandler) Routes(r chi.Router) {
	r.Get("/.well-known/jwks.json", h.jwks)
}
