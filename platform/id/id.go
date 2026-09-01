// Package id generates the opaque public identifiers used across services.
//
// docs/software-design.md requires public IDs to be opaque strings that never
// leak a database type or structure. The encoding here is a ULID: a 48-bit
// millisecond timestamp followed by 80 random bits, rendered in Crockford
// base32. Clients must treat the value as opaque, but lexical order matching
// creation order lets repositories page on the primary key alone.
package id

import (
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"
)

// crockford is the Crockford base32 alphabet: no I, L, O or U.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// encodedLen is the fixed ULID length: 128 bits at 5 bits per character.
const encodedLen = 26

// ErrInvalid reports a value that is not a well-formed prefixed identifier.
var ErrInvalid = errors.New("id: malformed identifier")

// Prefix names the aggregate an identifier belongs to. It is cosmetic for
// clients but makes logs and traces readable.
type Prefix string

// Aggregate prefixes owned by the services in this repository.
const (
	PrefixAccount      Prefix = "u"
	PrefixProduct      Prefix = "p"
	PrefixProductImage Prefix = "img"
	PrefixTrade        Prefix = "t"
	PrefixConversation Prefix = "c"
	PrefixMessage      Prefix = "m"
	PrefixEvent        Prefix = "evt"
)

// Generator produces monotonically increasing identifiers. A single Generator
// is safe for concurrent use.
type Generator struct {
	now func() time.Time

	mu       sync.Mutex
	lastMS   uint64
	lastRand [10]byte
}

// NewGenerator builds a Generator reading the given clock. A nil clock uses
// time.Now.
func NewGenerator(now func() time.Time) *Generator {
	if now == nil {
		now = time.Now
	}
	return &Generator{now: now}
}

// New returns a new identifier for the aggregate named by prefix.
func (g *Generator) New(prefix Prefix) string {
	var raw [16]byte
	ms := uint64(g.now().UTC().UnixMilli())

	g.mu.Lock()
	if ms > g.lastMS {
		g.lastMS = ms
		fillRandom(&g.lastRand)
	} else {
		// Same or backwards clock tick: increment the random component so
		// identifiers minted in one millisecond keep creation order.
		ms = g.lastMS
		increment(&g.lastRand)
	}
	entropy := g.lastRand
	g.mu.Unlock()

	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	copy(raw[6:], entropy[:])

	return string(prefix) + "_" + encode(raw)
}

// Parse validates that value is an identifier with the expected prefix and
// returns its encoded body. It exists so transport layers can reject obviously
// forged identifiers before touching the database.
func Parse(prefix Prefix, value string) (string, error) {
	head := string(prefix) + "_"
	body, found := strings.CutPrefix(value, head)
	if !found || len(body) != encodedLen {
		return "", ErrInvalid
	}
	for i := range len(body) {
		if strings.IndexByte(crockford, body[i]) < 0 {
			return "", ErrInvalid
		}
	}
	return body, nil
}

// encode renders 128 bits as the canonical 26-character Crockford base32 ULID
// string. The value is right-aligned in the 130-bit encoding space, so the
// leading character never exceeds 7 and lexical order matches numeric order.
func encode(raw [16]byte) string {
	out := make([]byte, encodedLen)
	for i := encodedLen - 1; i >= 0; i-- {
		out[i] = crockford[raw[15]&0x1f]
		shiftRight5(&raw)
	}
	return string(out)
}

// shiftRight5 divides the big-endian 128-bit value by 32 in place.
func shiftRight5(raw *[16]byte) {
	var carry byte
	for i := range len(raw) {
		next := raw[i] & 0x1f
		raw[i] = carry<<3 | raw[i]>>5
		carry = next
	}
}

// fillRandom panics on a crypto/rand failure: an identifier generator that
// silently degrades to predictable values is worse than a crash.
func fillRandom(dst *[10]byte) {
	if _, err := rand.Read(dst[:]); err != nil {
		panic("id: crypto/rand unavailable: " + err.Error())
	}
}

// increment adds one to the entropy, big-endian. Overflow within a single
// millisecond is astronomically unlikely; it wraps rather than blocking.
func increment(dst *[10]byte) {
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i]++
		if dst[i] != 0 {
			return
		}
	}
}
