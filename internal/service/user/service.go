package user

import (
	"context"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Service interface {
	Create(ctx context.Context, querier repository.Querier, input CreateInput) (User, error)
}

type svc struct {
	log    *slog.Logger
	tracer trace.Tracer
}

var _ Service = (*svc)(nil)

func New(log *slog.Logger) Service {
	return &svc{
		log:    log,
		tracer: otel.Tracer("service/user"),
	}
}

func (s *svc) Create(ctx context.Context, querier repository.Querier, input CreateInput) (User, error) {
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
			return User{}, service.ErrEmailTaken
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "create user failed")
		s.log.ErrorContext(ctx, "create user failed", "error", err)
		return User{}, service.ErrInternal
	}

	return User{ID: row.ID, Name: row.Name, Email: row.Email}, nil
}
