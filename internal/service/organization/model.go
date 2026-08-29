package organization

import "github.com/google/uuid"

// List
type ListInput struct {
	UserID uuid.UUID
}

type ListOutput struct {
	Organizations []OrganizationOutput
}

// Create
type CreateInput struct {
	UserID uuid.UUID
	Name   string
	Type   string
}

type CreateOutput struct {
	Organization OrganizationOutput
}

// Get
type GetInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

type GetOutput struct {
	Organization OrganizationOutput
}

// Update
type UpdateInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	Name           string
}

type UpdateOutput struct {
	Organization OrganizationOutput
}

// Delete
type DeleteInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

type DeleteOutput struct{}

// Transfer
type TransferInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	TargetUserID   uuid.UUID
}

type TransferOutput struct {
	Organization OrganizationOutput
	From         TransferUserOutput
	To           TransferUserOutput
}

type TransferUserOutput struct {
	ID   uuid.UUID
	Name string
	Role string
}

type OrganizationOutput struct {
	ID   uuid.UUID
	Name string
	Type string
	Role string
}
