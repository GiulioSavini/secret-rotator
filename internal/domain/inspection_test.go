package domain

import (
	"fmt"
	"testing"

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
			secretType, fileRef := s.classifyKey(EnvKey(tt.key))
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

	secretType, fileRef := s.classifyKey(EnvKey("DB_PASSWORD_FILE"))
	assert.Equal(t, "password", secretType)
	assert.True(t, fileRef, "DB_PASSWORD_FILE should be file-referenced")

	secretType2, fileRef2 := s.classifyKey(EnvKey("REDIS_SECRET_FILE"))
	assert.Equal(t, "secret", secretType2)
	assert.True(t, fileRef2, "REDIS_SECRET_FILE should be file-referenced")

	// Not a file-referenced secret: HOSTNAME_FILE doesn't match any pattern before _FILE
	secretType3, fileRef3 := s.classifyKey(EnvKey("HOSTNAME_FILE"))
	assert.Empty(t, secretType3)
	assert.False(t, fileRef3)
}

// stubDocument is an in-memory EnvDocument, so the scanner can be tested
// without reaching for the filesystem adapter -- which would make the domain
// depend on infrastructure, the one thing this layer must not do.
type stubDocument struct {
	path   FilePath
	order  []EnvKey
	values map[EnvKey]string
}

func newStubDocument(path string, pairs ...[2]string) stubDocument {
	doc := stubDocument{path: FilePath(path), values: map[EnvKey]string{}}
	for _, p := range pairs {
		key := EnvKey(p[0])
		doc.order = append(doc.order, key)
		doc.values[key] = p[1]
	}
	return doc
}

func (d stubDocument) Path() FilePath { return d.path }
func (d stubDocument) Keys() []EnvKey { return d.order }
func (d stubDocument) Lookup(k EnvKey) (string, bool) {
	v, ok := d.values[k]
	return v, ok
}
func (d stubDocument) Snapshot() []byte { return nil }

func TestScanDocument(t *testing.T) {
	doc := newStubDocument(".env",
		[2]string{"MYSQL_PASSWORD", "weak"},
		[2]string{"REDIS_PASSWORD", "xK9#mP2$vL5@nQ8&jR4!wT7*yU0^zA3b"},
		[2]string{"HOSTNAME", "localhost"},
	)

	secrets := NewScanner().ScanDocument(doc)

	require.Len(t, secrets, 2, "should find exactly 2 secrets, not HOSTNAME")

	assert.Equal(t, EnvKey("MYSQL_PASSWORD"), secrets[0].Key)
	assert.Equal(t, "password", secrets[0].Category)
	assert.Equal(t, FilePath(".env"), secrets[0].Source)
	assert.Equal(t, StrengthWeak, secrets[0].Strength.Score)
	assert.False(t, secrets[0].FileReferenced)

	assert.Equal(t, EnvKey("REDIS_PASSWORD"), secrets[1].Key)
	assert.Equal(t, "password", secrets[1].Category)
	assert.Equal(t, StrengthStrong, secrets[1].Strength.Score)
}

func TestScanDocumentRedactsValues(t *testing.T) {
	doc := newStubDocument(".env", [2]string{"API_KEY", "super-secret-value"})

	secrets := NewScanner().ScanDocument(doc)
	require.Len(t, secrets, 1)

	// The scanner keeps the value for the strength audit, but formatting the
	// result must never print it.
	assert.NotContains(t, fmt.Sprintf("%v %s %q", secrets[0].Value, secrets[0].Value, secrets[0].Value),
		"super-secret-value")
	assert.Equal(t, "super-secret-value", secrets[0].Value.Expose())
}
