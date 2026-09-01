// Package application holds the Account Service use cases. It receives
// commands and queries that carry no transport or database detail, applies the
// domain rules and returns domain objects for an adapter to map.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

// Repository errors. They describe storage outcomes, not HTTP or gRPC
// results; the service translates them into contract error codes.
var (
	// ErrNotFound reports that no account matched.
	ErrNotFound = errors.New("account not found")
	// ErrStudentNoTaken reports that the student number is already registered.
	ErrStudentNoTaken = errors.New("student number already registered")
)

// Repository stores accounts. Implementations must map a unique-constraint
// violation on student_no to ErrStudentNoTaken rather than checking first,
// so two concurrent registrations cannot both succeed.
type Repository interface {
	Create(ctx context.Context, account *domain.Account) error
	ByID(ctx context.Context, id string) (*domain.Account, error)
	ByStudentNo(ctx context.Context, studentNo string) (*domain.Account, error)
	// ByIDs returns the accounts that exist, in no particular order. Missing
	// identifiers are simply absent from the result.
	ByIDs(ctx context.Context, ids []string) ([]*domain.Account, error)
	Update(ctx context.Context, account *domain.Account) error
}

// PasswordHasher derives and checks password hashes.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) error
	NeedsRehash(encoded string) bool
}

// TokenIssuer signs access tokens for an authenticated account.
type TokenIssuer interface {
	Issue(subject string) (token string, expiresAt time.Time, err error)
}

// IDGenerator mints opaque account identifiers.
type IDGenerator interface {
	New() string
}

// Clock reads the current time. Injecting it keeps timestamps deterministic in
// tests.
type Clock interface {
	Now() time.Time
}
