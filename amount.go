// Copyright (c) 2013, 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import (
	"errors"
	"math"
	"strconv"
)

// AmountUnit describes a method of converting an Amount to something
// other than the base unit of a bitcoin.  The value of the AmountUnit
// is the exponent component of the decadic multiple to convert from
// an amount in bitcoin to an amount counted in units.
type AmountUnit int

// These constants define the various standard units used when describing
// a bitcoin monetary amount.
const (
	AmountMegaBitcoin  AmountUnit = 6
	AmountKiloBitcoin  AmountUnit = 3
	AmountBitcoin      AmountUnit = 0
	AmountMilliBitcoin AmountUnit = -3
	AmountMicroBitcoin AmountUnit = -6
	AmountBaseBitcoin  AmountUnit = -8
)

// String returns the unit as a string.  For recognized units, the SI
// prefix is used, or "Satoshi" for the base unit.  For all unrecognized
// units, "1eN BTC" is returned, where N is the AmountUnit.
func (u AmountUnit) String() string {
	switch u {
	case AmountMegaBitcoin:
		return "MBTC"
	case AmountKiloBitcoin:
		return "kBTC"
	case AmountBitcoin:
		return "BTC"
	case AmountMilliBitcoin:
		return "mBTC"
	case AmountMicroBitcoin:
		return "μBTC"
	case AmountBaseBitcoin:
		return "Satoshi"
	default:
		return "1e" + strconv.FormatInt(int64(u), 10) + " BTC"
	}
}

// Amount represents the base bitcoin monetary unit (colloquially referred
// to as a `Satoshi').  A single Amount is equal to 1e-8 of a bitcoin.
type Amount int64

// NewAmount creates an Amount from a floating point value representing
// some value in bitcoin.
func NewAmount(f float64) (Amount, error) {
	a := f * float64(SatoshiPerBitcoin)

	// The amount is only valid if it does not exceed the total amount
	// of bitcoin producable, and is not a floating point number that
	// would otherwise fail that check such as NaN or +-Inf.
	switch abs := math.Abs(a); {
	case abs > float64(MaxSatoshi):
		fallthrough
	case math.IsNaN(abs) || math.IsInf(abs, 1):
		return 0, errors.New("invalid bitcoin amount")
	}

	// Depending on the sign, add or subtract 0.5 and rely on integer
	// truncation to correctly round the value up or down.
	if a < 0 {
		a = a - 0.5
	} else {
		a = a + 0.5
	}
	return Amount(a), nil
}

// ToUnit converts a monetary amount counted in bitcoin base units to a
// floating point value representing an amount of bitcoin.
func (a Amount) ToUnit(u AmountUnit) float64 {
	return float64(a) / math.Pow10(int(u+8))
}

// Format formats a monetary amount counted in bitcoin base units as a
// string for a given unit.  The conversion will succeed for any unit,
// however, known units will be formated with an appended label describing
// the units with SI notation.
func (a Amount) Format(u AmountUnit) string {
	units := " " + u.String()
	return strconv.FormatFloat(a.ToUnit(u), 'f', -int(u+8), 64) + units
}
