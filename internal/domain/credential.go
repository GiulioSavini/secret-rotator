package domain

import "fmt"

// Credential wraps a secret value so that it cannot reach a log line, an error
// message or a JSON payload by accident.
//
// The underlying string is only reachable through Expose(), which makes every
// place that genuinely needs the cleartext greppable. Everything else -- fmt,
// encoding/json, %v in a wrapped error -- sees the redaction.
type Credential struct {
	value string
}

// redacted is what a Credential renders as everywhere except Expose().
const redacted = "[REDACTED]"

// NewCredential wraps a cleartext secret value.
func NewCredential(value string) Credential { return Credential{value: value} }

// Expose returns the cleartext. Call it only at the boundary that needs it:
// a database driver, an .env write, an explicit audit decision.
func (c Credential) Expose() string { return c.value }

// IsZero reports whether the credential holds no value.
func (c Credential) IsZero() bool { return c.value == "" }

// Equal compares two credentials. It is not constant-time: the values compared
// here are ones this process generated or just read, never attacker-supplied
// guesses, so there is no timing oracle to protect.
func (c Credential) Equal(other Credential) bool { return c.value == other.value }

// Len returns the length of the cleartext, for strength reporting.
func (c Credential) Len() int { return len(c.value) }

// String implements fmt.Stringer with the redaction.
func (c Credential) String() string { return redacted }

// GoString implements fmt.GoStringer, so %#v redacts too.
func (c Credential) GoString() string { return redacted }

// Format implements fmt.Formatter so that every verb, including %s, %q and
// %v, renders the redaction rather than the value.
func (c Credential) Format(f fmt.State, verb rune) {
	if verb == 'q' {
		_, _ = f.Write([]byte(`"` + redacted + `"`))
		return
	}
	_, _ = f.Write([]byte(redacted))
}

// MarshalJSON keeps credentials out of anything that gets serialised.
// The audit adapter stores the cleartext deliberately, via Expose().
func (c Credential) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}

// Compile-time proof that the redaction actually takes effect in fmt verbs.
var _ fmt.Formatter = Credential{}
