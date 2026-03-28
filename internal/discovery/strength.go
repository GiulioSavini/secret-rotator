package discovery

// Strength represents a password strength level.
type Strength int

const (
	StrengthWeak   Strength = 0
	StrengthFair   Strength = 1
	StrengthGood   Strength = 2
	StrengthStrong Strength = 3
)

// String returns the human-readable strength label.
func (s Strength) String() string {
	switch s {
	case StrengthWeak:
		return "weak"
	case StrengthFair:
		return "fair"
	case StrengthGood:
		return "good"
	case StrengthStrong:
		return "strong"
	default:
		return "unknown"
	}
}

// StrengthResult holds the detailed result of a password strength audit.
type StrengthResult struct {
	Score   Strength
	Entropy float64
	Length  int
	Issues  []string
}

// AuditStrength analyzes a password value and returns its strength assessment.
func AuditStrength(value string) StrengthResult {
	return StrengthResult{}
}
