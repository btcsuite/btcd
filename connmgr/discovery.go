// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"net"
)

const (
	// These constants are used by the DNS seed code to pick a random last
	// seen time.
	secondsIn3Days int32 = 24 * 60 * 60 * 3
	secondsIn4Days int32 = 24 * 60 * 60 * 4
)

// DnsDiscover looks up the list of peers resolved by DNS for all hosts in
// seeders. If proxy is not "" then it is used as a tor proxy for the
// resolution.
func DnsDiscover(seeder string) ([]net.IP, error) {
	peers, err := Lookup(seeder)
	if err != nil {
		return nil, err
	}

	return peers, nil
}
