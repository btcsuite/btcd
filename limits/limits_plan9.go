// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package limits

// SetLimits is a no-op on Plan 9 due to the lack of process accounting.
func SetLimits() error {
	return nil
}
