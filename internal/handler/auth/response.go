package auth

import authservice "github.com/disillusioned-labs/identity/internal/service/auth"

type RegisterResponse struct {
	AccessToken  string                       `json:"access_token"`
	RefreshToken string                       `json:"refresh_token"`
	ExpiresIn    int                          `json:"expires_in"`
	User         RegisterUserResponse         `json:"user"`
	Organization RegisterOrganizationResponse `json:"organization"`
}

type RegisterUserResponse struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type RegisterOrganizationResponse struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role"`
}

func toRegisterResponse(out authservice.RegisterOutput) RegisterResponse {
	return RegisterResponse{
		AccessToken:  out.Tokens.AccessToken,
		RefreshToken: out.Tokens.RefreshToken,
		ExpiresIn:    out.Tokens.ExpiresIn,
		User: RegisterUserResponse{
			Id:    out.User.ID.String(),
			Name:  out.User.Name,
			Email: out.User.Email,
		},
		Organization: RegisterOrganizationResponse{
			Id:   out.Organization.ID.String(),
			Name: out.Organization.Name,
			Type: out.Organization.Type,
			Role: out.Organization.Role,
		},
	}
}
