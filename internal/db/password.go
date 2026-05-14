package db

// Password hashing helpers for the Phase 2 auth_mode=password flow.
// Wraps golang.org/x/crypto/bcrypt with the project default cost (10).
//
// Used by:
//   - The Users GUI handler (admin web UI creates users with a
//     password; the plaintext is hashed before storage).
//   - The /login mode=password handler (validates a submitted
//     password against users.password_hash).
//
// Stored hashes go in users.password_hash (nullable text). NULL on
// the column means "this user has no password set" — for OAuth-only
// or pre-Phase-2 users.

import "golang.org/x/crypto/bcrypt"

const passwordHashCost = 10

// HashPassword wraps a plaintext password in a bcrypt hash suitable
// for storage. Cost is fixed at 10. The salt is random, so calling
// twice with the same plaintext produces different hashes — verify
// with VerifyPassword, not byte equality.
func HashPassword(plaintext string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), passwordHashCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// VerifyPassword reports whether a plaintext matches a stored hash.
// An empty stored hash never matches anything (defensive: the
// nullable password_hash on a non-password-mode user reads as ""
// from sql.NullString, and we don't want that to match).
func VerifyPassword(hash, plaintext string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
