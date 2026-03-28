package history

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/giulio/secret-rotator/internal/crypto"
)

// Store manages an encrypted history file on disk. It caches the derived key
// so that key derivation (Argon2id) happens at most once per Store instance.
type Store struct {
	path       string
	passphrase []byte
	derivedKey []byte // cached after first key derivation
	salt       []byte // cached from file or generated on first write
}

// NewStore creates a new Store for the given file path and passphrase.
func NewStore(path string, passphrase []byte) *Store {
	return &Store{
		path:       path,
		passphrase: passphrase,
	}
}

// Append encrypts and appends a HistoryEntry to the history file.
// If the file does not exist, it creates a new one with a random salt.
func (s *Store) Append(entry HistoryEntry) error {
	hf, err := s.readFile()
	if err != nil {
		return err
	}

	// Ensure we have a salt and derived key
	if err := s.ensureKey(hf); err != nil {
		return err
	}

	// Marshal the entry to JSON
	plaintext, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshalling entry: %w", err)
	}

	// Encrypt using cached key and salt
	encrypted, err := crypto.EncryptWithKey(plaintext, s.derivedKey, s.salt)
	if err != nil {
		return fmt.Errorf("encrypting entry: %w", err)
	}

	// Append to entries
	hf.Entries = append(hf.Entries, EncryptedEntry{
		Data:      base64.StdEncoding.EncodeToString(encrypted),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	return s.writeFile(hf)
}

// List reads and decrypts all entries from the history file.
// Returns an empty slice (not an error) if the file does not exist.
// Corrupted entries are skipped silently.
func (s *Store) List() ([]HistoryEntry, error) {
	hf, err := s.readFile()
	if err != nil {
		return nil, err
	}

	if len(hf.Entries) == 0 {
		return nil, nil
	}

	// Ensure we have a salt and derived key
	if err := s.ensureKey(hf); err != nil {
		return nil, err
	}

	var entries []HistoryEntry
	for _, enc := range hf.Entries {
		data, err := base64.StdEncoding.DecodeString(enc.Data)
		if err != nil {
			// Corrupted base64 -- skip
			continue
		}

		plaintext, err := crypto.DecryptWithKey(data, s.derivedKey)
		if err != nil {
			// Corrupted or wrong key -- skip
			continue
		}

		var he HistoryEntry
		if err := json.Unmarshal(plaintext, &he); err != nil {
			// Corrupted JSON -- skip
			continue
		}

		entries = append(entries, he)
	}

	return entries, nil
}

// readFile reads and parses the history file. Returns a new HistoryFile if the
// file does not exist.
func (s *Store) readFile() (*HistoryFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &HistoryFile{Version: 1}, nil
		}
		return nil, fmt.Errorf("reading history file: %w", err)
	}

	var hf HistoryFile
	if err := json.Unmarshal(data, &hf); err != nil {
		return nil, fmt.Errorf("parsing history file: %w", err)
	}

	return &hf, nil
}

// writeFile atomically writes the history file using a temp file + rename pattern.
func (s *Store) writeFile(hf *HistoryFile) error {
	// Update salt in file if we generated one
	if s.salt != nil {
		hf.Salt = base64.StdEncoding.EncodeToString(s.salt)
	}

	data, err := json.MarshalIndent(hf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling history file: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// ensureKey loads the salt from the history file (or generates a new one)
// and derives the encryption key if not already cached.
func (s *Store) ensureKey(hf *HistoryFile) error {
	if s.derivedKey != nil {
		return nil
	}

	// Try to load salt from file
	if hf.Salt != "" {
		salt, err := base64.StdEncoding.DecodeString(hf.Salt)
		if err != nil {
			return fmt.Errorf("decoding salt: %w", err)
		}
		s.salt = salt
	}

	// Generate new salt if needed
	if s.salt == nil {
		s.salt = make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, s.salt); err != nil {
			return fmt.Errorf("generating salt: %w", err)
		}
	}

	s.derivedKey = crypto.DeriveKey(s.passphrase, s.salt)
	return nil
}
