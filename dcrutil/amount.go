// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"errors"
	"math"
	"strconv"
)

// AmountUnit describes a method of converting an Amount to something
// other than the base unit of a coin.  The value of the AmountUnit
// is the exponent component of the decadic multiple to convert from
// an amount in coins to an amount counted in atomic units.
type AmountUnit int

// These constants define various units used when describing a coin
// monetary amount.
const (
	AmountMegaCoin  AmountUnit = 6
	AmountKiloCoin  AmountUnit = 3
	AmountCoin      AmountUnit = 0
	AmountMilliCoin AmountUnit = -3
	AmountMicroCoin AmountUnit = -6
	AmountAtom      AmountUnit = -8
)

// String returns the unit as a string.  For recognized units, the SI
// prefix is used, or "Atom" for the base unit.  For all unrecognized
// units, "1eN DCR" is returned, where N is the AmountUnit.
func (u AmountUnit) String() string {
	switch u {
	case AmountMegaCoin:
		return "MDCR"
	case AmountKiloCoin:
		return "kDCR"
	case AmountCoin:
		return "DCR"
	case AmountMilliCoin:
		return "mDCR"
	case AmountMicroCoin:
		return "μDCR"
	case AmountAtom:
		return "Atom"
	default:
		return "1e" + strconv.FormatInt(int64(u), 10) + " DCR"
	}
}

// Amount represents the base coin monetary unit (colloquially referred
// to as an `Atom').  A single Amount is equal to 1e-8 of a coin.
type Amount int64

// round converts a floating point number, which may or may not be representable
// as an integer, to the Amount integer type by rounding to the nearest integer.
// This is performed by adding or subtracting 0.5 depending on the sign, and
// relying on integer truncation to round the value to the nearest Amount.
func round(f float64) Amount {
	if f < 0 {
		return Amount(f - 0.5)
	}
	return Amount(f + 0.5)
}

// NewAmount creates an Amount from a floating point value representing
// some value in the currency.  NewAmount errors if f is NaN or +-Infinity,
// but does not check that the amount is within the total amount of coins
// producible as f may not refer to an amount at a single moment in time.
//
// NewAmount is for specifically for converting DCR to Atoms (atomic units).
// For creating a new Amount with an int64 value which denotes a quantity of
// Atoms, do a simple type conversion from type int64 to Amount.
// See GoDoc for example: http://godoc.org/github.com/decred/dcrd/dcrutil#example-Amount
func NewAmount(f float64) (Amount, error) {
	// The amount is only considered invalid if it cannot be represented
	// as an integer type.  This may happen if f is NaN or +-Infinity.
	switch {
	case math.IsNaN(f):
		fallthrough
	case math.IsInf(f, 1):
		fallthrough
	case math.IsInf(f, -1):
		return 0, errors.New("invalid coin amount")
	}

	return round(f * AtomsPerCoin), nil
}

// ToUnit converts a monetary amount counted in coin base units to a
// floating point value representing an amount of coins.
func (a Amount) ToUnit(u AmountUnit) float64 {
	return float64(a) / math.Pow10(int(u+8))
}

// ToCoin is the equivalent of calling ToUnit with AmountCoin.
func (a Amount) ToCoin() float64 {
	return a.ToUnit(AmountCoin)
}

// Format formats a monetary amount counted in coin base units as a
// string for a given unit.  The conversion will succeed for any unit,
// however, known units will be formated with an appended label describing
// the units with SI notation, or "atom" for the base unit.
func (a Amount) Format(u AmountUnit) string {
	units := " " + u.String()
	return strconv.FormatFloat(a.ToUnit(u), 'f', -int(u+8), 64) + units
}

// String is the equivalent of calling Format with AmountCoin.
func (a Amount) String() string {
	return a.Format(AmountCoin)
}

// MulF64 multiplies an Amount by a floating point value.  While this is not
// an operation that must typically be done by a full node or wallet, it is
// useful for services that build on top of decred (for example, calculating
// a fee by multiplying by a percentage).
func (a Amount) MulF64(f float64) Amount {
	return round(float64(a) * f)
}

// AmountSorter implements sort.Interface to allow a slice of Amounts to
// be sorted.
type AmountSorter []Amount

// Len returns the number of Amounts in the slice.  It is part of the
// sort.Interface implementation.
func (s AmountSorter) Len() int {
	return len(s)
}

// Swap swaps the Amounts at the passed indices.  It is part of the
// sort.Interface implementation.
func (s AmountSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less returns whether the Amount with index i should sort before the
// Amount with index j.  It is part of the sort.Interface
// implementation.
func (s AmountSorter) Less(i, j int) bool {
	return s[i] < s[j]
}
