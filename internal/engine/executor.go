package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/envfile"
	"github.com/giulio/secret-rotator/internal/history"
	"github.com/giulio/secret-rotator/internal/provider"
)

// Engine orchestrates the rotation pipeline for a single secret.
type Engine struct {
	provider provider.Provider
	docker   docker.Manager
	history  *history.Store
	timeout  time.Duration
	dryRun   bool
}

// NewEngine creates an Engine with the given dependencies.
// histStore may be nil to disable history recording.
func NewEngine(prov provider.Provider, dockerMgr docker.Manager, histStore *history.Store, timeout time.Duration, dryRun bool) *Engine {
	return &Engine{
		provider: prov,
		docker:   dockerMgr,
		history:  histStore,
		timeout:  timeout,
		dryRun:   dryRun,
	}
}

// Execute runs the full rotation pipeline for the given secret configuration.
// On failure after mutation, it triggers LIFO rollback and records the failure.
func (e *Engine) Execute(ctx context.Context, secretCfg config.SecretConfig) error {
	state := &RotationState{
		SecretName: secretCfg.Name,
		Containers: secretCfg.Containers,
	}

	// Resolve env file paths
	envFilePaths := resolveEnvFilePaths(secretCfg)
	if len(envFilePaths) == 0 {
		return fmt.Errorf("no env file configured for secret %s", secretCfg.Name)
	}
	state.EnvFilePath = envFilePaths[0]
	state.EnvFilePaths = envFilePaths

	// --- StepInit: Read and validate .env file ---
	state.CurrentStep = StepInit

	if e.dryRun {
		return e.executeDryRun(ctx, state, secretCfg)
	}

	ef, err := envfile.Read(state.EnvFilePath)
	if err != nil {
		return fmt.Errorf("reading env file %s: %w", state.EnvFilePath, err)
	}

	oldVal, found := ef.Get(secretCfg.EnvKey)
	if !found {
		return fmt.Errorf("secret key %s not found in %s", secretCfg.EnvKey, state.EnvFilePath)
	}

	// --- StepBackup: Save old secret and raw .env content ---
	state.CurrentStep = StepBackup
	state.OldSecret = oldVal

	rawContent, err := os.ReadFile(state.EnvFilePath)
	if err != nil {
		return fmt.Errorf("backup env file %s: %w", state.EnvFilePath, err)
	}
	state.OldEnvContent = rawContent

	// --- StepGenerate: Call provider.Rotate ---
	state.CurrentStep = StepGenerate
	provCfg := buildProviderConfig(secretCfg)

	result, err := e.provider.Rotate(ctx, provCfg, state.OldSecret)
	if err != nil {
		e.recordFailure(state, err)
		return fmt.Errorf("rotate: %w", err)
	}
	state.NewSecret = result.NewSecret

	// --- StepApplyDB: Implicit (handled inside provider.Rotate for DB providers) ---
	state.CurrentStep = StepApplyDB

	// --- StepVerifyDB: Verify new secret works ---
	state.CurrentStep = StepVerifyDB
	if e.provider.Name() != "generic" {
		if err := e.provider.Verify(ctx, provCfg, state.NewSecret); err != nil {
			rollbackErr := rollback(ctx, state, e.provider, e.docker, e.timeout)
			e.recordFailure(state, err)
			if rollbackErr != nil {
				return fmt.Errorf("verify: %w (rollback also failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("verify: %w", err)
		}
	}

	// --- StepUpdateEnv: Update all .env files ---
	state.CurrentStep = StepUpdateEnv
	for _, envPath := range envFilePaths {
		ef2, err := envfile.Read(envPath)
		if err != nil {
			rollbackErr := rollback(ctx, state, e.provider, e.docker, e.timeout)
			e.recordFailure(state, err)
			if rollbackErr != nil {
				return fmt.Errorf("read env %s: %w (rollback also failed: %v)", envPath, err, rollbackErr)
			}
			return fmt.Errorf("read env %s: %w", envPath, err)
		}
		ef2.Set(secretCfg.EnvKey, state.NewSecret)
		if err := ef2.WriteAtomic(); err != nil {
			rollbackErr := rollback(ctx, state, e.provider, e.docker, e.timeout)
			e.recordFailure(state, err)
			if rollbackErr != nil {
				return fmt.Errorf("write env %s: %w (rollback also failed: %v)", envPath, err, rollbackErr)
			}
			return fmt.Errorf("write env %s: %w", envPath, err)
		}
	}

	// --- StepRestart: Restart containers ---
	state.CurrentStep = StepRestart
	if len(secretCfg.Containers) > 0 {
		if err := docker.RestartInOrder(ctx, e.docker, secretCfg.Containers, e.timeout); err != nil {
			rollbackErr := rollback(ctx, state, e.provider, e.docker, e.timeout)
			e.recordFailure(state, err)
			if rollbackErr != nil {
				return fmt.Errorf("restart: %w (rollback also failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("restart: %w", err)
		}
	}

	// --- StepHealthCheck: Implicit (RestartInOrder waits for healthy) ---
	state.CurrentStep = StepHealthCheck

	// --- StepRecord: Record success in history ---
	state.CurrentStep = StepRecord
	e.recordSuccess(state)

	// --- StepDone ---
	state.CurrentStep = StepDone
	return nil
}

// executeDryRun logs what would happen without performing any mutations.
func (e *Engine) executeDryRun(_ context.Context, state *RotationState, secretCfg config.SecretConfig) error {
	log.Printf("[DRY RUN] Would read env file: %s", state.EnvFilePath)
	log.Printf("[DRY RUN] Would backup secret key: %s", secretCfg.EnvKey)
	log.Printf("[DRY RUN] Would call provider.Rotate for %s (type: %s)", secretCfg.Name, e.provider.Name())

	if e.provider.Name() != "generic" {
		log.Printf("[DRY RUN] Would call provider.Verify")
	} else {
		log.Printf("[DRY RUN] Would skip DB verify (generic provider)")
	}

	for _, p := range state.EnvFilePaths {
		log.Printf("[DRY RUN] Would update env file: %s, key: %s", p, secretCfg.EnvKey)
	}

	for _, c := range secretCfg.Containers {
		log.Printf("[DRY RUN] Would restart container: %s", c)
	}

	log.Printf("[DRY RUN] Would record rotation in history")
	return nil
}

// recordSuccess appends a success entry to the history store.
func (e *Engine) recordSuccess(state *RotationState) {
	if e.history == nil {
		return
	}
	if err := e.history.Append(history.HistoryEntry{
		SecretName: state.SecretName,
		RotatedAt:  time.Now().UTC(),
		OldValue:   state.OldSecret,
		NewHash:    "",
		Status:     "success",
	}); err != nil {
		log.Printf("warning: failed to record success history for %s: %v", state.SecretName, err)
	}
}

// recordFailure appends a failure entry to the history store.
func (e *Engine) recordFailure(state *RotationState, originalErr error) {
	if e.history == nil {
		return
	}
	if err := e.history.Append(history.HistoryEntry{
		SecretName: state.SecretName,
		RotatedAt:  time.Now().UTC(),
		OldValue:   state.OldSecret,
		Status:     "failed",
		Details:    originalErr.Error(),
	}); err != nil {
		log.Printf("warning: failed to record failure history for %s: %v", state.SecretName, err)
	}
}

// resolveEnvFilePaths returns the list of .env file paths from the config.
// Prefers EnvFile if set, otherwise uses EnvFiles.
func resolveEnvFilePaths(cfg config.SecretConfig) []string {
	if cfg.EnvFile != "" {
		return []string{cfg.EnvFile}
	}
	if len(cfg.EnvFiles) > 0 {
		return cfg.EnvFiles
	}
	return nil
}

// buildProviderConfig converts SecretConfig.Provider map to a typed ProviderConfig.
func buildProviderConfig(cfg config.SecretConfig) provider.ProviderConfig {
	pc := provider.ProviderConfig{
		Options: cfg.Provider,
	}
	if v, ok := cfg.Provider["host"]; ok {
		pc.Host = v
	}
	if v, ok := cfg.Provider["port"]; ok {
		if port, err := strconv.Atoi(v); err == nil {
			pc.Port = port
		}
	}
	if v, ok := cfg.Provider["username"]; ok {
		pc.Username = v
	}
	if v, ok := cfg.Provider["database"]; ok {
		pc.Database = v
	}
	return pc
}
