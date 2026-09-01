package mapper

import (
	"errors"
	"testing"
)

func TestParsePrice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{name: "zero", value: "0", want: 0},
		{name: "zero with fraction", value: "0.00", want: 0},
		{name: "whole amount", value: "20", want: 2000},
		{name: "two fraction digits", value: "20.00", want: 2000},
		{name: "one fraction digit", value: "20.5", want: 2050},
		{name: "cents", value: "0.01", want: 1},
		{name: "rounding-sensitive value", value: "0.07", want: 7},
		{name: "another rounding-sensitive value", value: "1234.56", want: 123456},
		{name: "contract maximum", value: "99999999.99", want: MaxPriceMinor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParsePrice(tt.value)
			if err != nil {
				t.Fatalf("ParsePrice(%q) error = %v", tt.value, err)
			}
			if got != tt.want {
				t.Fatalf("ParsePrice(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestParsePriceRejectsValuesOutsideTheContractPattern(t *testing.T) {
	t.Parallel()

	tests := []string{
		"", "-1", "1.234", ".5", "5.", "1e3", "abc", "1 000", "０", "+1",
		"007", "00", "100000000", "99999999.999", "１.００",
	}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			if _, err := ParsePrice(value); !errors.Is(err, ErrPriceFormat) {
				t.Fatalf("ParsePrice(%q) error = %v, want ErrPriceFormat", value, err)
			}
		})
	}
}

func TestFormatPriceAlwaysUsesTwoFractionDigits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		minor int64
		want  string
	}{
		{minor: 0, want: "0.00"},
		{minor: 1, want: "0.01"},
		{minor: 7, want: "0.07"},
		{minor: 50, want: "0.50"},
		{minor: 2000, want: "20.00"},
		{minor: 123456, want: "1234.56"},
		{minor: MaxPriceMinor, want: "99999999.99"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			if got := FormatPrice(tt.minor); got != tt.want {
				t.Fatalf("FormatPrice(%d) = %q, want %q", tt.minor, got, tt.want)
			}
		})
	}
}

func TestPriceRoundTripsWithoutLoss(t *testing.T) {
	t.Parallel()

	// Values chosen because binary floating point cannot represent them
	// exactly; a float-based conversion drifts on exactly these.
	for _, value := range []string{"0.07", "0.29", "1.15", "8.28", "1234.56", "99999999.99"} {
		minor, err := ParsePrice(value)
		if err != nil {
			t.Fatalf("ParsePrice(%q): %v", value, err)
		}
		if got := FormatPrice(minor); got != normalise(value) {
			t.Fatalf("round trip of %q produced %q", value, got)
		}
	}
}

// normalise renders the expected two-fraction-digit form of a test value.
func normalise(value string) string {
	minor, err := ParsePrice(value)
	if err != nil {
		return value
	}
	return FormatPrice(minor)
}
