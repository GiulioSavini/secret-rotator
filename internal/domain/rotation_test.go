package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSecret(t *testing.T, mutate func(*SecretSpec)) *Secret {
	t.Helper()

	spec := SecretSpec{
		Name:       "db_password",
		Kind:       "postgres",
		EnvKey:     "DB_PASSWORD",
		EnvFiles:   []string{".env"},
		Containers: []string{"db", "app"},
		Target:     Target{Host: "db", AdminUser: "postgres", Database: "app"},
	}
	if mutate != nil {
		mutate(&spec)
	}

	secret, _, err := NewSecret(spec)
	require.NoError(t, err)
	return secret
}

func beganRotation(t *testing.T, secret *Secret) *Rotation {
	t.Helper()
	r := BeginRotation(secret, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC))
	r.RecordCurrentValue(NewCredential("old"), []byte("DB_PASSWORD=old\n"))
	return r
}

func TestNothingToCompensateBeforeGeneration(t *testing.T) {
	r := beganRotation(t, testSecret(t, nil))
	assert.Empty(t, r.CompensationPlan(), "reading a file changes nothing")
}

func TestNothingToCompensateAfterGenerationAlone(t *testing.T) {
	r := beganRotation(t, testSecret(t, nil))
	r.RecordGenerated(NewCredential("new"))

	// A value exists but has not been applied anywhere, so there is no state
	// to undo.
	assert.Empty(t, r.CompensationPlan())
}

func TestCompensationRestoresTheCredentialOnceApplied(t *testing.T) {
	r := beganRotation(t, testSecret(t, nil))
	r.RecordGenerated(NewCredential("new"))
	r.RecordApplied()

	plan := r.CompensationPlan()
	require.Len(t, plan, 1)
	assert.Equal(t, CompensationRestoreCredential, plan[0].Type)
}

func TestCompensationRestoresEveryWrittenFile(t *testing.T) {
	secret := testSecret(t, func(spec *SecretSpec) {
		spec.EnvFiles = []string{".env", ".env.local", "docker/.env"}
	})

	r := beganRotation(t, secret)
	r.RecordGenerated(NewCredential("new"))
	r.RecordApplied()
	r.RecordFileWritten(".env", []byte("a"))
	r.RecordFileWritten(".env.local", []byte("b"))

	plan := r.CompensationPlan()
	require.Len(t, plan, 3)

	assert.Equal(t, CompensationRestoreFile, plan[0].Type)
	assert.Equal(t, FilePath(".env"), plan[0].Path)
	assert.Equal(t, CompensationRestoreFile, plan[1].Type)
	assert.Equal(t, FilePath(".env.local"), plan[1].Path)
	assert.Equal(t, CompensationRestoreCredential, plan[2].Type)

	// The file that was never written is not in the plan: restoring it would
	// mean writing a snapshot nobody captured.
	for _, action := range plan {
		assert.NotEqual(t, FilePath("docker/.env"), action.Path)
	}
}

func TestCompensationOrdersFilesBeforeRestartBeforeCredential(t *testing.T) {
	r := beganRotation(t, testSecret(t, nil))
	r.RecordGenerated(NewCredential("new"))
	r.RecordApplied()
	r.RecordVerified()
	r.RecordFileWritten(".env", []byte("a"))
	r.RecordRestarted([]ContainerRef{"proj-db-1", "proj-app-1"})

	plan := r.CompensationPlan()
	require.Len(t, plan, 3)

	// Files first, so the restart picks up the restored values; the backing
	// service last, so running containers keep working until then.
	assert.Equal(t, CompensationRestoreFile, plan[0].Type)
	assert.Equal(t, CompensationRestartContainers, plan[1].Type)
	assert.Equal(t, []ContainerRef{"proj-db-1", "proj-app-1"}, plan[1].Containers)
	assert.Equal(t, CompensationRestoreCredential, plan[2].Type)
}

func TestGenericSecretNeverRestoresACredential(t *testing.T) {
	secret := testSecret(t, func(spec *SecretSpec) {
		spec.Kind = "generic"
		spec.Target = Target{}
	})

	r := beganRotation(t, secret)
	r.RecordGenerated(NewCredential("new"))
	r.RecordApplied()
	r.RecordFileWritten(".env", []byte("a"))

	plan := r.CompensationPlan()
	require.Len(t, plan, 1)
	assert.Equal(t, CompensationRestoreFile, plan[0].Type)
}

func TestStepNeverGoesBackwards(t *testing.T) {
	r := beganRotation(t, testSecret(t, nil))
	r.RecordGenerated(NewCredential("new"))
	r.RecordApplied()
	r.RecordVerified()
	r.RecordFileWritten(".env", []byte("a"))

	// Writing a second file must not drag the pipeline back to StepFilesUpdated
	// from a later step.
	r.RecordRestarted([]ContainerRef{"x"})
	r.RecordFileWritten(".env.local", []byte("b"))

	assert.Equal(t, StepRestarted, r.Step())
}

func TestRecordsCarryTheOutcome(t *testing.T) {
	finished := time.Date(2026, 9, 28, 12, 5, 0, 0, time.UTC)

	r := beganRotation(t, testSecret(t, nil))
	r.RecordGenerated(NewCredential("new"))
	r.RecordDone()

	success := r.Succeeded(finished)
	assert.Equal(t, OutcomeSuccess, success.Outcome)
	assert.Equal(t, SecretName("db_password"), success.SecretName)
	assert.Equal(t, finished, success.FinishedAt)

	failure := r.Failed(finished, errors.New("boom"))
	assert.Equal(t, OutcomeFailed, failure.Outcome)
	assert.Equal(t, "boom", failure.Details)
	assert.Equal(t, "old", failure.Previous.Expose())
}

func TestStateDescriptionDistinguishesCleanRollback(t *testing.T) {
	r := beganRotation(t, testSecret(t, nil))
	r.RecordGenerated(NewCredential("new"))
	r.RecordApplied()

	assert.Contains(t, r.StateDescription(nil), "fully rolled back")
	assert.Contains(t, r.StateDescription([]error{errors.New("x")}), "did not fully succeed")
}
