package organization_invitation

import (
	"time"

	organizationinvitationservice "github.com/disillusioned-labs/identity/internal/service/organization_invitation"
	"github.com/google/uuid"
)

type ListMyInvitationsResponse struct {
	Invitations []MyInvitationResponse `json:"invitations"`
}

type MyInvitationResponse struct {
	ID               uuid.UUID `json:"id"`
	OrganizationID   uuid.UUID `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	Role             string    `json:"role"`
	Status           string    `json:"status"`
	ExpiresAt        time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
}

func toListMyInvitationsResponse(output organizationinvitationservice.ListMyInvitationsOutput) ListMyInvitationsResponse {
	invitations := make([]MyInvitationResponse, 0, len(output.Invitations))

	for _, invitation := range output.Invitations {
		invitations = append(invitations, MyInvitationResponse{
			ID:               invitation.ID,
			OrganizationID:   invitation.OrganizationID,
			OrganizationName: invitation.OrganizationName,
			Role:             invitation.Role,
			Status:           invitation.Status,
			ExpiresAt:        invitation.ExpiresAt,
			CreatedAt:        invitation.CreatedAt,
		})
	}

	return ListMyInvitationsResponse{
		Invitations: invitations,
	}
}

type InvitationResponse struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	Email          string     `json:"email"`
	Role           string     `json:"role"`
	Status         string     `json:"status"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	AcceptedBy     *uuid.UUID `json:"accepted_by,omitempty"`
	InvitedBy      uuid.UUID  `json:"invited_by"`
	InvitedByName  string     `json:"invited_by_name,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func toInvitationResponse(input organizationinvitationservice.InvitationOutput) InvitationResponse {
	return InvitationResponse{
		ID:             input.ID,
		OrganizationID: input.OrganizationID,
		Email:          input.Email,
		Role:           input.Role,
		Status:         input.Status,
		ExpiresAt:      input.ExpiresAt,
		AcceptedAt:     input.AcceptedAt,
		AcceptedBy:     input.AcceptedBy,
		InvitedBy:      input.InvitedBy,
		InvitedByName:  input.InvitedByName,
		CreatedAt:      input.CreatedAt,
	}
}

type CreateInvitationResponse struct {
	Invitation InvitationResponse `json:"invitation"`
	Token      string             `json:"token"`
}

func toCreateInvitationResponse(input organizationinvitationservice.CreateInvitationOutput) CreateInvitationResponse {
	return CreateInvitationResponse{
		Invitation: toInvitationResponse(input.Invitation),
		Token:      input.Token,
	}
}

type ListInvitationsResponse struct {
	Invitations []InvitationResponse `json:"invitations"`
}

func toListInvitationsResponse(input organizationinvitationservice.ListInvitationsOutput) ListInvitationsResponse {
	invitations := make([]InvitationResponse, 0, len(input.Invitations))

	for _, invitation := range input.Invitations {
		invitations = append(invitations, toInvitationResponse(invitation))
	}

	return ListInvitationsResponse{
		Invitations: invitations,
	}
}

type GetInvitationResponse struct {
	Invitation InvitationDetailResponse `json:"invitation"`
}

type InvitationDetailResponse struct {
	OrganizationName     string    `json:"organization_name"`
	InvitedByName        string    `json:"invited_by_name"`
	Role                 string    `json:"role"`
	ExpiresAt            time.Time `json:"expires_at"`
	RequiresRegistration bool      `json:"requires_registration"`
}

func toGetInvitationResponse(input organizationinvitationservice.GetInvitationOutput) GetInvitationResponse {
	return GetInvitationResponse{
		Invitation: InvitationDetailResponse{
			OrganizationName:     input.Invitation.OrganizationName,
			InvitedByName:        input.Invitation.InvitedByName,
			Role:                 input.Invitation.Role,
			ExpiresAt:            input.Invitation.ExpiresAt,
			RequiresRegistration: input.Invitation.RequiresRegistration,
		},
	}
}

type AcceptInvitationResponse struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	Role           string    `json:"role"`
}

func toAcceptInvitationResponse(input organizationinvitationservice.AcceptInvitationOutput) AcceptInvitationResponse {
	return AcceptInvitationResponse{
		OrganizationID: input.OrganizationID,
		Role:           input.Role,
	}
}

type RevokeInvitationResponse struct{}

func toRevokeInvitationResponse(input organizationinvitationservice.RevokeInvitationOutput) RevokeInvitationResponse {
	return RevokeInvitationResponse{}
}
