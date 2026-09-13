package device

import "github.com/google/uuid"

// Event types emitted to the audit topic. The push token itself is a secret
// and never appears in any event payload - only its platform and metadata.
const (
	EventDeviceRegistered = "user_device.registered"
	EventDeviceRevoked    = "user_device.revoked"
)

// DeviceRegisteredEvent announces that a device can now receive push.
type DeviceRegisteredEvent struct {
	UserID     uuid.UUID `json:"user_id"`
	DeviceID   uuid.UUID `json:"device_id"`
	Platform   string    `json:"platform"`
	AppVersion string    `json:"app_version,omitempty"`
}

// DeviceRevokedEvent announces that a device no longer receives push.
type DeviceRevokedEvent struct {
	UserID   uuid.UUID `json:"user_id"`
	DeviceID uuid.UUID `json:"device_id"`
	Platform string    `json:"platform"`
}
