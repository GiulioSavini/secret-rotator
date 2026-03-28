package discovery

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditStrength_Weak(t *testing.T) {
	// Empty password
	r := AuditStrength("")
	assert.Equal(t, StrengthWeak, r.Score)
	assert.Contains(t, r.Issues, "empty password")

	// Common default
	r2 := AuditStrength("password")
	assert.Equal(t, StrengthWeak, r2.Score)
	assert.Contains(t, r2.Issues, "common default password")

	// Too short
	r3 := AuditStrength("abc")
	assert.Equal(t, StrengthWeak, r3.Score)
}

func TestAuditStrength_Fair(t *testing.T) {
	// 8 chars, 2 classes (lower + digit)
	r := AuditStrength("mypass99")
	assert.Equal(t, StrengthFair, r.Score, "8 char password with 2 classes should be fair")
	assert.Equal(t, 8, r.Length)
}

func TestAuditStrength_Good(t *testing.T) {
	// 16 chars, 4 classes
	r := AuditStrength("MyP@ssw0rd!2024x")
	assert.Equal(t, StrengthGood, r.Score, "16 char password with 4 classes should be good")
}

func TestAuditStrength_Strong(t *testing.T) {
	// 32+ chars, mixed classes
	r := AuditStrength("xK9#mP2$vL5@nQ8&jR4!wT7*yU0^zA3b")
	assert.Equal(t, StrengthStrong, r.Score, "32+ char password with all classes should be strong")
}

func TestAuditStrength_DefaultDetection(t *testing.T) {
	defaults := []string{
		"changeme", "password", "secret", "admin", "root",
		"default", "test", "example", "12345678", "qwerty",
		"letmein", "welcome", "monkey", "master", "dragon",
	}

	for _, pwd := range defaults {
		t.Run(pwd, func(t *testing.T) {
			r := AuditStrength(pwd)
			assert.Equal(t, StrengthWeak, r.Score, "default password %q should be weak", pwd)
			found := false
			for _, issue := range r.Issues {
				if issue == "common default password" {
					found = true
					break
				}
			}
			assert.True(t, found, "default password %q should have 'common default password' issue", pwd)
		})
	}
}

func TestEntropy(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		minEntropy  float64
		maxEntropy  float64
	}{
		// 8 lowercase chars: 8 * log2(26) ≈ 37.6
		{"lowercase_8", "abcdefgh", 37.0, 38.5},
		// 8 chars, lower+digit: 8 * log2(36) ≈ 41.4
		{"alphanumeric_8", "abcdef12", 41.0, 42.0},
		// 16 chars, all classes: 16 * log2(94) ≈ 104.8
		{"full_16", "Abc1!efg2@hij3#k", 100.0, 110.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := AuditStrength(tt.password)
			require.Greater(t, r.Entropy, 0.0, "entropy should be positive")
			assert.GreaterOrEqual(t, r.Entropy, tt.minEntropy, "entropy too low")
			assert.LessOrEqual(t, r.Entropy, tt.maxEntropy, "entropy too high")
		})
	}

	// Verify entropy formula consistency: more chars = more entropy
	r8 := AuditStrength("abcdefgh")
	r16 := AuditStrength("abcdefghabcdefgh")
	assert.Greater(t, r16.Entropy, r8.Entropy, "16 chars should have more entropy than 8")
	// Should be exactly double for same charset
	assert.InDelta(t, r8.Entropy*2, r16.Entropy, 0.01)

	_ = math.Log2 // verify import used
}
