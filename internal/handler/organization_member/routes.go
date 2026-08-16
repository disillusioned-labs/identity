package organization_member

import "github.com/go-chi/chi/v5"

func (h *OrganizationMemberHandler) ProtectedRoutes(r chi.Router) {
	r.Get("/{id}/members", h.listOrganizationMembers)
	r.Patch("/{id}/members/{user_id}", h.updateOrganizationMemberRole)
	r.Delete("/{id}/members/{user_id}", h.removeOrganizationMember)
	r.Delete("/{id}/members/me", h.leaveOrganization)
}
