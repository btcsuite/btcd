// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"math"
	"testing"
	"time"
)

// TestDynamicBanScore tests dynamicBanScore.
func TestDynamicBanScore(t *testing.T) {
	var bs dynamicBanScore
	base := time.Now()
	bs.increase(100, 0, base)
	bs.Reset()
	if bs.Int() != 0 {
		t.Errorf("Failed to reset ban score.")
	}

	r := bs.increase(100, 50, base)
	if r != 150 {
		t.Errorf("Unexpected result %d after ban score increase.", r)
	}

	r = bs.int(base.Add(time.Minute))
	if r != 125 {
		t.Errorf("Halflife check failed - %d instead of 125", r)
	}

	r = bs.int(base.Add(7 * time.Minute))
	if r != 100 {
		t.Errorf("Decay after 7m - %d instead of 100", r)
	}

	bs.Reset()
	r = bs.increase(0, math.MaxUint32, base)
	r = bs.int(base.Add(Lifetime * time.Second))
	if r != 3 { // 3, not 4 due to precision loss and truncating 3.999...
		t.Errorf("Pre max age check with MaxUint32 failed - %d", r)
	}
	r = bs.int(base.Add((Lifetime + 1) * time.Second))
	if r != 0 {
		t.Errorf("Zero after max age check failed - %d instead of 0", r)
	}
}
