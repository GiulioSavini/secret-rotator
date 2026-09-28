package credential

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/giulio/secret-rotator/internal/domain"
)

// Generator produces credentials from crypto/rand, encoded as URL-safe base64
// without padding.
//
// That alphabet is deliberate: the resulting value contains no quote,
// backslash, @ or / and is therefore safe to place in a connection string, an
// SQL literal and an unquoted .env line without any escaping.
type Generator struct{}

// NewGenerator returns the default password generator.
func NewGenerator() *Generator { return &Generator{} }

// Generate returns a new credential with the requested entropy in bytes. The
// encoded string is about a third longer than the byte count.
func (g *Generator) Generate(length domain.PasswordLength) (domain.Credential, error) {
	n := length.Int()
	if n <= 0 {
		n = domain.DefaultPasswordLength.Int()
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return domain.Credential{}, fmt.Errorf("reading random bytes: %w", err)
	}
	return domain.NewCredential(base64.RawURLEncoding.EncodeToString(buf)), nil
}

// Compile-time check that the adapter satisfies the port.
var _ domain.PasswordGenerator = (*Generator)(nil)
