package modules_test

import (
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/testutil"
	"github.com/stretchr/testify/require"
)

func TestMustAddressMarshalText(t *testing.T) {

	testCases := []struct {
		addr     address.Address
		expected string
	}{
		{
			testutil.Must(address.NewFromString("t3vvmn62lofvhjd2ugzca6sof2j2ubwok6cj4xxbfzz4yuxfkgobpihhd2thlanmsh3w2ptld2gqkn2jvlss4a")),
			"t3vvmn62lofvhjd2ugzca6sof2j2ubwok6cj4xxbfzz4yuxfkgobpihhd2thlanmsh3w2ptld2gqkn2jvlss4a",
		},
		{
			address.Undef,
			address.UndefAddressString,
		},
	}

	for _, tc := range testCases {
		actual, err := modules.MustAddress(tc.addr).MarshalText()
		require.NoError(t, err)
		require.Equal(t, tc.expected, string(actual))
	}
}

func TestMustAddressUnmarshalText(t *testing.T) {

	testCases := []struct {
		addr     string
		expected address.Address
	}{
		{
			"t3vvmn62lofvhjd2ugzca6sof2j2ubwok6cj4xxbfzz4yuxfkgobpihhd2thlanmsh3w2ptld2gqkn2jvlss4a",
			testutil.Must(address.NewFromString("t3vvmn62lofvhjd2ugzca6sof2j2ubwok6cj4xxbfzz4yuxfkgobpihhd2thlanmsh3w2ptld2gqkn2jvlss4a")),
		},
		{
			address.UndefAddressString,
			address.Undef,
		},
	}

	for _, tc := range testCases {
		var actual modules.MustAddress
		require.NoError(t, actual.UnmarshalText([]byte(tc.addr)))
		require.Equal(t, tc.expected, actual.Std())
	}
}
