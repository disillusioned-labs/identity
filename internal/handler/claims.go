package handler

import (
	"context"

	"github.com/disillusioned-labs/authkit"
)

func ClaimsFrom(ctx context.Context) (authkit.Claims, bool) {
	return authkit.FromContext(ctx)
}
