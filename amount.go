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

// These constants define various units used when describing a bitcoin
// monetary amount.
const (
	AmountMegaBTC  AmountUnit = 6
	AmountKiloBTC  AmountUnit = 3
	AmountBTC      AmountUnit = 0
	AmountMilliBTC AmountUnit = -3
	AmountMicroBTC AmountUnit = -6
	AmountSatoshi  AmountUnit = -8
)

// String returns the unit as a string.  For recognized units, the SI
// prefix is used, or "Satoshi" for the base unit.  For all unrecognized
// units, "1eN BTC" is returned, where N is the AmountUnit.
func (u AmountUnit) String() string {
	switch u {
	case AmountMegaBTC:
		return "MBTC"
	case AmountKiloBTC:
		return "kBTC"
	case AmountBTC:
		return "BTC"
	case AmountMilliBTC:
		return "mBTC"
	case AmountMicroBTC:
		return "μBTC"
	case AmountSatoshi:
		return "Satoshi"
	default:
		return "1e" + strconv.FormatInt(int64(u), 10) + " BTC"
	}
}

// Amount represents the base bitcoin monetary unit (colloquially referred
// to as a `Satoshi').  A single Amount is equal to 1e-8 of a bitcoin.
type Amount int64

// NewAmount creates an Amount from a floating point value representing
// some value in bitcoin.  NewAmount errors if f is NaN or +-Infinity, but
// does not check that the amount is within the total amount of bitcoin
// producable as f may not refer to an amount at a single moment in time.
func NewAmount(f float64) (Amount, error) {
	// The amount is only considered invalid if it cannot be represented
	// as an integer type.  This may happen if f is NaN or +-Infinity.
	switch {
	case math.IsNaN(f):
		fallthrough
	case math.IsInf(f, 1):
		fallthrough
	case math.IsInf(f, -1):
		return 0, errors.New("invalid bitcoin amount")
	}

	a := f * float64(SatoshiPerBitcoin)

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
// the units with SI notation, or "Satoshi" for the base unit.
func (a Amount) Format(u AmountUnit) string {
	units := " " + u.String()
	return strconv.FormatFloat(a.ToUnit(u), 'f', -int(u+8), 64) + units
}

// String is the equivalent of calling Format with AmountBTC.
func (a Amount) String() string {
	return a.Format(AmountBTC)
}
