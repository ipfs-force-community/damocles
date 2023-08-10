package slices

import "golang.org/x/exp/constraints"

func Map[T, U any](data []T, f func(T) U) []U {

	res := make([]U, len(data))

	for i, e := range data {
		res[i] = f(e)
	}

	return res
}

func Filter[T any](data []T, f func(T) bool) []T {

	res := make([]T, 0)

	for _, e := range data {
		if f(e) {
			res = append(res, e)
		}
	}
	return res
}

func Min[T constraints.Ordered](s []T) T {
	if len(s) == 0 {
		var zero T
		return zero
	}
	m := s[0]
	for _, v := range s[1:] {
		if m > v {
			m = v
		}
	}
	return m
}
