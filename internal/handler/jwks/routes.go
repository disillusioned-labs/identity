package jwks

import "github.com/go-chi/chi/v5"

func (h *JwksHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/.well-known/jwks.json", h.jwks)
	return r
}
