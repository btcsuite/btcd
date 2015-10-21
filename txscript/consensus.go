// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

const (
	// LockTimeThreshold is the number below which a lock time is
	// interpreted to be a block number.  Since an average of one block
	// is generated per 10 minutes, this allows blocks for about 9,512
	// years.  However, if the field is interpreted as a timestamp, given
	// the lock time is a uint32, the max is sometime around 2106.
	LockTimeThreshold uint32 = 5e8 // Tue Nov 5 00:53:20 1985 UTC
)
