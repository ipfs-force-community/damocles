package dep

import (
	"regexp"
	"strings"
)

// token:addr
var infoWithToken = regexp.MustCompile(`^[a-zA-Z0-9\-_]+?\.[a-zA-Z0-9\-_]+?\.([a-zA-Z0-9\-_]+)?:.+$`)

func extractAPIInfo(raw string, commonToken string) (addr string, token string) {
	if !infoWithToken.Match([]byte(raw)) {
		return raw, commonToken
	}

	sp := strings.SplitN(raw, ":", 2)
	if sp[0] == "" {
		return sp[1], commonToken
	}

	return sp[1], sp[0]
}
