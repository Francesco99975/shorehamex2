// helpers_test.go
package helpers

import (
	"strconv"
	"testing"
)

func TestMapSlice(t *testing.T) {
	t.Run("int to string", func(t *testing.T) {
		input := []int{1, 2, 3}
		result := MapSlice(input, func(i int) string { return strconv.Itoa(i * 10) })
		expected := []string{"10", "20", "30"}
		assertEqualSlice(t, result, expected)
	})

	t.Run("square", func(t *testing.T) {
		input := []int{1, 2, 3}
		result := MapSlice(input, func(i int) int { return i * i })
		expected := []int{1, 4, 9}
		assertEqualSlice(t, result, expected)
	})

	t.Run("empty", func(t *testing.T) {
		result := MapSlice([]int{}, func(i int) string { return "x" })
		if len(result) != 0 {
			t.Errorf("expected empty, got %v", result)
		}
	})
}

func TestFoldSlice(t *testing.T) {
	t.Run("sum", func(t *testing.T) {
		input := []int{1, 2, 3, 4}
		result := FoldSlice(input, func(a, acc int) int { return a + acc }, 0)
		if result != 10 {
			t.Errorf("got %d, want 10", result)
		}
	})

	t.Run("product", func(t *testing.T) {
		input := []int{2, 3, 4}
		result := FoldSlice(input, func(a, acc int) int { return a * acc }, 1)
		if result != 24 {
			t.Errorf("got %d, want 24", result)
		}
	})

	t.Run("concat strings", func(t *testing.T) {
		input := []string{"a", "b", "c"}
		result := FoldSlice(input, func(s, acc string) string { return acc + s }, "")
		if result != "abc" {
			t.Errorf("got %q, want %q", result, "abc")
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		var input []int
		result := FoldSlice(input, func(a, acc int) int { return a + acc }, 42)
		if result != 42 {
			t.Errorf("got %d, want 42", result)
		}
	})
}

func TestFilteredSlice(t *testing.T) {
	input := []int{1, 2, 3, 4, 5, 6}
	result := FilteredSlice(input, func(i int) bool { return i%2 == 0 })
	expected := []int{2, 4, 6}
	assertEqualSlice(t, result, expected)
}

func TestSortSlice(t *testing.T) {
	t.Run("ints ascending", func(t *testing.T) {
		data := []int{3, 1, 4, 1, 5}
		SortSlice(data, func(a, b int) bool { return a < b })
		expected := []int{1, 1, 3, 4, 5}
		assertEqualSlice(t, data, expected)
	})

	t.Run("strings by length", func(t *testing.T) {
		data := []string{"apple", "hi", "banana", "a"}
		SortSlice(data, func(a, b string) bool { return len(a) < len(b) })
		expected := []string{"a", "hi", "apple", "banana"}
		assertEqualSlice(t, data, expected)
	})
}

// Generic helper to compare slices
func assertEqualSlice[T comparable](t *testing.T, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("len: got %d, want %d", len(got), len(want))
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("at [%d]: got %v, want %v", i, got[i], want[i])
		}
	}
}

// ——————————————————— BENCHMARKS ———————————————————

func benchmarkMapSlice(b *testing.B, size int) {
	data := make([]int, size)
	for i := range data {
		data[i] = i
	}

	for b.Loop() {
		_ = MapSlice(data, func(x int) int { return x * 2 })
	}
}

func BenchmarkMapSlice_10(b *testing.B)   { benchmarkMapSlice(b, 10) }
func BenchmarkMapSlice_100(b *testing.B)  { benchmarkMapSlice(b, 100) }
func BenchmarkMapSlice_1K(b *testing.B)   { benchmarkMapSlice(b, 1_000) }
func BenchmarkMapSlice_10K(b *testing.B)  { benchmarkMapSlice(b, 10_000) }
func BenchmarkMapSlice_100K(b *testing.B) { benchmarkMapSlice(b, 100_000) }

func benchmarkFoldSlice(b *testing.B, size int) {
	data := make([]int, size)
	for i := range data {
		data[i] = 1
	}

	for b.Loop() {
		_ = FoldSlice(data, func(a, acc int) int { return a + acc }, 0)
	}
}

func BenchmarkFoldSlice_10(b *testing.B)   { benchmarkFoldSlice(b, 10) }
func BenchmarkFoldSlice_100(b *testing.B)  { benchmarkFoldSlice(b, 100) }
func BenchmarkFoldSlice_1K(b *testing.B)   { benchmarkFoldSlice(b, 1_000) }
func BenchmarkFoldSlice_10K(b *testing.B)  { benchmarkFoldSlice(b, 10_000) }
func BenchmarkFoldSlice_100K(b *testing.B) { benchmarkFoldSlice(b, 100_000) }

func benchmarkFilteredSlice(b *testing.B, size int) {
	data := make([]int, size)
	for i := range data {
		data[i] = i
	}

	for b.Loop() {
		_ = FilteredSlice(data, func(x int) bool { return x%2 == 0 })
	}
}

func BenchmarkFilteredSlice_10(b *testing.B)   { benchmarkFilteredSlice(b, 10) }
func BenchmarkFilteredSlice_100(b *testing.B)  { benchmarkFilteredSlice(b, 100) }
func BenchmarkFilteredSlice_1K(b *testing.B)   { benchmarkFilteredSlice(b, 1_000) }
func BenchmarkFilteredSlice_10K(b *testing.B)  { benchmarkFilteredSlice(b, 10_000) }
func BenchmarkFilteredSlice_100K(b *testing.B) { benchmarkFilteredSlice(b, 100_000) }

func benchmarkSortSlice(b *testing.B, size int) {
	for b.Loop() {
		b.StopTimer()
		data := make([]int, size)
		for j := range data {
			data[j] = size - j // worst-case reverse order
		}
		b.StartTimer()
		SortSlice(data, func(a, b int) bool { return a < b })
	}
}

func BenchmarkSortSlice_10(b *testing.B)   { benchmarkSortSlice(b, 10) }
func BenchmarkSortSlice_100(b *testing.B)  { benchmarkSortSlice(b, 100) }
func BenchmarkSortSlice_1K(b *testing.B)   { benchmarkSortSlice(b, 1_000) }
func BenchmarkSortSlice_10K(b *testing.B)  { benchmarkSortSlice(b, 10_000) }
func BenchmarkSortSlice_100K(b *testing.B) { benchmarkSortSlice(b, 100_000) }
