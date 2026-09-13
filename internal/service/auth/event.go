package auth

import "github.com/google/uuid"

const (
	EventUserRegistered  = "auth.user_registered"
	EventUserLoggedIn    = "auth.user_logged_in"
	EventUserLoggedOut   = "auth.user_logged_out"
	EventTokenRefreshed  = "auth.token_refreshed"
	EventLoginFailed     = "auth.login_failed"
	EventOrgSwitched     = "auth.org_switched"
	EventOrgSwitchFailed = "auth.org_switch_failed"
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

// LoginFailedEvent records an authentication decision (ASVS 7.2.1). The
// attempted identifier is included because it is the brute-force signal;
// the HTTP response stays uniform regardless of existence.
type LoginFailedEvent struct {
	AttemptedEmail string    `json:"attempted_email,omitempty"`
	UserID         uuid.UUID `json:"user_id,omitempty"`
	Reason         string    `json:"reason"`
	UserAgent      string    `json:"user_agent,omitempty"`
	IPAddress      string    `json:"ip_address,omitempty"`
}

type OrgSwitchedEvent struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	UserAgent      string    `json:"user_agent,omitempty"`
	IPAddress      string    `json:"ip_address,omitempty"`
}

type OrgSwitchFailedEvent struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Reason         string    `json:"reason"`
	UserAgent      string    `json:"user_agent,omitempty"`
	IPAddress      string    `json:"ip_address,omitempty"`
}
