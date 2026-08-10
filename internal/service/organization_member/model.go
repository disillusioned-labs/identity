package organizationmember

import "github.com/google/uuid"

type CreateInput struct {
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	Role           string
}

type ListByUserInput struct {
	UserID uuid.UUID
}

type ListByUserOutput struct {
	OrganizationID   uuid.UUID
	OrganizationName string
	OrganizationType string
	Role             string
}

type GetInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

type GetOutput struct {
	OrganizationID   uuid.UUID
	OrganizationName string
	OrganizationType string
	Role             string
}
