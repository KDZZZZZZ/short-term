// Package system adapts process facilities to the Marketplace application
// ports.
package system

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/id"
)

// Clock reads the process clock.
type Clock struct{}

// Now returns the current UTC time truncated to the precision PostgreSQL
// stores, so a value returned by a write matches a later read exactly.
func (Clock) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// IDs mints the Marketplace identifiers.
type IDs struct {
	generator *id.Generator
}

// NewIDs builds an identifier generator.
func NewIDs() *IDs { return &IDs{generator: id.NewGenerator(nil)} }

// NewProductID returns a fresh opaque product identifier.
func (i *IDs) NewProductID() string { return i.generator.New(id.PrefixProduct) }

// NewImageID returns a fresh opaque image identifier.
func (i *IDs) NewImageID() string { return i.generator.New(id.PrefixProductImage) }

// NewTradeID returns a fresh opaque trade identifier.
func (i *IDs) NewTradeID() string { return i.generator.New(id.PrefixTrade) }

// NewEventID returns a fresh outbox event identifier.
func (i *IDs) NewEventID() string { return i.generator.New(id.PrefixEvent) }
