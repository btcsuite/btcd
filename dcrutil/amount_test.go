// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil_test

import (
	"math"
	"reflect"
	"sort"
	"testing"

	. "github.com/decred/dcrd/dcrutil"
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
			expected: MaxAmount,
		},
		{
			name:     "min producable",
			amount:   -21e6,
			valid:    true,
			expected: -MaxAmount,
		},
		{
			name:     "exceeds max producable",
			amount:   21e6 + 1e-8,
			valid:    true,
			expected: MaxAmount + 1,
		},
		{
			name:     "exceeds min producable",
			amount:   -21e6 - 1e-8,
			valid:    true,
			expected: -MaxAmount - 1,
		},
		{
			name:     "one hundred",
			amount:   100,
			valid:    true,
			expected: 100 * AtomsPerCoin,
		},
		{
			name:     "fraction",
			amount:   0.01234567,
			valid:    true,
			expected: 1234567,
		},
		{
			name:     "rounding up",
			amount:   54.999999999999943157,
			valid:    true,
			expected: 55 * AtomsPerCoin,
		},
		{
			name:     "rounding down",
			amount:   55.000000000000056843,
			valid:    true,
			expected: 55 * AtomsPerCoin,
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
			name:      "MDCR",
			amount:    MaxAmount,
			unit:      AmountMegaCoin,
			converted: 21,
			s:         "21 MDCR",
		},
		{
			name:      "kDCR",
			amount:    44433322211100,
			unit:      AmountKiloCoin,
			converted: 444.33322211100,
			s:         "444.333222111 kDCR",
		},
		{
			name:      "Coin",
			amount:    44433322211100,
			unit:      AmountCoin,
			converted: 444333.22211100,
			s:         "444333.222111 DCR",
		},
		{
			name:      "mDCR",
			amount:    44433322211100,
			unit:      AmountMilliCoin,
			converted: 444333222.11100,
			s:         "444333222.111 mDCR",
		},
		{

			name:      "μDCR",
			amount:    44433322211100,
			unit:      AmountMicroCoin,
			converted: 444333222111.00,
			s:         "444333222111 μDCR",
		},
		{

			name:      "atom",
			amount:    44433322211100,
			unit:      AmountAtom,
			converted: 44433322211100,
			s:         "44433322211100 Atom",
		},
		{

			name:      "non-standard unit",
			amount:    44433322211100,
			unit:      AmountUnit(-1),
			converted: 4443332.2211100,
			s:         "4443332.22111 1e-1 DCR",
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

		// Verify that Amount.ToCoin works as advertised.
		f1 := test.amount.ToUnit(AmountCoin)
		f2 := test.amount.ToCoin()
		if f1 != f2 {
			t.Errorf("%v: ToCoin does not match ToUnit(AmountCoin): %v != %v", test.name, f1, f2)
		}

		// Verify that Amount.String works as advertised.
		s1 := test.amount.Format(AmountCoin)
		s2 := test.amount.String()
		if s1 != s2 {
			t.Errorf("%v: String does not match Format(AmountCoin): %v != %v", test.name, s1, s2)
		}
	}
}

func TestAmountMulF64(t *testing.T) {
	tests := []struct {
		name string
		amt  Amount
		mul  float64
		res  Amount
	}{
		{
			name: "Multiply 0.1 DCR by 2",
			amt:  100e5, // 0.1 DCR
			mul:  2,
			res:  200e5, // 0.2 DCR
		},
		{
			name: "Multiply 0.2 DCR by 0.02",
			amt:  200e5, // 0.2 DCR
			mul:  1.02,
			res:  204e5, // 0.204 DCR
		},
		{
			name: "Multiply 0.1 DCR by -2",
			amt:  100e5, // 0.1 DCR
			mul:  -2,
			res:  -200e5, // -0.2 DCR
		},
		{
			name: "Multiply 0.2 DCR by -0.02",
			amt:  200e5, // 0.2 DCR
			mul:  -1.02,
			res:  -204e5, // -0.204 DCR
		},
		{
			name: "Multiply -0.1 DCR by 2",
			amt:  -100e5, // -0.1 DCR
			mul:  2,
			res:  -200e5, // -0.2 DCR
		},
		{
			name: "Multiply -0.2 DCR by 0.02",
			amt:  -200e5, // -0.2 DCR
			mul:  1.02,
			res:  -204e5, // -0.204 DCR
		},
		{
			name: "Multiply -0.1 DCR by -2",
			amt:  -100e5, // -0.1 DCR
			mul:  -2,
			res:  200e5, // 0.2 DCR
		},
		{
			name: "Multiply -0.2 DCR by -0.02",
			amt:  -200e5, // -0.2 DCR
			mul:  -1.02,
			res:  204e5, // 0.204 DCR
		},
		{
			name: "Round down",
			amt:  49, // 49 Atoms
			mul:  0.01,
			res:  0,
		},
		{
			name: "Round up",
			amt:  50, // 50 Atoms
			mul:  0.01,
			res:  1, // 1 Atom
		},
		{
			name: "Multiply by 0.",
			amt:  1e8, // 1 DCR
			mul:  0,
			res:  0, // 0 DCR
		},
		{
			name: "Multiply 1 by 0.5.",
			amt:  1, // 1 Atom
			mul:  0.5,
			res:  1, // 1 Atom
		},
		{
			name: "Multiply 100 by 66%.",
			amt:  100, // 100 Atoms
			mul:  0.66,
			res:  66, // 66 Atoms
		},
		{
			name: "Multiply 100 by 66.6%.",
			amt:  100, // 100 Atoms
			mul:  0.666,
			res:  67, // 67 Atoms
		},
		{
			name: "Multiply 100 by 2/3.",
			amt:  100, // 100 Atoms
			mul:  2.0 / 3,
			res:  67, // 67 Atoms
		},
	}

	for _, test := range tests {
		a := test.amt.MulF64(test.mul)
		if a != test.res {
			t.Errorf("%v: expected %v got %v", test.name, test.res, a)
		}
	}
}

func TestAmountSorter(t *testing.T) {
	tests := []struct {
		name string
		as   []Amount
		want []Amount
	}{
		{
			name: "Sort zero length slice of Amounts",
			as:   []Amount{},
			want: []Amount{},
		},
		{
			name: "Sort 1-element slice of Amounts",
			as:   []Amount{7},
			want: []Amount{7},
		},
		{
			name: "Sort 2-element slice of Amounts",
			as:   []Amount{7, 5},
			want: []Amount{5, 7},
		},
		{
			name: "Sort 6-element slice of Amounts",
			as:   []Amount{0, 9e8, 4e6, 4e6, 3, 9e12},
			want: []Amount{0, 3, 4e6, 4e6, 9e8, 9e12},
		},
	}

	for i, test := range tests {
		result := make([]Amount, len(test.as))
		copy(result, test.as)
		sort.Sort(AmountSorter(result))
		if !reflect.DeepEqual(result, test.want) {
			t.Errorf("AmountSorter #%d got %v want %v", i, result,
				test.want)
			continue
		}
	}
}
