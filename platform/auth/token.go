// Package auth issues and verifies the access tokens that carry the acting
// user between the Gateway and the services.
//
// It lives in platform because the Account Service signs tokens and the
// Gateway verifies them: a second, drifting implementation of claim validation
// is a security risk, not an independence benefit. The package contains no
// domain rules — only the token format and its checks.
//
// The checks follow RFC 8725 (JSON Web Token Best Current Practices): the
// algorithm is fixed by the verifier instead of read from the header, and
// issuer, audience and expiry are all required.
package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Errors returned by Verify. Callers map them to UNAUTHORIZED; they exist
// separately so logs can tell an expired token from a forged one.
var (
	ErrTokenMalformed = errors.New("auth: token is malformed")
	ErrTokenExpired   = errors.New("auth: token is expired")
	ErrTokenInvalid   = errors.New("auth: token is invalid")
)

// signingMethod is fixed for the whole system. HMAC is appropriate while the
// issuer and the verifier are operated together; moving verification outside
// this trust boundary would require an asymmetric algorithm instead.
var signingMethod = jwt.SigningMethodHS256

// MinKeyLength is the shortest accepted signing key. HS256 keys shorter than
// the 256-bit hash output weaken the MAC, so short keys are rejected at
// construction rather than at first use.
const MinKeyLength = 32

// Config describes the token format shared by the issuer and the verifier.
type Config struct {
	// SigningKey is the shared HMAC secret. It is a credential: never log it.
	SigningKey []byte
	// Issuer is the expected iss claim.
	Issuer string
	// Audience is the expected aud claim.
	Audience string
	// TTL is how long an issued token stays valid.
	TTL time.Duration
	// Leeway absorbs small clock differences between services.
	Leeway time.Duration
}

// Claims is the verified content of an access token.
type Claims struct {
	// Subject is the account identifier of the acting user.
	Subject string
	// ID is the jti claim, unique per issued token.
	ID string
	// ExpiresAt is when the token stops being accepted.
	ExpiresAt time.Time
}

// Issuer signs access tokens.
type Issuer struct {
	cfg Config
	now func() time.Time
	id  func() string
}

// NewIssuer validates the configuration and builds an Issuer. The now and
// newID functions are injectable for tests; nil uses the process clock and the
// caller must then supply newID.
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

// Issue signs a token for the given account and reports when it expires.
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

// TTL reports the configured token lifetime.
func (i *Issuer) TTL() time.Duration { return i.cfg.TTL }

// Verifier checks access tokens.
type Verifier struct {
	cfg    Config
	parser *jwt.Parser
}

// NewVerifier validates the configuration and builds a Verifier.
func NewVerifier(cfg Config, now func() time.Time) (*Verifier, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	opts := []jwt.ParserOption{
		// Pinning the algorithm is what stops an attacker from presenting a
		// token signed with "none" or with an algorithm confusion trick.
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

// Verify parses and validates a token, returning its claims.
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
