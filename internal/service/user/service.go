package user

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
)

type UserService interface {
	Create(ctx context.Context, querier repository.Querier, input CreateInput) (CreateOutput, error)
	GetByEmail(ctx context.Context, querier repository.Querier, input GetByEmailInput) (GetByEmailOutput, error)
	GetByID(ctx context.Context, querier repository.Querier, input GetByIDInput) (GetByIDOutput, error)
	SetLastActiveOrganization(ctx context.Context, querier repository.Querier, input SetLastActiveOrganizationInput) error
}

type userService struct {
	log    *slog.Logger
	tracer trace.Tracer
}

func NewUserService(log *slog.Logger) UserService {
	return &userService{
		log:    log,
		tracer: otel.Tracer("service/user"),
	}
}

func (s *userService) Create(ctx context.Context, querier repository.Querier, input CreateInput) (CreateOutput, error) {
	ctx, span := s.tracer.Start(ctx, "UserService.Create")
	defer span.End()

	row, err := querier.CreateUser(ctx, repository.CreateUserParams{
		Email:    input.Email,
		Password: input.HashedPassword,
		Name:     input.Name,
	})
	if err != nil {
		if service.IsUniqueViolation(err) {
			span.RecordError(err)
			span.SetStatus(codes.Error, "email already taken")
			return CreateOutput{}, service.ErrEmailTaken
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "create user failed")
		s.log.ErrorContext(ctx, "create user failed", "error", err)
		return CreateOutput{}, service.ErrInternal
	}

	return CreateOutput{ID: row.ID, Name: row.Name, Email: row.Email}, nil
}

func (s *userService) GetByEmail(ctx context.Context, querier repository.Querier, input GetByEmailInput) (GetByEmailOutput, error) {
	ctx, span := s.tracer.Start(ctx, "UserService.GetByEmail")
	defer span.End()

	row, err := querier.GetUserByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "user not found")
			return GetByEmailOutput{}, service.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by email failed")
		s.log.ErrorContext(ctx, "get user by email failed", "error", err)
		return GetByEmailOutput{}, service.ErrInternal
	}

	return GetByEmailOutput{
		ID:                       row.ID,
		Name:                     row.Name,
		Email:                    row.Email,
		HashedPassword:           row.Password,
		LastActiveOrganizationID: row.LastActiveOrganizationID,
	}, nil
}

func (s *userService) GetByID(ctx context.Context, querier repository.Querier, input GetByIDInput) (GetByIDOutput, error) {
	ctx, span := s.tracer.Start(ctx, "UserService.GetByID")
	defer span.End()

	row, err := querier.GetUserByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Error, "user not found")
			return GetByIDOutput{}, service.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by id failed")
		s.log.ErrorContext(ctx, "get user by id failed", "error", err)
		return GetByIDOutput{}, service.ErrInternal
	}

	return GetByIDOutput{
		ID:                       row.ID,
		Name:                     row.Name,
		Email:                    row.Email,
		LastActiveOrganizationID: row.LastActiveOrganizationID,
	}, nil
}

func (s *userService) SetLastActiveOrganization(ctx context.Context, querier repository.Querier, input SetLastActiveOrganizationInput) error {
	ctx, span := s.tracer.Start(ctx, "UserService.SetLastActiveOrganization")
	defer span.End()

	rows, err := querier.SetLastActiveOrganization(ctx, repository.SetLastActiveOrganizationParams{
		ID:                       input.UserID,
		LastActiveOrganizationID: &input.OrganizationID,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "set last active organization failed")
		s.log.ErrorContext(ctx, "set last active organization failed", "error", err)
		return service.ErrInternal
	}
	if rows == 0 {
		span.SetStatus(codes.Error, "user not found")
		return service.ErrNotFound
	}

	return nil
}
