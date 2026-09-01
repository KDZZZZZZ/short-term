package mapper

import (
	"errors"
	"strconv"
	"strings"
)

// Price conversion between the public decimal string and the internal minor
// units.
//
// openapi/components/schemas.yaml#/Price transports money as a decimal string
// precisely so no binary floating point is involved. The conversion here is
// integer arithmetic on the digits; parsing to float64 would reintroduce the
// rounding the contract exists to avoid.

// MaxPriceMinor is the largest value the Price pattern can express:
// 99999999.99 in minor units.
const MaxPriceMinor = 9999999999

// ErrPriceFormat reports a value that does not match the Price schema.
var ErrPriceFormat = errors.New("price must be a decimal amount with at most two fraction digits")

// ParsePrice converts a public price string into minor units.
func ParsePrice(value string) (int64, error) {
	if value == "" {
		return 0, ErrPriceFormat
	}

	whole, fraction, hasFraction := strings.Cut(value, ".")
	if !validDigits(whole) || len(whole) > 8 {
		return 0, ErrPriceFormat
	}
	// The pattern forbids leading zeros, so "007.00" and "00" are rejected.
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

// FormatPrice converts minor units into the public price string, always with
// two fraction digits.
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
