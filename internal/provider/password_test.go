package provider

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePasswordLength(t *testing.T) {
	// 32 bytes -> 43 chars in base64 RawURL encoding (ceil(32*4/3) = 43)
	pw, err := GeneratePassword(32)
	require.NoError(t, err)
	assert.Len(t, pw, 43)
}

func TestGeneratePasswordDefaultLength(t *testing.T) {
	pw, err := GeneratePassword(0)
	require.NoError(t, err)
	// DefaultPasswordLength=32 -> 43 chars
	assert.Len(t, pw, 43)
}

func TestGeneratePasswordURLSafe(t *testing.T) {
	pw, err := GeneratePassword(32)
	require.NoError(t, err)
	matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, pw)
	assert.True(t, matched, "password should only contain URL-safe base64 chars, got: %s", pw)
}

func TestGeneratePasswordUniqueness(t *testing.T) {
	pw1, err := GeneratePassword(32)
	require.NoError(t, err)
	pw2, err := GeneratePassword(32)
	require.NoError(t, err)
	assert.NotEqual(t, pw1, pw2)
}
