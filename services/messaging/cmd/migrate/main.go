// Command migrate applies Messaging Service schema changes and exits.
package main

import (
	"fmt"
	"os"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/config"
	"github.com/KDZZZZZZ/short-term/services/messaging/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "messaging migrate: %v\n", err)
		os.Exit(1)
	}
	version, err := pg.Migrate(cfg.DatabaseURL, migrations.FS, migrations.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "messaging migrate: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("messaging: schema version %d\n", version)
}
