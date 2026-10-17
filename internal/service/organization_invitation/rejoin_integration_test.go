//go:build integration

package organization_invitation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/disillusioned-labs/identity/db/migrations"
	"github.com/disillusioned-labs/identity/internal/contract"
	"github.com/disillusioned-labs/identity/internal/repository"
	identityservice "github.com/disillusioned-labs/identity/internal/service"
	"github.com/disillusioned-labs/platform/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestAcceptInvitationReactivatesRemovedMemberAfterProjectAccessCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := newInvitationIntegrationPool(t, ctx)

	t.Run("rejoin clears old placements before restoring membership", func(t *testing.T) {
		fixture := createInvitationRejoinFixture(t, ctx, pool, "removed")
		expense := &rejoinExpenseClient{removed: 2}
		service := NewOrganizationInvitationService(repository.NewStore(pool), expense, "https://example.test/invite", slog.New(slog.DiscardHandler))

		if _, err := service.AcceptInvitation(ctx, AcceptInvitationInput{UserID: fixture.userID, InvitationID: fixture.invitationID}); err != nil {
			t.Fatalf("AcceptInvitation(): %v", err)
		}
		if expense.calls != 1 || expense.orgID != fixture.organizationID || expense.userID != fixture.userID || expense.actorID != fixture.ownerID {
			t.Fatalf("project access cleanup calls = %+v; want one call scoped to member and active organization admin", expense)
		}
		assertInvitationRejoinState(t, ctx, pool, fixture, false, true)
	})

	t.Run("expense outage leaves removed member and invitation unchanged", func(t *testing.T) {
		fixture := createInvitationRejoinFixture(t, ctx, pool, "removed")
		expense := &rejoinExpenseClient{err: errors.New("expense unavailable")}
		service := NewOrganizationInvitationService(repository.NewStore(pool), expense, "https://example.test/invite", slog.New(slog.DiscardHandler))

		if _, err := service.AcceptInvitationByToken(ctx, AcceptInvitationByTokenInput{
			UserID: fixture.userID,
			Token:  fixture.token,
		}); !errors.Is(err, identityservice.ErrExpenseServiceDown) {
			t.Fatalf("AcceptInvitationByToken() error = %v, want expense unavailable", err)
		}
		if expense.calls != 1 {
			t.Fatalf("project access cleanup calls = %d, want 1", expense.calls)
		}
		assertInvitationRejoinState(t, ctx, pool, fixture, true, false)
	})

	t.Run("new member does not call cleanup", func(t *testing.T) {
		fixture := createInvitationRejoinFixture(t, ctx, pool, "new")
		expense := &rejoinExpenseClient{}
		service := NewOrganizationInvitationService(repository.NewStore(pool), expense, "https://example.test/invite", slog.New(slog.DiscardHandler))

		if _, err := service.AcceptInvitation(ctx, AcceptInvitationInput{UserID: fixture.userID, InvitationID: fixture.invitationID}); err != nil {
			t.Fatalf("AcceptInvitation(): %v", err)
		}
		if expense.calls != 0 {
			t.Fatalf("project access cleanup calls = %d, want 0", expense.calls)
		}
		assertInvitationRejoinState(t, ctx, pool, fixture, false, true)
	})

	t.Run("already active member is rejected without cleanup", func(t *testing.T) {
		fixture := createInvitationRejoinFixture(t, ctx, pool, "active")
		expense := &rejoinExpenseClient{}
		service := NewOrganizationInvitationService(repository.NewStore(pool), expense, "https://example.test/invite", slog.New(slog.DiscardHandler))

		if _, err := service.AcceptInvitation(ctx, AcceptInvitationInput{UserID: fixture.userID, InvitationID: fixture.invitationID}); !errors.Is(err, identityservice.ErrConflict) {
			t.Fatalf("AcceptInvitation() error = %v, want conflict", err)
		}
		if expense.calls != 0 {
			t.Fatalf("project access cleanup calls = %d, want 0", expense.calls)
		}
		assertInvitationRejoinState(t, ctx, pool, fixture, false, false)
	})
}

type invitationRejoinFixture struct {
	organizationID uuid.UUID
	ownerID        uuid.UUID
	userID         uuid.UUID
	invitationID   uuid.UUID
	token          string
}

func createInvitationRejoinFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, membership string) invitationRejoinFixture {
	t.Helper()
	fixture := invitationRejoinFixture{
		organizationID: uuid.New(),
		ownerID:        uuid.New(),
		userID:         uuid.New(),
		invitationID:   uuid.New(),
		token:          uuid.NewString(),
	}
	ownerEmail := fmt.Sprintf("owner-%s@example.test", fixture.ownerID)
	userEmail := fmt.Sprintf("member-%s@example.test", fixture.userID)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password, name) VALUES ($1, $2, 'hash', 'Owner'), ($3, $4, 'hash', 'Member')`, fixture.ownerID, ownerEmail, fixture.userID, userEmail); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name, type) VALUES ($1, 'Rejoin Test', 'business')`, fixture.organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, 'owner')`, fixture.organizationID, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	if membership != "new" {
		deletedAt := "NULL"
		if membership == "removed" {
			deletedAt = "now() - interval '1 day'"
		}
		query := fmt.Sprintf(`INSERT INTO organization_members (organization_id, user_id, role, deleted_at) VALUES ($1, $2, 'member', %s)`, deletedAt)
		if _, err := pool.Exec(ctx, query, fixture.organizationID, fixture.userID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_invitations (id, organization_id, email, role, token_hash, invited_by, expires_at) VALUES ($1, $2, $3, 'member', $4, $5, now() + interval '1 hour')`, fixture.invitationID, fixture.organizationID, userEmail, hashInvitationToken(fixture.token), fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func assertInvitationRejoinState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fixture invitationRejoinFixture, wantRemoved, wantAccepted bool) {
	t.Helper()
	var removed bool
	var role, invitationStatus string
	if err := pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL, role FROM organization_members WHERE organization_id = $1 AND user_id = $2`, fixture.organizationID, fixture.userID).Scan(&removed, &role); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM organization_invitations WHERE id = $1`, fixture.invitationID).Scan(&invitationStatus); err != nil {
		t.Fatal(err)
	}
	wantInvitationStatus := "pending"
	if wantAccepted {
		wantInvitationStatus = "accepted"
	}
	if removed != wantRemoved || role != "member" || invitationStatus != wantInvitationStatus {
		t.Fatalf("membership removed=%t role=%q invitation status=%q; want removed=%t role=member invitation status=%q", removed, role, invitationStatus, wantRemoved, wantInvitationStatus)
	}
}

type rejoinExpenseClient struct {
	calls   int
	orgID   uuid.UUID
	userID  uuid.UUID
	actorID uuid.UUID
	removed int64
	err     error
}

func (*rejoinExpenseClient) CheckApproverAssignments(context.Context, uuid.UUID, uuid.UUID) (bool, []contract.ApproverRuleRef, error) {
	return false, nil, nil
}

func (*rejoinExpenseClient) ReassignApproverRules(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (c *rejoinExpenseClient) ClearRejoinedMemberProjectAccess(_ context.Context, orgID, userID, actorID uuid.UUID) (int64, error) {
	c.calls++
	c.orgID, c.userID, c.actorID = orgID, userID, actorID
	return c.removed, c.err
}

func newInvitationIntegrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "identity",
				"POSTGRES_USER":     "postgres",
				"POSTGRES_PASSWORD": "postgres",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start PostgreSQL container: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := container.Terminate(cleanupCtx); err != nil {
			t.Errorf("terminate PostgreSQL container: %v", err)
		}
	})
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	dsn := fmt.Sprintf("postgres://postgres:postgres@%s/identity?sslmode=disable", net.JoinHostPort(host, port.Port()))
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool, migrations.FS, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("apply Identity migrations: %v", err)
	}
	return pool
}
