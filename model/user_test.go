package model

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	const pw = "s3cret-pass"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == pw {
		t.Fatal("hash must not equal the plaintext password")
	}
	if !isBcryptHash(hash) {
		t.Errorf("HashPassword output %q not recognized as a bcrypt hash", hash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)); err != nil {
		t.Errorf("generated hash failed to verify against original password: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong")); err == nil {
		t.Error("verification unexpectedly succeeded for a wrong password")
	}
}

func TestIsBcryptHash(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"$2a$10$abcdefghijklmnopqrstuv", true},
		{"$2b$10$abcdefghijklmnopqrstuv", true},
		{"$2y$10$abcdefghijklmnopqrstuv", true},
		{"plaintextpassword", false},
		{"", false},
		{"$1$md5hash", false},
	}
	for _, tt := range tests {
		if got := isBcryptHash(tt.in); got != tt.want {
			t.Errorf("isBcryptHash(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
