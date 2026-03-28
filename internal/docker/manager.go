// Package docker provides container lifecycle operations for the secret rotator.
// The Manager interface enables mocking in tests while the SDKClient provides
// real Docker operations.
package docker

import (
	"context"
	"time"
)

// Container holds the subset of container info the rotator needs.
type Container struct {
	ID      string
	Name    string
	Image   string
	Status  string
	Health  string // "healthy", "unhealthy", "starting", "none", ""
	Labels  map[string]string
	EnvVars []string
}

// ContainerFilter defines criteria for listing containers.
type ContainerFilter struct {
	Names  []string
	Labels map[string]string
}

// Manager defines the contract for Docker operations.
// All container interactions go through this interface.
type Manager interface {
	ListContainers(ctx context.Context, filter ContainerFilter) ([]Container, error)
	InspectContainer(ctx context.Context, id string) (*Container, error)
	StopContainer(ctx context.Context, id string, timeout time.Duration) error
	StartContainer(ctx context.Context, id string) error
	RestartContainer(ctx context.Context, id string, timeout time.Duration) error
	WaitHealthy(ctx context.Context, id string, timeout time.Duration) error
}

// RestartInOrder restarts containers in the given order, waiting for each
// to become healthy before proceeding to the next. This enables dependency-aware
// restarts (databases before apps).
func RestartInOrder(ctx context.Context, mgr Manager, serviceNames []string, timeout time.Duration) error {
	for _, name := range serviceNames {
		if err := mgr.RestartContainer(ctx, name, timeout); err != nil {
			return err
		}
		if err := mgr.WaitHealthy(ctx, name, timeout); err != nil {
			return err
		}
	}
	return nil
}
