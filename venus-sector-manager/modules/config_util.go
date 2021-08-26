package modules

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
)

func ActorID2ConfigKey(aid abi.ActorID) string {
	return strconv.FormatUint(uint64(aid), 10)
}

func ActorIDFromConfigKey(key string) (abi.ActorID, error) {
	num, err := strconv.ParseUint(key, 10, 64)
	if err != nil {
		return 0, err
	}

	return abi.ActorID(num), nil
}

func cloneConfig(dest, src, optional interface{}) error {
	var buf bytes.Buffer
	err := toml.NewEncoder(&buf).Encode(src)
	if err != nil {
		return fmt.Errorf("encode src: %w", err)
	}

	_, err = toml.Decode(string(buf.Bytes()), dest)
	if err != nil {
		return fmt.Errorf("decode src onto dest: %w", err)
	}

	if optional != nil {
		buf.Reset()
		err = toml.NewEncoder(&buf).Encode(optional)
		if err != nil {
			return fmt.Errorf("encode optional: %w", err)
		}

		_, err = toml.Decode(string(buf.Bytes()), dest)
		if err != nil {
			return fmt.Errorf("decode optional onto dest: %w", err)
		}
	}

	return nil
}

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
