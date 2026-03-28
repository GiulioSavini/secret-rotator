package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockNotifier captures sent events for assertion.
type mockNotifier struct {
	mu     sync.Mutex
	events []notify.Event
}

func (m *mockNotifier) Send(_ context.Context, event notify.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockNotifier) getEvents() []notify.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]notify.Event, len(m.events))
	copy(cp, m.events)
	return cp
}

func TestAddJob_ValidCron(t *testing.T) {
	sched := NewScheduler(
		func(_ context.Context, _ config.SecretConfig) error { return nil },
		notify.NewDispatcher(),
	)
	defer sched.Stop()

	err := sched.AddJob(config.SecretConfig{Name: "db-pass"}, "*/5 * * * *")
	assert.NoError(t, err, "valid cron expression should be accepted")
}

func TestAddJob_InvalidCron(t *testing.T) {
	sched := NewScheduler(
		func(_ context.Context, _ config.SecretConfig) error { return nil },
		notify.NewDispatcher(),
	)
	defer sched.Stop()

	err := sched.AddJob(config.SecretConfig{Name: "db-pass"}, "not-a-cron")
	assert.Error(t, err, "invalid cron expression should return error")
}

func TestAddJob_DescriptorCron(t *testing.T) {
	sched := NewScheduler(
		func(_ context.Context, _ config.SecretConfig) error { return nil },
		notify.NewDispatcher(),
	)
	defer sched.Stop()

	err := sched.AddJob(config.SecretConfig{Name: "db-pass"}, "@daily")
	assert.NoError(t, err, "@daily descriptor should be accepted")
}

func TestCronFires_CallsRotateFunc(t *testing.T) {
	called := make(chan config.SecretConfig, 1)

	sched := NewScheduler(
		func(_ context.Context, cfg config.SecretConfig) error {
			called <- cfg
			return nil
		},
		notify.NewDispatcher(),
	)

	secret := config.SecretConfig{Name: "api-key", Type: "generic"}
	require.NoError(t, sched.AddJob(secret, "@every 1s"))
	sched.Start()
	defer sched.Stop()

	select {
	case got := <-called:
		assert.Equal(t, "api-key", got.Name)
	case <-time.After(5 * time.Second):
		t.Fatal("rotate function was not called within timeout")
	}
}

func TestConcurrentRotation_Blocked(t *testing.T) {
	var callCount atomic.Int32
	gate := make(chan struct{}) // blocks the first rotation

	sched := NewScheduler(
		func(_ context.Context, cfg config.SecretConfig) error {
			callCount.Add(1)
			<-gate // block until released
			return nil
		},
		notify.NewDispatcher(),
	)

	secret := config.SecretConfig{Name: "db-pass"}

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
	dispatcher := notify.NewDispatcher(mn)

	sched := NewScheduler(
		func(_ context.Context, _ config.SecretConfig) error { return nil },
		dispatcher,
	)

	secret := config.SecretConfig{Name: "db-pass"}
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
	dispatcher := notify.NewDispatcher(mn)

	sched := NewScheduler(
		func(_ context.Context, _ config.SecretConfig) error {
			return errors.New("rotation failed: timeout")
		},
		dispatcher,
	)

	secret := config.SecretConfig{Name: "db-pass"}
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
		func(_ context.Context, _ config.SecretConfig) error { return nil },
		notify.NewDispatcher(),
	)

	require.NoError(t, sched.AddJob(config.SecretConfig{Name: "test"}, "*/5 * * * *"))
	sched.Start()
	sched.Stop() // should not panic or hang

	// Verify no more entries after stop
	entries := sched.cron.Entries()
	assert.Empty(t, entries, "cron entries should be empty after stop")
}

func TestLoadFromConfig(t *testing.T) {
	var loaded []string
	sched := NewScheduler(
		func(_ context.Context, cfg config.SecretConfig) error {
			loaded = append(loaded, cfg.Name)
			return nil
		},
		notify.NewDispatcher(),
	)
	defer sched.Stop()

	secrets := []config.SecretConfig{
		{Name: "with-schedule", Schedule: "*/5 * * * *"},
		{Name: "no-schedule", Schedule: ""},
		{Name: "also-scheduled", Schedule: "@daily"},
	}

	err := sched.LoadFromConfig(secrets)
	require.NoError(t, err)

	// Only secrets with non-empty Schedule should be registered
	entries := sched.cron.Entries()
	assert.Len(t, entries, 2, "only secrets with schedules should be added")
}
