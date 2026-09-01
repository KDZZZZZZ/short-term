// Command migrate 应用 Account Service 数据库迁移后退出。
// 部署会在启动服务端前单独执行此命令，从而让发布过程可以使用
// docs/software-design.md 第 9.3 节中的扩展/收缩序列。
package main

import (
	"fmt"
	"os"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/account/internal/config"
	"github.com/KDZZZZZZ/short-term/services/account/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "account migrate: %v\n", err)
		os.Exit(1)
	}

	version, err := pg.Migrate(cfg.DatabaseURL, migrations.FS, migrations.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "account migrate: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("account: schema version %d\n", version)
}
