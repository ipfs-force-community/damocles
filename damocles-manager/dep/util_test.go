package dep

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractAPIInfo(t *testing.T) {
	cases := []struct {
		raw    string
		common string

		api   string
		token string
	}{
		{
			raw:    "/ip4/127.0.0.1/tcp/1789",
			common: "abcde",
			api:    "/ip4/127.0.0.1/tcp/1789",
			token:  "abcde",
		},

		{
			raw:    "x.y.z:/ip4/127.0.0.1/tcp/1789",
			common: "abcde",
			api:    "/ip4/127.0.0.1/tcp/1789",
			token:  "x.y.z",
		},

		{
			raw:    "x.y.z:/ip4/127.0.0.1/tcp/1789",
			common: "",
			api:    "/ip4/127.0.0.1/tcp/1789",
			token:  "x.y.z",
		},

		{
			raw:    ":/ip4/127.0.0.1/tcp/1789",
			common: "abcde",
			api:    ":/ip4/127.0.0.1/tcp/1789",
			token:  "abcde",
		},

		{
			raw:    "x.y:/ip4/127.0.0.1/tcp/1789",
			common: "abcde",
			api:    "x.y:/ip4/127.0.0.1/tcp/1789",
			token:  "abcde",
		},

		{
			raw:    "x:/ip4/127.0.0.1/tcp/1789",
			common: "abcde",
			api:    "x:/ip4/127.0.0.1/tcp/1789",
			token:  "abcde",
		},

		{
			raw:    "..:/ip4/127.0.0.1/tcp/1789",
			common: "abcde",
			api:    "..:/ip4/127.0.0.1/tcp/1789",
			token:  "abcde",
		},

		{
			raw:    "xyz:/ip4/127.0.0.1/tcp/1789",
			common: "abcde",
			api:    "xyz:/ip4/127.0.0.1/tcp/1789",
			token:  "abcde",
		},
	}

	for ci := range cases {
		c := cases[ci]

		api, token := extractAPIInfo(c.raw, c.common)
		require.Equalf(t, c.api, api, "api extracted from %s, %s", c.raw, c.common)
		require.Equalf(t, c.token, token, "token extracted from %s, %s", c.raw, c.common)
	}
}
