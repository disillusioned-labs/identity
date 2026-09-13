package device

import "github.com/go-chi/chi/v5"

func (h *DeviceHandler) ProtectedRoutes(r chi.Router) {
	r.Post("/devices", h.register)
	r.Delete("/devices", h.revoke)
}
