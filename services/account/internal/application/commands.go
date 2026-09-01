package application

import (
	"time"

	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

// RegisterCommand creates an account and signs in the new user.
type RegisterCommand struct {
	StudentNo string
	Password  string
	// Nickname is optional. When absent the service assigns a neutral default
	// that does not disclose the student number.
	Nickname *string
	Wechat   *string
	QQ       *string
}

// LoginCommand authenticates a student number and password pair.
type LoginCommand struct {
	StudentNo string
	Password  string
}

// StringPatch expresses the three states a nullable JSON field can take in a
// PATCH body: absent, set to a value, or explicitly set to null.
type StringPatch struct {
	// Present is false when the client omitted the field.
	Present bool
	// Value is nil when the client sent null.
	Value *string
}

// Keep returns an unset patch, meaning "leave the field unchanged".
func Keep() StringPatch { return StringPatch{} }

// Set returns a patch that assigns value.
func Set(value string) StringPatch { return StringPatch{Present: true, Value: &value} }

// Clear returns a patch that sets the field to null.
func Clear() StringPatch { return StringPatch{Present: true} }

// UpdateProfileCommand changes the caller's own profile.
type UpdateProfileCommand struct {
	ActorID  string
	Nickname *string
	Wechat   StringPatch
	QQ       StringPatch
}

// ChangePasswordCommand replaces the caller's own password.
type ChangePasswordCommand struct {
	ActorID     string
	OldPassword string
	NewPassword string
}

// AuthResult is the outcome of a successful register or login.
type AuthResult struct {
	AccessToken string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	Account     *domain.Account
}

// ExpiresIn reports the token lifetime in whole seconds, which is what the
// public AuthData schema carries. The value is derived from the issuing clock
// rather than from a later read, so a slow response cannot report a lifetime
// the token does not have.
func (r AuthResult) ExpiresIn() int64 {
	remaining := r.ExpiresAt.Sub(r.IssuedAt)
	if remaining <= 0 {
		return 0
	}
	return int64(remaining.Round(time.Second) / time.Second)
}
