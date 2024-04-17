package strings

import (
	"fmt"
	"strings"
)

func Join[T any](data []T, sep string) string {
	if len(data) < 1 {
		return ""
	}

	var sb strings.Builder

	_, _ = sb.WriteString(fmt.Sprint(data[0]))

	for _, item := range data[1:] {
		_, _ = sb.WriteString(sep)
		_, _ = sb.WriteString(fmt.Sprint(item))
	}

	return sb.String()
}
