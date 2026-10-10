package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMaxAge(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"90d", 90 * 24 * time.Hour},
		{"12w", 12 * 7 * 24 * time.Hour},
		{"36h", 36 * time.Hour},
		{"30m", 30 * time.Minute},
		// Mixed units: the d/w prefix is consumed, the rest goes to
		// time.ParseDuration.
		{"1d12h", 36 * time.Hour},
		{"2w3d", (2*7 + 3) * 24 * time.Hour},
		{"1w1d1h", (7+1)*24*time.Hour + time.Hour},
		{"  90d  ", 90 * 24 * time.Hour},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseMaxAge(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, got.Duration())
		})
	}
}

func TestParseMaxAgeRejectsNonsense(t *testing.T) {
	for _, in := range []string{"soon", "90", "90x", "-1d", "0d", "0", "d90"} {
		t.Run(in, func(t *testing.T) {
			_, err := ParseMaxAge(in)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidSecret)
		})
	}
}

func TestMaxAgeRendersInTheUnitItWasWritten(t *testing.T) {
	// A config that says 90d should read back as 90d in the status table,
	// not as 2160h0m0s.
	for in, want := range map[string]string{
		"90d": "90d",
		"12w": "12w",
		"7d":  "1w",
		"36h": "36h0m0s",
		"":    "none",
	} {
		got, err := ParseMaxAge(in)
		require.NoError(t, err)
		assert.Equal(t, want, got.String(), "input %q", in)
	}
}

func policySecret(t *testing.T, maxAge string) *Secret {
	t.Helper()
	secret, _, err := NewSecret(SecretSpec{
		Name:     "db_password",
		Kind:     "generic",
		EnvKey:   "DB_PASSWORD",
		EnvFiles: []string{".env"},
		MaxAge:   maxAge,
	})
	require.NoError(t, err)
	return secret
}

func TestEvaluateWithinPolicy(t *testing.T) {
	now := time.Now()
	st := policySecret(t, "90d").Evaluate(now.Add(-10*24*time.Hour), true, true, now)

	assert.Equal(t, ComplianceOK, st.Compliance)
	assert.False(t, st.DueForRotation())
	assert.Zero(t, st.OverdueBy)
	assert.InDelta(t, (10 * 24 * time.Hour).Hours(), st.Age.Hours(), 0.01)
}

func TestEvaluatePastPolicy(t *testing.T) {
	now := time.Now()
	st := policySecret(t, "90d").Evaluate(now.Add(-100*24*time.Hour), true, true, now)

	assert.Equal(t, ComplianceDue, st.Compliance)
	assert.True(t, st.DueForRotation())
	assert.True(t, st.Compliance.NeedsAttention())
	assert.InDelta(t, (10 * 24 * time.Hour).Hours(), st.OverdueBy.Hours(), 0.01)
}

func TestEvaluateExactlyAtThePolicyIsStillCompliant(t *testing.T) {
	// The policy is "no older than 90 days", so 90 days exactly is allowed.
	now := time.Now()
	last := now.Add(-90 * 24 * time.Hour)
	st := policySecret(t, "90d").Evaluate(last, true, true, now)

	assert.Equal(t, ComplianceOK, st.Compliance)
}

func TestEvaluateNeverRotatedWithPolicy(t *testing.T) {
	now := time.Now()
	st := policySecret(t, "90d").Evaluate(now, false, true, now)

	assert.Equal(t, ComplianceNeverRotated, st.Compliance)
	assert.True(t, st.DueForRotation(), "with a policy in force, no record needs the same action as too old")
	assert.False(t, st.HasRotated)
}

func TestEvaluateWithoutPolicy(t *testing.T) {
	now := time.Now()
	st := policySecret(t, "").Evaluate(now.Add(-5*365*24*time.Hour), true, true, now)

	assert.Equal(t, ComplianceNoPolicy, st.Compliance)
	assert.False(t, st.DueForRotation(), "no policy means nothing is due by age")
	assert.False(t, st.Compliance.NeedsAttention())
}

func TestEvaluateWithUnreadableHistory(t *testing.T) {
	// An unreadable history must not be reported as a clean bill of health,
	// and must not trigger a rotation either.
	now := time.Now()
	st := policySecret(t, "90d").Evaluate(now, false, false, now)

	assert.Equal(t, ComplianceUnknown, st.Compliance)
	assert.True(t, st.Compliance.NeedsAttention(), "a policy that cannot be evaluated is not a policy that is met")
	assert.False(t, st.DueForRotation(), "rotating on an unknown age would be rotating blind")
}

func TestEvaluateIgnoresAFutureTimestamp(t *testing.T) {
	// Clock skew between hosts writing the same history must not produce a
	// negative age.
	now := time.Now()
	st := policySecret(t, "90d").Evaluate(now.Add(time.Hour), true, true, now)

	assert.Zero(t, st.Age)
	assert.Equal(t, ComplianceOK, st.Compliance)
}

func TestSecretCarriesItsPolicy(t *testing.T) {
	secret := policySecret(t, "6w")
	assert.True(t, secret.MaxAge().IsSet())
	assert.Equal(t, "6w", secret.MaxAge().String())

	assert.False(t, policySecret(t, "").MaxAge().IsSet())
}

func TestInvalidMaxAgeRejectsTheSecret(t *testing.T) {
	_, _, err := NewSecret(SecretSpec{
		Name:     "db_password",
		Kind:     "generic",
		EnvKey:   "DB_PASSWORD",
		EnvFiles: []string{".env"},
		MaxAge:   "whenever",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSecret)
}
