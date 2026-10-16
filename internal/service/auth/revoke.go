package auth

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/disillusioned-labs/platform/authkit"
)

// RevocationStore is the Redis-backed denylist store. Identity is the only
// writer: entries are written at the moment access is revoked, with a TTL
// equal to the access-token lifetime, because after that window every
// pre-revocation token has expired on its own. All other services read the
// same keys through authkit's Verifier.
type RevocationStore struct {
	client *goredis.Client
	reader authkit.Denylist
	ttl    time.Duration
}

func NewRevocationStore(client *goredis.Client, ttl time.Duration) *RevocationStore {
	return &RevocationStore{
		client: client,
		reader: authkit.NewRedisDenylist(client),
		ttl:    ttl,
	}
}

// RevokeUser denies every access token of the user issued so far, across all
// organizations. Later revocations overwrite the timestamp and extend the
// TTL; the check in authkit treats tokens issued at or before it as revoked.
func (s *RevocationStore) RevokeUser(ctx context.Context, subject string) error {
	return s.revoke(ctx, authkit.UserRevokeKey(subject))
}

// RevokeMember denies only the tokens carrying the given organization in
// org_id, leaving the user's sessions in their other organizations alone.
func (s *RevocationStore) RevokeMember(ctx context.Context, organizationID, subject string) error {
	return s.revoke(ctx, authkit.MemberRevokeKey(organizationID, subject))
}

func (s *RevocationStore) revoke(ctx context.Context, key string) error {
	err := s.client.Set(
		ctx,
		key,
		strconv.FormatInt(time.Now().Unix(), 10),
		s.ttl,
	).Err()
	if err != nil {
		return fmt.Errorf("write revoke key %q: %w", key, err)
	}
	return nil
}

// RevokedAt returns the latest revocation time among the keys, or the zero
// time when none of them is set. Malformed entries fail verification closed.
func (s *RevocationStore) RevokedAt(ctx context.Context, keys ...string) (time.Time, error) {
	return s.reader.RevokedAt(ctx, keys...)
}
