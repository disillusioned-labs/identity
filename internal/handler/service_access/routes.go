package service_access

import "github.com/go-chi/chi/v5"

func (h *ServiceAccessHandler) ProtectedRoutes(r chi.Router) {
	r.Post("/{id}/service-access/{user_id}", h.grantAccess)
	r.Delete("/{id}/service-access/{user_id}", h.revokeAccess)
	r.Get("/{id}/service-access/{user_id}", h.listAccessByOrgUser)
	r.Get("/{id}/service-access", h.listAccessByOrg)
}
