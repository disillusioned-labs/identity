package auth

import "github.com/google/uuid"

type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

type RegisterOutput struct {
	User         UserOutput
	Organization OrganizationOutput
	Tokens       TokensOutput
}

type UserOutput struct {
	ID    uuid.UUID
	Name  string
	Email string
}

type OrganizationOutput struct {
	ID   uuid.UUID
	Name string
	Type string
	Role string
}

type TokensOutput struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}
