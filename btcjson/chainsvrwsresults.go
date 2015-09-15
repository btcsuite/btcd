// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

// SessionResult models the data from the session command.
type SessionResult struct {
	SessionID uint64 `json:"sessionid"`
}
