package organizationmember

import "github.com/google/uuid"

type CreateInput struct {
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	Role           string
}
