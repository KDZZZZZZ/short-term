// Package postgres implements the Account Service repository ports on top of
// the service's own database. Nothing outside this package knows the table
// layout, and no other service may connect to this database.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/account/internal/application"
	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

// studentNoConstraint is the unique index that serialises concurrent
// registrations of the same student number.
const studentNoConstraint = "accounts_student_no_key"

// columns is the projection every read shares, so scanRow stays in step.
const columns = `id, student_no, password_hash, nickname, wechat, qq, created_at, updated_at`

// AccountRepository stores accounts in PostgreSQL.
type AccountRepository struct {
	pool *pgxpool.Pool
}

// NewAccountRepository builds a repository over an open pool.
func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

var _ application.Repository = (*AccountRepository)(nil)

// Create inserts a new account.
//
// It inserts first and interprets the unique violation, rather than checking
// for an existing student number and then inserting: only the database
// constraint can make two concurrent registrations mutually exclusive.
func (r *AccountRepository) Create(ctx context.Context, account *domain.Account) error {
	const query = `
		INSERT INTO accounts (` + columns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.pool.Exec(ctx, query,
		account.ID, account.StudentNo, account.PasswordHash, account.Nickname,
		account.Wechat, account.QQ, account.CreatedAt, account.UpdatedAt,
	)
	if err != nil {
		if pg.IsUniqueViolation(err, studentNoConstraint) {
			return application.ErrStudentNoTaken
		}
		return fmt.Errorf("postgres: insert account: %w", err)
	}
	return nil
}

// ByID loads one account by its opaque identifier.
func (r *AccountRepository) ByID(ctx context.Context, id string) (*domain.Account, error) {
	const query = `SELECT ` + columns + ` FROM accounts WHERE id = $1`

	account, err := scanRow(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		return nil, fmt.Errorf("postgres: select account by id: %w", err)
	}
	return account, nil
}

// ByStudentNo loads one account by its student number.
func (r *AccountRepository) ByStudentNo(ctx context.Context, studentNo string) (*domain.Account, error) {
	const query = `SELECT ` + columns + ` FROM accounts WHERE student_no = $1`

	account, err := scanRow(r.pool.QueryRow(ctx, query, studentNo))
	if err != nil {
		return nil, fmt.Errorf("postgres: select account by student number: %w", err)
	}
	return account, nil
}

// ByIDs loads every account that exists among ids in one round trip.
func (r *AccountRepository) ByIDs(ctx context.Context, ids []string) ([]*domain.Account, error) {
	const query = `SELECT ` + columns + ` FROM accounts WHERE id = ANY($1)`

	rows, err := r.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("postgres: select accounts by ids: %w", err)
	}
	defer rows.Close()

	accounts := make([]*domain.Account, 0, len(ids))
	for rows.Next() {
		account, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read accounts: %w", err)
	}
	return accounts, nil
}

// Update writes back the mutable fields of an account.
func (r *AccountRepository) Update(ctx context.Context, account *domain.Account) error {
	const query = `
		UPDATE accounts
		   SET password_hash = $2, nickname = $3, wechat = $4, qq = $5, updated_at = $6
		 WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query,
		account.ID, account.PasswordHash, account.Nickname,
		account.Wechat, account.QQ, account.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: update account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}

// scanRow reads the shared projection into a domain account.
func scanRow(row pgx.Row) (*domain.Account, error) {
	var account domain.Account
	err := row.Scan(
		&account.ID, &account.StudentNo, &account.PasswordHash, &account.Nickname,
		&account.Wechat, &account.QQ, &account.CreatedAt, &account.UpdatedAt,
	)
	if pg.IsNoRows(err) {
		return nil, application.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &account, nil
}
