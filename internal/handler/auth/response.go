package auth

import authservice "github.com/disillusioned-labs/identity/internal/service/auth"

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
