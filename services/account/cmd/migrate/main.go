// Command migrate applies the Account Service database migrations and exits.
// Deployments run it as a separate step before starting the server, which is
// what lets a release use the expand/contract sequence in
// docs/software-design.md section 9.3.
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
