package jwt

import "time"

type Claims struct {
	Subject   string
	OrgID     string
	Role      string
	Issuer    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type AuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}
