// Package application 保存 Account Service 用例。它接收不包含传输或数据库细节的
// 命令和查询，应用领域规则，并返回供适配器映射的领域对象。
package application

import (
	"context"
	"errors"
	"time"

	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

// Repository 错误。它们描述存储结果，而不是 HTTP 或 gRPC 结果；
// 服务会将它们转换为契约错误码。
var (
	// ErrNotFound 表示没有匹配的账户。
	ErrNotFound = errors.New("account not found")
	// ErrStudentNoTaken 表示学号已经注册。
	ErrStudentNoTaken = errors.New("student number already registered")
)

// Repository 存储账户。实现必须将 student_no 的唯一约束冲突映射为
// ErrStudentNoTaken，而不是先查询再插入，从而避免两个并发注册同时成功。
type Repository interface {
	Create(ctx context.Context, account *domain.Account) error
	ByID(ctx context.Context, id string) (*domain.Account, error)
	ByStudentNo(ctx context.Context, studentNo string) (*domain.Account, error)
	// ByIDs 返回存在的账户，不保证顺序。不存在的标识直接从结果中省略。
	ByIDs(ctx context.Context, ids []string) ([]*domain.Account, error)
	Update(ctx context.Context, account *domain.Account) error
}

// PasswordHasher 派生并校验密码哈希。
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) error
	NeedsRehash(encoded string) bool
}

// TokenIssuer 为通过认证的账户签发访问令牌。
type TokenIssuer interface {
	Issue(subject string) (token string, expiresAt time.Time, err error)
}

// IDGenerator 生成不透明账户标识。
type IDGenerator interface {
	New() string
}

// Clock 读取当前时间。注入该接口可以让测试中的时间戳保持确定。
type Clock interface {
	Now() time.Time
}
