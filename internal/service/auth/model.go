package auth

import "github.com/google/uuid"

type RegisterInput struct {
	Name      string
	Email     string
	Password  string
	UserAgent string
	IPAddress string
}

type RegisterOutput struct {
	User         UserOutput
	Organization OrganizationOutput
	Tokens       TokensOutput
}

type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IPAddress string
}

type LoginOutput struct {
	User         UserOutput
	Organization OrganizationOutput
	Tokens       TokensOutput
}

type RefreshInput struct {
	RefreshToken string
	UserAgent    string
	IPAddress    string
}

type RefreshOutput struct {
	Tokens TokensOutput
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

type issueParams struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	Role           string
	UserAgent      string
	IPAddress      string
}

type tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}
