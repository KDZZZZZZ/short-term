// Package migrations embeds the Account Service database migrations so the
// service binary can apply them without shipping a separate file tree.
package migrations

import "embed"

// FS holds every migration file, named per docs/backend-conventions.md.
//
//go:embed *.sql
var FS embed.FS

// Dir is the path of the migrations inside FS.
const Dir = "."
