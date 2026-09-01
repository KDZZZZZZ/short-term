package auth

import (
	"errors"
	"strings"
	"testing"
)

func testHasher(t *testing.T) *Hasher {
	t.Helper()

	hasher, err := NewHasher(DefaultArgon2Params())
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}
	return hasher
}

func TestHashVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	hasher := testHasher(t)
	const password = "correct-horse-battery-staple"

	encoded, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if strings.Contains(encoded, password) {
		t.Fatal("the encoded hash contains the plaintext password")
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Fatalf("encoded hash is not in PHC format: %q", encoded)
	}
	if err := hasher.Verify(password, encoded); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := hasher.Verify("wrong-password", encoded); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("Verify with a wrong password = %v, want ErrPasswordMismatch", err)
	}
}

func TestHashUsesAFreshSaltEachTime(t *testing.T) {
	t.Parallel()

	hasher := testHasher(t)
	first, err := hasher.Hash("same-password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	second, err := hasher.Hash("same-password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if first == second {
		t.Fatal("hashing the same password twice produced identical output")
	}
	if err := hasher.Verify("same-password", second); err != nil {
		t.Fatalf("Verify the second hash: %v", err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	t.Parallel()

	hasher := testHasher(t)
	valid, err := hasher.Hash("password-1234")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "empty", encoded: ""},
		{name: "bcrypt", encoded: "$2y$10$abcdefghijklmnopqrstuv"},
		{name: "argon2i", encoded: strings.Replace(valid, "argon2id", "argon2i", 1)},
		{name: "truncated", encoded: valid[:len(valid)-10]},
		{name: "no parameters", encoded: "$argon2id$v=19$$c2FsdA$a2V5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := hasher.Verify("password-1234", tt.encoded); !errors.Is(err, ErrHashMalformed) && !errors.Is(err, ErrPasswordMismatch) {
				t.Fatalf("Verify = %v, want a rejection", err)
			}
		})
	}
}

func TestNewHasherRejectsWeakParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params Argon2Params
	}{
		{name: "too little memory", params: Argon2Params{Memory: 4096, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32}},
		{name: "too few iterations", params: Argon2Params{Memory: 19456, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}},
		{name: "short salt", params: Argon2Params{Memory: 19456, Iterations: 2, Parallelism: 1, SaltLength: 8, KeyLength: 32}},
		{name: "short key", params: Argon2Params{Memory: 19456, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 16}},
		{name: "zero", params: Argon2Params{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewHasher(tt.params); err == nil {
				t.Fatal("NewHasher accepted parameters below the OWASP minimum")
			}
		})
	}
}

func TestNeedsRehashDetectsWeakerStoredParameters(t *testing.T) {
	t.Parallel()

	weak := DefaultArgon2Params()
	weakHasher, err := NewHasher(weak)
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}
	encoded, err := weakHasher.Hash("password-1234")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	strong := weak
	strong.Memory = weak.Memory * 2
	strongHasher, err := NewHasher(strong)
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}

	if !strongHasher.NeedsRehash(encoded) {
		t.Fatal("NeedsRehash = false for a hash below the current parameters")
	}
	if weakHasher.NeedsRehash(encoded) {
		t.Fatal("NeedsRehash = true for a hash at the current parameters")
	}
}

// BenchmarkHash 用于按照 docs/backend-development-plan.md 的要求，在目标主机上
// 选择生产环境的 Argon2id 参数。应报告实测数据，不要假定默认值适合所有机器。
func BenchmarkHash(b *testing.B) {
	hasher, err := NewHasher(DefaultArgon2Params())
	if err != nil {
		b.Fatalf("NewHasher: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := hasher.Hash("correct-horse-battery-staple"); err != nil {
			b.Fatalf("Hash: %v", err)
		}
	}
}
