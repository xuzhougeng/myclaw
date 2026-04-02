package modelconfig

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	masterKeySize          = 32
	noChangeSecretSentinel = "\x00baize:keep-secret\x00"
)

func (s *Store) loadMasterKeyLocked() ([]byte, error) {
	if strings.TrimSpace(s.keyPath) == "" {
		return nil, errors.New("model secret key store is read-only")
	}

	data, err := os.ReadFile(s.keyPath)
	if err == nil {
		if len(data) != masterKeySize {
			return nil, fmt.Errorf("invalid model secret key length %d", len(data))
		}
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, masterKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.keyPath), 0o700); err != nil {
		return nil, err
	}
	tmpPath := s.keyPath + ".tmp"
	if err := os.WriteFile(tmpPath, key, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, s.keyPath); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptSecret(key []byte, secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(secret), nil)
	return "v1:" + base64.StdEncoding.EncodeToString(nonce) + ":" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptSecret(key []byte, encrypted string) (string, error) {
	encrypted = strings.TrimSpace(encrypted)
	if encrypted == "" {
		return "", nil
	}
	parts := strings.Split(encrypted, ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", fmt.Errorf("unsupported encrypted secret format")
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
