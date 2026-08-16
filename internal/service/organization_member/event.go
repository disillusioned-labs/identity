package organization_member

import "github.com/google/uuid"

const (
	EventOrganizationMemberRoleUpdated = "organization.member.role_updated"
	EventOrganizationMemberRemoved     = "organization.member.removed"
	EventOrganizationMemberLeft        = "organization.member.left"
	EventOrganizationDeleted           = "organization.deleted"
)

type OrganizationMemberRoleUpdatedEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	UpdatedBy      uuid.UUID `json:"updated_by"`
	Role           string    `json:"role"`
}

type OrganizationMemberRemovedEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	RemovedBy      uuid.UUID `json:"removed_by"`
}

type OrganizationMemberLeftEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
}

type OrganizationDeletedEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	DeletedBy      uuid.UUID `json:"deleted_by"`
}
