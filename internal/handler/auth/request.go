package auth

type RegisterRequest struct {
	Name     string `json:"name"     validate:"required,min=1,max=100"`
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}
