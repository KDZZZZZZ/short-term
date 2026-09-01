// Package migrations embeds Messaging Service database migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

const Dir = "."
