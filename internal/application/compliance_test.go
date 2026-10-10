package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GiulioSavini/secret-rotator/internal/domain"
)

// fakeHistory serves fixed last-rotation timestamps.
type fakeHistory struct {
	last map[string]time.Time
	err  error
}

func (h *fakeHistory) LastSuccess(_ context.Context, name domain.SecretName) (time.Time, bool, error) {
	if h.err != nil {
		return time.Time{}, false, h.err
	}
	t, ok := h.last[name.String()]
	return t, ok, nil
}

func policySecretFor(t *testing.T, name, maxAge string) *domain.Secret {
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

func TestComplianceSeparatesCompliantFromOverdue(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	secrets := []*domain.Secret{
		policySecretFor(t, "fresh", "90d"),
		policySecretFor(t, "stale", "30d"),
		policySecretFor(t, "unmanaged", ""),
		policySecretFor(t, "untouched", "90d"),
	}

	history := &fakeHistory{last: map[string]time.Time{
		"fresh":     now.Add(-10 * 24 * time.Hour),
		"stale":     now.Add(-45 * 24 * time.Hour),
		"unmanaged": now.Add(-5 * 365 * 24 * time.Hour),
		// "untouched" has no record at all.
	}}

	report := NewCheckCompliance(history, fixedClock{t: now}).Execute(context.Background(), secrets)
	require.True(t, report.HistoryReadable)
	require.Len(t, report.Standings, 4)

	byName := map[string]domain.Standing{}
	for _, s := range report.Standings {
		byName[s.Secret.Name().String()] = s
	}

	assert.Equal(t, domain.ComplianceOK, byName["fresh"].Compliance)
	assert.Equal(t, domain.ComplianceDue, byName["stale"].Compliance)
	assert.Equal(t, domain.ComplianceNoPolicy, byName["unmanaged"].Compliance)
	assert.Equal(t, domain.ComplianceNeverRotated, byName["untouched"].Compliance)

	attention := report.NeedsAttention()
	assert.Len(t, attention, 2, "only 'stale' and 'untouched' need attention")

	due := report.DueForRotation()
	require.Len(t, due, 2)
	names := []string{due[0].Name().String(), due[1].Name().String()}
	assert.ElementsMatch(t, []string{"stale", "untouched"}, names)
}

func TestComplianceWithoutHistoryIsUnknownNotClean(t *testing.T) {
	now := time.Now()
	secrets := []*domain.Secret{policySecretFor(t, "db", "90d")}

	// nil history: no passphrase, so ages cannot be known.
	report := NewCheckCompliance(nil, fixedClock{t: now}).Execute(context.Background(), secrets)

	assert.False(t, report.HistoryReadable)
	assert.Contains(t, report.Reason, "ROTATOR_MASTER_KEY")
	require.Len(t, report.Standings, 1)
	assert.Equal(t, domain.ComplianceUnknown, report.Standings[0].Compliance)

	assert.Len(t, report.NeedsAttention(), 1, "an unevaluable policy must be surfaced")
	assert.Empty(t, report.DueForRotation(), "nothing may be rotated on an unknown age")
}

func TestComplianceSurvivesAnUnreadableEntry(t *testing.T) {
	now := time.Now()
	secrets := []*domain.Secret{policySecretFor(t, "db", "90d")}
	history := &fakeHistory{err: errors.New("wrong passphrase")}

	report := NewCheckCompliance(history, fixedClock{t: now}).Execute(context.Background(), secrets)

	require.Len(t, report.Standings, 1)
	assert.Equal(t, domain.ComplianceUnknown, report.Standings[0].Compliance,
		"a read failure must not be reported as 'never rotated', which would invite a needless rotation")
	assert.Contains(t, report.Reason, "wrong passphrase")
	assert.Empty(t, report.DueForRotation())
}

func TestComplianceWithoutPoliciesNeedsNoAttention(t *testing.T) {
	now := time.Now()
	secrets := []*domain.Secret{
		policySecretFor(t, "a", ""),
		policySecretFor(t, "b", ""),
	}

	report := NewCheckCompliance(&fakeHistory{}, fixedClock{t: now}).Execute(context.Background(), secrets)

	assert.Empty(t, report.NeedsAttention())
	assert.Empty(t, report.DueForRotation())
}
