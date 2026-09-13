package service_access

import (
	"context"
	"errors"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("service/service_access")

type ServiceAccessService interface {
	GrantAccess(ctx context.Context, input GrantAccessInput) error
	RevokeAccess(ctx context.Context, input RevokeAccessInput) error
	RevokeAllAccess(ctx context.Context, input RevokeAllAccessInput) error
	ListAccessByOrgUser(ctx context.Context, input ListAccessByOrgUserInput) (ListAccessOutput, error)
	ListAccessByOrg(ctx context.Context, input ListAccessByOrgInput) (ListAccessOutput, error)
	IsServiceAllowed(ctx context.Context, organizationID, userID uuid.UUID, serviceName string) (bool, error)
}

type serviceAccessService struct {
	repo repository.Store
	log  *slog.Logger
}

func NewServiceAccessService(repo repository.Store, log *slog.Logger) ServiceAccessService {
	return &serviceAccessService{repo: repo, log: log}
}

func (s *serviceAccessService) GrantAccess(ctx context.Context, input GrantAccessInput) error {
	ctx, span := tracer.Start(ctx, "ServiceAccessService.GrantAccess")
	defer span.End()

	// Verify the granter is admin/owner of the organization.
	member, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         input.GrantedBy,
		OrganizationID: input.OrganizationID,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user organization failed")
		s.log.ErrorContext(ctx, "get user organization failed", "error", err)
		return service.ErrNotFound
	}
	if member.Role != constant.RoleOwner && member.Role != constant.RoleAdmin {
		return service.ErrForbidden
	}

	if err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		if err := q.GrantServiceAccess(ctx, repository.GrantServiceAccessParams{
			OrganizationID: input.OrganizationID,
			UserID:         input.UserID,
			ServiceName:    input.ServiceName,
			GrantedBy:      input.GrantedBy,
		}); err != nil {
			return err
		}
		return service.Emit(ctx, q, "service_access", input.UserID,
			EventAccessGranted, eventVersion, constant.TopicAudit, AccessGrantedEvent{
				OrganizationID: input.OrganizationID,
				UserID:         input.UserID,
				ServiceName:    input.ServiceName,
				ActorID:        input.GrantedBy,
			})
	}); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "grant service access failed")
		s.log.ErrorContext(ctx, "grant service access failed", "error", err)
		return service.ErrInternal
	}

	return nil
}

func (s *serviceAccessService) RevokeAccess(ctx context.Context, input RevokeAccessInput) error {
	ctx, span := tracer.Start(ctx, "ServiceAccessService.RevokeAccess")
	defer span.End()

	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		rows, err := q.RevokeServiceAccess(ctx, repository.RevokeServiceAccessParams{
			OrganizationID: input.OrganizationID,
			UserID:         input.UserID,
			ServiceName:    input.ServiceName,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return service.ErrNotFound
		}
		return service.Emit(ctx, q, "service_access", input.UserID,
			EventAccessRevoked, eventVersion, constant.TopicAudit, AccessRevokedEvent{
				OrganizationID: input.OrganizationID,
				UserID:         input.UserID,
				ServiceName:    input.ServiceName,
			})
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return service.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke service access failed")
		s.log.ErrorContext(ctx, "revoke service access failed", "error", err)
		return service.ErrInternal
	}
	return nil
}

func (s *serviceAccessService) RevokeAllAccess(ctx context.Context, input RevokeAllAccessInput) error {
	ctx, span := tracer.Start(ctx, "ServiceAccessService.RevokeAllAccess")
	defer span.End()

	if err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		if err := q.RevokeAllServiceAccess(ctx, repository.RevokeAllServiceAccessParams{
			OrganizationID: input.OrganizationID,
			UserID:         input.UserID,
		}); err != nil {
			return err
		}
		return service.Emit(ctx, q, "service_access", input.UserID,
			EventAccessRevokedAll, eventVersion, constant.TopicAudit, AccessRevokedAllEvent{
				OrganizationID: input.OrganizationID,
				UserID:         input.UserID,
			})
	}); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke all service access failed")
		s.log.ErrorContext(ctx, "revoke all service access failed", "error", err)
		return service.ErrInternal
	}
	return nil
}

func (s *serviceAccessService) ListAccessByOrgUser(ctx context.Context, input ListAccessByOrgUserInput) (ListAccessOutput, error) {
	ctx, span := tracer.Start(ctx, "ServiceAccessService.ListAccessByOrgUser")
	defer span.End()

	rows, err := s.repo.ListServiceAccessByOrgUser(ctx, repository.ListServiceAccessByOrgUserParams{
		OrganizationID: input.OrganizationID,
		UserID:         input.UserID,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list service access failed")
		s.log.ErrorContext(ctx, "list service access failed", "error", err)
		return ListAccessOutput{}, service.ErrInternal
	}

	return toListOutput(rows), nil
}

func (s *serviceAccessService) ListAccessByOrg(ctx context.Context, input ListAccessByOrgInput) (ListAccessOutput, error) {
	ctx, span := tracer.Start(ctx, "ServiceAccessService.ListAccessByOrg")
	defer span.End()

	rows, err := s.repo.ListServiceAccessByOrg(ctx, input.OrganizationID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list service access failed")
		s.log.ErrorContext(ctx, "list service access failed", "error", err)
		return ListAccessOutput{}, service.ErrInternal
	}

	return toListOutput(rows), nil
}

// IsServiceAllowed checks whether a user can access a service within an organization.
// Owner/admin members are always allowed (implicit access). Other members need
// an explicit row in organization_service_access.
//
// TODO: This method will be exposed via gRPC for cross-service calls.
// For now it is called directly within identity for testing purposes.
func (s *serviceAccessService) IsServiceAllowed(ctx context.Context, organizationID, userID uuid.UUID, serviceName string) (bool, error) {
	ctx, span := tracer.Start(ctx, "ServiceAccessService.IsServiceAllowed")
	defer span.End()

	// Owner/admin always have access to all services.
	member, err := s.repo.GetUserOrganization(ctx, repository.GetUserOrganizationParams{
		UserID:         userID,
		OrganizationID: organizationID,
	})
	if err != nil {
		if err.Error() == "no rows" {
			return false, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user organization failed")
		s.log.ErrorContext(ctx, "get user organization failed", "error", err)
		return false, service.ErrInternal
	}
	if member.Role == constant.RoleOwner || member.Role == constant.RoleAdmin {
		return true, nil
	}

	// Non-admin members need explicit access.
	allowed, err := s.repo.IsServiceAccessAllowed(ctx, repository.IsServiceAccessAllowedParams{
		OrganizationID: organizationID,
		UserID:         userID,
		ServiceName:    serviceName,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check service access failed")
		s.log.ErrorContext(ctx, "check service access failed", "error", err)
		return false, service.ErrInternal
	}

	return allowed, nil
}

func toListOutput(rows []repository.OrganizationServiceAccess) ListAccessOutput {
	out := make([]ServiceAccessOutput, 0, len(rows))
	for _, r := range rows {
		out = append(out, ServiceAccessOutput{
			ID:             r.ID,
			OrganizationID: r.OrganizationID,
			UserID:         r.UserID,
			ServiceName:    r.ServiceName,
			GrantedBy:      r.GrantedBy,
			CreatedAt:      r.CreatedAt,
		})
	}
	return ListAccessOutput{Accesses: out}
}
