package discovery

import (
	"strings"

	"github.com/giulio/secret-rotator/internal/envfile"
)

// Scanner identifies secrets in environment files by key naming patterns.
type Scanner struct {
	suffixPatterns []SecretPattern
	exactPatterns  map[string]string
}

// DiscoveredSecret represents a secret found during scanning.
type DiscoveredSecret struct {
	Key            string
	Type           string
	Source         string
	Value          string // For internal strength check only -- NEVER expose in output.
	Strength       StrengthResult
	FileReferenced bool
}

// NewScanner creates a Scanner with default patterns.
func NewScanner() *Scanner {
	return &Scanner{
		suffixPatterns: DefaultPatterns,
		exactPatterns:  ExactPatterns,
	}
}

// classifyKey determines whether a key name represents a secret.
// Returns the secret type and whether it is a file reference (_FILE suffix).
func (s *Scanner) classifyKey(key string) (string, bool) {
	upper := strings.ToUpper(key)

	// Check _FILE suffix first: if the key ends with _FILE, try to classify
	// the base key (without _FILE). If that matches, it's a file-referenced secret.
	if strings.HasSuffix(upper, "_FILE") {
		base := upper[:len(upper)-5] // strip "_FILE"
		if secretType := s.matchKey(base); secretType != "" {
			return secretType, true
		}
		// _FILE suffix but base doesn't match any pattern -- not a secret.
		return "", false
	}

	secretType := s.matchKey(upper)
	return secretType, false
}

// matchKey checks exact and suffix patterns against an uppercase key.
func (s *Scanner) matchKey(upper string) string {
	// Exact matches first (higher priority).
	if t, ok := s.exactPatterns[upper]; ok {
		return t
	}

	// Suffix matches -- longer suffixes checked first (DefaultPatterns order matters).
	for _, p := range s.suffixPatterns {
		if strings.HasSuffix(upper, p.Suffix) {
			return p.Type
		}
	}

	return ""
}

// ScanFile scans an EnvFile and returns discovered secrets.
func (s *Scanner) ScanFile(ef *envfile.EnvFile) []DiscoveredSecret {
	var results []DiscoveredSecret
	for _, key := range ef.Keys() {
		secretType, fileRef := s.classifyKey(key)
		if secretType == "" {
			continue
		}

		value, _ := ef.Get(key)
		ds := DiscoveredSecret{
			Key:            key,
			Type:           secretType,
			Source:         ef.Path,
			Value:          value,
			FileReferenced: fileRef,
		}

		// Don't audit strength for file-referenced secrets (the value is a path).
		if !fileRef {
			ds.Strength = AuditStrength(value)
		}

		results = append(results, ds)
	}
	return results
}

// ScanFiles scans multiple EnvFiles and returns all discovered secrets.
func (s *Scanner) ScanFiles(files []*envfile.EnvFile) []DiscoveredSecret {
	var all []DiscoveredSecret
	for _, ef := range files {
		all = append(all, s.ScanFile(ef)...)
	}
	return all
}
