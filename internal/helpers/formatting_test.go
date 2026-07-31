// helpers_test.go
package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func TestCapitalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal string", "hello", "Hello"},
		{"already capitalized", "Hello", "Hello"},
		{"all caps", "HELLO", "HELLO"},
		{"single letter", "a", "A"},
		{"empty string", "", ""},
		{"non-ascii", "élixir", "Élixir"},
		{"mixed case", "goLang", "GoLang"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Capitalize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Test panic on empty string edge case more strictly
	t.Run("empty string does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() { Capitalize("") })
		assert.Equal(t, "", Capitalize(""))
	})
}

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		name        string
		price       float64
		curr        string
		expected    string
		expectError bool
	}{
		{"USD whole", 1234.56, "USD", "USD 1,234.56", false},
		{"EUR with comma", 999.99, "EUR", "EUR 999.99", false},
		{"JPY no decimals", 5000, "JPY", "JPY 5,000", false},
		{"GBP negative", -123.45, "GBP", "GBP -123.45", false},
		{"invalid currency", 100, "XYZ", "", true},
		{"zero value", 0.0, "USD", "USD 0", false},
		{"large number", 1_234_567.89, "USD", "USD 1,234,567.89", false},
	}

	p := message.NewPrinter(language.English)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatPrice(tt.price, tt.curr)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				// Use printer directly to verify expected output matches actual formatting
				cur, _ := currency.ParseISO(tt.curr)
				expected := p.Sprintf("%v", cur.Amount(tt.price))
				assert.Equal(t, expected, result)
			}
		})
	}
}

func TestParseNumberString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    int64
		expectError bool
	}{
		{"basic number", "100", 100, false},
		{"thousands K", "25K", 25000, false},
		{"millions M", "1.5M", 1_500_000, false},
		{"billions B", "3B", 3_000_000_000, false},
		{"trillions T", "2.1T", 2_100_000_000_000, false},
		{"lowercase k", "10k", 10000, false},
		{"with spaces", " 50 K ", 0, true}, // fails due to space
		{"decimal only", "0.5M", 500_000, false},
		{"no suffix", "123456", 123456, false},
		{"invalid suffix", "100X", 0, true},
		{"empty string", "", 0, true},
		{"float without suffix", "12.34", 12, false}, // truncates
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseNumberString(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Zero(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestNormalizeFloatStrToIntStr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"basic commas", "1,234.56", "123456"},
		{"percentage no decimal", "25%", "2500"},
		{"percentage with decimal", "12.5%", "1250"},
		{"dollar and parens", "$1,234.56", "123456"},
		{"negative in parens", "($1,234.56)", "123456"},
		{"spaces and commas", " 1 234 , 56 ", "123456"},
		{"multiple cleanup", "$ 1,234 . 56 (US)", "123456"},
		{"percentage only", "99.9%", "9990"},
		{"whole number percent", "100%", "10000"},
		{"no decimal percent", "5%", "500"},
		{"empty string", "", ""},
		{"just dollar", "$100", "100"},
		{"trailing percent no dot", "42%", "4200"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeFloatStrToIntStr(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark examples (optional but recommended)
func BenchmarkCapitalize(b *testing.B) {
	for b.Loop() {
		Capitalize("benchmark this string please")
	}
}

func BenchmarkParseNumberString(b *testing.B) {
	for b.Loop() {
		_, _ = ParseNumberString("1.234M")
	}
}

func BenchmarkNormalizeFloatStrToIntStr(b *testing.B) {
	for b.Loop() {
		NormalizeFloatStrToIntStr("$1,234.56 (US)")
	}
}
