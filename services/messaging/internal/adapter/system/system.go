// Package system adapts process facilities to Messaging application ports.
package system

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/id"
)

type Clock struct{}

func (Clock) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

type IDs struct{ generator *id.Generator }

func NewIDs() *IDs { return &IDs{generator: id.NewGenerator(nil)} }

func (i *IDs) NewConversationID() string { return i.generator.New(id.PrefixConversation) }
func (i *IDs) NewMessageID() string      { return i.generator.New(id.PrefixMessage) }
func (i *IDs) NewEventID() string        { return i.generator.New(id.PrefixEvent) }
