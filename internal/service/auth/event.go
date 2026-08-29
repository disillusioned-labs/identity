package auth

import "github.com/google/uuid"

const (
	EventUserRegistered = "auth.user_registered"
	EventUserLoggedIn   = "auth.user_logged_in"
	EventUserLoggedOut  = "auth.user_logged_out"
	EventTokenRefreshed = "auth.token_refreshed"
)

type UserRegisteredEvent struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Role           string    `json:"role"`
	UserAgent      string    `json:"user_agent,omitempty"`
	IPAddress      string    `json:"ip_address,omitempty"`
}

type UserLoggedInEvent struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          string    `json:"email"`
	UserAgent      string    `json:"user_agent,omitempty"`
	IPAddress      string    `json:"ip_address,omitempty"`
}

type UserLoggedOutEvent struct {
	UserID    uuid.UUID `json:"user_id"`
	UserAgent string    `json:"user_agent,omitempty"`
	IPAddress string    `json:"ip_address,omitempty"`
}

type TokenRefreshedEvent struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	UserAgent      string    `json:"user_agent,omitempty"`
	IPAddress      string    `json:"ip_address,omitempty"`
}
