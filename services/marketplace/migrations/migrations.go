// Package migrations embeds the Marketplace Service database migrations.
package migrations

import "embed"

// FS holds every migration file, named per docs/backend-conventions.md.
//
//go:embed *.sql
var FS embed.FS

// Dir is the path of the migrations inside FS.
const Dir = "."
