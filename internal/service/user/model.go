package user

import "github.com/google/uuid"

type CreateInput struct {
	Name           string
	Email          string
	HashedPassword string
}

type CreateOutput struct {
	ID    uuid.UUID
	Name  string
	Email string
}

type GetByEmailInput struct {
	Email string
}

type GetByEmailOutput struct {
	ID                       uuid.UUID
	Name                     string
	Email                    string
	HashedPassword           string
	LastActiveOrganizationID *uuid.UUID
}

type SetLastActiveOrganizationInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}
