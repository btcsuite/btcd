// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/conformal/btcchain"
	"github.com/conformal/btcwire"
	"math/big"
)

// activeNetParams is a pointer to the parameters specific to the
// currently active bitcoin network.
var activeNetParams = netParams(defaultBtcnet)

// params is used to group parameters for various networks such as the main
// network and test networks.
type params struct {
	netName      string
	btcnet       btcwire.BitcoinNet
	genesisBlock *btcwire.MsgBlock
	genesisHash  *btcwire.ShaHash
	powLimit     *big.Int
	powLimitBits uint32
	peerPort     string
	listenPort   string
	rpcPort      string
	dnsSeeds     []string
}

// mainNetParams contains parameters specific to the main network
// (btcwire.MainNet).  NOTE: The RPC port is intentionally different than the
// reference implementation because btcd does not handle wallet requests.  The
// separate wallet process listens on the well-known port and forwards requests
// it does not handle on to btcd.  This approach allows the wallet process
// to emulate the full reference implementation RPC API.
var mainNetParams = params{
	netName:      "mainnet",
	btcnet:       btcwire.MainNet,
	genesisBlock: btcchain.ChainParams(btcwire.MainNet).GenesisBlock,
	genesisHash:  btcchain.ChainParams(btcwire.MainNet).GenesisHash,
	powLimit:     btcchain.ChainParams(btcwire.MainNet).PowLimit,
	powLimitBits: btcchain.ChainParams(btcwire.MainNet).PowLimitBits,
	listenPort:   btcwire.MainPort,
	peerPort:     btcwire.MainPort,
	rpcPort:      "8334",
	dnsSeeds: []string{
		"seed.bitcoin.sipa.be",
		"dnsseed.bluematt.me",
		"dnsseed.bitcoin.dashjr.org",
		"seed.bitcoinstats.com",
		"bitseed.xf2.org",
	},
}

// regressionParams contains parameters specific to the regression test network
// (btcwire.TestNet).  NOTE: The RPC port is intentionally different than the
// reference implementation - see the mainNetParams comment for details.
var regressionParams = params{
	netName:      "regtest",
	btcnet:       btcwire.TestNet,
	genesisBlock: btcchain.ChainParams(btcwire.TestNet).GenesisBlock,
	genesisHash:  btcchain.ChainParams(btcwire.TestNet).GenesisHash,
	powLimit:     btcchain.ChainParams(btcwire.TestNet).PowLimit,
	powLimitBits: btcchain.ChainParams(btcwire.TestNet).PowLimitBits,
	listenPort:   btcwire.RegressionTestPort,
	peerPort:     btcwire.TestNetPort,
	rpcPort:      "18334",
	dnsSeeds:     []string{},
}

// testNet3Params contains parameters specific to the test network (version 3)
// (btcwire.TestNet3).  NOTE: The RPC port is intentionally different than the
// reference implementation - see the mainNetParams comment for details.
var testNet3Params = params{
	netName:      "testnet",
	btcnet:       btcwire.TestNet3,
	genesisBlock: btcchain.ChainParams(btcwire.TestNet3).GenesisBlock,
	genesisHash:  btcchain.ChainParams(btcwire.TestNet3).GenesisHash,
	powLimit:     btcchain.ChainParams(btcwire.TestNet3).PowLimit,
	powLimitBits: btcchain.ChainParams(btcwire.TestNet3).PowLimitBits,
	listenPort:   btcwire.TestNetPort,
	peerPort:     btcwire.TestNetPort,
	rpcPort:      "18334",
	dnsSeeds: []string{
		"testnet-seed.bitcoin.petertodd.org",
		"testnet-seed.bluematt.me",
	},
}

// netParams returns parameters specific to the passed bitcoin network.
func netParams(btcnet btcwire.BitcoinNet) *params {
	switch btcnet {
	case btcwire.TestNet:
		return &regressionParams

	case btcwire.TestNet3:
		return &testNet3Params

	// Return main net by default.
	case btcwire.MainNet:
		fallthrough
	default:
		return &mainNetParams
	}
}
