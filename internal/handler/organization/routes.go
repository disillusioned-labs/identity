package organization

import "github.com/go-chi/chi/v5"

func (h *OrganizationHandler) ProtectedRoutes(r chi.Router) {
	r.Get("/", h.listOrganizations)
	r.Post("/", h.createOrganization)
	r.Get("/{id}", h.getOrganization)
	r.Patch("/{id}", h.updateOrganization)
	r.Delete("/{id}", h.deleteOrganization)
	r.Post("/{id}/transfer", h.transferOrganization)
}
