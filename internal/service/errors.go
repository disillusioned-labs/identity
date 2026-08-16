// Package service holds cross-resource service bits: the domain error type
// and storage-error helpers. Each resource's business logic lives in its own
// subpackage (service/user, ...), exposing an interface plus an unexported
// implementation so handlers can mock the contract in tests.
//
// Domain errors are self-describing: each carries the HTTP status and
// machine-readable code it maps to. handler.WriteServiceError reads those
// fields, so adding a resource never means editing the shared handler layer.
package service

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

// Error is a domain error that knows how it surfaces over HTTP.
//
// Status and Code live here rather than in a switch inside the handler layer
// because the handler layer is shared by every resource: a per-resource switch
// would make each new vertical edit a common file. Code strings are duplicated
// from handler's generic codes instead of imported because handler imports
// service - importing back would be a cycle.
type Error struct {
	// Code is the stable, machine-readable identifier clients branch on.
	Code string
	// Status is the HTTP status WriteServiceError responds with.
	Status int
	// Message is the client-safe text; it must never carry internal detail.
	Message string
}

func (e *Error) Error() string { return e.Message }

// NewError builds a domain error for a resource-specific failure. Declare the
// result as a package-level var so callers can compare it with errors.Is:
//
//	var ErrOrderLocked = service.NewError("ORDER_LOCKED", http.StatusConflict, "order is locked")
func NewError(code string, status int, message string) *Error {
	return &Error{Code: code, Status: status, Message: message}
}

var (
	ErrUnauthenticated = NewError("UNAUTHENTICATED", http.StatusUnauthorized, "invalid credentials")
	ErrForbidden       = NewError("FORBIDDEN", http.StatusForbidden, "forbidden")
	ErrNotFound        = NewError("NOT_FOUND", http.StatusNotFound, "resource not found")
	ErrConflict        = NewError("CONFLICT", http.StatusConflict, "conflicts with existing data")
	ErrInternal        = NewError("INTERNAL", http.StatusInternalServerError, "internal server error")

	ErrEmailTaken              = NewError("EMAIL_TAKEN", http.StatusConflict, "email already taken")
	ErrLastOwnerCannotLeave    = NewError("LAST_OWNER_CANNOT_LEAVE", http.StatusForbidden, "last owner cannot leave")
	ErrInvalidRole             = NewError("INVALID_ROLE", http.StatusBadRequest, "invalid organization member role")
	ErrCannotModifySelf        = NewError("CANNOT_MODIFY_SELF", http.StatusBadRequest, "cannot modify your own organization membership")
	ErrInvalidOrganizationType = NewError("INVALID_ORGANIZATION_TYPE", http.StatusBadRequest, "invalid organization type")

	ErrInvitationExpired         = NewError("INVITATION_EXPIRED", http.StatusGone, "invitation has expired")
	ErrInvitationRevoked         = NewError("INVITATION_REVOKED", http.StatusGone, "invitation has been revoked")
	ErrInvitationAlreadyAccepted = NewError("INVITATION_ALREADY_ACCEPTED", http.StatusConflict, "invitation has already been accepted")
	ErrInvalidEmail              = NewError("INVALID_EMAIL", http.StatusBadRequest, "invalid email address")
)

const pgUniqueViolation = "23505"

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}
