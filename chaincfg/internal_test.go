// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

func TestInvalidHashStr(t *testing.T) {
	_, err := chainhash.NewHashFromStr("banana")
	if err == nil {
		t.Error("Invalid string should fail.")
	}
}
