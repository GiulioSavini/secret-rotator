package container

import (
	"context"
	"time"

	"github.com/giulio/secret-rotator/internal/domain"
)

// DefaultStepTimeout bounds one container restart and its health check.
const DefaultStepTimeout = 30 * time.Second

// Fleet is the Docker-backed implementation of domain.ContainerFleet.
type Fleet struct {
	manager Manager
	timeout time.Duration
}

// NewFleet wraps a Docker manager as a container fleet.
// A zero timeout selects DefaultStepTimeout.
func NewFleet(manager Manager, timeout time.Duration) *Fleet {
	if timeout <= 0 {
		timeout = DefaultStepTimeout
	}
	return &Fleet{manager: manager, timeout: timeout}
}

// Resolve maps configured references onto real container names.
func (f *Fleet) Resolve(ctx context.Context, refs []domain.ContainerRef) ([]domain.ContainerRef, error) {
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, r.String())
	}

	resolved, err := ResolveContainerNames(ctx, f.manager, names)
	if err != nil {
		return nil, err
	}

	out := make([]domain.ContainerRef, 0, len(resolved))
	for _, name := range resolved {
		out = append(out, domain.ContainerRef(name))
	}
	return out, nil
}

// Restart restarts the containers in order, waiting for each to report healthy
// before starting the next, so a database is back up before the services that
// depend on it are bounced.
func (f *Fleet) Restart(ctx context.Context, refs []domain.ContainerRef) error {
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, r.String())
	}
	return RestartInOrder(ctx, f.manager, names, f.timeout)
}

var _ domain.ContainerFleet = (*Fleet)(nil)
