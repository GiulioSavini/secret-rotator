package scheduling

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/giulio/secret-rotator/internal/domain"
	"github.com/giulio/secret-rotator/internal/infrastructure/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockNotifier captures sent events for assertion.
type mockNotifier struct {
	mu     sync.Mutex
	events []notifier.Event
}

func (m *mockNotifier) Send(_ context.Context, event notifier.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockNotifier) getEvents() []notifier.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]notifier.Event, len(m.events))
	copy(cp, m.events)
	return cp
}

func TestAddJob_ValidCron(t *testing.T) {
	sched := NewScheduler(
		func(_ context.Context, _ *domain.Secret) error { return nil },
		notifier.NewDispatcher(),
	)
	defer sched.Stop()

	err := sched.AddJob(testSecret(t, "db-pass", ""), "*/5 * * * *")
	assert.NoError(t, err, "valid cron expression should be accepted")
}

func TestAddJob_InvalidCron(t *testing.T) {
	sched := NewScheduler(
		func(_ context.Context, _ *domain.Secret) error { return nil },
		notifier.NewDispatcher(),
	)
	defer sched.Stop()

	err := sched.AddJob(testSecret(t, "db-pass", ""), "not-a-cron")
	assert.Error(t, err, "invalid cron expression should return error")
}

func TestAddJob_DescriptorCron(t *testing.T) {
	sched := NewScheduler(
		func(_ context.Context, _ *domain.Secret) error { return nil },
		notifier.NewDispatcher(),
	)
	defer sched.Stop()

	err := sched.AddJob(testSecret(t, "db-pass", ""), "@daily")
	assert.NoError(t, err, "@daily descriptor should be accepted")
}

func TestCronFires_CallsRotateFunc(t *testing.T) {
	called := make(chan *domain.Secret, 1)

	sched := NewScheduler(
		func(_ context.Context, secret *domain.Secret) error {
			called <- secret
			return nil
		},
		notifier.NewDispatcher(),
	)

	secret := testSecret(t, "api-key", "")
	require.NoError(t, sched.AddJob(secret, "@every 1s"))
	sched.Start()
	defer sched.Stop()

	select {
	case got := <-called:
		assert.Equal(t, domain.SecretName("api-key"), got.Name())
	case <-time.After(5 * time.Second):
		t.Fatal("rotate function was not called within timeout")
	}
}

func TestConcurrentRotation_Blocked(t *testing.T) {
	var callCount atomic.Int32
	gate := make(chan struct{}) // blocks the first rotation

	sched := NewScheduler(
		func(_ context.Context, secret *domain.Secret) error {
			callCount.Add(1)
			<-gate // block until released
			return nil
		},
		notifier.NewDispatcher(),
	)

	secret := testSecret(t, "db-pass", "")

	// Invoke the job function directly to control timing
	require.NoError(t, sched.AddJob(secret, "@every 1h")) // won't fire naturally

	// Manually trigger two concurrent invocations using the internal job
	jobFn := sched.jobFunc(secret)

	go jobFn()
	time.Sleep(50 * time.Millisecond) // let first goroutine acquire lock

	// Second invocation should return immediately (lock held)
	jobFn()

	close(gate) // release first goroutine
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(1), callCount.Load(), "concurrent rotation should be blocked")
}

func TestNotification_OnSuccess(t *testing.T) {
	mn := &mockNotifier{}
	dispatcher := notifier.NewDispatcher(mn)

	sched := NewScheduler(
		func(_ context.Context, _ *domain.Secret) error { return nil },
		dispatcher,
	)

	secret := testSecret(t, "db-pass", "")
	require.NoError(t, sched.AddJob(secret, "@every 1s"))
	sched.Start()
	defer sched.Stop()

	// Wait for at least one notification
	require.Eventually(t, func() bool {
		return len(mn.getEvents()) > 0
	}, 5*time.Second, 100*time.Millisecond)

	events := mn.getEvents()
	assert.Equal(t, "db-pass", events[0].SecretName)
	assert.Equal(t, "success", events[0].Status)
}

func TestNotification_OnFailure(t *testing.T) {
	mn := &mockNotifier{}
	dispatcher := notifier.NewDispatcher(mn)

	sched := NewScheduler(
		func(_ context.Context, _ *domain.Secret) error {
			return errors.New("rotation failed: timeout")
		},
		dispatcher,
	)

	secret := testSecret(t, "db-pass", "")
	require.NoError(t, sched.AddJob(secret, "@every 1s"))
	sched.Start()
	defer sched.Stop()

	require.Eventually(t, func() bool {
		return len(mn.getEvents()) > 0
	}, 5*time.Second, 100*time.Millisecond)

	events := mn.getEvents()
	assert.Equal(t, "db-pass", events[0].SecretName)
	assert.Equal(t, "failed", events[0].Status)
	assert.Contains(t, events[0].Details, "rotation failed")
}

func TestStop_CleansUp(t *testing.T) {
	sched := NewScheduler(
		func(_ context.Context, _ *domain.Secret) error { return nil },
		notifier.NewDispatcher(),
	)

	require.NoError(t, sched.AddJob(testSecret(t, "test", ""), "*/5 * * * *"))
	sched.Start()

	// Verify entries exist before stop
	assert.NotEmpty(t, sched.cron.Entries(), "should have entries before stop")

	sched.Stop() // should not panic or hang
	// After Stop, cron is halted -- no further jobs will fire.
	// This test verifies Stop completes without deadlock or panic.
}

func TestLoadFromConfig(t *testing.T) {
	var loaded []string
	sched := NewScheduler(
		func(_ context.Context, secret *domain.Secret) error {
			loaded = append(loaded, secret.Name().String())
			return nil
		},
		notifier.NewDispatcher(),
	)
	defer sched.Stop()

	secrets := []*domain.Secret{
		testSecret(t, "with-schedule", "*/5 * * * *"),
		testSecret(t, "no-schedule", ""),
		testSecret(t, "also-scheduled", "@daily"),
	}

	count, err := sched.LoadFromConfig(secrets)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Only secrets with non-empty Schedule should be registered
	entries := sched.cron.Entries()
	assert.Len(t, entries, 2, "only secrets with schedules should be added")
}

// testSecret builds a minimal valid generic secret for scheduling tests.
func testSecret(t *testing.T, name, schedule string) *domain.Secret {
	t.Helper()
	secret, _, err := domain.NewSecret(domain.SecretSpec{
		Name:     name,
		Kind:     "generic",
		EnvKey:   "APP_SECRET_KEY",
		EnvFiles: []string{".env"},
		Schedule: schedule,
	})
	require.NoError(t, err)
	return secret
}
