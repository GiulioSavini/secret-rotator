package discovery

import (
	"testing"

	"github.com/giulio/secret-rotator/internal/envfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyKey(t *testing.T) {
	s := NewScanner()

	tests := []struct {
		key            string
		wantType       string
		wantFileRef    bool
		wantClassified bool
	}{
		{"DB_PASSWORD", "password", false, true},
		{"MYSQL_ROOT_PASSWORD", "password", false, true},
		{"APP_SECRET", "secret", false, true},
		{"API_KEY", "api_key", false, true},
		{"AUTH_TOKEN", "token", false, true},
		{"STRIPE_API_KEY", "api_key", false, true},
		{"JWT_SECRET", "secret", false, true},
		{"DATABASE_URL", "connection_string", false, true},
		{"REDIS_URL", "connection_string", false, true},
		{"HOSTNAME", "", false, false},
		{"APP_NAME", "", false, false},
		{"PORT", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			secretType, fileRef := s.classifyKey(tt.key)
			if tt.wantClassified {
				assert.Equal(t, tt.wantType, secretType, "type mismatch for %s", tt.key)
			} else {
				assert.Empty(t, secretType, "expected no classification for %s", tt.key)
			}
			assert.Equal(t, tt.wantFileRef, fileRef, "fileRef mismatch for %s", tt.key)
		})
	}
}

func TestClassifyKey_FILE(t *testing.T) {
	s := NewScanner()

	secretType, fileRef := s.classifyKey("DB_PASSWORD_FILE")
	assert.Equal(t, "password", secretType)
	assert.True(t, fileRef, "DB_PASSWORD_FILE should be file-referenced")

	secretType2, fileRef2 := s.classifyKey("REDIS_SECRET_FILE")
	assert.Equal(t, "secret", secretType2)
	assert.True(t, fileRef2, "REDIS_SECRET_FILE should be file-referenced")

	// Not a file-referenced secret: HOSTNAME_FILE doesn't match any pattern before _FILE
	secretType3, fileRef3 := s.classifyKey("HOSTNAME_FILE")
	assert.Empty(t, secretType3)
	assert.False(t, fileRef3)
}

func TestScanFile(t *testing.T) {
	ef := &envfile.EnvFile{
		Path: ".env",
		Lines: []envfile.Line{
			{Key: "MYSQL_PASSWORD", Value: "weak"},
			{Key: "REDIS_PASSWORD", Value: "xK9#mP2$vL5@nQ8&jR4!wT7*yU0^zA3b"},
			{Key: "HOSTNAME", Value: "localhost"},
		},
	}
	// Build the index so Get/Keys works
	ef.Reindex()

	s := NewScanner()
	secrets := s.ScanFile(ef)

	require.Len(t, secrets, 2, "should find exactly 2 secrets, not HOSTNAME")

	// Verify MYSQL_PASSWORD
	assert.Equal(t, "MYSQL_PASSWORD", secrets[0].Key)
	assert.Equal(t, "password", secrets[0].Type)
	assert.Equal(t, ".env", secrets[0].Source)
	assert.Equal(t, StrengthWeak, secrets[0].Strength.Score)
	assert.False(t, secrets[0].FileReferenced)

	// Verify REDIS_PASSWORD
	assert.Equal(t, "REDIS_PASSWORD", secrets[1].Key)
	assert.Equal(t, "password", secrets[1].Type)
	assert.Equal(t, StrengthStrong, secrets[1].Strength.Score)
}
