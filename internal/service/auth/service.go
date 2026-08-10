package auth

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"

	"github.com/disillusioned-labs/identity/internal/constant"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	jwtservice "github.com/disillusioned-labs/identity/internal/service/jwt"
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
	userservice "github.com/disillusioned-labs/identity/internal/service/user"
)

type AuthService interface {
	Register(ctx context.Context, input RegisterInput) (RegisterOutput, error)
	Login(ctx context.Context, input LoginInput) (LoginOutput, error)
}

type authService struct {
	repo    repository.Store
	users   userservice.UserService
	orgs    organizationservice.OrganizationService
	members organizationmemberservice.OrganizationMemberService
	jwt     jwtservice.JWTService
	log     *slog.Logger
	tracer  trace.Tracer
}

func NewAuthService(
	repo repository.Store,
	users userservice.UserService,
	orgs organizationservice.OrganizationService,
	members organizationmemberservice.OrganizationMemberService,
	jwt jwtservice.JWTService,
	log *slog.Logger,
) AuthService {
	return &authService{
		repo:    repo,
		users:   users,
		orgs:    orgs,
		members: members,
		jwt:     jwt,
		log:     log,
		tracer:  otel.Tracer("service/auth"),
	}
}

func (s *authService) Register(ctx context.Context, input RegisterInput) (RegisterOutput, error) {
	ctx, span := s.tracer.Start(ctx, "AuthService.Register")
	defer span.End()

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash password failed")
		s.log.ErrorContext(ctx, "hash password failed", "error", err)
		return RegisterOutput{}, service.ErrInternal
	}

	var createdUser userservice.CreateOutput
	var createdOrg organizationservice.CreateOutput

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

		if err := s.users.SetLastActiveOrganization(ctx, querier, userservice.SetLastActiveOrganizationInput{
			UserID:         u.ID,
			OrganizationID: org.ID,
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
		UserID:    createdUser.ID,
		OrgID:     createdOrg.ID,
		Role:      constant.RoleOwner,
		UserAgent: input.UserAgent,
		IPAddress: input.IPAddress,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "issue tokens failed")
		s.log.ErrorContext(ctx, "issue tokens failed", "error", err)
		return RegisterOutput{}, service.ErrInternal
	}

	return RegisterOutput{
		User: UserOutput{
			ID:    createdUser.ID,
			Name:  createdUser.Name,
			Email: createdUser.Email,
		},
		Organization: OrganizationOutput{
			ID:   createdOrg.ID,
			Name: createdOrg.Name,
			Type: createdOrg.Type,
			Role: constant.RoleOwner,
		},
		Tokens: TokensOutput{
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
			ExpiresIn:    tokens.ExpiresIn,
		},
	}, nil
}

var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

func (s *authService) Login(ctx context.Context, input LoginInput) (LoginOutput, error) {
	ctx, span := s.tracer.Start(ctx, "AuthService.Login")
	defer span.End()

	user, err := s.users.GetByEmail(ctx, s.repo, userservice.GetByEmailInput{Email: input.Email})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(input.Password))
			span.SetStatus(codes.Error, "invalid credentials")
			return LoginOutput{}, service.ErrUnauthenticated
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "lookup user failed")
		return LoginOutput{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(input.Password)); err != nil {
		span.SetStatus(codes.Error, "invalid credentials")
		return LoginOutput{}, service.ErrUnauthenticated
	}

	memberships, err := s.members.ListByUser(ctx, s.repo, organizationmemberservice.ListByUserInput{UserID: user.ID})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list memberships failed")
		return LoginOutput{}, err
	}
	if len(memberships) == 0 {
		span.SetStatus(codes.Error, "user has no active organization")
		s.log.ErrorContext(ctx, "user has no active organization", "user_id", user.ID)
		return LoginOutput{}, service.ErrInternal
	}

	active := selectActiveOrganization(memberships, user.LastActiveOrganizationID)

	span.SetAttributes(
		attribute.String("user.id", user.ID.String()),
		attribute.String("organization.id", active.OrganizationID.String()),
	)

	if user.LastActiveOrganizationID == nil || *user.LastActiveOrganizationID != active.OrganizationID {
		if err := s.users.SetLastActiveOrganization(ctx, s.repo, userservice.SetLastActiveOrganizationInput{
			UserID:         user.ID,
			OrganizationID: active.OrganizationID,
		}); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "set last active organization failed")
			return LoginOutput{}, err
		}
	}

	tokens, err := s.jwt.Issue(ctx, jwtservice.IssueInput{
		UserID:    user.ID,
		OrgID:     active.OrganizationID,
		Role:      active.Role,
		UserAgent: input.UserAgent,
		IPAddress: input.IPAddress,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "issue tokens failed")
		s.log.ErrorContext(ctx, "issue tokens failed", "error", err)
		return LoginOutput{}, service.ErrInternal
	}

	return LoginOutput{
		User: UserOutput{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
		},
		Organization: OrganizationOutput{
			ID:   active.OrganizationID,
			Name: active.OrganizationName,
			Type: active.OrganizationType,
			Role: active.Role,
		},
		Tokens: TokensOutput{
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
			ExpiresIn:    tokens.ExpiresIn,
		},
	}, nil
}

func selectActiveOrganization(
	memberships []organizationmemberservice.ListByUserOutput,
	preferred *uuid.UUID,
) organizationmemberservice.ListByUserOutput {
	if preferred != nil {
		for _, m := range memberships {
			if m.OrganizationID == *preferred {
				return m
			}
		}
	}
	return memberships[0]
}
