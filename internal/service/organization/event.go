package organization

import "github.com/google/uuid"

const (
	EventOrganizationCreated  = "organization.created"
	EventOrganizationUpdated  = "organization.updated"
	EventOrganizationDeleted  = "organization.deleted"
	EventOrganizationFrozen   = "organization.frozen"
	EventOrganizationUnfrozen = "organization.unfrozen"

	EventOrganizationOwnershipTransferred = "organization.ownership_transferred"
)

type OrganizationCreatedEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Role           string    `json:"role"`
}

type OrganizationUpdatedEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Role           string    `json:"role"`
}

type OrganizationDeletedEvent struct {
	OrganizationID            uuid.UUID  `json:"organization_id"`
	UserID                    uuid.UUID  `json:"user_id"`
	Type                      string     `json:"type"`
	ReplacementOrganizationID *uuid.UUID `json:"replacement_organization_id,omitempty"`
}

// OrganizationFreezeEvent records the actor who changed an organization's frozen state.
type OrganizationFreezeEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
}

type OrganizationOwnershipTransferredEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	FromUserID     uuid.UUID `json:"from_user_id"`
	ToUserID       uuid.UUID `json:"to_user_id"`
}
