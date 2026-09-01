// Package migrations embeds Favorite Service schema migrations.
package migrations

import "embed"

// FS contains every migration file for this service.
//
//go:embed *.sql
var FS embed.FS

// Dir is the migration directory within FS.
const Dir = "."
