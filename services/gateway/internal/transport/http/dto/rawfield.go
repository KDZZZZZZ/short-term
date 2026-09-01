package dto

import (
	"bytes"
	"encoding/json"
	"errors"
)

// RawField 区分 JSON 属性的三种状态：缺失、存在且有值、存在但为 null。
// encoding/json 无法用普通指针表达这三种状态，而契约中的 PATCH 正文依赖此能力。
type RawField struct {
	// Present 在请求正文中出现该属性时为 true。
	Present bool
	raw     json.RawMessage
}

// ErrFieldNotString 表示字段值既不是字符串也不是 null。
var ErrFieldNotString = errors.New("field must be a string or null")

// UnmarshalJSON 记录原始值，并标记字段已出现。
func (f *RawField) UnmarshalJSON(data []byte) error {
	f.Present = true
	f.raw = append(f.raw[:0], data...)
	return nil
}

// MarshalJSON 按字段接收时的形式渲染，使 RawField 可以往返转换。
func (f RawField) MarshalJSON() ([]byte, error) {
	if !f.Present || len(f.raw) == 0 {
		return []byte("null"), nil
	}
	return f.raw, nil
}

// IsNull 判断属性是否存在且明确为 null。
func (f RawField) IsNull() bool {
	return f.Present && bytes.Equal(bytes.TrimSpace(f.raw), []byte("null"))
}

// String 将存在且非 null 的字段解码为字符串。
func (f RawField) String() (string, error) {
	var value string
	if err := json.Unmarshal(f.raw, &value); err != nil {
		return "", ErrFieldNotString
	}
	return value, nil
}
