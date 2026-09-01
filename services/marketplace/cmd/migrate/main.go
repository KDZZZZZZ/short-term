// Command migrate 应用 Marketplace Service 数据库迁移后退出，
// 使发布过程可以单独执行 schema 变更。
package main

import (
	"fmt"
	"os"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/config"
	"github.com/KDZZZZZZ/short-term/services/marketplace/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "marketplace migrate: %v\n", err)
		os.Exit(1)
	}

	version, err := pg.Migrate(cfg.DatabaseURL, migrations.FS, migrations.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marketplace migrate: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("marketplace: schema version %d\n", version)
}
