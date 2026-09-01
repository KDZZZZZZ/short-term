package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		SigningKey: []byte(strings.Repeat("k", 32)),
		Issuer:     "shortterm-account",
		Audience:   "shortterm-api",
		TTL:        time.Hour,
		Leeway:     time.Second,
	}
}

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	issuer, err := NewIssuer(testConfig(), func() time.Time { return now }, func() string { return "jti-1" })
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	verifier, err := NewVerifier(testConfig(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	token, expiresAt, err := issuer.Issue("u_01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !expiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("expiresAt = %s, want %s", expiresAt, now.Add(time.Hour))
	}

	claims, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "u_01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("Subject = %q", claims.Subject)
	}
	if claims.ID != "jti-1" {
		t.Fatalf("ID = %q, want jti-1", claims.ID)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	issued := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	issuer, _ := NewIssuer(testConfig(), func() time.Time { return issued }, func() string { return "jti" })
	token, _, err := issuer.Issue("u_1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	later := issued.Add(2 * time.Hour)
	verifier, _ := NewVerifier(testConfig(), func() time.Time { return later })

	if _, err := verifier.Verify(token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("Verify = %v, want ErrTokenExpired", err)
	}
}

func TestVerifyRejectsTamperedAndForeignTokens(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	issuer, _ := NewIssuer(testConfig(), func() time.Time { return now }, func() string { return "jti" })
	token, _, err := issuer.Issue("u_1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	otherKey := testConfig()
	otherKey.SigningKey = []byte(strings.Repeat("z", 32))
	otherIssuer := testConfig()
	otherIssuer.Issuer = "somebody-else"
	otherAudience := testConfig()
	otherAudience.Audience = "another-api"

	tests := []struct {
		name  string
		cfg   Config
		token string
	}{
		{name: "wrong signing key", cfg: otherKey, token: token},
		{name: "wrong issuer", cfg: otherIssuer, token: token},
		{name: "wrong audience", cfg: otherAudience, token: token},
		{name: "tampered payload", cfg: testConfig(), token: tamper(token)},
		{name: "not a token", cfg: testConfig(), token: "not-a-token"},
		{name: "empty", cfg: testConfig(), token: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			verifier, err := NewVerifier(tt.cfg, func() time.Time { return now })
			if err != nil {
				t.Fatalf("NewVerifier: %v", err)
			}
			if _, err := verifier.Verify(tt.token); err == nil {
				t.Fatal("Verify accepted an invalid token")
			}
		})
	}
}

func TestVerifyRejectsAlgNone(t *testing.T) {
	t.Parallel()

	// A classic downgrade attempt: an unsigned token whose header claims the
	// "none" algorithm. RFC 8725 requires the verifier to pin the algorithm.
	const unsigned = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJpc3MiOiJzaG9ydHRlcm0tYWNjb3VudCIsInN1YiI6InVfMSIsImF1ZCI6InNob3J0dGVybS1hcGkiLCJleHAiOjQ4NzE1MTA0MDB9."

	verifier, err := NewVerifier(testConfig(), nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	if _, err := verifier.Verify(unsigned); err == nil {
		t.Fatal("Verify accepted an alg=none token")
	}
}

func TestConfigRejectsWeakKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "short key", cfg: Config{SigningKey: []byte("short"), Issuer: "i", Audience: "a", TTL: time.Hour}},
		{name: "zero key", cfg: Config{SigningKey: make([]byte, 32), Issuer: "i", Audience: "a", TTL: time.Hour}},
		{name: "no issuer", cfg: Config{SigningKey: []byte(strings.Repeat("k", 32)), Audience: "a", TTL: time.Hour}},
		{name: "no audience", cfg: Config{SigningKey: []byte(strings.Repeat("k", 32)), Issuer: "i", TTL: time.Hour}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewVerifier(tt.cfg, nil); err == nil {
				t.Fatal("NewVerifier accepted an unsafe configuration")
			}
		})
	}
}

// tamper flips a character in the payload segment, leaving the signature stale.
func tamper(token string) string {
	parts := strings.Split(token, ".")
	payload := []byte(parts[1])
	if payload[3] == 'A' {
		payload[3] = 'B'
	} else {
		payload[3] = 'A'
	}
	parts[1] = string(payload)
	return strings.Join(parts, ".")
}
