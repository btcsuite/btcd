// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/decred/dcrd/chaincfg"
)

// activeNetParams is a pointer to the parameters specific to the
// currently active Decred network.
var activeNetParams = &mainNetParams

// params is used to group parameters for various networks such as the main
// network and test networks.
type params struct {
	*chaincfg.Params
	rpcPort string
}

// mainNetParams contains parameters specific to the main network
// (wire.MainNet).  NOTE: The RPC port is intentionally different than the
// reference implementation because dcrd does not handle wallet requests.  The
// separate wallet process listens on the well-known port and forwards requests
// it does not handle on to dcrd.  This approach allows the wallet process
// to emulate the full reference implementation RPC API.
var mainNetParams = params{
	Params:  &chaincfg.MainNetParams,
	rpcPort: "9109",
}

// testNet3Params contains parameters specific to the test network (version 3)
// (wire.TestNet3).
var testNet3Params = params{
	Params:  &chaincfg.TestNet3Params,
	rpcPort: "19109",
}

// simNetParams contains parameters specific to the simulation test network
// (wire.SimNet).
var simNetParams = params{
	Params:  &chaincfg.SimNetParams,
	rpcPort: "19556",
}
