package auth

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Instruments built once on the global provider; before telemetry.Setup the
// proxy provider makes them no-ops.
var (
	meter = otel.Meter("service/auth")

	// loginFailed feeds the Security row of the platform overview dashboard -
	// the real-time brute-force signal; the audit trail holds the details.
	loginFailed metric.Int64Counter
)

func init() {
	c, err := meter.Int64Counter("auth.login.failed",
		metric.WithDescription("Failed authentication attempts by reason"))
	if err != nil {
		panic(err)
	}
	loginFailed = c
}
