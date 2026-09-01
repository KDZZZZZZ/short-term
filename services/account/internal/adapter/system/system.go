// Package system adapts process-level facilities — the clock and the
// identifier generator — to the Account Service application ports.
package system

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/id"
)

// Clock reads the process clock.
type Clock struct{}

// Now returns the current UTC time, truncated to microseconds.
//
// Storing UTC keeps timestamps comparable regardless of the host time zone,
// and truncating to the precision PostgreSQL keeps means the value a caller
// receives from a write is byte-identical to the one a later read returns.
func (Clock) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// IDs mints account identifiers.
type IDs struct {
	generator *id.Generator
}

// NewIDs builds an identifier generator for accounts.
func NewIDs() *IDs { return &IDs{generator: id.NewGenerator(nil)} }

// New returns a fresh opaque account identifier.
func (i *IDs) New() string { return i.generator.New(id.PrefixAccount) }
