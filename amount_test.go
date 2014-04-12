// Copyright (c) 2013, 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	. "github.com/conformal/btcutil"
	"math"
	"testing"
)

func TestAmountCreation(t *testing.T) {
	tests := []struct {
		name     string
		amount   float64
		valid    bool
		expected Amount
	}{
		// Positive tests.
		{
			name:     "zero",
			amount:   0,
			valid:    true,
			expected: 0,
		},
		{
			name:     "max producable",
			amount:   21e6,
			valid:    true,
			expected: Amount(MaxSatoshi),
		},
		{
			name:     "min producable",
			amount:   -21e6,
			valid:    true,
			expected: Amount(-MaxSatoshi),
		},
		{
			name:     "exceeds max producable",
			amount:   21e6 + 1e-8,
			valid:    true,
			expected: Amount(MaxSatoshi + 1),
		},
		{
			name:     "exceeds min producable",
			amount:   -21e6 - 1e-8,
			valid:    true,
			expected: Amount(-MaxSatoshi - 1),
		},
		{
			name:     "one hundred",
			amount:   100,
			valid:    true,
			expected: Amount(100 * SatoshiPerBitcoin),
		},
		{
			name:     "fraction",
			amount:   0.01234567,
			valid:    true,
			expected: Amount(1234567),
		},
		{
			name:     "rounding up",
			amount:   54.999999999999943157,
			valid:    true,
			expected: Amount(55 * SatoshiPerBitcoin),
		},
		{
			name:     "rounding down",
			amount:   55.000000000000056843,
			valid:    true,
			expected: Amount(55 * SatoshiPerBitcoin),
		},

		// Negative tests.
		{
			name:   "not-a-number",
			amount: math.NaN(),
			valid:  false,
		},
		{
			name:   "-infinity",
			amount: math.Inf(-1),
			valid:  false,
		},
		{
			name:   "+infinity",
			amount: math.Inf(1),
			valid:  false,
		},
	}

	for _, test := range tests {
		a, err := NewAmount(test.amount)
		switch {
		case test.valid && err != nil:
			t.Errorf("%v: Positive test Amount creation failed with: %v", test.name, err)
			continue
		case !test.valid && err == nil:
			t.Errorf("%v: Negative test Amount creation succeeded (value %v) when should fail", test.name, a)
			continue
		}

		if a != test.expected {
			t.Errorf("%v: Created amount %v does not match expected %v", test.name, a, test.expected)
			continue
		}
	}
}

func TestAmountUnitConversions(t *testing.T) {
	tests := []struct {
		name      string
		amount    Amount
		unit      AmountUnit
		converted float64
		s         string
	}{
		{
			name:      "MBTC",
			amount:    Amount(MaxSatoshi),
			unit:      AmountMegaBTC,
			converted: 21,
			s:         "21 MBTC",
		},
		{
			name:      "kBTC",
			amount:    Amount(44433322211100),
			unit:      AmountKiloBTC,
			converted: 444.33322211100,
			s:         "444.333222111 kBTC",
		},
		{
			name:      "BTC",
			amount:    Amount(44433322211100),
			unit:      AmountBTC,
			converted: 444333.22211100,
			s:         "444333.222111 BTC",
		},
		{
			name:      "mBTC",
			amount:    Amount(44433322211100),
			unit:      AmountMilliBTC,
			converted: 444333222.11100,
			s:         "444333222.111 mBTC",
		},
		{

			name:      "μBTC",
			amount:    Amount(44433322211100),
			unit:      AmountMicroBTC,
			converted: 444333222111.00,
			s:         "444333222111 μBTC",
		},
		{

			name:      "satoshi",
			amount:    Amount(44433322211100),
			unit:      AmountSatoshi,
			converted: 44433322211100,
			s:         "44433322211100 Satoshi",
		},
		{

			name:      "non-standard unit",
			amount:    Amount(44433322211100),
			unit:      AmountUnit(-1),
			converted: 4443332.2211100,
			s:         "4443332.22111 1e-1 BTC",
		},
	}

	for _, test := range tests {
		f := test.amount.ToUnit(test.unit)
		if f != test.converted {
			t.Errorf("%v: converted value %v does not match expected %v", test.name, f, test.converted)
			continue
		}

		s := test.amount.Format(test.unit)
		if s != test.s {
			t.Errorf("%v: format '%v' does not match expected '%v'", test.name, s, test.s)
			continue
		}

		// Verify that Amount.String works as advertised.
		s1 := test.amount.Format(AmountBTC)
		s2 := test.amount.String()
		if s1 != s2 {
			t.Errorf("%v: String does not match Format(AmountBitcoin): %v != %v", test.name, s1, s2)
		}
	}
}
