// Package domain 保存账户实体及其不变量。
package domain

import (
	"errors"
	"regexp"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	studentNoPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,32}$`)
	qqPattern        = regexp.MustCompile(`^[0-9]{5,20}$`)
)

// 字段校验错误；应用层将它们映射为 API 错误码。
var (
	ErrStudentNoFormat = errors.New("student_no must be 4-32 characters of letters, digits, '_' or '-'")
	ErrPasswordLength  = errors.New("password must be 8-64 characters")
	ErrNicknameLength  = errors.New("nickname must be 1-50 characters")
	ErrWechatLength    = errors.New("wechat must be 1-64 characters")
	ErrQQFormat        = errors.New("qq must be 5-20 digits")
	ErrContactRemoval  = errors.New("a saved contact cannot be removed")

	ErrIDRequired           = errors.New("account id is required")
	ErrPasswordHashRequired = errors.New("password hash is required")
)

// Account 是 Account Service 的聚合根。
type Account struct {
	ID           string
	StudentNo    string
	PasswordHash string
	Nickname     string
	Wechat       *string
	QQ           *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ValidateStudentNo 强制执行 OpenAPI StudentNumber schema。
func ValidateStudentNo(v string) error {
	if !studentNoPattern.MatchString(v) {
		return ErrStudentNoFormat
	}
	return nil
}

// ValidatePassword 强制执行 OpenAPI PasswordInput schema。
func ValidatePassword(v string) error {
	length := utf8.RuneCountInString(v)
	if length < 8 || length > 64 {
		return ErrPasswordLength
	}
	return nil
}

// ValidateNickname 强制执行 OpenAPI Nickname schema。
func ValidateNickname(v string) error {
	length := utf8.RuneCountInString(v)
	if length < 1 || length > 50 {
		return ErrNicknameLength
	}
	return nil
}

// ValidateWechat 在字段存在时强制执行 OpenAPI Wechat schema。
func ValidateWechat(v *string) error {
	if v == nil {
		return nil
	}
	length := utf8.RuneCountInString(*v)
	if length < 1 || length > 64 {
		return ErrWechatLength
	}
	for _, r := range *v {
		if unicode.IsSpace(r) {
			return ErrWechatLength
		}
	}
	return nil
}

// ValidateQQ 在字段存在时强制执行 OpenAPI QQ schema。
func ValidateQQ(v *string) error {
	if v == nil {
		return nil
	}
	if !qqPattern.MatchString(*v) {
		return ErrQQFormat
	}
	return nil
}

// New 在检查所有字段不变量后构造账户。调用方提供标识和密码哈希：
// 生成标识和哈希密码属于适配器关注点，而不是领域规则。
func New(id, studentNo, passwordHash, nickname string, wechat, qq *string, now time.Time) (*Account, error) {
	if id == "" {
		return nil, ErrIDRequired
	}
	if passwordHash == "" {
		return nil, ErrPasswordHashRequired
	}
	if err := ValidateStudentNo(studentNo); err != nil {
		return nil, err
	}
	if err := ValidateNickname(nickname); err != nil {
		return nil, err
	}
	if err := ValidateWechat(wechat); err != nil {
		return nil, err
	}
	if err := ValidateQQ(qq); err != nil {
		return nil, err
	}

	return &Account{
		ID:           id,
		StudentNo:    studentNo,
		PasswordHash: passwordHash,
		Nickname:     nickname,
		Wechat:       wechat,
		QQ:           qq,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// Rename 修改显示昵称。
func (a *Account) Rename(nickname string, now time.Time) error {
	if err := ValidateNickname(nickname); err != nil {
		return err
	}
	a.Nickname = nickname
	a.UpdatedAt = now
	return nil
}

// SetWechat 新增或修改微信联系方式。已填写的联系方式不能删除。
func (a *Account) SetWechat(wechat *string, now time.Time) error {
	if wechat == nil {
		return ErrContactRemoval
	}
	if err := ValidateWechat(wechat); err != nil {
		return err
	}
	a.Wechat = wechat
	a.UpdatedAt = now
	return nil
}

// SetQQ 新增或修改 QQ 联系方式。已填写的联系方式不能删除。
func (a *Account) SetQQ(qq *string, now time.Time) error {
	if qq == nil {
		return ErrContactRemoval
	}
	if err := ValidateQQ(qq); err != nil {
		return err
	}
	a.QQ = qq
	a.UpdatedAt = now
	return nil
}

// SetPasswordHash 替换已存储的密码哈希。明文密码永远不会进入领域层。
func (a *Account) SetPasswordHash(hash string, now time.Time) error {
	if hash == "" {
		return ErrPasswordHashRequired
	}
	a.PasswordHash = hash
	a.UpdatedAt = now
	return nil
}

// HasContact 返回账户是否至少公开了一种联系方式。
// 发布商品要求至少有一种联系方式（OpenAPI createProduct）。
func (a *Account) HasContact() bool {
	return (a.Wechat != nil && *a.Wechat != "") || (a.QQ != nil && *a.QQ != "")
}
