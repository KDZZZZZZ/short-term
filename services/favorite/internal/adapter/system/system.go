// Package system supplies production clock implementations.
package system

import "time"

// Clock reads UTC wall time.
type Clock struct{}

// Now implements application.Clock.
func (Clock) Now() time.Time { return time.Now().UTC() }
