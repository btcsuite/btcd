// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import "testing"

// TestServiceFlagStringer tests the stringized output for service flag types.
func TestServiceFlagStringer(t *testing.T) {
	tests := []struct {
		in   ServiceFlag
		want string
	}{
		{0, "0x0"},
		{SFNodeNetwork, "SFNodeNetwork"},
		{SFNodeBloom, "SFNodeBloom"},
		{SFNodeCF, "SFNodeCF"},
		{0xffffffff, "SFNodeNetwork|SFNodeBloom|SFNodeCF|0xfffffff8"},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}

// TestCurrencyNetStringer tests the stringized output for Decred net types.
func TestCurrencyNetStringer(t *testing.T) {
	tests := []struct {
		in   CurrencyNet
		want string
	}{
		{MainNet, "MainNet"},
		{TestNet3, "TestNet3"},
		{SimNet, "SimNet"},
		{0xffffffff, "Unknown CurrencyNet (4294967295)"},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}
