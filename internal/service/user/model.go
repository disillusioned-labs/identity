package user

import (
	"time"

	"github.com/disillusioned-labs/identity/internal/repository"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID
	Name      string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func fromCreate(r repository.CreateUserRow) User {
	return User{ID: r.ID, Name: r.Name, Email: r.Email, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func fromGet(r repository.GetUserRow) User {
	return User{ID: r.ID, Name: r.Name, Email: r.Email, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func fromUpdate(r repository.UpdateUserRow) User {
	return User{ID: r.ID, Name: r.Name, Email: r.Email, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func toUsers(rows []repository.ListUsersRow) []User {
	out := make([]User, 0, len(rows))
	for _, r := range rows {
		out = append(out, User{ID: r.ID, Name: r.Name, Email: r.Email, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	}
	return out
}
