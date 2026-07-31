package helpers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func Capitalize(s string) string {
	if len(s) == 0 {
		return s
	}

	// Convert string to []rune to handle multi-byte UTF-8 characters correctly
	runes := []rune(s)
	first := unicode.ToUpper(runes[0])
	return string(first) + string(runes[1:])
}

func FormatPrice(price float64, curr string) (string, error) {
	p := message.NewPrinter(language.English)
	cur, err := currency.ParseISO(curr)
	if err != nil {
		return "", fmt.Errorf("failed to parse currency: %w", err)
	}
	return p.Sprintf("%v", cur.Amount(price)), nil
}

func ParseNumberString(input string) (int64, error) {
	if len(input) == 0 {
		return 0, errors.New("empty string")
	}

	multipliers := map[string]float64{
		"K": 1_000,
		"k": 1_000,
		"M": 1_000_000,
		"B": 1_000_000_000,
		"T": 1_000_000_000_000,
	}

	lastChar := input[len(input)-1:]
	multiplier, hasSuffix := multipliers[lastChar]

	var numStr string
	if hasSuffix {
		numStr = input[:len(input)-1]
	} else {
		numStr = input
		multiplier = 1
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number format: %s", input)
	}

	result := int64(num * multiplier)
	return result, nil
}

func NormalizeFloatStrToIntStr(number string) string {
	number = strings.ReplaceAll(number, ",", "")
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "$", "")
	number = strings.ReplaceAll(number, "(", "")
	number = strings.ReplaceAll(number, ")", "")

	isPercent := strings.HasSuffix(number, "%")
	if isPercent {
		number = strings.TrimSuffix(number, "%")
	}

	if strings.Contains(number, ".") {
		parts := strings.SplitN(number, ".", 2)
		integer := parts[0]
		decimal := "00"
		if len(parts) > 1 {
			d := parts[1]
			d = strings.Map(func(r rune) rune {
				if unicode.IsDigit(r) {
					return r
				}
				return -1
			}, d)
			if len(d) > 2 {
				d = d[:2]
			}
			decimal = d
			for len(decimal) < 2 {
				decimal += "0"
			}
		}
		number = integer + decimal
	} else {
		number = strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return r
			}
			return -1
		}, number)
		if isPercent {
			number += "00"
		}
	}

	return number
}
