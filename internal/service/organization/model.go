package organization

import "github.com/google/uuid"

type CreateInput struct {
	Name string
	Type string
}

type CreateOutput struct {
	ID   uuid.UUID
	Name string
	Type string
}
