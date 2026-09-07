package auth

import "github.com/google/uuid"

// Register
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

// Login
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

// Me
type MeInput struct {
	UserID uuid.UUID
}

type MeOutput struct {
	User                 MeUserOutput
	ActiveOrganizationId *uuid.UUID
	Organizations        []MeOrganizationOutput
}

type MeUserOutput struct {
	ID    uuid.UUID
	Name  string
	Email string
}

type MeOrganizationOutput struct {
	ID   uuid.UUID
	Name string
	Type string
	Role string
}

// GetUsersByIDs
type UserOrgPair struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

type GetUsersByIDsInput struct {
	Users []UserOrgPair
}

type GetUsersByIDsOutput struct {
	Users []UserOutput
}

// Refresh
type RefreshInput struct {
	RefreshToken string
	UserAgent    string
	IPAddress    string
}

type RefreshOutput struct {
	Tokens TokensOutput
}

// Logout
type LogoutInput struct {
	RefreshToken string
	UserAgent    string
	IPAddress    string
}

type LogoutOutput struct{}

// SwitchOrg
type SwitchOrgInput struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	RefreshToken   string
	UserAgent      string
	IPAddress      string
}

type SwitchOrgOutput struct {
	Tokens TokensOutput
}

type UserOutput struct {
	ID       uuid.UUID
	Name     string
	Email    string
	Role     string
	IsActive bool
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
