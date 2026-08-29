package organization

import "github.com/google/uuid"

const (
	EventOrganizationCreated = "organization.created"
	EventOrganizationUpdated = "organization.updated"
	EventOrganizationDeleted = "organization.deleted"

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

type OrganizationOwnershipTransferredEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	FromUserID     uuid.UUID `json:"from_user_id"`
	ToUserID       uuid.UUID `json:"to_user_id"`
}
