package grpc

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authservice "github.com/disillusioned-labs/identity/internal/service/auth"
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
	serviceservice "github.com/disillusioned-labs/identity/internal/service/service_access"
	identitypb "github.com/disillusioned-labs/platform/contract/identity"
	platformerrors "github.com/disillusioned-labs/platform/errors"
)

var tracer = otel.Tracer("handler/grpc")

type IdentityServer struct {
	identitypb.UnimplementedIdentityServiceServer
	authService               authservice.AuthService
	organizationService       organizationservice.OrganizationService
	organizationMemberService organizationmemberservice.OrganizationMemberService
	serviceAccessService      serviceservice.ServiceAccessService
	log                       *slog.Logger
}

func NewIdentityServer(
	authService authservice.AuthService,
	organizationService organizationservice.OrganizationService,
	organizationMemberService organizationmemberservice.OrganizationMemberService,
	serviceAccessService serviceservice.ServiceAccessService,
	log *slog.Logger,
) *IdentityServer {
	return &IdentityServer{
		authService:               authService,
		organizationService:       organizationService,
		organizationMemberService: organizationMemberService,
		serviceAccessService:      serviceAccessService,
		log:                       log,
	}
}

// IsMemberActive reports whether a user is an active member of an organization.
func (s *IdentityServer) IsMemberActive(
	ctx context.Context,
	req *identitypb.IsMemberActiveRequest,
) (*identitypb.IsMemberActiveResponse, error) {
	ctx, span := tracer.Start(ctx, "IdentityServer.IsMemberActive")
	defer span.End()

	organizationID, err := uuid.Parse(req.GetOrganizationId())
	if err != nil {
		span.SetStatus(otelcodes.Error, "invalid organization_id")
		return nil, status.Error(codes.InvalidArgument, "invalid organization_id")
	}

	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		span.SetStatus(otelcodes.Error, "invalid user_id")
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	output, err := s.organizationMemberService.GetMember(ctx, organizationmemberservice.GetMemberInput{
		OrganizationID: organizationID,
		UserID:         userID,
	})
	if err != nil {
		return nil, writeServiceErr(ctx, span, s.log, err, "get member failed",
			"organization_id", organizationID, "user_id", userID)
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
	)

	return &identitypb.IsMemberActiveResponse{
		IsActive: true,
		Role:     output.Member.Role,
	}, nil
}

// GetMemberRole returns the user's role within an organization.
func (s *IdentityServer) GetMemberRole(
	ctx context.Context,
	req *identitypb.GetMemberRoleRequest,
) (*identitypb.GetMemberRoleResponse, error) {
	ctx, span := tracer.Start(ctx, "IdentityServer.GetMemberRole")
	defer span.End()

	organizationID, err := uuid.Parse(req.GetOrganizationId())
	if err != nil {
		span.SetStatus(otelcodes.Error, "invalid organization_id")
		return nil, status.Error(codes.InvalidArgument, "invalid organization_id")
	}

	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		span.SetStatus(otelcodes.Error, "invalid user_id")
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	output, err := s.organizationMemberService.GetMember(ctx, organizationmemberservice.GetMemberInput{
		OrganizationID: organizationID,
		UserID:         userID,
	})
	if err != nil {
		return nil, writeServiceErr(ctx, span, s.log, err, "get member role failed",
			"organization_id", organizationID, "user_id", userID)
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
	)

	return &identitypb.GetMemberRoleResponse{
		Role: output.Member.Role,
	}, nil
}

// GetUsersInfo returns information for a batch of users.
func (s *IdentityServer) GetUsersInfo(
	ctx context.Context,
	req *identitypb.GetUsersInfoRequest,
) (*identitypb.GetUsersInfoResponse, error) {
	ctx, span := tracer.Start(ctx, "IdentityServer.GetUsersInfo")
	defer span.End()

	pairs := make([]authservice.UserOrgPair, 0, len(req.GetUsers()))
	for _, pair := range req.GetUsers() {
		userID, err := uuid.Parse(pair.GetUserId())
		if err != nil {
			span.SetStatus(otelcodes.Error, "invalid user_id")
			return nil, status.Errorf(codes.InvalidArgument, "invalid user_id: %s", pair.GetUserId())
		}
		orgID, err := uuid.Parse(pair.GetOrganizationId())
		if err != nil {
			span.SetStatus(otelcodes.Error, "invalid organization_id")
			return nil, status.Errorf(codes.InvalidArgument, "invalid organization_id: %s", pair.GetOrganizationId())
		}
		pairs = append(pairs, authservice.UserOrgPair{
			UserID:         userID,
			OrganizationID: orgID,
		})
	}

	output, err := s.authService.GetUsersByIDs(ctx, authservice.GetUsersByIDsInput{
		Users: pairs,
	})
	if err != nil {
		return nil, writeServiceErr(ctx, span, s.log, err, "get users info failed",
			"user_count", len(pairs))
	}

	users := make([]*identitypb.UserInfo, 0, len(output.Users))
	for _, u := range output.Users {
		users = append(users, &identitypb.UserInfo{
			UserId:   u.ID.String(),
			Name:     u.Name,
			Email:    u.Email,
			Role:     u.Role,
			IsActive: u.IsActive,
		})
	}

	span.SetAttributes(attribute.Int("user.count", len(users)))

	return &identitypb.GetUsersInfoResponse{
		Users: users,
	}, nil
}

// GetOrgStatus returns the current status of an organization.
func (s *IdentityServer) GetOrgStatus(
	ctx context.Context,
	req *identitypb.GetOrgStatusRequest,
) (*identitypb.GetOrgStatusResponse, error) {
	ctx, span := tracer.Start(ctx, "IdentityServer.GetOrgStatus")
	defer span.End()

	organizationID, err := uuid.Parse(req.GetOrganizationId())
	if err != nil {
		span.SetStatus(otelcodes.Error, "invalid organization_id")
		return nil, status.Error(codes.InvalidArgument, "invalid organization_id")
	}

	output, err := s.organizationService.GetOrganizationStatus(ctx, organizationservice.GetOrganizationStatusInput{
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, writeServiceErr(ctx, span, s.log, err, "get org status failed",
			"organization_id", organizationID)
	}

	span.SetAttributes(attribute.String("organization.id", organizationID.String()))

	return &identitypb.GetOrgStatusResponse{
		IsFrozen:  output.IsFrozen,
		IsDeleted: output.IsDeleted,
	}, nil
}

// IsServiceAccessAllowed reports whether a user is allowed to access a service
// within an organization.
func (s *IdentityServer) IsServiceAccessAllowed(
	ctx context.Context,
	req *identitypb.IsServiceAccessAllowedRequest,
) (*identitypb.IsServiceAccessAllowedResponse, error) {
	ctx, span := tracer.Start(ctx, "IdentityServer.IsServiceAccessAllowed")
	defer span.End()

	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		span.SetStatus(otelcodes.Error, "invalid user_id")
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	organizationID, err := uuid.Parse(req.GetOrganizationId())
	if err != nil {
		span.SetStatus(otelcodes.Error, "invalid organization_id")
		return nil, status.Error(codes.InvalidArgument, "invalid organization_id")
	}

	serviceName := req.GetServiceName()
	if serviceName == "" {
		span.SetStatus(otelcodes.Error, "service_name is required")
		return nil, status.Error(codes.InvalidArgument, "service_name is required")
	}

	allowed, err := s.serviceAccessService.IsServiceAllowed(ctx, organizationID, userID, serviceName)
	if err != nil {
		return nil, writeServiceErr(ctx, span, s.log, err, "check service access failed",
			"organization_id", organizationID, "user_id", userID, "service_name", serviceName)
	}

	span.SetAttributes(
		attribute.String("organization.id", organizationID.String()),
		attribute.String("user.id", userID.String()),
		attribute.String("service.name", serviceName),
	)

	return &identitypb.IsServiceAccessAllowedResponse{
		Allowed: allowed,
	}, nil
}

// writeServiceErr maps context errors, domain errors, and unexpected errors to
// gRPC status codes. It mirrors handler.WriteServiceError's precedence: context
// errors first, then domain errors, then anything else as internal.
func writeServiceErr(ctx context.Context, span trace.Span, log *slog.Logger, err error, msg string, args ...any) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		switch {
		case errors.Is(ctxErr, context.DeadlineExceeded):
			span.SetStatus(otelcodes.Error, "request timeout")
			log.WarnContext(ctx, msg, append(args, "error", err)...)
			return status.Error(codes.DeadlineExceeded, "request timed out")
		case errors.Is(ctxErr, context.Canceled):
			span.SetStatus(otelcodes.Error, "client disconnected")
			log.DebugContext(ctx, msg, append(args, "error", err)...)
			return status.Error(codes.Canceled, "context canceled")
		}
	}

	var domainErr *platformerrors.Error
	if errors.As(err, &domainErr) {
		span.SetStatus(otelcodes.Error, domainErr.Code)
		return serviceErrToGRPC(domainErr)
	}

	span.RecordError(err)
	span.SetStatus(otelcodes.Error, "internal error")
	log.ErrorContext(ctx, "unhandled "+msg, append(args, "error", err)...)
	return status.Error(codes.Internal, "internal server error")
}

// serviceErrToGRPC maps a domain error to a gRPC status error.
func serviceErrToGRPC(err *platformerrors.Error) error {
	switch err.Status {
	case 400:
		return status.Error(codes.InvalidArgument, err.Message)
	case 401:
		return status.Error(codes.Unauthenticated, err.Message)
	case 403:
		return status.Error(codes.PermissionDenied, err.Message)
	case 404:
		return status.Error(codes.NotFound, err.Message)
	case 409:
		return status.Error(codes.AlreadyExists, err.Message)
	case 410:
		return status.Error(codes.NotFound, err.Message)
	default:
		return status.Error(codes.Internal, err.Message)
	}
}
