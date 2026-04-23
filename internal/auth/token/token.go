// Package token provides cryptographically secure token generation and hashing
// for authentication, session management, and verification flows.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// GenerateRandomToken generates a cryptographically secure, URL-safe string.
//
// Note: The `size` parameter specifies the number of raw random bytes generated,
// not the final string length. Due to Base64 encoding, the returned string
// will be approximately 33% longer than `size`.
//
// Time Complexity: $O(N)$ where N is size. Space Complexity: $O(N)$ for the buffer.
func GenerateRandomToken(size int) (string, error) {
	// Note: We intentionally do not bound-check size > 0 here to keep the API minimal.
	// It is assumed `size` is driven by internal server constants, not user input.
	// If user input drives size, add a max-bound to prevent $O(N)$ memory exhaustion (OOM DoS).
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// RawURLEncoding is specifically chosen over StdEncoding to omit padding '='
	// and unsafe URL characters ('+' and '/'), making it safe for email links and headers.
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashSHA256 returns the hexadecimal string representation of the SHA-256 hash.
//
// Time Complexity: $O(N)$ where N is the length of the token.
// Space Complexity: $O(N)$ to cast the string to a byte slice.
func HashSHA256(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
