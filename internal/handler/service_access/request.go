package service_access

type GrantAccessRequest struct {
	ServiceName string `json:"service_name" validate:"required,max=50"`
}

type RevokeAccessRequest struct {
	ServiceName string `json:"service_name" validate:"required,max=50"`
}
