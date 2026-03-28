package envfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Set updates a key's value, preserving the line's formatting and quote style.
// If the key does not exist in the file, this is a no-op.
func (ef *EnvFile) Set(key, newValue string) {
	idx, ok := ef.index[key]
	if !ok {
		return
	}
	line := &ef.Lines[idx]
	line.Value = newValue
	// Reconstruct raw line preserving quote style
	if line.Quoted != "" {
		line.Raw = fmt.Sprintf("%s=%s%s%s", line.Key, line.Quoted, newValue, line.Quoted)
	} else {
		line.Raw = fmt.Sprintf("%s=%s", line.Key, newValue)
	}
}

// WriteAtomic writes the .env file atomically via temp-file + sync + rename.
// Preserves the original file permissions.
func (ef *EnvFile) WriteAtomic() error {
	// 1. Preserve original permissions
	info, err := os.Stat(ef.Path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", ef.Path, err)
	}

	// 2. Create temp file in SAME directory (required for atomic rename on same filesystem)
	dir := filepath.Dir(ef.Path)
	tmp, err := os.CreateTemp(dir, ".env.tmp.*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // cleanup on any error path

	// 3. Write all lines
	var buf strings.Builder
	for i, line := range ef.Lines {
		buf.WriteString(line.Raw)
		if i < len(ef.Lines)-1 {
			buf.WriteByte('\n')
		}
	}
	if _, err := tmp.WriteString(buf.String()); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}

	// 4. Sync to disk before rename
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	tmp.Close()

	// 5. Match original permissions
	if err := os.Chmod(tmpPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}

	// 6. Atomic rename
	if err := os.Rename(tmpPath, ef.Path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, ef.Path, err)
	}

	return nil
}
