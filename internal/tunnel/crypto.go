package tunnel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// Cipher handles encryption/decryption of VPN packets.
type Cipher struct {
	gcm cipher.AEAD
}

// NewCipher creates a new AES-256-GCM cipher.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &Cipher{gcm: gcm}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns nonce + ciphertext.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends ciphertext to nonce
	ciphertext := c.gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext that was encrypted with Encrypt.
// Expects nonce + ciphertext format.
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := c.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := c.gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// Overhead returns the number of bytes added by encryption.
// This is nonce (12 bytes) + auth tag (16 bytes) = 28 bytes.
func (c *Cipher) Overhead() int {
	return c.gcm.NonceSize() + c.gcm.Overhead()
}
