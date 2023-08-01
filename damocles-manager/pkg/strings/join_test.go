package strings_test

import (
	"testing"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/strings"
	"github.com/stretchr/testify/require"
)

func TestJoin(t *testing.T) {
	require.Equal(t, "", strings.Join([]uint64{}, ","))
	require.Equal(t, "1,2,3", strings.Join([]uint64{1, 2, 3}, ","))
}
