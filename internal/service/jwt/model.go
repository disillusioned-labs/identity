package jwt

import (
	"time"

	"github.com/google/uuid"
)

type IssueInput struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      string
	UserAgent string
	IPAddress string
}

type IssueOutput struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

type LookupRefreshTokenInput struct {
	RefreshToken string
}

type LookupRefreshTokenOutput struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type RevokeRefreshTokenInput struct {
	ID uuid.UUID
}

type RevokeAllUserRefreshTokensInput struct {
	UserID uuid.UUID
}

type Claims struct {
	Subject   string
	OrgID     string
	Role      string
	Issuer    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}
