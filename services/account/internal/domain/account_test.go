package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateStudentNo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr error
	}{
		{name: "minimum length", value: "A_1-", wantErr: nil},
		{name: "maximum length", value: strings.Repeat("a", 32), wantErr: nil},
		{name: "too short", value: "123", wantErr: ErrStudentNoFormat},
		{name: "too long", value: strings.Repeat("a", 33), wantErr: ErrStudentNoFormat},
		{name: "space is rejected", value: "2026 001", wantErr: ErrStudentNoFormat},
		{name: "non ascii is rejected", value: "学号2026", wantErr: ErrStudentNoFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := ValidateStudentNo(tt.value); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateStudentNo(%q) error = %v, want %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr error
	}{
		{name: "minimum ascii length", value: "12345678", wantErr: nil},
		{name: "unicode counts code points", value: "一二三四五六七八", wantErr: nil},
		{name: "too short ascii", value: "1234567", wantErr: ErrPasswordLength},
		{name: "too short unicode", value: "一二三四五六七", wantErr: ErrPasswordLength},
		{name: "maximum length", value: strings.Repeat("p", 64), wantErr: nil},
		{name: "too long", value: strings.Repeat("p", 65), wantErr: ErrPasswordLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := ValidatePassword(tt.value); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidatePassword(%q) error = %v, want %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidateNickname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr error
	}{
		{name: "single unicode character", value: "明", wantErr: nil},
		{name: "space follows public contract", value: " ", wantErr: nil},
		{name: "maximum unicode length", value: strings.Repeat("名", 50), wantErr: nil},
		{name: "empty", value: "", wantErr: ErrNicknameLength},
		{name: "too long unicode", value: strings.Repeat("名", 51), wantErr: ErrNicknameLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := ValidateNickname(tt.value); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateNickname(%q) error = %v, want %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWechat(t *testing.T) {
	t.Parallel()

	empty := ""
	minimum := "微"
	maximum := strings.Repeat("微", 64)
	tooLong := strings.Repeat("微", 65)
	withSpace := "wx xiaoming"
	withUnicodeSpace := "微信\u3000号"
	tests := []struct {
		name    string
		value   *string
		wantErr error
	}{
		{name: "unset", value: nil, wantErr: nil},
		{name: "minimum unicode length", value: &minimum, wantErr: nil},
		{name: "maximum unicode length", value: &maximum, wantErr: nil},
		{name: "empty", value: &empty, wantErr: ErrWechatLength},
		{name: "too long unicode", value: &tooLong, wantErr: ErrWechatLength},
		{name: "ascii whitespace", value: &withSpace, wantErr: ErrWechatLength},
		{name: "unicode whitespace", value: &withUnicodeSpace, wantErr: ErrWechatLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := ValidateWechat(tt.value); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateWechat(%v) error = %v, want %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestSavedContactsCannotBeRemoved(t *testing.T) {
	t.Parallel()

	wechat := "wx_xiaoming"
	qq := "123456789"
	account, err := New("u_1", "20260001", "hash", "小明", &wechat, &qq, time.Now())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := account.SetWechat(nil, time.Now()); !errors.Is(err, ErrContactRemoval) {
		t.Fatalf("SetWechat(nil) = %v, want ErrContactRemoval", err)
	}
	if err := account.SetQQ(nil, time.Now()); !errors.Is(err, ErrContactRemoval) {
		t.Fatalf("SetQQ(nil) = %v, want ErrContactRemoval", err)
	}
	if account.Wechat == nil || *account.Wechat != wechat || account.QQ == nil || *account.QQ != qq {
		t.Fatal("a rejected removal changed the saved contacts")
	}
}

func TestValidateQQ(t *testing.T) {
	t.Parallel()

	minimum := "12345"
	maximum := strings.Repeat("9", 20)
	tooShort := "1234"
	tooLong := strings.Repeat("9", 21)
	nonDigit := "1234a"
	tests := []struct {
		name    string
		value   *string
		wantErr error
	}{
		{name: "unset", value: nil, wantErr: nil},
		{name: "minimum length", value: &minimum, wantErr: nil},
		{name: "maximum length", value: &maximum, wantErr: nil},
		{name: "too short", value: &tooShort, wantErr: ErrQQFormat},
		{name: "too long", value: &tooLong, wantErr: ErrQQFormat},
		{name: "non digit", value: &nonDigit, wantErr: ErrQQFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := ValidateQQ(tt.value); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateQQ(%v) error = %v, want %v", tt.value, err, tt.wantErr)
			}
		})
	}
}
