package slices

func Map[T, U any](data []T, f func(T) U) []U {
	res := make([]U, len(data))

	for i, e := range data {
		res[i] = f(e)
	}

	return res
}
