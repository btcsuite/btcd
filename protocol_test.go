// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"github.com/conformal/btcwire"
	"testing"
)

// TestServiceFlagStringer tests the stringized output for service flag types.
func TestServiceFlagStringer(t *testing.T) {
	tests := []struct {
		in   btcwire.ServiceFlag
		want string
	}{
		{0, "0x0"},
		{btcwire.SFNodeNetwork, "SFNodeNetwork"},
		{0xffffffff, "SFNodeNetwork|0xfffffffe"},
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
