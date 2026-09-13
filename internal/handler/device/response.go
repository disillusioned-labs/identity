package device

type RegisterResponse struct {
	Registered bool `json:"registered"`
}

type RevokeResponse struct {
	Revoked bool `json:"revoked"`
}
