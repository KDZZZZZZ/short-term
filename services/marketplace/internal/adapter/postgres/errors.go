package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isForeignKeyViolation reports whether err is a foreign key violation, which
// happens when an image is written for a product that no longer exists.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
