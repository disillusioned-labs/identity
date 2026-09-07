package service_access

import (
	"time"

	"github.com/google/uuid"
)

type GrantAccessInput struct {
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	ServiceName    string
	GrantedBy      uuid.UUID
}

type RevokeAccessInput struct {
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	ServiceName    string
}

type RevokeAllAccessInput struct {
	OrganizationID uuid.UUID
	UserID         uuid.UUID
}

type ListAccessByOrgUserInput struct {
	OrganizationID uuid.UUID
	UserID         uuid.UUID
}

type ListAccessByOrgInput struct {
	OrganizationID uuid.UUID
}

type ServiceAccessOutput struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	ServiceName    string
	GrantedBy      uuid.UUID
	CreatedAt      time.Time
}

type ListAccessOutput struct {
	Accesses []ServiceAccessOutput
}
