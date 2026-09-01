// Package postgres 基于 Account Service 自有数据库实现仓储端口。
// 本包之外没有代码知道表结构，其他服务也不能连接此数据库。
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

// studentNoConstraint 是将同一学号的并发注册串行化的唯一索引。
const studentNoConstraint = "accounts_student_no_key"

// columns 是所有读取共享的投影，因此 scanRow 始终与其保持同步。
const columns = `id, student_no, password_hash, nickname, wechat, qq, created_at, updated_at`

// AccountRepository 将账户存储在 PostgreSQL 中。
type AccountRepository struct {
	pool *pgxpool.Pool
}

// NewAccountRepository 基于已打开的连接池构造仓储。
func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

var _ application.Repository = (*AccountRepository)(nil)

// Create 插入新账户。
//
// 它先插入并解释唯一约束冲突，而不是先检查学号是否存在再插入：
// 只有数据库约束才能让两个并发注册互斥。
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

// ByID 按不透明标识加载一个账户。
func (r *AccountRepository) ByID(ctx context.Context, id string) (*domain.Account, error) {
	const query = `SELECT ` + columns + ` FROM accounts WHERE id = $1`

	account, err := scanRow(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		return nil, fmt.Errorf("postgres: select account by id: %w", err)
	}
	return account, nil
}

// ByStudentNo 按学号加载一个账户。
func (r *AccountRepository) ByStudentNo(ctx context.Context, studentNo string) (*domain.Account, error) {
	const query = `SELECT ` + columns + ` FROM accounts WHERE student_no = $1`

	account, err := scanRow(r.pool.QueryRow(ctx, query, studentNo))
	if err != nil {
		return nil, fmt.Errorf("postgres: select account by student number: %w", err)
	}
	return account, nil
}

// ByIDs 在一次往返中加载 ids 中存在的所有账户。
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

// Update 回写账户的可变字段。
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

// scanRow 将共享投影读取为领域账户。
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
