package ui

import (
	"strconv"
	"strings"
)

// jsStringArray serialises a Go []string into a JS array literal that can be
// dropped verbatim into an Alpine x-data expression.
//
//	jsStringArray([]string{"a","b"})  // => ['a','b']
//
// Single quotes are escaped so Alpine parses cleanly.
func jsStringArray(xs []string) string {
	parts := make([]string, 0, len(xs))
	for _, s := range xs {
		parts = append(parts, jsStr(s))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// jsStr wraps s in single quotes and escapes embedded single quotes / backslashes.
func jsStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}

// dropOptsToJS turns a []DropOption into a JS array literal of {v,l} objects,
// suitable for pasting into Alpine x-init.
func dropOptsToJS(opts []DropOption) string {
	parts := make([]string, 0, len(opts))
	for _, o := range opts {
		parts = append(parts, "{v:"+jsStr(o.Value)+",l:"+jsStr(o.Label)+"}")
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// commandsToJS turns a []Command into a JS array literal of {l,k} objects
// for the @CommandPalette component.
func commandsToJS(cs []Command) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, "{l:"+jsStr(c.Label)+",k:"+jsStr(c.Keys)+"}")
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// itoa is a terse alias used inside templ expressions to stamp ints into
// Alpine x-data blobs, style values, aria counts, etc.
func itoa(n int) string { return strconv.Itoa(n) }

// ftoa returns a float formatted without a trailing zero-fraction.
func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// nonEmpty returns fallback if s is the zero string.
func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// nonZero returns fallback if n is 0.
func nonZero(n, fallback int) int {
	if n == 0 {
		return fallback
	}
	return n
}

func floatsToJS(xs []float64) string {
	parts := make([]string, 0, len(xs))
	for _, f := range xs {
		parts = append(parts, ftoa(f))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// firstRune returns the upper-cased first rune of s, or fallback if s is empty.
// Used by HeaderBrand to derive a one-letter logo glyph from the app title.
func firstRune(s string, fallback rune) rune {
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return r - 32
		} else {
			return r
		}
	}
	return fallback
}

// repeat returns s concatenated n times. Used for input placeholders such as
// the OTPSingle "000000" dots that scale with Length.
func repeat(s string, n int) string { return strings.Repeat(s, n) }

// paginationPages computes the numbered-button sequence for @Pagination given
// the current page, total page count, and the requested sliding window size.
//
// The returned slice contains 1-based page numbers, with 0 representing an
// ellipsis placeholder. First and last page are always included so users can
// jump to the ends in a single click; gaps between them and the window are
// collapsed into a single "…" entry.
//
// Examples (window=5):
//
//	current=1, total=3      -> [1 2 3]
//	current=1, total=10     -> [1 2 3 4 5 0 10]
//	current=6, total=10     -> [1 0 4 5 6 7 8 0 10]
//	current=10, total=10    -> [1 0 6 7 8 9 10]
func paginationPages(current, total, window int) []int {
	if window <= 0 {
		window = 5
	}
	if total <= 0 {
		return nil
	}
	if total <= window+2 {
		out := make([]int, total)
		for i := range out {
			out[i] = i + 1
		}
		return out
	}

	half := window / 2
	start := current - half
	end := current + half
	if window%2 == 0 {
		end--
	}
	if start < 2 {
		end += 2 - start
		start = 2
	}
	if end > total-1 {
		start -= end - (total - 1)
		end = total - 1
	}
	if start < 2 {
		start = 2
	}

	out := []int{1}
	if start > 2 {
		out = append(out, 0)
	}
	for i := start; i <= end; i++ {
		out = append(out, i)
	}
	if end < total-1 {
		out = append(out, 0)
	}
	out = append(out, total)
	return out
}
