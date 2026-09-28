package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giulio/secret-rotator/internal/domain"
)

// harness wires the use case to a set of doubles and exposes them for
// assertions.
type harness struct {
	env      *fakeEnvStore
	rotator  *fakeRotator
	gen      *fakeGenerator
	fleet    *fakeFleet
	audit    *fakeAudit
	useCase  *RotateSecret
	registry fakeRegistry
}

func newHarness(t *testing.T, kind domain.Kind) *harness {
	t.Helper()

	h := &harness{
		env:     newFakeEnvStore(),
		rotator: &fakeRotator{kind: kind},
		gen:     &fakeGenerator{value: "brand-new-value"},
		fleet:   &fakeFleet{},
		audit:   &fakeAudit{},
	}
	h.registry = fakeRegistry{rotator: h.rotator}
	h.useCase = NewRotateSecret(RotateSecretDeps{
		Rotators:  h.registry,
		Generator: h.gen,
		Env:       h.env,
		Fleet:     h.fleet,
		Audit:     h.audit,
		Clock:     fixedClock{t: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)},
	})
	return h
}

func TestRotationAppliesVerifiesWritesAndRestarts(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")

	secret := secretFor(t, nil)
	require.NoError(t, h.useCase.Execute(context.Background(), secret))

	assert.Equal(t, []domain.Credential{domain.NewCredential("brand-new-value")}, h.rotator.applied)
	assert.Len(t, h.rotator.verified, 1, "the new credential must be proven before the files change")
	assert.Equal(t, "brand-new-value", h.env.valueOf(".env", "DB_PASSWORD"))
	require.Len(t, h.fleet.restarts, 1)
	assert.Equal(t, []domain.ContainerRef{"db", "app"}, h.fleet.restarts[0])

	require.Len(t, h.audit.records, 1)
	assert.Equal(t, domain.OutcomeSuccess, h.audit.records[0].Outcome)
	assert.Equal(t, domain.StepDone, h.audit.records[0].LastStep)
}

func TestGenericSecretSkipsTheBackingService(t *testing.T) {
	h := newHarness(t, domain.KindGeneric)
	h.env.put(".env", "APP_SECRET_KEY", "old-value")

	secret := secretFor(t, func(spec *domain.SecretSpec) {
		spec.Kind = "generic"
		spec.EnvKey = "APP_SECRET_KEY"
		spec.Target = domain.Target{}
	})
	require.NoError(t, h.useCase.Execute(context.Background(), secret))

	assert.Empty(t, h.rotator.applied, "a generic secret has no service to update")
	assert.Empty(t, h.rotator.verified, "a generic secret has nothing to authenticate against")
	assert.Equal(t, "brand-new-value", h.env.valueOf(".env", "APP_SECRET_KEY"))
}

func TestMissingKeyAbortsBeforeAnythingChanges(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "SOMETHING_ELSE", "value")

	err := h.useCase.Execute(context.Background(), secretFor(t, nil))

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSecretKeyMissing)
	assert.Empty(t, h.gen.calls, "nothing should be generated for a key that is not there")
	assert.Empty(t, h.audit.records)
}

func TestUnresolvableContainerAbortsBeforeAnythingChanges(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.fleet.resolveErr = errors.New(`container "app" not found`)

	err := h.useCase.Execute(context.Background(), secretFor(t, nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `container "app" not found`)
	assert.Empty(t, h.rotator.applied)
	assert.Equal(t, "old-value", h.env.valueOf(".env", "DB_PASSWORD"))
}

func TestFailedVerifyRestoresTheCredentialAndLeavesFilesAlone(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.rotator.verifyErr = errors.New("password authentication failed")

	err := h.useCase.Execute(context.Background(), secretFor(t, nil))

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrRotationFailed)
	assert.Equal(t, []domain.Credential{domain.NewCredential("old-value")}, h.rotator.restored)
	assert.Equal(t, "old-value", h.env.valueOf(".env", "DB_PASSWORD"))
	assert.Empty(t, h.fleet.restarts)

	require.Len(t, h.audit.records, 1)
	assert.Equal(t, domain.OutcomeFailed, h.audit.records[0].Outcome)
}

func TestFailedApplyNeedsNoCompensation(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.rotator.applyErr = errors.New("connection refused")

	err := h.useCase.Execute(context.Background(), secretFor(t, nil))

	require.Error(t, err)
	assert.Empty(t, h.rotator.restored, "nothing was applied, so nothing has to be put back")
	assert.Equal(t, "old-value", h.env.valueOf(".env", "DB_PASSWORD"))
}

// This is the case the previous implementation got wrong: it restored only the
// first file, leaving the others holding a value the database had rolled back.
func TestFailedWriteRestoresEveryFileAlreadyWritten(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.env.put(".env.local", "DB_PASSWORD", "old-value")
	h.env.put("docker/.env", "DB_PASSWORD", "old-value")
	h.env.failWriteOn = "docker/.env"

	secret := secretFor(t, func(spec *domain.SecretSpec) {
		spec.EnvFiles = []string{".env", ".env.local", "docker/.env"}
	})

	err := h.useCase.Execute(context.Background(), secret)

	require.Error(t, err)
	assert.Equal(t, []domain.FilePath{".env", ".env.local"}, h.env.restores,
		"every file written before the failure must be restored, not just the first")
	assert.Equal(t, "DB_PASSWORD=old-value\n", h.env.contentOf(".env"))
	assert.Equal(t, "DB_PASSWORD=old-value\n", h.env.contentOf(".env.local"))
	assert.Equal(t, []domain.Credential{domain.NewCredential("old-value")}, h.rotator.restored)
}

func TestFailedRestartRestoresFilesAndBouncesContainersAgain(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.fleet.restartErr = errors.New("container app is unhealthy")

	err := h.useCase.Execute(context.Background(), secretFor(t, nil))

	require.Error(t, err)
	assert.Equal(t, "DB_PASSWORD=old-value\n", h.env.contentOf(".env"))
	require.Len(t, h.fleet.restarts, 2,
		"containers must be restarted again so they pick the restored values back up")
	assert.Equal(t, []domain.Credential{domain.NewCredential("old-value")}, h.rotator.restored)
}

func TestFilesAreRestoredBeforeContainersRestart(t *testing.T) {
	// If the order were reversed, the containers would come back holding the
	// new value while the database had been rolled back to the old one.
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.fleet.restartErr = errors.New("unhealthy")

	var order []string
	h.env.failRestoreOn = "" // restores succeed
	trackingEnv := &orderTrackingEnvStore{fakeEnvStore: h.env, order: &order}
	trackingFleet := &orderTrackingFleet{fakeFleet: h.fleet, order: &order}

	useCase := NewRotateSecret(RotateSecretDeps{
		Rotators:  h.registry,
		Generator: h.gen,
		Env:       trackingEnv,
		Fleet:     trackingFleet,
		Audit:     h.audit,
	})

	require.Error(t, useCase.Execute(context.Background(), secretFor(t, nil)))

	require.Contains(t, order, "restore")
	require.Contains(t, order, "restart-compensating")
	assert.Less(t, indexOf(order, "restore"), indexOf(order, "restart-compensating"))
}

func TestFailedCompensationReportsTheResultingState(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.fleet.restartErr = errors.New("unhealthy")
	h.env.failRestoreOn = ".env"
	h.rotator.restoreErr = errors.New("cannot connect")

	err := h.useCase.Execute(context.Background(), secretFor(t, nil))

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrRollbackFailed)
	assert.Contains(t, err.Error(), "read-only filesystem")
	assert.Contains(t, err.Error(), "cannot connect")
	assert.Contains(t, err.Error(), "inspect the backing service",
		"the operator must be told the system is in a mixed state")
}

func TestSecretLengthReachesTheGenerator(t *testing.T) {
	h := newHarness(t, domain.KindGeneric)
	h.env.put(".env", "APP_SECRET_KEY", "old-value")

	secret := secretFor(t, func(spec *domain.SecretSpec) {
		spec.Kind = "generic"
		spec.EnvKey = "APP_SECRET_KEY"
		spec.Target = domain.Target{}
		spec.Length = 48
	})
	require.NoError(t, h.useCase.Execute(context.Background(), secret))

	assert.Equal(t, []domain.PasswordLength{48}, h.gen.calls)
}

func TestAdminPasswordComesFromTheEnvFile(t *testing.T) {
	// A daemon keeps the environment it started with, so after the first
	// rotation only the file holds the current admin password.
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "current-admin-pw")

	secret := secretFor(t, func(spec *domain.SecretSpec) {
		spec.Target.AdminPassword = domain.Credential{}
		spec.Target.AdminPasswordEnv = "DB_PASSWORD"
	})
	require.NoError(t, h.useCase.Execute(context.Background(), secret))

	assert.Equal(t, "current-admin-pw", h.rotator.seenAdminPassword.Expose())
}

func TestExplicitAdminPasswordWins(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "value-in-file")

	secret := secretFor(t, func(spec *domain.SecretSpec) {
		spec.Target.AdminPassword = domain.NewCredential("explicit-pw")
		spec.Target.AdminPasswordEnv = "DB_PASSWORD"
	})
	require.NoError(t, h.useCase.Execute(context.Background(), secret))

	assert.Equal(t, "explicit-pw", h.rotator.seenAdminPassword.Expose())
}

func TestSelfRotatingAccountUsesItsOwnCurrentValue(t *testing.T) {
	// A Redis entry configures nothing but a host: requirepass is both the
	// credential being rotated and the one used to connect.
	h := newHarness(t, domain.KindRedis)
	h.env.put(".env", "REDIS_PASSWORD", "the-current-requirepass")

	secret := secretFor(t, func(spec *domain.SecretSpec) {
		spec.Kind = "redis"
		spec.EnvKey = "REDIS_PASSWORD"
		spec.Target = domain.Target{Host: "cache"}
	})
	require.NoError(t, h.useCase.Execute(context.Background(), secret))

	assert.Equal(t, "the-current-requirepass", h.rotator.seenAdminPassword.Expose())
}

func TestUnknownKindIsReportedBeforeAnyWork(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.useCase = NewRotateSecret(RotateSecretDeps{
		Rotators:  fakeRegistry{err: domain.ErrUnknownKind},
		Generator: h.gen,
		Env:       h.env,
		Fleet:     h.fleet,
		Audit:     h.audit,
	})

	err := h.useCase.Execute(context.Background(), secretFor(t, nil))

	require.ErrorIs(t, err, domain.ErrUnknownKind)
	assert.Empty(t, h.env.writes)
}

func TestAFailingAuditTrailDoesNotFailTheRotation(t *testing.T) {
	h := newHarness(t, domain.KindPostgres)
	h.env.put(".env", "DB_PASSWORD", "old-value")
	h.audit.err = errors.New("history file is not writable")

	// The credential has already been changed everywhere by the time the
	// record is written; failing here would report a rotation that happened
	// as one that did not.
	require.NoError(t, h.useCase.Execute(context.Background(), secretFor(t, nil)))
	assert.Equal(t, "brand-new-value", h.env.valueOf(".env", "DB_PASSWORD"))
}

// --- order-tracking wrappers ---

type orderTrackingEnvStore struct {
	*fakeEnvStore
	order *[]string
}

func (s *orderTrackingEnvStore) Restore(path domain.FilePath, snapshot []byte) error {
	*s.order = append(*s.order, "restore")
	return s.fakeEnvStore.Restore(path, snapshot)
}

type orderTrackingFleet struct {
	*fakeFleet
	order *[]string
}

func (f *orderTrackingFleet) Restart(ctx context.Context, refs []domain.ContainerRef) error {
	err := f.fakeFleet.Restart(ctx, refs)
	if err != nil {
		*f.order = append(*f.order, "restart-failed")
	} else {
		*f.order = append(*f.order, "restart-compensating")
	}
	return err
}

func indexOf(list []string, want string) int {
	for i, v := range list {
		if v == want {
			return i
		}
	}
	return -1
}
