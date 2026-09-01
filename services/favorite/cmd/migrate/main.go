// Command migrate applies Favorite Service database migrations and exits.
package main

import (
	"fmt"
	"os"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/config"
	"github.com/KDZZZZZZ/short-term/services/favorite/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "favorite migrate: %v\n", err)
		os.Exit(1)
	}

	version, err := pg.Migrate(cfg.DatabaseURL, migrations.FS, migrations.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "favorite migrate: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("favorite: schema version %d\n", version)
}
