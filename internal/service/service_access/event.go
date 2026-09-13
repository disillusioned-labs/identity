package service_access

import "github.com/google/uuid"

const (
	eventVersion          = 1
	EventAccessGranted    = "service_access.granted"
	EventAccessRevoked    = "service_access.revoked"
	EventAccessRevokedAll = "service_access.revoked_all"
)

type AccessGrantedEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	ServiceName    string    `json:"service_name"`
	ActorID        uuid.UUID `json:"actor_id"`
}

type AccessRevokedEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	ServiceName    string    `json:"service_name"`
	ActorID        uuid.UUID `json:"actor_id,omitempty"`
}

type AccessRevokedAllEvent struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	ActorID        uuid.UUID `json:"actor_id,omitempty"`
}
