// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import "github.com/conformal/btcutil/base58"

// Base58Decode wraps the new base58 package for backwards compatibility.
// TODO: Remove as soon as the other repos have been updated to use the new package.
func Base58Decode(b string) []byte {
	return base58.Decode(b)
}

// Base58Encode wraps the new base58 package for backwards compatibility.
// TODO: Remove as soon as the other repos have been updated to use the new package.
func Base58Encode(b []byte) string {
	return base58.Encode(b)
}
