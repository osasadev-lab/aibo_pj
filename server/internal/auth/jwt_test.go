package auth

import (
	"testing"

	"github.com/google/uuid"
)

func TestIssueAndParseToken_RoundTrip(t *testing.T) {
	secret := "test-secret"
	userID := uuid.New()

	token, err := IssueToken(secret, userID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if token == "" {
		t.Fatal("IssueToken returned empty string")
	}

	got, err := ParseToken(secret, token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if got != userID {
		t.Errorf("got user id %s, want %s", got, userID)
	}
}

func TestParseToken_WrongSecretRejected(t *testing.T) {
	userID := uuid.New()
	token, err := IssueToken("secret-a", userID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	if _, err := ParseToken("secret-b", token); err == nil {
		t.Error("expected error when parsing token with wrong secret, got nil")
	}
}

func TestParseToken_GarbageRejected(t *testing.T) {
	if _, err := ParseToken("secret", "not-a-jwt"); err == nil {
		t.Error("expected error when parsing a non-JWT string, got nil")
	}
}

func TestParseToken_EmptyRejected(t *testing.T) {
	if _, err := ParseToken("secret", ""); err == nil {
		t.Error("expected error when parsing an empty string, got nil")
	}
}
