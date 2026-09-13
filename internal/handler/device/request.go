package device

// RegisterRequest is the body of POST /devices. Token length follows FCM's
// documented maximum; the value is opaque to identity.
type RegisterRequest struct {
	Token      string `json:"token" validate:"required,max=4096"`
	Platform   string `json:"platform" validate:"required,oneof=android ios web"`
	AppVersion string `json:"app_version" validate:"omitempty,max=50"`
}

// RevokeRequest is the body of DELETE /devices. Sent in the body rather than
// the path so long tokens never appear in access logs or route metrics.
type RevokeRequest struct {
	Token string `json:"token" validate:"required,max=4096"`
}
