package application

import (
	"time"

	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

// RegisterCommand 创建账户并让新用户登录。
type RegisterCommand struct {
	StudentNo string
	Password  string
	// Nickname 可选。缺失时服务分配不泄露学号的中性默认昵称。
	Nickname *string
	Wechat   *string
	QQ       *string
}

// LoginCommand 验证学号和密码组合。
type LoginCommand struct {
	StudentNo string
	Password  string
}

// StringPatch 表示 PATCH 字段的三种线路状态：缺失、设置值或显式 null。
// 显式 null 会被应用层拒绝，以执行“联系方式不可删除”的公开契约。
type StringPatch struct {
	// Present 在客户端省略字段时为 false。
	Present bool
	// Value 在客户端发送 null 时为 nil。
	Value *string
}

// Keep 返回未设置的 patch，表示“不改变字段”。
func Keep() StringPatch { return StringPatch{} }

// Set 返回将字段设置为 value 的 patch。
func Set(value string) StringPatch { return StringPatch{Present: true, Value: &value} }

// Clear 返回显式 null patch；它只用于验证服务端会拒绝旧客户端的删除请求。
func Clear() StringPatch { return StringPatch{Present: true} }

// UpdateProfileCommand 修改调用方自己的资料。
type UpdateProfileCommand struct {
	ActorID  string
	Nickname *string
	Wechat   StringPatch
	QQ       StringPatch
}

// ChangePasswordCommand 替换调用方自己的密码。
type ChangePasswordCommand struct {
	ActorID     string
	OldPassword string
	NewPassword string
}

// AuthResult 是注册或登录成功后的结果。
type AuthResult struct {
	AccessToken string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	Account     *domain.Account
}

// ExpiresIn 返回以整秒计的令牌有效期，这正是公开 AuthData schema 携带的值。
// 该值根据签发时的时钟计算，而不是稍后读取，因此缓慢响应不会报告令牌并不具备的
// 有效期。
func (r AuthResult) ExpiresIn() int64 {
	remaining := r.ExpiresAt.Sub(r.IssuedAt)
	if remaining <= 0 {
		return 0
	}
	return int64(remaining.Round(time.Second) / time.Second)
}
