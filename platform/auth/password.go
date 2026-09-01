package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id password hashing.
//
// The OWASP Password Storage Cheat Sheet recommends Argon2id and lists
// m=19456 KiB, t=2, p=1 as a minimum configuration; those are the defaults
// here. docs/backend-development-plan.md requires the working parameters to be
// measured on the target ECS instance before production use, which is why they
// are configuration rather than constants. BenchmarkHash in this package is
// the harness for that measurement.

// Errors returned by the password functions.
var (
	ErrPasswordMismatch = errors.New("auth: password does not match")
	ErrHashMalformed    = errors.New("auth: stored hash is malformed")
)

// Argon2Params are the Argon2id work factors.
type Argon2Params struct {
	// Memory is the memory cost in KiB.
	Memory uint32
	// Iterations is the time cost.
	Iterations uint32
	// Parallelism is the number of lanes.
	Parallelism uint8
	// SaltLength is the random salt size in bytes.
	SaltLength uint32
	// KeyLength is the derived key size in bytes.
	KeyLength uint32
}

// DefaultArgon2Params returns the OWASP minimum configuration, with
// parallelism capped by the CPUs actually available to the process.
func DefaultArgon2Params() Argon2Params {
	parallelism := uint8(1)
	if cpus := runtime.NumCPU(); cpus > 1 {
		parallelism = 2
	}
	return Argon2Params{
		Memory:      19456,
		Iterations:  2,
		Parallelism: parallelism,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// Hasher derives and verifies password hashes.
type Hasher struct {
	params Argon2Params
}

// NewHasher builds a Hasher, rejecting parameters weaker than the OWASP
// minimum so a misconfigured deployment cannot silently downgrade security.
func NewHasher(params Argon2Params) (*Hasher, error) {
	switch {
	case params.Memory < 19456:
		return nil, fmt.Errorf("auth: argon2 memory %d KiB is below the 19456 KiB minimum", params.Memory)
	case params.Iterations < 2:
		return nil, fmt.Errorf("auth: argon2 iterations %d is below the minimum of 2", params.Iterations)
	case params.Parallelism < 1:
		return nil, errors.New("auth: argon2 parallelism must be at least 1")
	case params.SaltLength < 16:
		return nil, fmt.Errorf("auth: argon2 salt length %d is below the 16 byte minimum", params.SaltLength)
	case params.KeyLength < 32:
		return nil, fmt.Errorf("auth: argon2 key length %d is below the 32 byte minimum", params.KeyLength)
	}
	return &Hasher{params: params}, nil
}

// Hash derives a new hash with a fresh random salt. The result is the PHC
// string format, so the parameters travel with the hash and can be raised
// later without invalidating stored passwords.
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, h.params.Iterations, h.params.Memory, h.params.Parallelism, h.params.KeyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.params.Memory, h.params.Iterations, h.params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify checks a password against a stored hash, using the parameters
// recorded in the hash rather than the current configuration.
func (h *Hasher) Verify(password, encoded string) error {
	params, salt, want, err := decodeHash(encoded)
	if err != nil {
		return err
	}

	got := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// NeedsRehash reports whether a stored hash was derived with weaker parameters
// than the current configuration, so callers can upgrade it on next login.
func (h *Hasher) NeedsRehash(encoded string) bool {
	params, _, key, err := decodeHash(encoded)
	if err != nil {
		return true
	}
	return params.Memory < h.params.Memory ||
		params.Iterations < h.params.Iterations ||
		uint32(len(key)) < h.params.KeyLength
}

func decodeHash(encoded string) (params Argon2Params, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return params, nil, nil, ErrHashMalformed
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return params, nil, nil, ErrHashMalformed
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return params, nil, nil, ErrHashMalformed
	}

	salt, err = base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return params, nil, nil, ErrHashMalformed
	}
	key, err = base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return params, nil, nil, ErrHashMalformed
	}
	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))
	return params, salt, key, nil
}
