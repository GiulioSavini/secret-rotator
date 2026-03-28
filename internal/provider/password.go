package provider

import (
	"crypto/rand"
	"encoding/base64"
)

// DefaultPasswordLength is the number of random bytes used when
// no explicit length is provided.
const DefaultPasswordLength = 32

// GeneratePassword produces a cryptographically random password encoded
// as URL-safe base64 without padding. If length <= 0, DefaultPasswordLength
// is used.
func GeneratePassword(length int) (string, error) {
	if length <= 0 {
		length = DefaultPasswordLength
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
