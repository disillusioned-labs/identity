// Package contract adapts the expense gRPC service to domain types. Identity
// consumes expense in exactly one flow (decision D2): before committing a
// member removal, expense is the only party that can answer whether the
// member is still an active approver, because approval_rules live in the
// expense database.
package contract

import (
	"context"

	"github.com/google/uuid"
)

// ApproverRuleRef is one rule blocking a member removal; it is relayed to
// the client inside the APPROVER_STILL_ASSIGNED error details (rules list).
type ApproverRuleRef struct {
	ID        uuid.UUID
	ProjectID *uuid.UUID
	Step      int
}

// ExpenseClient is the internal expense surface identity depends on.
type ExpenseClient interface {
	// CheckApproverAssignments reports whether userID still has live
	// approval rules in the organization, and which ones.
	CheckApproverAssignments(ctx context.Context, orgID, userID uuid.UUID) (bool, []ApproverRuleRef, error)
	// ReassignApproverRules moves every live rule of fromUserID to
	// toUserID, recording actorID as the deciding admin in the audit event.
	ReassignApproverRules(ctx context.Context, orgID, fromUserID, toUserID, actorID uuid.UUID) error
}
