package envstore

import (
	"os"
	"strings"
)

// Read parses an .env file preserving all formatting.
func Read(path string) (*EnvFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, data)
}

// Parse builds an EnvFile from bytes already read from path. Splitting it out
// of Read lets the store capture the exact bytes once and reuse them both for
// parsing and as the restore snapshot.
func Parse(path string, data []byte) (*EnvFile, error) {
	ef := &EnvFile{Path: path, index: make(map[string]int)}
	for i, raw := range strings.Split(string(data), "\n") {
		line := parseLine(raw)
		ef.Lines = append(ef.Lines, line)
		if line.Key != "" {
			ef.index[line.Key] = i
		}
	}
	return ef, nil
}

// parseLine extracts key, value, quote style from a single line.
// Handles: KEY=VALUE, KEY="VALUE", KEY='VALUE', # comment, blank,
// export KEY=VALUE, and malformed lines (no = sign).
func parseLine(raw string) Line {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Line{Raw: raw}
	}
	if strings.HasPrefix(trimmed, "#") {
		return Line{Raw: raw, Comment: true}
	}

	// Strip optional "export " prefix
	work := trimmed
	if strings.HasPrefix(work, "export ") {
		work = strings.TrimPrefix(work, "export ")
		work = strings.TrimSpace(work)
	}

	// Parse KEY=VALUE with optional quotes
	idx := strings.IndexByte(work, '=')
	if idx < 0 {
		return Line{Raw: raw} // malformed, preserve as-is
	}
	key := strings.TrimSpace(work[:idx])
	val := work[idx+1:]
	quoted := ""
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'') {
			quoted = string(val[0])
			val = val[1 : len(val)-1]
		}
	}
	return Line{Raw: raw, Key: key, Value: val, Quoted: quoted}
}
