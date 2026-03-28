package docker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockManager implements Manager for testing.
type MockManager struct {
	// Recorded calls for verification
	Calls []MockCall

	// Configurable return values
	ListFunc      func(ctx context.Context, filter ContainerFilter) ([]Container, error)
	InspectFunc   func(ctx context.Context, id string) (*Container, error)
	StopFunc      func(ctx context.Context, id string, timeout time.Duration) error
	StartFunc     func(ctx context.Context, id string) error
	RestartFunc   func(ctx context.Context, id string, timeout time.Duration) error
	WaitHealthyFn func(ctx context.Context, id string, timeout time.Duration) error
}

// MockCall records a method call with its arguments.
type MockCall struct {
	Method string
	Args   []interface{}
}

func (m *MockManager) record(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

func (m *MockManager) ListContainers(ctx context.Context, filter ContainerFilter) ([]Container, error) {
	m.record("ListContainers", filter)
	if m.ListFunc != nil {
		return m.ListFunc(ctx, filter)
	}
	return nil, nil
}

func (m *MockManager) InspectContainer(ctx context.Context, id string) (*Container, error) {
	m.record("InspectContainer", id)
	if m.InspectFunc != nil {
		return m.InspectFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockManager) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.record("StopContainer", id, timeout)
	if m.StopFunc != nil {
		return m.StopFunc(ctx, id, timeout)
	}
	return nil
}

func (m *MockManager) StartContainer(ctx context.Context, id string) error {
	m.record("StartContainer", id)
	if m.StartFunc != nil {
		return m.StartFunc(ctx, id)
	}
	return nil
}

func (m *MockManager) RestartContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.record("RestartContainer", id, timeout)
	if m.RestartFunc != nil {
		return m.RestartFunc(ctx, id, timeout)
	}
	return nil
}

func (m *MockManager) WaitHealthy(ctx context.Context, id string, timeout time.Duration) error {
	m.record("WaitHealthy", id, timeout)
	if m.WaitHealthyFn != nil {
		return m.WaitHealthyFn(ctx, id, timeout)
	}
	return nil
}

func TestListContainers(t *testing.T) {
	expected := []Container{
		{ID: "abc123", Name: "mydb", Image: "mysql:8", Labels: map[string]string{"app": "web"}},
		{ID: "def456", Name: "myapp", Image: "nginx:latest", Labels: map[string]string{"app": "web"}},
	}

	mock := &MockManager{
		ListFunc: func(ctx context.Context, filter ContainerFilter) ([]Container, error) {
			assert.Equal(t, map[string]string{"app": "web"}, filter.Labels)
			return expected, nil
		},
	}

	containers, err := mock.ListContainers(context.Background(), ContainerFilter{
		Labels: map[string]string{"app": "web"},
	})

	require.NoError(t, err)
	require.Len(t, containers, 2)
	assert.Equal(t, "mydb", containers[0].Name)
	assert.Equal(t, "myapp", containers[1].Name)
	assert.Equal(t, map[string]string{"app": "web"}, containers[0].Labels)
}

func TestInspectContainer(t *testing.T) {
	expected := &Container{
		ID:      "abc123",
		Name:    "mydb",
		Image:   "mysql:8",
		Status:  "running",
		Health:  "healthy",
		Labels:  map[string]string{"app": "web"},
		EnvVars: []string{"MYSQL_ROOT_PASSWORD=secret"},
	}

	mock := &MockManager{
		InspectFunc: func(ctx context.Context, id string) (*Container, error) {
			assert.Equal(t, "abc123", id)
			return expected, nil
		},
	}

	c, err := mock.InspectContainer(context.Background(), "abc123")

	require.NoError(t, err)
	assert.Equal(t, "abc123", c.ID)
	assert.Equal(t, "mydb", c.Name)
	assert.Equal(t, "healthy", c.Health)
	assert.Equal(t, []string{"MYSQL_ROOT_PASSWORD=secret"}, c.EnvVars)
}

func TestStopContainer(t *testing.T) {
	mock := &MockManager{
		StopFunc: func(ctx context.Context, id string, timeout time.Duration) error {
			assert.Equal(t, "abc123", id)
			assert.Equal(t, 30*time.Second, timeout)
			return nil
		},
	}

	err := mock.StopContainer(context.Background(), "abc123", 30*time.Second)
	require.NoError(t, err)
	require.Len(t, mock.Calls, 1)
	assert.Equal(t, "StopContainer", mock.Calls[0].Method)
}

func TestStartContainer(t *testing.T) {
	mock := &MockManager{
		StartFunc: func(ctx context.Context, id string) error {
			assert.Equal(t, "abc123", id)
			return nil
		},
	}

	err := mock.StartContainer(context.Background(), "abc123")
	require.NoError(t, err)
	require.Len(t, mock.Calls, 1)
	assert.Equal(t, "StartContainer", mock.Calls[0].Method)
}

func TestRestartContainer(t *testing.T) {
	mock := &MockManager{
		RestartFunc: func(ctx context.Context, id string, timeout time.Duration) error {
			assert.Equal(t, "abc123", id)
			assert.Equal(t, 10*time.Second, timeout)
			return nil
		},
	}

	err := mock.RestartContainer(context.Background(), "abc123", 10*time.Second)
	require.NoError(t, err)
	require.Len(t, mock.Calls, 1)
	assert.Equal(t, "RestartContainer", mock.Calls[0].Method)
}

func TestWaitHealthySuccess(t *testing.T) {
	callCount := 0
	mock := &MockManager{
		WaitHealthyFn: func(ctx context.Context, id string, timeout time.Duration) error {
			callCount++
			// Simulates success on second poll (but mock returns immediately)
			return nil
		},
	}

	err := mock.WaitHealthy(context.Background(), "abc123", 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestWaitHealthyTimeout(t *testing.T) {
	mock := &MockManager{
		WaitHealthyFn: func(ctx context.Context, id string, timeout time.Duration) error {
			return fmt.Errorf("container %s did not become healthy within %s", id, timeout)
		},
	}

	err := mock.WaitHealthy(context.Background(), "abc123", 5*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not become healthy")
}

func TestWaitHealthyNoHealthcheck(t *testing.T) {
	// Container has no health check but is running -- WaitHealthy returns nil
	mock := &MockManager{
		WaitHealthyFn: func(ctx context.Context, id string, timeout time.Duration) error {
			// Treat running-without-healthcheck as healthy
			return nil
		},
	}

	err := mock.WaitHealthy(context.Background(), "abc123", 5*time.Second)
	require.NoError(t, err)
}

func TestRestartInOrder(t *testing.T) {
	var restartOrder []string

	mock := &MockManager{
		RestartFunc: func(ctx context.Context, id string, timeout time.Duration) error {
			restartOrder = append(restartOrder, "restart:"+id)
			return nil
		},
		WaitHealthyFn: func(ctx context.Context, id string, timeout time.Duration) error {
			restartOrder = append(restartOrder, "healthy:"+id)
			return nil
		},
	}

	services := []string{"database", "cache", "api", "frontend"}
	err := RestartInOrder(context.Background(), mock, services, 30*time.Second)

	require.NoError(t, err)
	expected := []string{
		"restart:database", "healthy:database",
		"restart:cache", "healthy:cache",
		"restart:api", "healthy:api",
		"restart:frontend", "healthy:frontend",
	}
	assert.Equal(t, expected, restartOrder)
}

func TestRestartInOrderErrorStops(t *testing.T) {
	mock := &MockManager{
		RestartFunc: func(ctx context.Context, id string, timeout time.Duration) error {
			if id == "cache" {
				return fmt.Errorf("restart failed: %s", id)
			}
			return nil
		},
		WaitHealthyFn: func(ctx context.Context, id string, timeout time.Duration) error {
			return nil
		},
	}

	services := []string{"database", "cache", "api"}
	err := RestartInOrder(context.Background(), mock, services, 30*time.Second)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache")
	// Verify api was never attempted
	for _, call := range mock.Calls {
		assert.NotEqual(t, "api", call.Args[0])
	}
}
