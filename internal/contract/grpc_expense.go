package contract

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/disillusioned-labs/identity/internal/service"
	memberpb "github.com/disillusioned-labs/platform/contract/member"
	platformgrpc "github.com/disillusioned-labs/platform/grpc"
)

var tracer = otel.Tracer("contract/expense")

// grpcExpenseClient wraps the expense gRPC client with domain types and
// observability.
type grpcExpenseClient struct {
	client memberpb.MemberServiceClient
	log    *slog.Logger
}

// NewGRPCExpenseClient creates an ExpenseClient backed by a gRPC connection.
func NewGRPCExpenseClient(conn *platformgrpc.Client, log *slog.Logger) ExpenseClient {
	return &grpcExpenseClient{
		client: memberpb.NewMemberServiceClient(conn.Conn()),
		log:    log,
	}
}

func (c *grpcExpenseClient) CheckApproverAssignments(ctx context.Context, orgID, userID uuid.UUID) (bool, []ApproverRuleRef, error) {
	ctx, span := tracer.Start(ctx, "ExpenseClient.CheckApproverAssignments")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization.id", orgID.String()),
		attribute.String("user.id", userID.String()),
	)

	resp, err := c.client.CheckApproverAssignments(ctx, &memberpb.CheckApproverAssignmentsRequest{
		OrganizationId: orgID.String(),
		UserId:         userID.String(),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "gRPC call failed")
		return false, nil, err
	}

	rules := make([]ApproverRuleRef, 0, len(resp.GetRules()))
	for _, r := range resp.GetRules() {
		ref := ApproverRuleRef{Step: int(r.GetStep())}
		if r.GetId() != "" {
			id, err := uuid.Parse(r.GetId())
			if err != nil {
				c.log.WarnContext(ctx, "invalid rule id in CheckApproverAssignments response",
					"rule_id", r.GetId(), "error", err)
				continue
			}
			ref.ID = id
		}
		if r.GetProjectId() != "" {
			projectID, err := uuid.Parse(r.GetProjectId())
			if err != nil {
				c.log.WarnContext(ctx, "invalid project id in CheckApproverAssignments response",
					"project_id", r.GetProjectId(), "error", err)
			} else {
				ref.ProjectID = &projectID
			}
		}
		rules = append(rules, ref)
	}

	return resp.GetHasActiveRules(), rules, nil
}

func (c *grpcExpenseClient) ReassignApproverRules(ctx context.Context, orgID, fromUserID, toUserID, actorID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "ExpenseClient.ReassignApproverRules")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization.id", orgID.String()),
		attribute.String("from_user.id", fromUserID.String()),
		attribute.String("to_user.id", toUserID.String()),
		attribute.String("actor.id", actorID.String()),
	)

	_, err := c.client.ReassignApproverRules(ctx, &memberpb.ReassignApproverRulesRequest{
		OrganizationId: orgID.String(),
		FromUserId:     fromUserID.String(),
		ToUserId:       toUserID.String(),
		ActorId:        actorID.String(),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "gRPC call failed")
		// Domain failures from expense (the replacement is not a member,
		// is already an approver, rules vanished mid-flow) carry their own
		// status and message so the admin sees the real reason - only an
		// unreachable expense stays a transport error.
		return asServiceError(err)
	}
	return nil
}

// asServiceError re-maps expense's gRPC codes onto self-describing domain
// errors; anything unmapped stays a transport error for the caller to wrap.
func asServiceError(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	switch st.Code() {
	case codes.AlreadyExists:
		return service.NewError("APPROVER_ALREADY_EXISTS", http.StatusConflict, st.Message())
	case codes.NotFound:
		return service.NewError("NOT_FOUND", http.StatusNotFound, st.Message())
	case codes.InvalidArgument, codes.OutOfRange:
		return service.NewError("INVALID_REASSIGN_TARGET", http.StatusUnprocessableEntity, st.Message())
	case codes.PermissionDenied:
		return service.NewError("FORBIDDEN", http.StatusForbidden, st.Message())
	default:
		return err
	}
}
