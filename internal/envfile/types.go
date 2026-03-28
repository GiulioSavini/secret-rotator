package envfile

// Line represents a single line in an .env file.
// Comments and blank lines have Key == "".
type Line struct {
	Raw     string // Original line text (preserved exactly)
	Key     string // Parsed key (empty for comments/blanks)
	Value   string // Parsed value (unquoted)
	Quoted  string // Quote style: "", "'", "\""
	Comment bool   // True if this is a comment line
}

// EnvFile represents a parsed .env file that preserves formatting.
type EnvFile struct {
	Path  string
	Lines []Line
	index map[string]int // key -> line index for O(1) lookup
}

// Get returns the value for a key using O(1) index lookup.
func (ef *EnvFile) Get(key string) (string, bool) {
	if idx, ok := ef.index[key]; ok {
		return ef.Lines[idx].Value, true
	}
	return "", false
}

// Keys returns all keys in file order.
func (ef *EnvFile) Keys() []string {
	var keys []string
	for _, line := range ef.Lines {
		if line.Key != "" {
			keys = append(keys, line.Key)
		}
	}
	return keys
}
