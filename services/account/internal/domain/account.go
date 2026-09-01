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
