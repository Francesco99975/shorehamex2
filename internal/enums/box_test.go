package enums

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBox_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		box  Box
		want string
	}{
		{Boxes.BELOW, "BELOW"},
		{Boxes.REPLACE, "REPLACE"},
		{Boxes.ABOVE, "ABOVE"},
		{Boxes.TOAST_TR, "TOAST_TR"},
		{Boxes.TOAST_TM, "TOAST_TM"},
		{Boxes.TOAST_TL, "TOAST_TL"},
		{Boxes.TOAST_BR, "TOAST_BR"},
		{Boxes.TOAST_BM, "TOAST_BM"},
		{Boxes.TOAST_BL, "TOAST_BL"},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(string(tt.box), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.box.String())
		})
	}
}

func TestGetBoxFromString_CaseInsensitive(t *testing.T) {
	t.Parallel()

	cases := map[string]Box{
		"below":    Boxes.BELOW,
		"BELOW":    Boxes.BELOW,
		"Below":    Boxes.BELOW,
		"bElOw":    Boxes.BELOW,
		"replace":  Boxes.REPLACE,
		"REPLACE":  Boxes.REPLACE,
		"toast_tr": Boxes.TOAST_TR,
		"TOAST_TR": Boxes.TOAST_TR,
		"Toast_Tr": Boxes.TOAST_TR,
		"TOAST_tm": Boxes.TOAST_TM,
		"toast_bl": Boxes.TOAST_BL,
	}

	for input, expected := range cases {
		input := input // capture
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got := GetBoxFromString(input)
			assert.Equal(t, expected, got)
			assert.Equal(t, expected.String(), got.String())
		})
	}
}

func TestGetBoxFromString_DefaultsToBELOW(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"", " ", "INVALID", "NULL", "123", "toast_xx", "ABOVE!", "below ",
	}

	for _, s := range invalid {
		s := s
		t.Run(fmt.Sprintf("invalid_%q", s), func(t *testing.T) {
			t.Parallel()
			got := GetBoxFromString(s)
			assert.Equal(t, Boxes.BELOW, got)
			assert.Equal(t, "BELOW", got.String())
		})
	}
}

func TestIsBoxValid(t *testing.T) {
	t.Parallel()

	valid := []string{
		"BELOW", "below", "Below",
		"REPLACE", "replace",
		"ABOVE", "above",
		"TOAST_TR", "toast_tr", "Toast_Tr",
		"TOAST_TM", "toast_tm",
		"TOAST_TL", "toast_tl",
		"TOAST_BR", "toast_br",
		"TOAST_BM", "toast_bm",
		"TOAST_BL", "toast_bl",
	}

	invalid := []string{
		"", " ", "INVALID", "TOAST", "TOAST_T", "BELOW!", "replace ",
	}

	for _, s := range valid {
		s := s
		t.Run("valid_"+s, func(t *testing.T) {
			t.Parallel()
			assert.True(t, IsBoxValid(s))
		})
	}

	for _, s := range invalid {
		s := s
		t.Run("invalid_"+s, func(t *testing.T) {
			t.Parallel()
			assert.False(t, IsBoxValid(s))
		})
	}
}

func TestBoxes_AllValuesAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[Box]struct{})
	values := []Box{
		Boxes.BELOW, Boxes.REPLACE, Boxes.ABOVE,
		Boxes.TOAST_TR, Boxes.TOAST_TM, Boxes.TOAST_TL,
		Boxes.TOAST_BR, Boxes.TOAST_BM, Boxes.TOAST_BL,
	}

	for _, v := range values {
		if _, exists := seen[v]; exists {
			t.Fatalf("duplicate enum value: %v", v)
		}
		seen[v] = struct{}{}
	}

	assert.Equal(t, 9, len(seen), "expected 9 unique box types")
}

func TestBoxes_CanBeUsedAsMapKeys(t *testing.T) {
	t.Parallel()

	m := make(map[Box]string)
	m[Boxes.TOAST_TR] = "top-right"
	m[Boxes.BELOW] = "bottom"

	assert.Equal(t, "top-right", m[Boxes.TOAST_TR])
	assert.Equal(t, "bottom", m[Boxes.BELOW])
}

func TestGetBoxFromString_NeverPanics(t *testing.T) {
	t.Parallel()

	for i := 0; i < 1000; i++ {
		input := fmt.Sprintf("stress-%d-%s", i, strings.Repeat("X", i%100))
		assert.NotPanics(t, func() {
			_ = GetBoxFromString(input)
		})
	}
}

// ——————— BENCHMARKS ———————
func BenchmarkGetBoxFromString_Valid(b *testing.B) {
	for b.Loop() {
		_ = GetBoxFromString("TOAST_TR")
	}
}

func BenchmarkGetBoxFromString_Invalid(b *testing.B) {
	for b.Loop() {
		_ = GetBoxFromString("INVALID")
	}
}

func BenchmarkIsBoxValid(b *testing.B) {
	for b.Loop() {
		_ = IsBoxValid("toast_bl")
	}
}
