package docker

import (
	"context"
	"fmt"
	"time"
)

// waitHealthy polls a container via the Manager interface until it reports
// healthy status or the timeout expires. Containers without a health check
// that are running are treated as healthy.
func waitHealthy(ctx context.Context, mgr Manager, id string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("container %s did not become healthy within %s", id, timeout)
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c, err := mgr.InspectContainer(ctx, id)
			if err != nil {
				return err
			}
			// No health check configured but container is running
			if c.Health == "none" || c.Health == "" {
				if c.Status == "running" {
					return nil
				}
				continue
			}
			if c.Health == "healthy" {
				return nil
			}
		}
	}
}
