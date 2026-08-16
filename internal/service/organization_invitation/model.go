package organization_invitation

import (
	"time"

	"github.com/google/uuid"
)

type ListMyInvitationsInput struct {
	UserID uuid.UUID
}

type ListMyInvitationsOutput struct {
	Invitations []MyInvitationOutput
}

type MyInvitationOutput struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	OrganizationName string
	Role             string
	Status           string
	ExpiresAt        time.Time
	CreatedAt        time.Time
}

type CreateInvitationInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	Email          string
	Role           string
}

type CreateInvitationOutput struct {
	Invitation InvitationOutput
	Token      string
}

type ListInvitationsInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

type ListInvitationsOutput struct {
	Invitations []InvitationOutput
}

type GetInvitationInput struct {
	Token string
}

type GetInvitationOutput struct {
	Invitation InvitationDetailOutput
}

type AcceptInvitationInput struct {
	UserID       uuid.UUID
	InvitationID uuid.UUID
}

type AcceptInvitationByTokenInput struct {
	UserID uuid.UUID
	Token  string
}

type AcceptInvitationOutput struct {
	OrganizationID uuid.UUID
	Role           string
}

type RevokeInvitationInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	InvitationID   uuid.UUID
}

type RevokeInvitationOutput struct{}

type InvitationOutput struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Email          string
	Role           string
	Status         string
	ExpiresAt      time.Time
	AcceptedAt     *time.Time
	AcceptedBy     *uuid.UUID
	InvitedBy      uuid.UUID
	InvitedByName  string
	CreatedAt      time.Time
}

type InvitationDetailOutput struct {
	OrganizationName     string
	InvitedByName        string
	Role                 string
	ExpiresAt            time.Time
	RequiresRegistration bool
}
