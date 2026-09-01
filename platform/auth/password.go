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

// Argon2id 密码哈希。
//
// OWASP《密码存储备忘单》推荐 Argon2id，并将 m=19456 KiB、t=2、p=1 列为最低配置；
// 这里使用这些值作为默认值。docs/backend-development-plan.md 要求在生产使用前，
// 于目标 ECS 实例上测量实际参数，因此这些参数被设为配置而不是常量。本包中的
// BenchmarkHash 用于执行该测量。

// 密码函数返回的错误。
var (
	ErrPasswordMismatch = errors.New("auth: password does not match")
	ErrHashMalformed    = errors.New("auth: stored hash is malformed")
)

// Argon2Params 是 Argon2id 的工作因子。
type Argon2Params struct {
	// Memory 是以 KiB 为单位的内存成本。
	Memory uint32
	// Iterations 是时间成本。
	Iterations uint32
	// Parallelism 是并行通道数。
	Parallelism uint8
	// SaltLength 是随机盐的字节数。
	SaltLength uint32
	// KeyLength 是派生密钥的字节数。
	KeyLength uint32
}

// DefaultArgon2Params 返回 OWASP 最低配置，并将并行度限制为进程实际可用的 CPU 数。
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

// Hasher 派生并验证密码哈希。
type Hasher struct {
	params Argon2Params
}

// NewHasher 构造 Hasher，并拒绝弱于 OWASP 最低要求的参数，避免错误配置的部署
// 在不易察觉的情况下降低安全性。
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

// Hash 使用新的随机盐派生哈希。结果采用 PHC 字符串格式，因此参数会随哈希保存，
// 日后提高参数强度时无需使已存储的密码失效。
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

// Verify 使用哈希中记录的参数，而不是当前配置，校验密码与已存储哈希是否匹配。
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

// NeedsRehash 判断已存储哈希的派生参数是否弱于当前配置，以便调用方在下次登录时
// 升级哈希。
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
