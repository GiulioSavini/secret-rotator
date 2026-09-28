package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubManager serves a fixed container inventory to ResolveContainerNames.
type stubManager struct {
	containers []Container
	listErr    error
}

func (s *stubManager) ListContainers(context.Context, ContainerFilter) ([]Container, error) {
	return s.containers, s.listErr
}
func (s *stubManager) InspectContainer(context.Context, string) (*Container, error) {
	return nil, nil
}
func (s *stubManager) StopContainer(context.Context, string, time.Duration) error    { return nil }
func (s *stubManager) StartContainer(context.Context, string) error                  { return nil }
func (s *stubManager) RestartContainer(context.Context, string, time.Duration) error { return nil }
func (s *stubManager) WaitHealthy(context.Context, string, time.Duration) error      { return nil }

func composeContainer(name, project, service, number string) Container {
	return Container{
		Name: name,
		Labels: map[string]string{
			ComposeProjectLabel: project,
			ComposeServiceLabel: service,
			ComposeNumberLabel:  number,
		},
	}
}

func TestResolveExactContainerName(t *testing.T) {
	mgr := &stubManager{containers: []Container{{Name: "legacy-db"}}}

	got, err := ResolveContainerNames(context.Background(), mgr, []string{"legacy-db"})
	require.NoError(t, err)
	assert.Equal(t, []string{"legacy-db"}, got)
}

func TestResolveComposeServiceName(t *testing.T) {
	// This is the case that silently failed before: the config names the
	// Compose service, but the container is called <project>-<service>-<n>.
	mgr := &stubManager{containers: []Container{
		composeContainer("myproj-db-1", "myproj", "db", "1"),
		composeContainer("myproj-app-1", "myproj", "app", "1"),
	}}

	got, err := ResolveContainerNames(context.Background(), mgr, []string{"db", "app"})
	require.NoError(t, err)
	assert.Equal(t, []string{"myproj-db-1", "myproj-app-1"}, got)
}

func TestResolveExpandsScaledService(t *testing.T) {
	mgr := &stubManager{containers: []Container{
		composeContainer("myproj-worker-2", "myproj", "worker", "2"),
		composeContainer("myproj-worker-1", "myproj", "worker", "1"),
		composeContainer("myproj-worker-10", "myproj", "worker", "10"),
	}}

	got, err := ResolveContainerNames(context.Background(), mgr, []string{"worker"})
	require.NoError(t, err)
	assert.Equal(t, []string{"myproj-worker-1", "myproj-worker-2", "myproj-worker-10"}, got)
}

func TestResolveAmbiguousAcrossProjects(t *testing.T) {
	mgr := &stubManager{containers: []Container{
		composeContainer("alpha-db-1", "alpha", "db", "1"),
		composeContainer("beta-db-1", "beta", "db", "1"),
	}}

	_, err := ResolveContainerNames(context.Background(), mgr, []string{"db"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
	assert.Contains(t, err.Error(), "alpha")
	assert.Contains(t, err.Error(), "beta")
}

func TestResolveUnknownNameListsCandidates(t *testing.T) {
	mgr := &stubManager{containers: []Container{
		composeContainer("myproj-db-1", "myproj", "db", "1"),
	}}

	_, err := ResolveContainerNames(context.Background(), mgr, []string{"database"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database")
	assert.Contains(t, err.Error(), "myproj-db-1")
	assert.Contains(t, err.Error(), "Compose services [db]")
}

func TestResolveExactNameBeatsServiceLabel(t *testing.T) {
	mgr := &stubManager{containers: []Container{
		{Name: "db"},
		composeContainer("myproj-db-1", "myproj", "db", "1"),
	}}

	got, err := ResolveContainerNames(context.Background(), mgr, []string{"db"})
	require.NoError(t, err)
	assert.Equal(t, []string{"db"}, got)
}

func TestResolveFallsBackWhenDaemonUnavailable(t *testing.T) {
	// Without an inventory, behave exactly like `docker restart <name>` and
	// let the restart itself report the problem.
	mgr := &stubManager{listErr: errors.New("cannot connect to the Docker daemon")}

	got, err := ResolveContainerNames(context.Background(), mgr, []string{"db", "app"})
	require.NoError(t, err)
	assert.Equal(t, []string{"db", "app"}, got)
}

func TestResolveFallsBackWhenInventoryEmpty(t *testing.T) {
	got, err := ResolveContainerNames(context.Background(), &stubManager{}, []string{"db"})
	require.NoError(t, err)
	assert.Equal(t, []string{"db"}, got)
}

func TestResolveNoContainersRequested(t *testing.T) {
	got, err := ResolveContainerNames(context.Background(), &stubManager{}, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
