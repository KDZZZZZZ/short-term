package mapper

import (
	"errors"
	"strconv"
	"strings"
)

// Price 在公开十进制字符串和内部最小货币单位之间进行转换。
//
// openapi/components/schemas.yaml#/Price 精确地以十进制字符串传输金额，从而不涉及
// 二进制浮点数。这里对数字执行整数运算；解析为 float64 会重新引入契约要避免的
// 舍入误差。

// MaxPriceMinor 是 Price 模式可以表达的最大值：
// 以最小货币单位表示为 99999999.99。
const MaxPriceMinor = 9999999999

// ErrPriceFormat 表示值不符合 Price schema。
var ErrPriceFormat = errors.New("price must be a decimal amount with at most two fraction digits")

// ParsePrice 将公开价格字符串转换为最小货币单位。
func ParsePrice(value string) (int64, error) {
	if value == "" {
		return 0, ErrPriceFormat
	}

	whole, fraction, hasFraction := strings.Cut(value, ".")
	if !validDigits(whole) || len(whole) > 8 {
		return 0, ErrPriceFormat
	}
	// 模式禁止前导零，因此 "007.00" 和 "00" 会被拒绝。
	if len(whole) > 1 && whole[0] == '0' {
		return 0, ErrPriceFormat
	}
	if hasFraction && (!validDigits(fraction) || len(fraction) < 1 || len(fraction) > 2) {
		return 0, ErrPriceFormat
	}

	major, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, ErrPriceFormat
	}

	minor := int64(0)
	switch len(fraction) {
	case 0:
	case 1:
		minor = int64(fraction[0]-'0') * 10
	default:
		minor = int64(fraction[0]-'0')*10 + int64(fraction[1]-'0')
	}

	total := major*100 + minor
	if total > MaxPriceMinor {
		return 0, ErrPriceFormat
	}
	return total, nil
}

// FormatPrice 将最小货币单位转换为公开价格字符串，并始终保留两位小数。
func FormatPrice(minor int64) string {
	if minor < 0 {
		minor = 0
	}
	return strconv.FormatInt(minor/100, 10) + "." + twoDigits(minor%100)
}

func twoDigits(value int64) string {
	if value < 10 {
		return "0" + strconv.FormatInt(value, 10)
	}
	return strconv.FormatInt(value, 10)
}

func validDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := range len(value) {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
