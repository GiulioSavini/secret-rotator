package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt(t *testing.T) {
	plaintext := []byte("my secret data 12345")
	passphrase := []byte("strong-passphrase-here")

	encrypted, err := Encrypt(plaintext, passphrase)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, encrypted)

	decrypted, err := Decrypt(encrypted, passphrase)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptProducesDifferentCiphertext(t *testing.T) {
	plaintext := []byte("same plaintext every time")
	passphrase := []byte("passphrase")

	enc1, err := Encrypt(plaintext, passphrase)
	require.NoError(t, err)

	enc2, err := Encrypt(plaintext, passphrase)
	require.NoError(t, err)

	assert.NotEqual(t, enc1, enc2, "two encryptions of same data should produce different ciphertext")
}

func TestDecryptWrongPassphrase(t *testing.T) {
	plaintext := []byte("secret stuff")
	passphrase := []byte("correct-passphrase")
	wrongPassphrase := []byte("wrong-passphrase")

	encrypted, err := Encrypt(plaintext, passphrase)
	require.NoError(t, err)

	_, err = Decrypt(encrypted, wrongPassphrase)
	assert.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestDecryptTooShort(t *testing.T) {
	_, err := Decrypt([]byte("short"), []byte("passphrase"))
	assert.ErrorIs(t, err, ErrDataTooShort)
}

func TestDeriveKey(t *testing.T) {
	passphrase := []byte("my-passphrase")
	salt1 := []byte("0123456789abcdef") // 16 bytes
	salt2 := []byte("fedcba9876543210") // 16 bytes

	key1a := DeriveKey(passphrase, salt1)
	key1b := DeriveKey(passphrase, salt1)
	key2 := DeriveKey(passphrase, salt2)

	assert.Equal(t, key1a, key1b, "same passphrase+salt should produce same key")
	assert.NotEqual(t, key1a, key2, "different salt should produce different key")
	assert.Len(t, key1a, 32, "key should be 32 bytes (AES-256)")
}
