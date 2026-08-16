package organization_invitation

import "github.com/google/uuid"

const (
	EventOrganizationInvitationCreated  = "organization.invitation.created"
	EventOrganizationInvitationAccepted = "organization.invitation.accepted"
	EventOrganizationInvitationRevoked  = "organization.invitation.revoked"
)

type OrganizationInvitationCreatedEvent struct {
	InvitationID   uuid.UUID `json:"invitation_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	InvitedBy      uuid.UUID `json:"invited_by"`
	ExpiresAt      string    `json:"expires_at"`
}

type OrganizationInvitationAcceptedEvent struct {
	InvitationID   uuid.UUID `json:"invitation_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	Role           string    `json:"role"`
}

type OrganizationInvitationRevokedEvent struct {
	InvitationID   uuid.UUID `json:"invitation_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	RevokedBy      uuid.UUID `json:"revoked_by"`
}
