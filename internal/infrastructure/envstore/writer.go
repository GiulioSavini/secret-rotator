package envstore

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

// WriteAtomic writes the .env file atomically, preserving its permissions.
func (ef *EnvFile) WriteAtomic() error {
	return writeAtomic(ef.Path, []byte(ef.render()))
}

// render reconstructs the file's text from its lines.
func (ef *EnvFile) render() string {
	var buf strings.Builder
	for i, line := range ef.Lines {
		buf.WriteString(line.Raw)
		if i < len(ef.Lines)-1 {
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

// writeAtomic replaces a file's contents via temp file, fsync and rename,
// keeping the original permissions.
//
// Every write to an .env file goes through here, including the restore that
// compensates a failed rotation: a rollback that widened a 0600 file to 0644
// would turn a failed rotation into a disclosure.
func writeAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	// The temp file must share a filesystem with the target for rename to be
	// atomic, so it is created in the same directory.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".env.tmp.*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename has succeeded

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
