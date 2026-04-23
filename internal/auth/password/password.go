// Package password provides secure, bcrypt-based password hashing and verification.
package password

import "golang.org/x/crypto/bcrypt"

// cost defines the bcrypt work factor. 12 is the modern security baseline,
// balancing brute-force resistance with acceptable login endpoint latency.
const cost = 12

// Hash generates a bcrypt hash from a plaintext password.
// Time Complexity: $O(2^{\text{cost}})$. Space Complexity: $O(1)$.
func Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	return string(b), err
}

// Compare verifies a plaintext password against a bcrypt hash safely in constant time.
// Time Complexity: $O(2^{\text{cost}})$.
func Compare(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
