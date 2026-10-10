package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/audit"
)

// seedHistory writes a successful rotation for name at the given time into a
// fresh history file, and points the CLI at it.
func seedHistory(t *testing.T, passphrase string, entries map[string]time.Time) {
	t.Helper()

	dir := t.TempDir()
	histDir := filepath.Join(dir, ".rotator")
	require.NoError(t, os.MkdirAll(histDir, 0o700))

	store := audit.NewStore(filepath.Join(histDir, "history.json"), []byte(passphrase))
	for name, at := range entries {
		require.NoError(t, store.Append(audit.HistoryEntry{
			SecretName: name,
			RotatedAt:  at,
			Status:     string(domain.OutcomeSuccess),
		}))
	}

	useHistoryIn(t, dir)
}

// policySecret builds a generic secret carrying a max_age policy.
func policySecret(t *testing.T, name, maxAge string) *domain.Secret {
	t.Helper()
	secret, _, err := domain.NewSecret(domain.SecretSpec{
		Name:     name,
		Kind:     "generic",
		EnvKey:   "APP_SECRET_KEY",
		EnvFiles: []string{".env"},
		MaxAge:   maxAge,
	})
	require.NoError(t, err)
	return secret
}

// runStatusCmd executes status with the given arguments.
func runStatusCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := NewStatusCmd()
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestStatusShowsPolicyColumns(t *testing.T) {
	const pp = "test-passphrase"
	withConfig(t, policySecret(t, "db_password", "90d"))
	seedHistory(t, pp, map[string]time.Time{
		"db_password": time.Now().Add(-10 * 24 * time.Hour),
	})

	out, _, err := runStatusCmd(t, "--passphrase", pp)
	require.NoError(t, err)

	assert.Contains(t, out, "MAX AGE")
	assert.Contains(t, out, "STATE")
	assert.Contains(t, out, "90d")
	assert.Contains(t, out, "ok")
}

func TestStatusReportsOverdueSecret(t *testing.T) {
	const pp = "test-passphrase"
	withConfig(t, policySecret(t, "db_password", "30d"))
	seedHistory(t, pp, map[string]time.Time{
		"db_password": time.Now().Add(-45 * 24 * time.Hour),
	})

	out, _, err := runStatusCmd(t, "--passphrase", pp)
	require.NoError(t, err, "plain status reports but does not fail")

	assert.Contains(t, out, "overdue by 15d")
	assert.Contains(t, out, "1 past their max_age")
}

func TestStatusOverdueExitsNonZero(t *testing.T) {
	const pp = "test-passphrase"
	withConfig(t,
		policySecret(t, "stale", "30d"),
		policySecret(t, "fresh", "90d"),
	)
	seedHistory(t, pp, map[string]time.Time{
		"stale": time.Now().Add(-45 * 24 * time.Hour),
		"fresh": time.Now().Add(-1 * 24 * time.Hour),
	})

	out, _, err := runStatusCmd(t, "--passphrase", pp, "--overdue")

	// The non-zero exit is the contract monitoring relies on.
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverdue), "monitoring distinguishes this from a crash")
	assert.Contains(t, out, "stale")
	assert.NotContains(t, out, "fresh", "--overdue lists only what needs attention")
}

func TestStatusOverdueSucceedsWhenCompliant(t *testing.T) {
	const pp = "test-passphrase"
	withConfig(t, policySecret(t, "db_password", "90d"))
	seedHistory(t, pp, map[string]time.Time{
		"db_password": time.Now().Add(-1 * 24 * time.Hour),
	})

	out, _, err := runStatusCmd(t, "--passphrase", pp, "--overdue")
	require.NoError(t, err)
	assert.Contains(t, out, "All secrets are within their max_age")
}

func TestStatusOverdueFlagsNeverRotated(t *testing.T) {
	const pp = "test-passphrase"
	withConfig(t, policySecret(t, "db_password", "90d"))
	seedHistory(t, pp, nil) // history exists but holds nothing

	out, _, err := runStatusCmd(t, "--passphrase", pp, "--overdue")
	require.Error(t, err)
	assert.Contains(t, out, "never rotated")
}

func TestStatusWithoutPassphraseReportsUnknown(t *testing.T) {
	t.Setenv("ROTATOR_MASTER_KEY", "")
	withConfig(t, policySecret(t, "db_password", "90d"))
	useHistoryIn(t, t.TempDir())

	out, errOut, err := runStatusCmd(t, "--overdue")

	// A policy that cannot be evaluated must not be reported as met.
	require.Error(t, err)
	assert.Contains(t, out, "unknown")
	assert.Contains(t, errOut, "ROTATOR_MASTER_KEY")
}

func TestStatusWithoutPolicyNeedsNoPassphrase(t *testing.T) {
	t.Setenv("ROTATOR_MASTER_KEY", "")
	withConfig(t, policySecret(t, "db_password", ""))
	useHistoryIn(t, t.TempDir())

	out, _, err := runStatusCmd(t, "--overdue")
	require.NoError(t, err, "no policy means nothing can be overdue")
	assert.Contains(t, out, "All secrets are within their max_age")
}
