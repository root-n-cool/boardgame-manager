package auth_test

import (
	"testing"

	"boardgames-manager/internal/auth"
)

func TestGenerateToken_ReturnsUniqueValues(t *testing.T) {
	a, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if a == b {
		t.Fatal("expected two generated tokens to differ")
	}
}

func TestHashToken_IsDeterministicAndDiffersFromInput(t *testing.T) {
	token := "sample-token"
	h1 := auth.HashToken(token)
	h2 := auth.HashToken(token)
	if h1 != h2 {
		t.Fatal("expected hashing the same token twice to produce the same result")
	}
	if h1 == token {
		t.Fatal("expected hash to differ from the raw token")
	}
}
