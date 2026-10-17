//go:build integration

package organization

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disillusioned-labs/identity/db/migrations"
	"github.com/disillusioned-labs/identity/internal/contract"
	"github.com/disillusioned-labs/identity/internal/repository"
	identityservice "github.com/disillusioned-labs/identity/internal/service"
	organizationmember "github.com/disillusioned-labs/identity/internal/service/organization_member"
	serviceaccess "github.com/disillusioned-labs/identity/internal/service/service_access"
	"github.com/disillusioned-labs/platform/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestExpenseWriteLeaseSerializesAuthorizationChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool := newWriteAccessIntegrationPool(t, ctx)
	store := repository.NewStore(pool)
	log := slog.New(slog.DiscardHandler)
	organizations := NewOrganizationService(store, log)
	serviceAccess := serviceaccess.NewServiceAccessService(store, log)
	members := organizationmember.NewOrganizationMemberService(store, noopRevocationWriter{}, noApproverRules{}, log)

	t.Run("freeze waits for in-flight expense write", func(t *testing.T) {
		fixture := createWriteAccessFixture(t, ctx, pool)
		assertMutationWaitsForWriteLease(t, ctx, pool, organizations, fixture, func() error {
			return organizations.SetFrozen(ctx, SetFrozenInput{
				UserID: fixture.ownerID, OrganizationID: fixture.organizationID, Frozen: true,
			})
		}, func() bool {
			var frozen bool
			if err := pool.QueryRow(ctx, `SELECT frozen_at IS NOT NULL FROM organizations WHERE id = $1`, fixture.organizationID).Scan(&frozen); err != nil {
				t.Fatalf("read organization freeze status: %v", err)
			}
			return frozen
		}, false, true)
	})

	t.Run("service access revocation waits for in-flight expense write", func(t *testing.T) {
		fixture := createWriteAccessFixture(t, ctx, pool)
		if err := serviceAccess.GrantAccess(ctx, serviceaccess.GrantAccessInput{
			OrganizationID: fixture.organizationID,
			UserID:         fixture.targetID,
			ServiceName:    "expense",
			GrantedBy:      fixture.ownerID,
		}); err != nil {
			t.Fatalf("grant service access: %v", err)
		}

		assertMutationWaitsForWriteLease(t, ctx, pool, organizations, fixture, func() error {
			return serviceAccess.RevokeAccess(ctx, serviceaccess.RevokeAccessInput{
				OrganizationID: fixture.organizationID,
				UserID:         fixture.targetID,
				ServiceName:    "expense",
				RevokedBy:      fixture.ownerID,
			})
		}, func() bool {
			var allowed bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organization_service_access WHERE organization_id = $1 AND user_id = $2 AND service_name = 'expense')`, fixture.organizationID, fixture.targetID).Scan(&allowed); err != nil {
				t.Fatalf("read service access: %v", err)
			}
			return !allowed
		}, false, true)
	})

	t.Run("member removal waits for in-flight expense write", func(t *testing.T) {
		fixture := createWriteAccessFixture(t, ctx, pool)
		assertMutationWaitsForWriteLease(t, ctx, pool, organizations, fixture, func() error {
			_, err := members.RemoveOrganizationMember(ctx, organizationmember.RemoveOrganizationMemberInput{
				UserID: fixture.ownerID, OrganizationID: fixture.organizationID, TargetUserID: fixture.targetID,
			})
			return err
		}, func() bool {
			var removed bool
			if err := pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM organization_members WHERE organization_id = $1 AND user_id = $2`, fixture.organizationID, fixture.targetID).Scan(&removed); err != nil {
				t.Fatalf("read target membership: %v", err)
			}
			return removed
		}, false, true)
	})
}

func TestMemberRemovalRechecksApproverRulesAfterExpenseWriteLease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := newWriteAccessIntegrationPool(t, ctx)
	store := repository.NewStore(pool)
	log := slog.New(slog.DiscardHandler)
	organizations := NewOrganizationService(store, log)
	fixture := createWriteAccessFixture(t, ctx, pool)
	expense := &lateApproverAssignments{}
	members := organizationmember.NewOrganizationMemberService(store, noopRevocationWriter{}, expense, log)

	leaseReady := make(chan error, 1)
	leaseDone := make(chan error, 1)
	leaseReleased := make(chan struct{})
	go func() {
		leaseDone <- organizations.HoldExpenseWriteAccess(ctx, HoldExpenseWriteAccessInput{
			OrganizationID: fixture.organizationID, UserID: fixture.ownerID,
		}, func(HoldExpenseWriteAccessOutput) error {
			leaseReady <- nil
			<-leaseReleased
			return nil
		})
	}()
	if err := receiveWriteAccessError(t, ctx, leaseReady); err != nil {
		t.Fatalf("acquire write lease: %v", err)
	}

	removeDone := make(chan error, 1)
	go func() {
		_, err := members.RemoveOrganizationMember(ctx, organizationmember.RemoveOrganizationMemberInput{
			UserID: fixture.ownerID, OrganizationID: fixture.organizationID, TargetUserID: fixture.targetID,
		})
		removeDone <- err
	}()
	waitForWriteAccessLockWaiter(t, ctx, pool)
	if got := expense.checks.Load(); got != 1 {
		t.Fatalf("approver checks before releasing in-flight write = %d, want only preflight check", got)
	}
	close(leaseReleased)
	if err := receiveWriteAccessError(t, ctx, leaseDone); err != nil {
		t.Fatalf("release write lease: %v", err)
	}
	var domainErr *identityservice.Error
	if err := receiveWriteAccessError(t, ctx, removeDone); !errors.As(err, &domainErr) || domainErr.Code != "APPROVER_STILL_ASSIGNED" {
		t.Fatalf("remove after late approver assignment error = %v, want APPROVER_STILL_ASSIGNED", err)
	}
	if got := expense.checks.Load(); got != 2 {
		t.Fatalf("approver checks = %d, want preflight + locked recheck", got)
	}
	var removed bool
	if err := pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM organization_members WHERE organization_id = $1 AND user_id = $2`, fixture.organizationID, fixture.targetID).Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("member removal committed despite an approver rule found under the organization lock")
	}
}

func TestMemberRemovalRetriesAfterCrossServiceReassignmentCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := newWriteAccessIntegrationPool(t, ctx)
	store := repository.NewStore(pool)
	log := slog.New(slog.DiscardHandler)
	fixture := createWriteAccessFixture(t, ctx, pool)
	expense := &mutableApproverAssignments{assignedToTarget: true}
	members := organizationmember.NewOrganizationMemberService(store, noopRevocationWriter{}, expense, log)
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_member_removal_event() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.event_type = 'organization.member.removed' AND NEW.topic = 'audit' THEN
				RAISE EXCEPTION 'forced removal outbox failure';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER reject_member_removal_event BEFORE INSERT ON outbox_events
		FOR EACH ROW EXECUTE FUNCTION reject_member_removal_event();
	`); err != nil {
		t.Fatalf("install removal outbox failure: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_member_removal_event ON outbox_events`)
		_, _ = pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS reject_member_removal_event()`)
	}()
	replacement := fixture.ownerID
	input := organizationmember.RemoveOrganizationMemberInput{
		UserID: fixture.ownerID, OrganizationID: fixture.organizationID, TargetUserID: fixture.targetID,
		ReassignRulesTo: &replacement,
	}
	if _, err := members.RemoveOrganizationMember(ctx, input); err == nil {
		t.Fatal("first removal succeeded despite the forced Identity outbox failure")
	}
	if expense.isAssignedToTarget() {
		t.Fatal("Expense reassignment did not commit before the forced Identity failure")
	}
	var removed bool
	if err := pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM organization_members WHERE organization_id = $1 AND user_id = $2`, fixture.organizationID, fixture.targetID).Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("Identity removal was not rolled back with its outbox failure")
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_member_removal_event ON outbox_events; DROP FUNCTION reject_member_removal_event()`); err != nil {
		t.Fatalf("remove forced failure: %v", err)
	}
	if _, err := members.RemoveOrganizationMember(ctx, input); err != nil {
		t.Fatalf("retry removal after committed reassignment: %v", err)
	}
	if got := expense.reassignCalls(); got != 1 {
		t.Fatalf("Expense reassignment calls = %d, want exactly one across retry", got)
	}
	if err := pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM organization_members WHERE organization_id = $1 AND user_id = $2`, fixture.organizationID, fixture.targetID).Scan(&removed); err != nil || !removed {
		t.Fatalf("membership removed = %t, error = %v; want true", removed, err)
	}
}

type lateApproverAssignments struct{ checks atomic.Int32 }

func (f *lateApproverAssignments) CheckApproverAssignments(context.Context, uuid.UUID, uuid.UUID) (bool, []contract.ApproverRuleRef, error) {
	if f.checks.Add(1) == 1 {
		return false, nil, nil
	}
	return true, []contract.ApproverRuleRef{{ID: uuid.New(), Step: 1}}, nil
}

func (*lateApproverAssignments) ReassignApproverRules(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (*lateApproverAssignments) ClearRejoinedMemberProjectAccess(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, nil
}

type mutableApproverAssignments struct {
	mu               sync.Mutex
	assignedToTarget bool
	reassignCount    int
}

func (f *mutableApproverAssignments) CheckApproverAssignments(context.Context, uuid.UUID, uuid.UUID) (bool, []contract.ApproverRuleRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.assignedToTarget {
		return false, nil, nil
	}
	return true, []contract.ApproverRuleRef{{ID: uuid.New(), Step: 1}}, nil
}

func (f *mutableApproverAssignments) ReassignApproverRules(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assignedToTarget = false
	f.reassignCount++
	return nil
}

func (*mutableApproverAssignments) ClearRejoinedMemberProjectAccess(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *mutableApproverAssignments) isAssignedToTarget() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.assignedToTarget
}

func (f *mutableApproverAssignments) reassignCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reassignCount
}

type writeAccessFixture struct {
	organizationID uuid.UUID
	ownerID        uuid.UUID
	targetID       uuid.UUID
}

func createWriteAccessFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) writeAccessFixture {
	t.Helper()
	fixture := writeAccessFixture{organizationID: uuid.New(), ownerID: uuid.New(), targetID: uuid.New()}
	for _, user := range []struct {
		id    uuid.UUID
		name  string
		email string
	}{{fixture.ownerID, "Owner", fixture.ownerID.String() + "@example.test"}, {fixture.targetID, "Member", fixture.targetID.String() + "@example.test"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password, name) VALUES ($1, $2, 'unused', $3)`, user.id, user.email, user.name); err != nil {
			t.Fatalf("create test user: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name, type) VALUES ($1, 'write lease integration', 'business')`, fixture.organizationID); err != nil {
		t.Fatalf("create test organization: %v", err)
	}
	for _, member := range []struct {
		id   uuid.UUID
		role string
	}{{fixture.ownerID, "owner"}, {fixture.targetID, "member"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, $3)`, fixture.organizationID, member.id, member.role); err != nil {
			t.Fatalf("create test organization member: %v", err)
		}
	}
	return fixture
}

func assertMutationWaitsForWriteLease(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	organizations OrganizationService,
	fixture writeAccessFixture,
	mutate func() error,
	readChangedState func() bool,
	wantBefore bool,
	wantAfter bool,
) {
	t.Helper()
	leaseReady := make(chan HoldExpenseWriteAccessOutput, 1)
	leaseDone := make(chan error, 1)
	leaseReleased := make(chan struct{})
	released := false
	releaseLease := func() {
		if !released {
			close(leaseReleased)
			released = true
		}
	}
	leaseFinished := false
	defer func() {
		releaseLease()
		if !leaseFinished {
			select {
			case <-leaseDone:
			case <-time.After(5 * time.Second):
				t.Error("write lease did not finish after release")
			}
		}
	}()

	go func() {
		leaseDone <- organizations.HoldExpenseWriteAccess(ctx, HoldExpenseWriteAccessInput{
			OrganizationID: fixture.organizationID, UserID: fixture.ownerID,
		}, func(output HoldExpenseWriteAccessOutput) error {
			leaseReady <- output
			<-leaseReleased
			return nil
		})
	}()
	select {
	case output := <-leaseReady:
		if !output.MemberActive || !output.ServiceAllowed || output.IsDeleted || output.IsFrozen {
			t.Fatalf("unexpected write lease authorization: %+v", output)
		}
	case err := <-leaseDone:
		leaseFinished = true
		t.Fatalf("acquire expense write lease: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for expense write lease")
	}

	mutationDone := make(chan error, 1)
	mutationFinished := false
	defer func() {
		releaseLease()
		if !mutationFinished {
			select {
			case <-mutationDone:
			case <-time.After(5 * time.Second):
				t.Error("authorization mutation did not finish after lease release")
			}
		}
	}()
	go func() { mutationDone <- mutate() }()
	waitForWriteAccessLockWaiter(t, ctx, pool)
	if got := readChangedState(); got != wantBefore {
		t.Fatalf("authorization changed while write lease was active: state = %t, want %t", got, wantBefore)
	}
	select {
	case err := <-mutationDone:
		mutationFinished = true
		t.Fatalf("authorization mutation returned before lease release: %v", err)
	default:
	}

	releaseLease()
	if err := receiveWriteAccessError(t, ctx, leaseDone); err != nil {
		leaseFinished = true
		t.Fatalf("release expense write lease: %v", err)
	}
	leaseFinished = true
	if err := receiveWriteAccessError(t, ctx, mutationDone); err != nil {
		mutationFinished = true
		t.Fatalf("authorization mutation: %v", err)
	}
	mutationFinished = true
	if got := readChangedState(); got != wantAfter {
		t.Fatalf("authorization state after release = %t, want %t", got, wantAfter)
	}
}

func waitForWriteAccessLockWaiter(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiters int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`).Scan(&waiters); err != nil {
			t.Fatalf("inspect PostgreSQL lock waiters: %v", err)
		}
		if waiters > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("authorization mutation never waited on the write lease lock")
}

func receiveWriteAccessError(t *testing.T, ctx context.Context, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		t.Fatal("operation did not finish before context expired")
		return ctx.Err()
	}
}

func newWriteAccessIntegrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
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
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90 * time.Second),
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
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool, migrations.FS, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("apply Identity migrations: %v", err)
	}
	return pool
}

type noopRevocationWriter struct{}

func (noopRevocationWriter) RevokeMember(context.Context, string, string) error { return nil }

type noApproverRules struct{}

func (noApproverRules) CheckApproverAssignments(context.Context, uuid.UUID, uuid.UUID) (bool, []contract.ApproverRuleRef, error) {
	return false, nil, nil
}

func (noApproverRules) ReassignApproverRules(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (noApproverRules) ClearRejoinedMemberProjectAccess(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, nil
}
