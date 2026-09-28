package envstore

import (
	"fmt"
	"os"

	"github.com/giulio/secret-rotator/internal/domain"
)

// Store is the filesystem-backed implementation of domain.EnvStore.
//
// Reads keep the file's exact bytes so a rotation can be undone byte for byte,
// and every write goes through the same atomic, permission-preserving path.
type Store struct{}

// NewStore returns the filesystem env store.
func NewStore() *Store { return &Store{} }

// Open reads and parses an .env file.
func (s *Store) Open(path domain.FilePath) (domain.EnvDocument, error) {
	raw, err := os.ReadFile(path.String())
	if err != nil {
		return nil, err
	}
	ef, err := Parse(path.String(), raw)
	if err != nil {
		return nil, err
	}
	return document{file: ef, raw: raw}, nil
}

// Write sets a key to a new value and returns the bytes the file held before.
// Formatting, comments, ordering, quote style and permissions are preserved.
func (s *Store) Write(path domain.FilePath, key domain.EnvKey, value domain.Credential) ([]byte, error) {
	before, err := os.ReadFile(path.String())
	if err != nil {
		return nil, err
	}

	ef, err := Parse(path.String(), before)
	if err != nil {
		return nil, err
	}
	if _, found := ef.Get(key.String()); !found {
		return nil, fmt.Errorf("%w: %s is not present in %s", domain.ErrSecretKeyMissing, key, path)
	}

	ef.Set(key.String(), value.Expose())
	if err := ef.WriteAtomic(); err != nil {
		return nil, err
	}
	return before, nil
}

// Restore rewrites a file with previously captured bytes.
func (s *Store) Restore(path domain.FilePath, snapshot []byte) error {
	if snapshot == nil {
		return fmt.Errorf("restoring %s: no snapshot was captured", path)
	}
	return writeAtomic(path.String(), snapshot)
}

// document adapts a parsed EnvFile to the domain.EnvDocument port.
type document struct {
	file *EnvFile
	raw  []byte
}

func (d document) Path() domain.FilePath { return domain.FilePath(d.file.Path) }

func (d document) Lookup(key domain.EnvKey) (string, bool) { return d.file.Get(key.String()) }

func (d document) Keys() []domain.EnvKey {
	raw := d.file.Keys()
	keys := make([]domain.EnvKey, 0, len(raw))
	for _, k := range raw {
		keys = append(keys, domain.EnvKey(k))
	}
	return keys
}

// Snapshot returns the file's bytes as read, not a re-render of the parsed
// lines, so a restore cannot be affected by a parsing quirk.
func (d document) Snapshot() []byte {
	out := make([]byte, len(d.raw))
	copy(out, d.raw)
	return out
}

var (
	_ domain.EnvStore    = (*Store)(nil)
	_ domain.EnvDocument = document{}
)
