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
	"github.com/disillusioned-labs/platform/errors"
)

// Error is a domain error that knows how it surfaces over HTTP.
type Error = errors.Error

var NewError = errors.NewError

var (
	ErrUnauthenticated = errors.ErrUnauthenticated
	ErrForbidden       = errors.ErrForbidden
	ErrNotFound        = errors.ErrNotFound
	ErrConflict        = errors.ErrConflict
	ErrInternal        = errors.ErrInternal
)

// IsUniqueViolation reports whether err is a Postgres unique-constraint violation.
var IsUniqueViolation = errors.IsUniqueViolation

// Identity-specific errors.
var (
	ErrEmailTaken              = NewError("EMAIL_TAKEN", 409, "email already taken")
	ErrLastOwnerCannotLeave    = NewError("LAST_OWNER_CANNOT_LEAVE", 403, "last owner cannot leave")
	ErrInvalidRole             = NewError("INVALID_ROLE", 400, "invalid organization member role")
	ErrCannotModifySelf        = NewError("CANNOT_MODIFY_SELF", 400, "cannot modify your own organization membership")
	ErrInvalidOrganizationType = NewError("INVALID_ORGANIZATION_TYPE", 400, "invalid organization type")

	ErrInvitationExpired         = NewError("INVITATION_EXPIRED", 410, "invitation has expired")
	ErrInvitationRevoked         = NewError("INVITATION_REVOKED", 410, "invitation has been revoked")
	ErrInvitationAlreadyAccepted = NewError("INVITATION_ALREADY_ACCEPTED", 409, "invitation has already been accepted")
	ErrInvalidEmail              = NewError("INVALID_EMAIL", 400, "invalid email address")
	ErrCannotTransferToSelf      = NewError("CANNOT_TRANSFER_TO_SELF", 400, "cannot transfer ownership to yourself")

	// Member removal blocked by expense's approval rules (decision D2). The
	// response carries the blocking rules under details.rules.
	ErrApproverStillAssigned = NewError("APPROVER_STILL_ASSIGNED", 409, "member is still an approver on active rules")
	ErrInvalidReassignTarget = NewError("INVALID_REASSIGN_TARGET", 422, "reassign target must differ from the removed member and be a member of the organization")
	ErrExpenseServiceDown    = NewError("EXPENSE_SERVICE_DOWN", 503, "approval rule check is unavailable; try again later")
)
