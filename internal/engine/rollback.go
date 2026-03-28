package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/provider"
)

// rollback undoes completed rotation steps in LIFO order.
// It collects all errors and returns a detailed message if any rollback step fails.
func rollback(ctx context.Context, state *RotationState, prov provider.Provider, dockerMgr docker.Manager, timeout time.Duration) error {
	var errs []string

	// LIFO: undo steps in reverse order of completion

	// If containers were restarted, restore .env first then restart with old values
	if state.CurrentStep >= StepRestart && len(state.Containers) > 0 {
		// Restore .env content before restarting so containers pick up old values
		if state.OldEnvContent != nil && state.EnvFilePath != "" {
			if err := os.WriteFile(state.EnvFilePath, state.OldEnvContent, 0o644); err != nil {
				errs = append(errs, fmt.Sprintf("restore .env: %v", err))
			}
		}
		// Restart containers to pick up restored .env
		if err := docker.RestartInOrder(ctx, dockerMgr, state.Containers, timeout); err != nil {
			errs = append(errs, fmt.Sprintf("restart containers: %v", err))
		}
	} else if state.CurrentStep >= StepUpdateEnv {
		// .env was updated but containers not yet restarted -- just restore file
		if state.OldEnvContent != nil && state.EnvFilePath != "" {
			if err := os.WriteFile(state.EnvFilePath, state.OldEnvContent, 0o644); err != nil {
				errs = append(errs, fmt.Sprintf("restore .env: %v", err))
			}
		}
	}

	// Rollback DB password if provider is not generic and rotation was called
	if state.CurrentStep >= StepGenerate && prov.Name() != "generic" {
		cfg := provider.ProviderConfig{} // rollback only needs old secret
		if err := prov.Rollback(ctx, cfg, state.OldSecret); err != nil {
			errs = append(errs, fmt.Sprintf("provider rollback: %v", err))
		}
	}

	if len(errs) > 0 {
		dbState := "new"
		envState := "new"
		containerState := "not restarted"
		if state.CurrentStep >= StepRestart {
			containerState = "restarted with unknown state"
		}
		return fmt.Errorf("rollback failed: %s; current state: DB has %s password, .env has %s password, containers %s",
			strings.Join(errs, "; "), dbState, envState, containerState)
	}

	return nil
}
