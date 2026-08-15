package organization_member

import (
	"time"

	"github.com/google/uuid"
)

// List
type ListOrganizationMembersInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

type ListOrganizationMembersOutput struct {
	Members []OrganizationMemberOutput
}

// Update role
type UpdateOrganizationMemberRoleInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	TargetUserID   uuid.UUID
	Role           string
}

type UpdateOrganizationMemberRoleOutput struct {
	Member OrganizationMemberOutput
}

// Remove member
type RemoveOrganizationMemberInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	TargetUserID   uuid.UUID
}

type RemoveOrganizationMemberOutput struct {
	OrganizationDeleted bool
}

// Leave organization
type LeaveOrganizationInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

type LeaveOrganizationOutput struct {
	OrganizationDeleted bool
}

// Output
type OrganizationMemberOutput struct {
	UserID   uuid.UUID
	Name     string
	Email    string
	Role     string
	JoinedAt time.Time
}
