package domain

import "strings"

// Scanner identifies secrets in environment documents by key naming patterns.
// Recognising that MYSQL_ROOT_PASSWORD is a credential and APP_PORT is not is
// a domain rule, so it lives here and works against the EnvDocument port
// rather than a concrete file type.
type Scanner struct {
	suffixPatterns []SecretPattern
	exactPatterns  map[string]string
}

// DiscoveredSecret represents a secret found during scanning.
type DiscoveredSecret struct {
	Key EnvKey
	// Category is the kind of credential the naming suggests ("password",
	// "api_key", ...). It is a hint for reporting, not a Kind.
	Category string
	Source   FilePath
	// Value is kept for the strength audit only and must never be printed.
	Value    Credential
	Strength StrengthResult
	// FileReferenced marks a *_FILE key, whose value is a path rather than a
	// credential.
	FileReferenced bool
}

// NewScanner creates a Scanner with the default patterns.
func NewScanner() *Scanner {
	return &Scanner{
		suffixPatterns: DefaultPatterns,
		exactPatterns:  ExactPatterns,
	}
}

// classifyKey determines whether a key name represents a secret.
// Returns the category and whether it is a file reference (_FILE suffix).
func (s *Scanner) classifyKey(key EnvKey) (string, bool) {
	upper := strings.ToUpper(key.String())

	// Check _FILE suffix first: if the key ends with _FILE, try to classify
	// the base key (without _FILE). If that matches, it's a file-referenced secret.
	if strings.HasSuffix(upper, "_FILE") {
		base := upper[:len(upper)-5] // strip "_FILE"
		if category := s.matchKey(base); category != "" {
			return category, true
		}
		// _FILE suffix but base doesn't match any pattern -- not a secret.
		return "", false
	}

	return s.matchKey(upper), false
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

// ScanDocument scans one environment document and returns discovered secrets.
func (s *Scanner) ScanDocument(doc EnvDocument) []DiscoveredSecret {
	var results []DiscoveredSecret
	for _, key := range doc.Keys() {
		category, fileRef := s.classifyKey(key)
		if category == "" {
			continue
		}

		raw, _ := doc.Lookup(key)
		ds := DiscoveredSecret{
			Key:            key,
			Category:       category,
			Source:         doc.Path(),
			Value:          NewCredential(raw),
			FileReferenced: fileRef,
		}

		// Don't audit strength for file-referenced secrets (the value is a path).
		if !fileRef {
			ds.Strength = AuditStrength(raw)
		}

		results = append(results, ds)
	}
	return results
}

// ScanDocuments scans several documents and returns all discovered secrets.
func (s *Scanner) ScanDocuments(docs []EnvDocument) []DiscoveredSecret {
	var all []DiscoveredSecret
	for _, doc := range docs {
		all = append(all, s.ScanDocument(doc)...)
	}
	return all
}
