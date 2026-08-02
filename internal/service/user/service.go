package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/disillusioned-labs/identity/internal/platform/cache"
	"github.com/disillusioned-labs/identity/internal/repository"
	"github.com/disillusioned-labs/identity/internal/service"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"
)

type Service interface {
	Create(ctx context.Context, in CreateInput) (User, error)
	Get(ctx context.Context, id uuid.UUID) (User, error)
	List(ctx context.Context, limit, offset int32) ([]User, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateInput) (User, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type svc struct {
	repo   repository.Store
	cache  cache.Cache
	log    *slog.Logger
	tracer trace.Tracer
}

var _ Service = (*svc)(nil)

func New(repo repository.Store, c cache.Cache, log *slog.Logger) Service {
	return &svc{
		repo:   repo,
		cache:  c,
		log:    log,
		tracer: otel.Tracer("service/user"),
	}
}

func cacheKey(id uuid.UUID) string { return "user:" + id.String() }

func (s *svc) Create(ctx context.Context, in CreateInput) (User, error) {
	ctx, span := s.tracer.Start(ctx, "UserService.Create",
		trace.WithAttributes(attribute.String("user.email", in.Email)),
	)
	defer span.End()

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash password failed")
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, repository.CreateUserParams{
		Email:    in.Email,
		Password: string(hash),
		Name:     in.Name,
	})
	if err != nil {
		if service.IsUniqueViolation(err) {
			span.RecordError(err)
			span.SetStatus(codes.Error, "email already taken")
			return User{}, service.ErrEmailTaken
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "create user failed")
		return User{}, fmt.Errorf("create user: %w", err)
	}
	span.SetAttributes(attribute.String("user.id", user.ID.String()))
	return fromCreate(user), nil
}

func (s *svc) Get(ctx context.Context, id uuid.UUID) (User, error) {
	ctx, span := s.tracer.Start(ctx, "UserService.Get",
		trace.WithAttributes(attribute.String("user.id", id.String())),
	)
	defer span.End()

	var user User
	if s.cache != nil {
		hit, err := s.cache.Get(ctx, cacheKey(id), &user)
		if err != nil {
			s.log.WarnContext(ctx, "cache read failed", "error", err)
		} else if hit {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			return user, nil
		}
	}
	span.SetAttributes(attribute.Bool("cache.hit", false))

	row, err := s.repo.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		span.SetStatus(codes.Error, "user not found")
		return User{}, service.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user failed")
		return User{}, fmt.Errorf("get user %s: %w", id, err)
	}
	user = fromGet(row)

	if s.cache != nil {
		if err := s.cache.Set(ctx, cacheKey(id), user); err != nil {
			s.log.WarnContext(ctx, "cache write failed", "error", err)
		}
	}
	return user, nil
}

func (s *svc) List(ctx context.Context, limit, offset int32) ([]User, error) {
	ctx, span := s.tracer.Start(ctx, "UserService.List",
		trace.WithAttributes(
			attribute.Int("db.limit", int(limit)),
			attribute.Int("db.offset", int(offset)),
		),
	)
	defer span.End()

	users, err := s.repo.ListUsers(ctx, repository.ListUsersParams{Limit: limit, Offset: offset})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list users failed")
		return nil, fmt.Errorf("list users: %w", err)
	}
	span.SetAttributes(attribute.Int("db.result_count", len(users)))
	return toUsers(users), nil
}

func (s *svc) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (User, error) {
	ctx, span := s.tracer.Start(ctx, "UserService.Update",
		trace.WithAttributes(attribute.String("user.id", id.String())),
	)
	defer span.End()

	var updated repository.UpdateUserRow
	err := s.repo.ExecTx(ctx, func(q repository.Querier) error {
		current, err := q.GetUserForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock user %s: %w", id, err)
		}

		params := repository.UpdateUserParams{ID: id, Name: current.Name, Email: current.Email}
		if in.Name != nil {
			params.Name = *in.Name
		}
		if in.Email != nil {
			params.Email = *in.Email
		}

		updated, err = q.UpdateUser(ctx, params)
		if err != nil {
			if service.IsUniqueViolation(err) {
				return service.ErrEmailTaken
			}
			return fmt.Errorf("update user %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update user failed")
		return User{}, err
	}

	if s.cache != nil {
		if err := s.cache.Delete(ctx, cacheKey(id)); err != nil {
			s.log.WarnContext(ctx, "cache invalidation failed", "error", err)
		}
	}
	return fromUpdate(updated), nil
}

func (s *svc) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "UserService.Delete",
		trace.WithAttributes(attribute.String("user.id", id.String())),
	)
	defer span.End()

	rows, err := s.repo.DeleteUser(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete user failed")
		return fmt.Errorf("delete user %s: %w", id, err)
	}
	if rows == 0 {
		span.SetStatus(codes.Error, "user not found")
		return service.ErrNotFound
	}
	if s.cache != nil {
		if err := s.cache.Delete(ctx, cacheKey(id)); err != nil {
			s.log.WarnContext(ctx, "cache invalidation failed", "error", err)
		}
	}
	return nil
}
