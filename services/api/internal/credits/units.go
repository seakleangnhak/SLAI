package credits

import (
	"errors"
	"math/big"
	"strings"
)

const Scale int64 = 1_000_000

var ErrInvalidDecimal = errors.New("invalid credit decimal")

func FromWhole(value int64) (int64, error) {
	if value > 0 && value > maxInt64()/Scale {
		return 0, ErrInvalidDecimal
	}
	if value < 0 && value < minInt64()/Scale {
		return 0, ErrInvalidDecimal
	}
	return value * Scale, nil
}

func FromDecimalString(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, ErrInvalidDecimal
	}
	rat, ok := new(big.Rat).SetString(trimmed)
	if !ok {
		return 0, ErrInvalidDecimal
	}
	return FromDecimalRat(rat)
}

func FromDecimalRat(value *big.Rat) (int64, error) {
	if value == nil {
		return 0, ErrInvalidDecimal
	}
	if value.Sign() <= 0 {
		return 0, nil
	}

	scaled := new(big.Rat).Mul(value, big.NewRat(Scale, 1))
	numerator := new(big.Int).Set(scaled.Num())
	denominator := scaled.Denom()
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, ErrInvalidDecimal
	}
	return quotient.Int64(), nil
}

func maxInt64() int64 { return int64(^uint64(0) >> 1) }

func minInt64() int64 { return -maxInt64() - 1 }
