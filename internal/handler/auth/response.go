package auth

import (
	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
	"github.com/google/uuid"
)

type SessionResponse struct {
	AccessToken  string               `json:"access_token"`
	RefreshToken string               `json:"refresh_token"`
	ExpiresIn    int                  `json:"expires_in"`
	User         UserResponse         `json:"user"`
	Organization OrganizationResponse `json:"organization"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type OrganizationResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role"`
}

func newUserResponse(u authservice.UserOutput) UserResponse {
	return UserResponse{
		ID:    u.ID.String(),
		Name:  u.Name,
		Email: u.Email,
	}
}

func newOrganizationResponse(o authservice.OrganizationOutput) OrganizationResponse {
	return OrganizationResponse{
		ID:   o.ID.String(),
		Name: o.Name,
		Type: o.Type,
		Role: o.Role,
	}
}

type TokensResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func toRefreshResponse(out authservice.RefreshOutput) TokensResponse {
	return TokensResponse{
		AccessToken:  out.Tokens.AccessToken,
		RefreshToken: out.Tokens.RefreshToken,
		ExpiresIn:    out.Tokens.ExpiresIn,
	}
}

func toRegisterResponse(out authservice.RegisterOutput) SessionResponse {
	return SessionResponse{
		AccessToken:  out.Tokens.AccessToken,
		RefreshToken: out.Tokens.RefreshToken,
		ExpiresIn:    out.Tokens.ExpiresIn,
		User:         newUserResponse(out.User),
		Organization: newOrganizationResponse(out.Organization),
	}
}

func toLoginResponse(out authservice.LoginOutput) SessionResponse {
	return SessionResponse{
		AccessToken:  out.Tokens.AccessToken,
		RefreshToken: out.Tokens.RefreshToken,
		ExpiresIn:    out.Tokens.ExpiresIn,
		User:         newUserResponse(out.User),
		Organization: newOrganizationResponse(out.Organization),
	}
}

type MeResponse struct {
	User                 MeUserResponse           `json:"user"`
	ActiveOrganizationID *uuid.UUID               `json:"active_organization_id"`
	Organizations        []MeOrganizationResponse `json:"organizations"`
}

type MeUserResponse struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type MeOrganizationResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Type string    `json:"type"`
	Role string    `json:"role"`
}

func toMeResponse(output authservice.MeOutput) MeResponse {
	organizations := make([]MeOrganizationResponse, 0, len(output.Organizations))

	for _, organization := range output.Organizations {
		organizations = append(organizations, MeOrganizationResponse{
			ID:   organization.ID,
			Name: organization.Name,
			Type: organization.Type,
			Role: organization.Role,
		})
	}

	return MeResponse{
		User: MeUserResponse{
			ID:    output.User.ID,
			Name:  output.User.Name,
			Email: output.User.Email,
		},
		ActiveOrganizationID: output.ActiveOrganizationId,
		Organizations:        organizations,
	}
}
