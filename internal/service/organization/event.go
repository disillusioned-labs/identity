package organization

import "github.com/google/uuid"

const (
	EventOrganizationCreated = "organization.created"
	EventOrganizationUpdated = "organization.updated"
	EventOrganizationDeleted = "organization.deleted"
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
