package service_access

import (
	"time"

	"github.com/google/uuid"

	serviceservice "github.com/disillusioned-labs/identity/internal/service/service_access"
)

type ServiceAccessResponse struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	ServiceName    string    `json:"service_name"`
	GrantedBy      uuid.UUID `json:"granted_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func toServiceAccessResponse(a serviceservice.ServiceAccessOutput) ServiceAccessResponse {
	return ServiceAccessResponse{
		ID:             a.ID,
		OrganizationID: a.OrganizationID,
		UserID:         a.UserID,
		ServiceName:    a.ServiceName,
		GrantedBy:      a.GrantedBy,
		CreatedAt:      a.CreatedAt,
	}
}

type ListAccessResponse struct {
	Accesses []ServiceAccessResponse `json:"accesses"`
}

func toListAccessResponse(output serviceservice.ListAccessOutput) ListAccessResponse {
	accesses := make([]ServiceAccessResponse, 0, len(output.Accesses))
	for _, a := range output.Accesses {
		accesses = append(accesses, toServiceAccessResponse(a))
	}
	return ListAccessResponse{Accesses: accesses}
}

type GrantAccessResponse struct {
	Granted bool `json:"granted"`
}

type RevokeAccessResponse struct {
	Revoked bool `json:"revoked"`
}
