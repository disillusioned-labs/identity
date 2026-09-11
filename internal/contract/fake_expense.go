package contract

import (
	"context"

	"github.com/google/uuid"
)

// FakeExpenseClient is the test double for ExpenseClient: a struct of
// function fields with nil-safe zero-value defaults, so a test only stubs
// the surface it cares about.
type FakeExpenseClient struct {
	CheckApproverAssignmentsFn func(ctx context.Context, orgID, userID uuid.UUID) (bool, []ApproverRuleRef, error)
	ReassignApproverRulesFn    func(ctx context.Context, orgID, fromUserID, toUserID, actorID uuid.UUID) error

	CheckApproverAssignmentsCalls int
	ReassignApproverRulesCalls    int
}

// CheckApproverAssignments records the call and delegates to the stub, or
// defaults to "no rules" so the removal proceeds.
func (f *FakeExpenseClient) CheckApproverAssignments(ctx context.Context, orgID, userID uuid.UUID) (bool, []ApproverRuleRef, error) {
	f.CheckApproverAssignmentsCalls++
	if f.CheckApproverAssignmentsFn != nil {
		return f.CheckApproverAssignmentsFn(ctx, orgID, userID)
	}
	return false, []ApproverRuleRef{}, nil
}

// ReassignApproverRules records the call and delegates to the stub, or
// defaults to success.
func (f *FakeExpenseClient) ReassignApproverRules(ctx context.Context, orgID, fromUserID, toUserID, actorID uuid.UUID) error {
	f.ReassignApproverRulesCalls++
	if f.ReassignApproverRulesFn != nil {
		return f.ReassignApproverRulesFn(ctx, orgID, fromUserID, toUserID, actorID)
	}
	return nil
}
