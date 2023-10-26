package modules_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
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

func TestStructWithNilField(t *testing.T) {
	type A struct {
		B *int
		C *bool
		D *string
		E map[int]int
	}

	a := A{}

	buf := bytes.Buffer{}
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	err := enc.Encode(a)
	require.NoError(t, err)
	fmt.Println(string(buf.Bytes()))

}

func TestTomlUnmarshalForFIL(t *testing.T) {
	type A struct {
		F modules.FIL
	}
	t.Run("Marshal", func(t *testing.T) {

		a := A{
			F: modules.AttoFIL,
		}

		buf := bytes.Buffer{}
		enc := toml.NewEncoder(&buf)
		enc.Indent = ""
		err := enc.Encode(a)
		require.NoError(t, err)
		require.Equalf(t, "F = \"1 attoFIL\"\n", string(buf.Bytes()), "got: %s", string(buf.Bytes()))
	})

	t.Run("Unmarshal", func(t *testing.T) {
		a := A{}

		buf := bytes.Buffer{}
		buf.Write([]byte("F = '1 attoFIL' \n"))
		dec := toml.NewDecoder(&buf)
		_, err := dec.Decode(&a)
		require.NoError(t, err)
		require.Equal(t, modules.AttoFIL, a.F)
	})
}

func TestParseFIL(t *testing.T) {
	t.Run("valid FIL string", func(t *testing.T) {
		testCases := []struct {
			input  string
			output modules.FIL
		}{
			{
				"0",
				modules.OneFIL.Mul(0),
			},
			{
				"1",
				modules.OneFIL,
			},
			{
				"1 attoFIL",
				modules.AttoFIL,
			},
			{
				"1attoFIL",
				modules.AttoFIL,
			},
			{
				"1attofil",
				modules.AttoFIL,
			},
			{
				"1ATTOFIL",
				modules.AttoFIL,
			},

			{
				"10attoFIL",
				modules.AttoFIL.Mul(10),
			},
			{
				"1nanoFIL",
				modules.NanoFIL,
			},
			{
				"1 FIL",
				modules.OneFIL,
			},
			{
				"0.5 FIL",
				modules.OneFIL.Div(2),
			},
		}

		for _, tc := range testCases {
			output, err := modules.ParseFIL(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.output, output)
		}
	})

	t.Run("invalid FIL string", func(t *testing.T) {
		testCase := []string{"1.5attoFil"}
		for _, tc := range testCase {
			_, err := modules.ParseFIL(tc)
			require.Error(t, err)
		}
	})

	t.Run("zero", func(t *testing.T) {
		zero := modules.FIL(big.NewInt(0))
		var empty modules.FIL
		require.True(t, zero.IsZero())
		require.True(t, empty.IsZero())
		require.False(t, modules.OneFIL.IsZero())
	})
}
