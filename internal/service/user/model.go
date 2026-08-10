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
