// Package migrations 嵌入 Marketplace Service 数据库迁移。
package migrations

import "embed"

// FS 保存所有迁移文件，文件名遵循 docs/backend-conventions.md。
//
//go:embed *.sql
var FS embed.FS

// Dir 是迁移文件在 FS 中的路径。
const Dir = "."
