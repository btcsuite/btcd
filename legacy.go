// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

// These constants are deprecated as they do not follow the standard Go style
// guidelines and are only provided for backwards compatibility.  Use the
// InvType* constants instead.
const (
	InvVect_Error InvType = InvTypeError // DEPRECATED
	InvVect_Tx    InvType = InvTypeTx    // DEPRECATED
	InvVect_Block InvType = InvTypeBlock // DEPRECATED
)
