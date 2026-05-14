package db

import (
	"strings"
	"testing"
)

// Phase 2 Cycle F: password hashing helpers + storage on users.
//
// HashPassword(plaintext) → bcrypt hash (string) for storage in
// users.password_hash. VerifyPassword(hash, plaintext) → bool checks
// a candidate plaintext against the stored hash. Both wrap
// golang.org/x/crypto/bcrypt with cost-10 (the project default).
//
// See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md Cycle F.

// TestHashPassword_RoundTrip — hashing a plaintext and then
// verifying with the same plaintext succeeds. Standard bcrypt
// guarantee — the hash is different each call (random salt), but
// VerifyPassword(hash, plaintext) is true.
func TestHashPassword_RoundTrip(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty string")
	}
	if hash == "hunter2" {
		t.Fatal("HashPassword returned plaintext — not a hash")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("HashPassword should produce a bcrypt hash starting with $2")
	}
	if !VerifyPassword(hash, "hunter2") {
		t.Error("VerifyPassword(hash, correct-plaintext) returned false")
	}
}

// TestVerifyPassword_RejectsWrong — verification fails on a
// different plaintext.
func TestVerifyPassword_RejectsWrong(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if VerifyPassword(hash, "wrong") {
		t.Error("VerifyPassword(hash, wrong-plaintext) returned true")
	}
}

// TestVerifyPassword_RejectsEmptyHash — empty stored hash never
// matches anything (defensive: a nil password_hash on a user row
// means "this user can't log in via password").
func TestVerifyPassword_RejectsEmptyHash(t *testing.T) {
	if VerifyPassword("", "any-plaintext") {
		t.Error("VerifyPassword(empty-hash, ...) returned true")
	}
}

