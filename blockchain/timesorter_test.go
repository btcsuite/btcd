// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"reflect"
	"sort"
	"testing"
)

// TestTimeSorter tests the timeSorter implementation.
func TestTimeSorter(t *testing.T) {
	tests := []struct {
		in   []int64
		want []int64
	}{
		{
			in: []int64{
				1351228575, // Fri Oct 26 05:16:15 UTC 2012 (Block #205000)
				1348310759, // Sat Sep 22 10:45:59 UTC 2012 (Block #200000)
				1305758502, // Wed May 18 22:41:42 UTC 2011 (Block #125000)
				1347777156, // Sun Sep 16 06:32:36 UTC 2012 (Block #199000)
				1349492104, // Sat Oct  6 02:55:04 UTC 2012 (Block #202000)
			},
			want: []int64{
				1305758502, // Wed May 18 22:41:42 UTC 2011 (Block #125000)
				1347777156, // Sun Sep 16 06:32:36 UTC 2012 (Block #199000)
				1348310759, // Sat Sep 22 10:45:59 UTC 2012 (Block #200000)
				1349492104, // Sat Oct  6 02:55:04 UTC 2012 (Block #202000)
				1351228575, // Fri Oct 26 05:16:15 UTC 2012 (Block #205000)
			},
		},
	}

	for i, test := range tests {
		result := make([]int64, len(test.in))
		copy(result, test.in)
		sort.Sort(timeSorter(result))
		if !reflect.DeepEqual(result, test.want) {
			t.Errorf("timeSorter #%d got %v want %v", i, result,
				test.want)
			continue
		}
	}
}
