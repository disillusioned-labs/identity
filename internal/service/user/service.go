package user

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
)

type UserService interface {
	Create(ctx context.Context, querier repository.Querier, input CreateInput) (CreateOutput, error)
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
