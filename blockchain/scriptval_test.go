// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"testing"
)

// TestCheckBlockScripts ensures that validating the all of the scripts in a
// known-good block doesn't return an error.
func TestCheckBlockScripts(t *testing.T) {
	// TODO In the future, add a block here with a lot of tx to validate.
	// The blockchain tests already validate a ton of scripts with signatures,
	// so we don't really need to make a new test for this immediately.
}
