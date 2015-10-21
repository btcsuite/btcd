// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
)

// TestTimeSorter tests the timeSorter implementation.
func TestTimeSorter(t *testing.T) {
	tests := []struct {
		in   []time.Time
		want []time.Time
	}{
		{
			in: []time.Time{
				time.Unix(1351228575, 0), // Fri Oct 26 05:16:15 UTC 2012 (Block #205000)
				time.Unix(1351228575, 1), // Fri Oct 26 05:16:15 UTC 2012 (+1 nanosecond)
				time.Unix(1348310759, 0), // Sat Sep 22 10:45:59 UTC 2012 (Block #200000)
				time.Unix(1305758502, 0), // Wed May 18 22:41:42 UTC 2011 (Block #125000)
				time.Unix(1347777156, 0), // Sun Sep 16 06:32:36 UTC 2012 (Block #199000)
				time.Unix(1349492104, 0), // Sat Oct  6 02:55:04 UTC 2012 (Block #202000)
			},
			want: []time.Time{
				time.Unix(1305758502, 0), // Wed May 18 22:41:42 UTC 2011 (Block #125000)
				time.Unix(1347777156, 0), // Sun Sep 16 06:32:36 UTC 2012 (Block #199000)
				time.Unix(1348310759, 0), // Sat Sep 22 10:45:59 UTC 2012 (Block #200000)
				time.Unix(1349492104, 0), // Sat Oct  6 02:55:04 UTC 2012 (Block #202000)
				time.Unix(1351228575, 0), // Fri Oct 26 05:16:15 UTC 2012 (Block #205000)
				time.Unix(1351228575, 1), // Fri Oct 26 05:16:15 UTC 2012 (+1 nanosecond)
			},
		},
	}

	for i, test := range tests {
		result := make([]time.Time, len(test.in))
		copy(result, test.in)
		sort.Sort(blockchain.TstTimeSorter(result))
		if !reflect.DeepEqual(result, test.want) {
			t.Errorf("timeSorter #%d got %v want %v", i, result,
				test.want)
			continue
		}
	}
}
