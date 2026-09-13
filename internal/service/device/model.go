package device

import "github.com/google/uuid"

// RegisterInput carries one device registration. Token is opaque to identity:
// it is stored as received and handed to the push provider verbatim.
type RegisterInput struct {
	Token      string
	Platform   string
	AppVersion string
}

// RegisterOutput reports the stored device. The token is not echoed back.
type RegisterOutput struct {
	DeviceID uuid.UUID
}

// RevokeInput identifies the device registration to revoke. The user comes
// from the JWT claims, never from the request body.
type RevokeInput struct {
	Token string
}

// UnregisterOutput reports which device row the revocation applied to, for
// the audit event.
type UnregisterOutput struct {
	DeviceID uuid.UUID
	Platform string
}
