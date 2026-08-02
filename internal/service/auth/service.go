package auth

import (
	"context"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	jwtservice "github.com/disillusioned-labs/identity/internal/service/jwt"
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
	userservice "github.com/disillusioned-labs/identity/internal/service/user"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"
)

type Service interface {
	Register(ctx context.Context, input RegisterInput) (RegisterOutput, error)
}

type svc struct {
	repo    repository.Store
	users   userservice.Service
	orgs    organizationservice.Service
	members organizationmemberservice.Service
	jwt     jwtservice.Service
	log     *slog.Logger
	tracer  trace.Tracer
}

var _ Service = (*svc)(nil)

func New(repo repository.Store, users userservice.Service, orgs organizationservice.Service, members organizationmemberservice.Service, jwt jwtservice.Service, log *slog.Logger) Service {
	return &svc{
		repo:    repo,
		users:   users,
		orgs:    orgs,
		members: members,
		jwt:     jwt,
		log:     log,
		tracer:  otel.Tracer("service/auth"),
	}
}

func (s *svc) Register(ctx context.Context, input RegisterInput) (RegisterOutput, error) {
	ctx, span := s.tracer.Start(ctx, "AuthService.Register",
		trace.WithAttributes(attribute.String("user.email", input.Email)),
	)
	defer span.End()

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash password failed")
		s.log.ErrorContext(ctx, "hash password failed", "error", err)
		return RegisterOutput{}, service.ErrInternal
	}

	var createdUser userservice.User
	var createdOrg organizationservice.Organization

	err = s.repo.ExecTx(ctx, func(querier repository.Querier) error {
		u, err := s.users.Create(ctx, querier, userservice.CreateInput{
			Name:           input.Name,
			Email:          input.Email,
			HashedPassword: string(hash),
		})
		if err != nil {
			return err
		}

		org, err := s.orgs.Create(ctx, querier, organizationservice.CreateInput{
			Name: "Personal " + input.Name,
			Type: constant.OrganizationTypePersonal,
		})
		if err != nil {
			return err
		}

		if err := s.members.Create(ctx, querier, organizationmemberservice.CreateInput{
			OrganizationID: org.ID,
			UserID:         u.ID,
			Role:           constant.RoleOwner,
		}); err != nil {
			return err
		}

		createdUser = u
		createdOrg = org
		return nil
	})
	if err != nil {
		if !service.IsError(err) {
			span.RecordError(err)
			span.SetStatus(codes.Error, "register transaction failed")
			s.log.ErrorContext(ctx, "register transaction failed", "error", err)
			return RegisterOutput{}, service.ErrInternal
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return RegisterOutput{}, err
	}

	span.SetAttributes(
		attribute.String("user.id", createdUser.ID.String()),
		attribute.String("organization.id", createdOrg.ID.String()),
	)

	tokens, err := s.jwt.Issue(ctx, jwtservice.IssueInput{
		UserID: createdUser.ID,
		OrgID:  createdOrg.ID,
		Role:   constant.RoleOwner,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "issue tokens failed")
		s.log.ErrorContext(ctx, "issue tokens failed", "error", err)
		return RegisterOutput{}, service.ErrInternal
	}

	return RegisterOutput{
		User: User{
			ID:    createdUser.ID,
			Name:  createdUser.Name,
			Email: createdUser.Email,
		},
		Organization: Organization{
			ID:   createdOrg.ID,
			Name: createdOrg.Name,
			Type: createdOrg.Type,
			Role: constant.RoleOwner,
		},
		Tokens: Tokens{
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
			ExpiresIn:    tokens.ExpiresIn,
		},
	}, nil
}
