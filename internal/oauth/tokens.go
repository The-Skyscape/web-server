package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"github.com/pkg/errors"
)

// GenerateToken generates a cryptographically secure random token
func GenerateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.Wrap(err, "failed to generate random token")
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// HashToken hashes a token using SHA-256
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// VerifyToken checks if a plaintext token matches a hash
func VerifyToken(plaintext, hashed string) bool {
	return HashToken(plaintext) == hashed
}
