package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MaxAge is how long a credential may stay in service before it has to be
// rotated.
//
// It exists because a rotation policy is usually written as "no credential
// older than 90 days", not as a cron expression. A schedule says *when* to
// rotate; a max age says *what must be true*, which is the form an audit asks
// about and the form that survives a daemon being down for a week.
type MaxAge time.Duration

// NoMaxAge means the secret is only rotated on demand or on its schedule.
const NoMaxAge MaxAge = 0

// durationUnits extends time.ParseDuration, which stops at hours, with the
// units a retention policy is actually written in.
var durationUnits = map[string]time.Duration{
	"d": 24 * time.Hour,
	"w": 7 * 24 * time.Hour,
}

// ParseMaxAge reads a duration such as "90d", "12w", "36h" or "90d12h".
// An empty value means no policy.
func ParseMaxAge(raw string) (MaxAge, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return NoMaxAge, nil
	}

	total, remainder, err := consumeExtendedUnits(trimmed)
	if err != nil {
		return 0, err
	}

	if remainder != "" {
		parsed, err := time.ParseDuration(remainder)
		if err != nil {
			return 0, fmt.Errorf(
				"%w: max_age %q is not a duration (use units d, w, h, m, s, e.g. 90d)",
				ErrInvalidSecret, raw)
		}
		total += parsed
	}

	if total <= 0 {
		return 0, fmt.Errorf("%w: max_age %q must be positive", ErrInvalidSecret, raw)
	}
	return MaxAge(total), nil
}

// consumeExtendedUnits strips the leading <number><d|w> groups and returns
// their total plus whatever is left for time.ParseDuration.
func consumeExtendedUnits(s string) (time.Duration, string, error) {
	var total time.Duration

	for {
		digits := 0
		for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
			digits++
		}
		if digits == 0 || digits >= len(s) {
			return total, s, nil
		}

		unit, ok := durationUnits[string(s[digits])]
		if !ok {
			return total, s, nil
		}

		n, err := strconv.Atoi(s[:digits])
		if err != nil {
			return 0, "", fmt.Errorf("%w: max_age %q has an unreadable number", ErrInvalidSecret, s)
		}
		total += time.Duration(n) * unit
		s = s[digits+1:]

		if s == "" {
			return total, "", nil
		}
	}
}

// IsSet reports whether a policy applies.
func (m MaxAge) IsSet() bool { return m > 0 }

// Duration returns the policy as a time.Duration.
func (m MaxAge) Duration() time.Duration { return time.Duration(m) }

// String renders the policy in the largest whole unit that fits, so a config
// written as "90d" reads back as "90d" rather than "2160h0m0s".
func (m MaxAge) String() string {
	if !m.IsSet() {
		return "none"
	}
	d := time.Duration(m)
	switch {
	case d%(7*24*time.Hour) == 0:
		return fmt.Sprintf("%dw", d/(7*24*time.Hour))
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	default:
		return d.String()
	}
}

// Compliance is a secret's standing against its own policy.
type Compliance string

const (
	// ComplianceOK means the credential is within its maximum age.
	ComplianceOK Compliance = "ok"
	// ComplianceDue means the maximum age has been exceeded.
	ComplianceDue Compliance = "overdue"
	// ComplianceNeverRotated means no successful rotation is on record while
	// a policy applies. It is reported separately from "overdue" because the
	// cause is usually a missing audit trail rather than a missed rotation.
	ComplianceNeverRotated Compliance = "never rotated"
	// ComplianceNoPolicy means the secret has no maximum age, so there is
	// nothing to be compliant with.
	ComplianceNoPolicy Compliance = "no policy"
	// ComplianceUnknown means the rotation history could not be read, so the
	// standing cannot be determined. It is never reported as compliant.
	ComplianceUnknown Compliance = "unknown"
)

// NeedsAttention reports whether a standing should make a check fail. Unknown
// counts: a policy that cannot be evaluated is not a policy that is met.
func (c Compliance) NeedsAttention() bool {
	return c == ComplianceDue || c == ComplianceNeverRotated || c == ComplianceUnknown
}

// Standing is the evaluated state of one secret against its policy.
type Standing struct {
	Secret *Secret
	// LastRotated is when the last successful rotation finished.
	LastRotated time.Time
	// HasRotated is false when no successful rotation is on record.
	HasRotated bool
	// Age is how long the current credential has been in service. Zero when
	// nothing is on record.
	Age time.Duration
	// OverdueBy is how far past the policy the credential is. Zero unless the
	// standing is ComplianceDue.
	OverdueBy  time.Duration
	Compliance Compliance
}

// Evaluate determines a secret's standing at the given time.
//
// historyReadable is false when the audit trail could not be consulted, which
// yields ComplianceUnknown rather than a false clean bill of health.
func (s *Secret) Evaluate(lastRotated time.Time, hasRotated, historyReadable bool, now time.Time) Standing {
	standing := Standing{
		Secret:      s,
		LastRotated: lastRotated,
		HasRotated:  hasRotated,
	}
	if hasRotated {
		standing.Age = now.Sub(lastRotated)
		if standing.Age < 0 {
			standing.Age = 0
		}
	}

	switch {
	case !s.maxAge.IsSet():
		standing.Compliance = ComplianceNoPolicy
	case !historyReadable:
		standing.Compliance = ComplianceUnknown
	case !hasRotated:
		standing.Compliance = ComplianceNeverRotated
	case standing.Age > s.maxAge.Duration():
		standing.Compliance = ComplianceDue
		standing.OverdueBy = standing.Age - s.maxAge.Duration()
	default:
		standing.Compliance = ComplianceOK
	}

	return standing
}

// DueForRotation reports whether the policy alone requires rotating now.
//
// A secret that was never rotated is due: with a policy in force, "no record"
// and "too old" need the same action.
func (st Standing) DueForRotation() bool {
	return st.Compliance == ComplianceDue || st.Compliance == ComplianceNeverRotated
}
