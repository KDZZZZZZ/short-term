// Package auth 负责签发和验证在网关与各服务之间传递当前用户身份的访问令牌。
//
// 该包位于 platform 中，是因为账户服务签发令牌、网关验证令牌：
// 另行维护一套可能逐渐偏离的声明校验逻辑会带来安全风险，而不是独立性收益。
// 本包不包含领域规则，只处理令牌格式及其校验。
//
// 校验遵循 RFC 8725（JSON Web Token 最佳实践）：算法由验证器固定，而不是从
// 请求头读取；签发者、受众和过期时间均为必填项。
package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Verify 返回的错误。调用方将它们映射为 UNAUTHORIZED；单独区分这些错误，
// 便于日志区分令牌过期和令牌伪造。
var (
	ErrTokenMalformed = errors.New("auth: token is malformed")
	ErrTokenExpired   = errors.New("auth: token is expired")
	ErrTokenInvalid   = errors.New("auth: token is invalid")
)

// signingMethod 在整个系统中固定。签发者和验证器共同运行时，HMAC 是合适的；
// 如果将验证移到该信任边界之外，则需要改用非对称算法。
var signingMethod = jwt.SigningMethodHS256

// MinKeyLength 是接受的最短签名密钥长度。短于 256 位哈希输出的 HS256 密钥会
// 削弱 MAC，因此在构造时而不是首次使用时拒绝过短密钥。
const MinKeyLength = 32

// Config 描述签发者和验证器共享的令牌格式。
type Config struct {
	// SigningKey 是共享的 HMAC 密钥，属于凭据，绝不能记录到日志中。
	SigningKey []byte
	// Issuer 是期望的 iss 声明。
	Issuer string
	// Audience 是期望的 aud 声明。
	Audience string
	// TTL 是签发的令牌保持有效的时长。
	TTL time.Duration
	// Leeway 用于吸收服务之间较小的时钟偏差。
	Leeway time.Duration
}

// Claims 是访问令牌中经过验证的内容。
type Claims struct {
	// Subject 是当前用户的账户标识。
	Subject string
	// ID 是 jti 声明，每个签发的令牌都唯一。
	ID string
	// ExpiresAt 是令牌停止被接受的时间。
	ExpiresAt time.Time
}

// Issuer 签发访问令牌。
type Issuer struct {
	cfg Config
	now func() time.Time
	id  func() string
}

// NewIssuer 校验配置并构造 Issuer。now 和 newID 函数可注入测试实现；
// now 为 nil 时使用进程时钟，而 newID 仍必须由调用方提供。
func NewIssuer(cfg Config, now func() time.Time, newID func() string) (*Issuer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.TTL <= 0 {
		return nil, errors.New("auth: TTL must be positive")
	}
	if newID == nil {
		return nil, errors.New("auth: token ID generator is required")
	}
	if now == nil {
		now = time.Now
	}
	return &Issuer{cfg: cfg, now: now, id: newID}, nil
}

// Issue 为指定账户签发令牌，并返回令牌过期时间。
func (i *Issuer) Issue(subject string) (token string, expiresAt time.Time, err error) {
	if subject == "" {
		return "", time.Time{}, errors.New("auth: subject is required")
	}

	issuedAt := i.now().UTC()
	expiresAt = issuedAt.Add(i.cfg.TTL)

	claims := jwt.RegisteredClaims{
		Issuer:    i.cfg.Issuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{i.cfg.Audience},
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		NotBefore: jwt.NewNumericDate(issuedAt),
		ID:        i.id(),
	}

	signed, err := jwt.NewWithClaims(signingMethod, claims).SignedString(i.cfg.SigningKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// TTL 返回配置的令牌有效期。
func (i *Issuer) TTL() time.Duration { return i.cfg.TTL }

// Verifier 校验访问令牌。
type Verifier struct {
	cfg    Config
	parser *jwt.Parser
}

// NewVerifier 校验配置并构造 Verifier。
func NewVerifier(cfg Config, now func() time.Time) (*Verifier, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	opts := []jwt.ParserOption{
		// 固定算法可以阻止攻击者提交使用 "none" 签名的令牌，或利用算法混淆技巧。
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(cfg.Leeway),
	}
	if now != nil {
		opts = append(opts, jwt.WithTimeFunc(now))
	}
	return &Verifier{cfg: cfg, parser: jwt.NewParser(opts...)}, nil
}

// Verify 解析并校验令牌，返回其中的声明。
func (v *Verifier) Verify(token string) (Claims, error) {
	var registered jwt.RegisteredClaims
	parsed, err := v.parser.ParseWithClaims(token, &registered, func(*jwt.Token) (any, error) {
		return v.cfg.SigningKey, nil
	})
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return Claims{}, ErrTokenExpired
	case errors.Is(err, jwt.ErrTokenMalformed):
		return Claims{}, ErrTokenMalformed
	case err != nil:
		return Claims{}, ErrTokenInvalid
	case !parsed.Valid:
		return Claims{}, ErrTokenInvalid
	}

	if registered.Subject == "" {
		return Claims{}, ErrTokenInvalid
	}
	return Claims{
		Subject:   registered.Subject,
		ID:        registered.ID,
		ExpiresAt: registered.ExpiresAt.Time,
	}, nil
}

func (c Config) validate() error {
	if len(c.SigningKey) < MinKeyLength {
		return fmt.Errorf("auth: signing key must be at least %d bytes", MinKeyLength)
	}
	if subtle.ConstantTimeCompare(c.SigningKey, make([]byte, len(c.SigningKey))) == 1 {
		return errors.New("auth: signing key must not be all zero bytes")
	}
	if c.Issuer == "" {
		return errors.New("auth: issuer is required")
	}
	if c.Audience == "" {
		return errors.New("auth: audience is required")
	}
	return nil
}
