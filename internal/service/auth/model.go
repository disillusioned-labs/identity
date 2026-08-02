package auth

import "github.com/google/uuid"

type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

type RegisterOutput struct {
	User         User
	Organization Organization
	Tokens       Tokens
}

type User struct {
	ID    uuid.UUID
	Name  string
	Email string
}

type Organization struct {
	ID   uuid.UUID
	Name string
	Type string
	Role string
}

type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}
