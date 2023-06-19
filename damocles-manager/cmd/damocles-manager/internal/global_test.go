package internal_test

import (
	"strings"
	"testing"

	"github.com/ipfs-force-community/damocles/damocles-manager/cmd/damocles-manager/internal"
	"github.com/stretchr/testify/require"
)

func TestOutputJSONWithNil(t *testing.T) {

	var sb strings.Builder

	require.NoError(t, internal.OutputJSON(&sb, nil))
	require.Equal(t, "null", strings.TrimRight(sb.String(), "\n"))
}
