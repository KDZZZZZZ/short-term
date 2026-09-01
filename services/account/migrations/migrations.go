// Package migrations 嵌入 Account Service 数据库迁移，使服务二进制文件无需附带
// 独立的文件目录树即可应用迁移。
package migrations

import "embed"

// FS 保存所有迁移文件，文件名遵循 docs/backend-conventions.md。
//
//go:embed *.sql
var FS embed.FS

// Dir 是迁移文件在 FS 中的路径。
const Dir = "."
