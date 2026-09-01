// Command migrate applies the Marketplace Service database migrations and
// exits, so a release can run schema changes as a separate step.
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
