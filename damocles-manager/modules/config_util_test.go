package modules_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
			testutil.Must(
				address.NewFromString(
					"t3vvmn62lofvhjd2ugzca6sof2j2ubwok6cj4xxbfzz4yuxfkgobpihhd2thlanmsh3w2ptld2gqkn2jvlss4a",
				),
			),
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
			testutil.Must(
				address.NewFromString(
					"t3vvmn62lofvhjd2ugzca6sof2j2ubwok6cj4xxbfzz4yuxfkgobpihhd2thlanmsh3w2ptld2gqkn2jvlss4a",
				),
			),
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
	fmt.Println(buf.String())
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
		require.Equalf(t, "F = \"1 attoFIL\"\n", buf.String(), "got: %s", buf.String())
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

func TestScanPersistStores(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "1"), 0755))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "2"), 0755))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "3"), 0755))

	jsonConf1 := `{
		"ID": "123",
		"Name": "",
		"Strict": false,
		"ReadOnly": false,
		"Weight": 0,
		"AllowMiners": [1],
		"DenyMiners": [2],
		"PluginName": "2234234",
		"Meta": {},
		"CanSeal": true
	}`
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpDir, "1", modules.FilenameSectorStoreJSON), []byte(jsonConf1), 0644),
	)

	jsonConf2 := `{
		"Name": "456",
		"Meta": {},
		"Strict": true,
		"ReadOnly": false,
		"AllowMiners": [1],
		"DenyMiners": [2],
		"PluginName": "2234234",
		"CanSeal": true
	}`
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpDir, "2", modules.FilenameSectorStoreJSON), []byte(jsonConf2), 0644),
	)

	cfgs, err := modules.ScanPersistStores([]string{filepath.Join(tmpDir, "*")})
	require.NoError(t, err)
	require.Len(t, cfgs, 2)

	require.Equal(t, "123", cfgs[0].Name)
	require.Equal(t, "456", cfgs[1].Name)
}
