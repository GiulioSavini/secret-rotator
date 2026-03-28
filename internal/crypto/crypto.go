// Package crypto provides encryption primitives using Argon2id key derivation
// and AES-256-GCM authenticated encryption for securing secrets at rest.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
	nonceLen     = 12 // AES-GCM standard nonce size
)

// Sentinel errors for decryption failures.
var (
	ErrDecryptionFailed = errors.New("decryption failed: wrong passphrase or corrupted data")
	ErrDataTooShort     = errors.New("encrypted data too short")
)

// DeriveKey derives a 32-byte key from a passphrase and salt using Argon2id.
func DeriveKey(passphrase, salt []byte) []byte {
	return argon2.IDKey(passphrase, salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

// Encrypt encrypts plaintext using AES-256-GCM with an Argon2id-derived key.
// The output format is: salt (16 bytes) || nonce (12 bytes) || ciphertext.
// Each call generates a random salt and nonce, so encrypting the same plaintext
// twice produces different ciphertext.
func Encrypt(plaintext, passphrase []byte) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	key := DeriveKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Output: salt || nonce || ciphertext
	result := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// Decrypt decrypts data produced by Encrypt using the given passphrase.
// Returns ErrDataTooShort if the data is smaller than salt+nonce minimum.
// Returns ErrDecryptionFailed if the passphrase is wrong or data is corrupted.
func Decrypt(data, passphrase []byte) ([]byte, error) {
	minLen := saltLen + nonceLen + 1 // at least 1 byte of ciphertext (GCM tag)
	if len(data) < minLen {
		return nil, ErrDataTooShort
	}

	salt := data[:saltLen]
	nonce := data[saltLen : saltLen+nonceLen]
	ciphertext := data[saltLen+nonceLen:]

	key := DeriveKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// EncryptWithKey encrypts plaintext using a pre-derived key and a given salt.
// This avoids repeated key derivation when encrypting multiple entries.
// The output format is: salt (16 bytes) || nonce (12 bytes) || ciphertext.
func EncryptWithKey(plaintext, key, salt []byte) ([]byte, error) {
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	result := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// DecryptWithKey decrypts data using a pre-derived key.
// The data format is: salt (16 bytes) || nonce (12 bytes) || ciphertext.
// The salt in the data is skipped (caller already used it to derive the key).
func DecryptWithKey(data, key []byte) ([]byte, error) {
	minLen := saltLen + nonceLen + 1
	if len(data) < minLen {
		return nil, ErrDataTooShort
	}

	// Skip salt, extract nonce and ciphertext
	nonce := data[saltLen : saltLen+nonceLen]
	ciphertext := data[saltLen+nonceLen:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}
