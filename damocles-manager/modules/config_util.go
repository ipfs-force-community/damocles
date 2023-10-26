package modules

import (
	"bytes"
	"encoding"
	"fmt"
	"math"
	mbig "math/big"
	"strconv"
	"strings"
	"time"

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

var (
	_ encoding.TextMarshaler   = Duration(0)
	_ encoding.TextUnmarshaler = (*Duration)(nil)
	_ encoding.TextMarshaler   = FIL{}
	_ encoding.TextUnmarshaler = (*FIL)(nil)
)

type MustAddress address.Address

func (ma MustAddress) MarshalText() ([]byte, error) {
	addr := address.Address(ma)
	return []byte(addr.String()), nil
}

func (ma *MustAddress) UnmarshalText(text []byte) error {
	addr, err := address.NewFromString(string(text))
	if err != nil {
		return err
	}
	*ma = MustAddress(addr)
	return nil
}

func (ma MustAddress) Std() address.Address {
	return address.Address(ma)
}

func (ma MustAddress) Valid() bool {
	return address.Address(ma) != address.Undef
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

// units should be desc ordered by precision
var filUnits = []struct {
	name      string
	short     string
	pretty    string
	precision int
}{
	{
		name:      "fil",
		short:     "",
		pretty:    "FIL",
		precision: 18,
	},
	{
		name:      "nanofil",
		short:     "nfil",
		pretty:    "nanoFIL",
		precision: 9,
	},
	{
		name:      "attofil",
		short:     "afil",
		pretty:    "attoFIL",
		precision: 0,
	},
}

type FIL big.Int

var (
	AttoFIL = FIL(big.NewInt(1))
	NanoFIL = AttoFIL.Mul(1_000_000_000)
	OneFIL  = NanoFIL.Mul(1_000_000_000)
	ZeroFIL = FIL(big.NewInt(0))
)

func (f FIL) Mul(num int64) FIL {
	return FIL(big.Mul(big.Int(f), big.NewInt(num)))
}

func (f FIL) Div(num int64) FIL {
	return FIL(big.Div(big.Int(f), big.NewInt(num)))
}

func (f FIL) Short() string {
	n := big.Int(f).Abs()

	var r *mbig.Rat
	var ui int
	for i, unit := range filUnits {
		p := big.NewInt(int64(math.Pow10(unit.precision)))
		if n.GreaterThanEqual(p) {
			r = new(mbig.Rat).SetFrac(f.Int, p.Int)
			ui = i
			break
		}
	}

	if r == nil || r.Sign() == 0 {
		return "0"
	}

	fl, _ := r.Float64()
	s := strconv.FormatFloat(fl, 'f', -1, 64)

	return fmt.Sprintf("%s %s", s, filUnits[ui].pretty)
}

func (f FIL) Std() big.Int {
	if f.Int == nil {
		return big.Int{}
	}

	return big.Int(f).Copy()
}

func (f FIL) IsZero() bool {
	return f.Int == nil || f.Std().Equals(big.Zero())
}

func ParseFIL(raw string) (FIL, error) {
	suffix := strings.TrimLeft(raw, "-.1234567890")
	s := raw[:len(raw)-len(suffix)]
	if len(s) > 50 {
		return FIL{}, fmt.Errorf("number string length too large: %d", len(s))
	}

	r, ok := new(mbig.Rat).SetString(s) //nolint:gosec
	if !ok {
		return FIL{}, fmt.Errorf("failed to parse %q as a decimal number", raw)
	}

	norm := strings.ToLower(strings.TrimSpace(suffix))
	for _, unit := range filUnits {
		if unit.name == norm || unit.short == norm {
			r = r.Mul(r, mbig.NewRat(int64(math.Pow10(unit.precision)), 1))
			if !r.IsInt() {
				return FIL{}, fmt.Errorf("invalid FIL string %q", raw)
			}

			return FIL(big.Int{Int: r.Num()}), nil
		}
	}

	return FIL{}, fmt.Errorf("invalid FIL unit %s", norm)
}

func (f FIL) MarshalText() ([]byte, error) {
	if f.Int == nil {
		// MarshalText returns nil will cause panic in the latest version of BurntSushi/toml
		// See: https://github.com/BurntSushi/toml/blob/fcbab7400715a3a2a7b2810317b7a17e7195e8fe/encode.go#L243-L245
		return []byte{}, nil
	}

	return []byte(f.Short()), nil
}

func (f *FIL) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		if f != nil {
			f.Int = nil
		}

		return nil
	}

	fil, err := ParseFIL(string(text))
	if err != nil {
		return fmt.Errorf("parse FIL: %w", err)
	}

	*f = fil
	return nil
}

func (f *FIL) UnmarshalJSON(text []byte) error {
	if bytes.Equal(text, []byte("null")) {
		return f.UnmarshalText([]byte{})
	}
	return f.UnmarshalText(text)
}
