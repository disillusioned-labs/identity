package organization_invitation

type CreateInvitationRequest struct {
	Email string `json:"email" validate:"required,email"`
	Role  string `json:"role" validate:"required"`
}

type AcceptInvitationRequest struct{}

type RevokeInvitationRequest struct{}
