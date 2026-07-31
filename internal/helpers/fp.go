package helpers

import "sort"

func MapSlice[T any, U any](slice []T, f func(T) U) []U {
	result := make([]U, len(slice))
	for i, v := range slice {
		result[i] = f(v)
	}
	return result
}

func FoldSlice[T any, R any](slice []T, f func(T, R) R, initial R) R {
	result := initial
	for _, v := range slice {
		result = f(v, result)
	}
	return result
}

func FilteredSlice[T any](arr []T, test func(T) bool) []T {
	var result []T

	for _, item := range arr {
		if test(item) {
			result = append(result, item)
		}
	}

	return result
}

func SortSlice[T any](arr []T, less func(a, b T) bool) {
	sort.Slice(arr, func(i, j int) bool {
		return less(arr[i], arr[j])
	})
}
