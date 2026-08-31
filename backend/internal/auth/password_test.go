package auth_test

import (
	"testing"

	"boardgames-manager/internal/auth"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "correct-horse-battery" {
		t.Fatal("expected hash to differ from plaintext")
	}
	if !auth.VerifyPassword(hash, "correct-horse-battery") {
		t.Fatal("expected correct password to verify")
	}
	if auth.VerifyPassword(hash, "wrong-password") {
		t.Fatal("expected wrong password to fail verification")
	}
}
