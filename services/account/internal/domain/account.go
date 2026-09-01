// Package domain holds the account entity and its invariants.
package domain

import (
	"errors"
	"regexp"
	"time"
	"unicode/utf8"
)

var (
	studentNoPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,32}$`)
	qqPattern        = regexp.MustCompile(`^[0-9]{5,20}$`)
)

// Field validation errors; the application layer maps them to API codes.
var (
	ErrStudentNoFormat = errors.New("student_no must be 4-32 characters of letters, digits, '_' or '-'")
	ErrPasswordLength  = errors.New("password must be 8-64 characters")
	ErrNicknameLength  = errors.New("nickname must be 1-50 characters")
	ErrWechatLength    = errors.New("wechat must be 1-64 characters")
	ErrQQFormat        = errors.New("qq must be 5-20 digits")

	ErrIDRequired           = errors.New("account id is required")
	ErrPasswordHashRequired = errors.New("password hash is required")
)

// Account is the aggregate root of the Account service.
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

// ValidateStudentNo enforces the OpenAPI StudentNumber schema.
func ValidateStudentNo(v string) error {
	if !studentNoPattern.MatchString(v) {
		return ErrStudentNoFormat
	}
	return nil
}

// ValidatePassword enforces the OpenAPI PasswordInput schema.
func ValidatePassword(v string) error {
	length := utf8.RuneCountInString(v)
	if length < 8 || length > 64 {
		return ErrPasswordLength
	}
	return nil
}

// ValidateNickname enforces the OpenAPI Nickname schema.
func ValidateNickname(v string) error {
	length := utf8.RuneCountInString(v)
	if length < 1 || length > 50 {
		return ErrNicknameLength
	}
	return nil
}

// ValidateWechat enforces the OpenAPI Wechat schema when present.
func ValidateWechat(v *string) error {
	if v == nil {
		return nil
	}
	length := utf8.RuneCountInString(*v)
	if length < 1 || length > 64 {
		return ErrWechatLength
	}
	return nil
}

// ValidateQQ enforces the OpenAPI QQ schema when present.
func ValidateQQ(v *string) error {
	if v == nil {
		return nil
	}
	if !qqPattern.MatchString(*v) {
		return ErrQQFormat
	}
	return nil
}

// New builds an account after checking every field invariant. The caller
// supplies the identifier and password hash: generating identifiers and
// hashing passwords are adapter concerns, not domain rules.
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

// Rename changes the display nickname.
func (a *Account) Rename(nickname string, now time.Time) error {
	if err := ValidateNickname(nickname); err != nil {
		return err
	}
	a.Nickname = nickname
	a.UpdatedAt = now
	return nil
}

// SetWechat sets or clears the WeChat contact. A nil value clears it.
func (a *Account) SetWechat(wechat *string, now time.Time) error {
	if err := ValidateWechat(wechat); err != nil {
		return err
	}
	a.Wechat = wechat
	a.UpdatedAt = now
	return nil
}

// SetQQ sets or clears the QQ contact. A nil value clears it.
func (a *Account) SetQQ(qq *string, now time.Time) error {
	if err := ValidateQQ(qq); err != nil {
		return err
	}
	a.QQ = qq
	a.UpdatedAt = now
	return nil
}

// SetPasswordHash replaces the stored password hash. The plaintext password
// never enters the domain.
func (a *Account) SetPasswordHash(hash string, now time.Time) error {
	if hash == "" {
		return ErrPasswordHashRequired
	}
	a.PasswordHash = hash
	a.UpdatedAt = now
	return nil
}

// HasContact reports whether the account published at least one contact
// method. Publishing a product requires one (OpenAPI createProduct).
func (a *Account) HasContact() bool {
	return (a.Wechat != nil && *a.Wechat != "") || (a.QQ != nil && *a.QQ != "")
}
