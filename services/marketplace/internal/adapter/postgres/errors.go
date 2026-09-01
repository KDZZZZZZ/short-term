package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isForeignKeyViolation 判断 err 是否为外键约束冲突；当为已不存在的商品写入图片时会发生这种情况。
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
