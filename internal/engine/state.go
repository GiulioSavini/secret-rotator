// Package engine provides the rotation execution engine with state tracking and LIFO rollback.
package engine

// RotationStep represents a step in the rotation pipeline.
type RotationStep int

const (
	StepInit        RotationStep = iota // Resolve and read .env file
	StepBackup                          // Back up old secret and .env content
	StepGenerate                        // Generate new secret (provider.Rotate)
	StepApplyDB                         // Apply new password to database (implicit in Rotate for DB providers)
	StepVerifyDB                        // Verify new password works on database
	StepUpdateEnv                       // Update .env file(s) with new secret
	StepRestart                         // Restart containers
	StepHealthCheck                     // Wait for containers to become healthy
	StepRecord                          // Record rotation outcome in history
	StepDone                            // Rotation complete
)

// String returns a human-readable name for the step.
func (s RotationStep) String() string {
	switch s {
	case StepInit:
		return "init"
	case StepBackup:
		return "backup"
	case StepGenerate:
		return "generate"
	case StepApplyDB:
		return "apply_db"
	case StepVerifyDB:
		return "verify_db"
	case StepUpdateEnv:
		return "update_env"
	case StepRestart:
		return "restart"
	case StepHealthCheck:
		return "health_check"
	case StepRecord:
		return "record"
	case StepDone:
		return "done"
	default:
		return "unknown"
	}
}

// RotationState tracks the current state of a rotation for rollback purposes.
type RotationState struct {
	SecretName    string
	CurrentStep   RotationStep
	OldSecret     string
	NewSecret     string
	OldEnvContent []byte   // Raw .env file bytes for restore on rollback
	EnvFilePath   string   // Resolved primary .env file path
	EnvFilePaths  []string // All .env file paths (for multi-file updates)
	Containers    []string
	Error         error
}
