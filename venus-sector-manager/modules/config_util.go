package modules

import (
	"fmt"
	"reflect"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
)

func checkOptionalConfig(original, optional reflect.Type) {
	fieldNum := original.NumField()
	for i := 0; i < fieldNum; i++ {
		fori := original.Field(i)
		fopt := optional.Field(i)
		if fori.Name != fopt.Name {
			panic(fmt.Errorf("field name not match: %s != %s", fori.Name, fopt.Name))
		}

		if fopt.Type.Kind() != reflect.Ptr || fopt.Type.Elem() != fori.Type {
			panic(fmt.Errorf("field type not match: %s vs %s", fori.Type, fopt.Type))
		}
	}
}

var (
	_ toml.TextMarshaler   = Duration(0)
	_ toml.TextUnmarshaler = (*Duration)(nil)
	_ toml.TextMarshaler   = BigInt{}
	_ toml.TextUnmarshaler = (*BigInt)(nil)
)

type MustAddress address.Address

func (ma MustAddress) MarshalText() ([]byte, error) {
	addr := address.Address(ma)
	if addr == address.Undef {
		return nil, nil
	}

	return []byte(addr.String()), nil
}

func (ma *MustAddress) UnmarshalText(text []byte) error {
	addr, err := address.NewFromString(string(text))
	if err != nil {
		return err
	}

	if addr == address.Undef {
		return fmt.Errorf("address.Undef is not allowed")
	}

	*ma = MustAddress(addr)
	return nil
}

func (ma MustAddress) Std() address.Address {
	return address.Address(ma)
}

type Duration time.Duration

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

func (d *Duration) UnmarshalText(text []byte) error {
	td, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}

	*d = Duration(td)
	return nil
}

func (d Duration) Std() time.Duration {
	return time.Duration(d)
}

type BigInt big.Int

func (b BigInt) MarshalText() ([]byte, error) {
	if b.Int == nil {
		return nil, nil
	}

	return big.Int(b).MarshalText()
}

func (b *BigInt) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		if b != nil {
			b.Int = nil
		}

		return nil
	}

	bi, err := big.FromString(string(text))
	if err != nil {
		return err
	}

	*b = BigInt(bi)
	return nil
}

func (b BigInt) Std() big.Int {
	if b.Int == nil {
		return big.Int{}
	}

	return big.Int(b).Copy()
}
