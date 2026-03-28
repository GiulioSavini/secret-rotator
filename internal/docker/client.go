package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	containerapi "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// SDKClient wraps the Docker SDK client and implements the Manager interface.
type SDKClient struct {
	cli *client.Client
}

// NewSDKClient creates a new Docker SDK client configured from environment variables.
func NewSDKClient() (*SDKClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	return &SDKClient{cli: cli}, nil
}

// Close closes the underlying Docker client connection.
func (s *SDKClient) Close() error {
	return s.cli.Close()
}

// ListContainers returns containers matching the given filter.
func (s *SDKClient) ListContainers(ctx context.Context, filter ContainerFilter) ([]Container, error) {
	opts := containerapi.ListOptions{
		All: true,
	}

	if len(filter.Names) > 0 || len(filter.Labels) > 0 {
		f := filters.NewArgs()
		for _, name := range filter.Names {
			f.Add("name", name)
		}
		for k, v := range filter.Labels {
			if v != "" {
				f.Add("label", k+"="+v)
			} else {
				f.Add("label", k)
			}
		}
		opts.Filters = f
	}

	apiContainers, err := s.cli.ContainerList(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	containers := make([]Container, 0, len(apiContainers))
	for _, c := range apiContainers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		containers = append(containers, Container{
			ID:     c.ID,
			Name:   name,
			Image:  c.Image,
			Status: c.State,
			Labels: c.Labels,
		})
	}
	return containers, nil
}

// InspectContainer returns detailed information about a container.
func (s *SDKClient) InspectContainer(ctx context.Context, id string) (*Container, error) {
	resp, err := s.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspecting container %s: %w", id, err)
	}

	health := ""
	if resp.State != nil && resp.State.Health != nil {
		health = resp.State.Health.Status
	} else if resp.State != nil && resp.State.Running {
		health = "none"
	}

	name := strings.TrimPrefix(resp.Name, "/")

	var envVars []string
	if resp.Config != nil {
		envVars = resp.Config.Env
	}

	return &Container{
		ID:      resp.ID,
		Name:    name,
		Image:   resp.Config.Image,
		Status:  resp.State.Status,
		Health:  health,
		Labels:  resp.Config.Labels,
		EnvVars: envVars,
	}, nil
}

// StopContainer stops a container with the given timeout.
func (s *SDKClient) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	timeoutSec := int(timeout.Seconds())
	opts := containerapi.StopOptions{
		Timeout: &timeoutSec,
	}
	if err := s.cli.ContainerStop(ctx, id, opts); err != nil {
		return fmt.Errorf("stopping container %s: %w", id, err)
	}
	return nil
}

// StartContainer starts a stopped container.
func (s *SDKClient) StartContainer(ctx context.Context, id string) error {
	if err := s.cli.ContainerStart(ctx, id, containerapi.StartOptions{}); err != nil {
		return fmt.Errorf("starting container %s: %w", id, err)
	}
	return nil
}

// RestartContainer restarts a container with the given timeout.
func (s *SDKClient) RestartContainer(ctx context.Context, id string, timeout time.Duration) error {
	timeoutSec := int(timeout.Seconds())
	opts := containerapi.StopOptions{
		Timeout: &timeoutSec,
	}
	if err := s.cli.ContainerRestart(ctx, id, opts); err != nil {
		return fmt.Errorf("restarting container %s: %w", id, err)
	}
	return nil
}

// WaitHealthy polls a container until it becomes healthy or the timeout expires.
// Containers without a health check that are running are considered healthy.
func (s *SDKClient) WaitHealthy(ctx context.Context, id string, timeout time.Duration) error {
	return waitHealthy(ctx, s, id, timeout)
}
