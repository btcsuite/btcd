// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
This test file is part of the peer package rather than than the peer_test
package so it can bridge access to the internals to properly test cases which
are either not possible or can't reliably be tested via the public interface.
The functions are only exported while the tests are being run.
*/

package peer

// TstAllowSelfConns allows the test package to allow self connections by
// disabling the detection logic.
func TstAllowSelfConns() {
	allowSelfConns = true
}
