package organization_invitation

import "github.com/go-chi/chi/v5"

func (h *OrganizationInvitationHandler) ProtectedRoutes(r chi.Router) {
	r.Get("/invitations", h.listMyInvitations)

	r.Get("/{id}/invitations", h.listInvitations)
	r.Post("/{id}/invitations", h.createInvitation)

	r.Get("/{id}/invitations/token/{token}", h.getInvitation)
	r.Post("/{id}/invitations/{invitation_id}/accept", h.acceptInvitation)
	r.Post("/{id}/invitations/token/{token}/accept", h.acceptInvitationByToken)

	r.Delete("/{id}/invitations/{invitation_id}", h.revokeInvitation)
}
