// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
)

// checkPowLimitsAreConsistent ensures PowLimit and PowLimitBits are consistent
// with each other
// PowLimit:         mainPowLimit,// big int
// PowLimitBits:     0x1d00ffff,  // conceptually the same
//                                // value, but in an obscure form
func checkPowLimitsAreConsistent(t *testing.T, params chaincfg.Params) {
	powLimitBigInt := params.PowLimit
	powLimitCompact := params.PowLimitBits

	toBig := blockchain.CompactToBig(powLimitCompact)
	toCompact := blockchain.BigToCompact(powLimitBigInt)

	// Check params.PowLimitBits matches params.PowLimit converted
	// into the compact form
	if toCompact != powLimitCompact {
		t.Fatalf("PowLimit values mismatch:\n"+
			"params.PowLimit    :%064x\n"+
			"                   :%x\n"+
			"params.PowLimitBits:%064x\n"+
			"                   :%x\n"+
			"params.PowLimit is not consistent with the params.PowLimitBits",
			powLimitBigInt, toCompact, toBig, powLimitCompact)
	}
}

// checkGenesisBlockRespectsNetworkPowLimit ensures genesis.Header.Bits value
// is within the network PoW limit.
//
// Genesis header bits define starting difficulty of the network.
// Header bits of each block define target difficulty of the subsequent block.
//
// The first few solved blocks of the network will inherit the genesis block
// bits value before the difficulty reajustment takes place.
//
// Solved block shouldn't be rejected due to the PoW limit check.
//
// This test ensures these blocks will respect the network PoW limit.
func checkGenesisBlockRespectsNetworkPowLimit(
	t *testing.T, params chaincfg.Params) {
	genesis := params.GenesisBlock
	bits := genesis.Header.Bits

	// Header bits as big.Int
	bitsAsBigInt := blockchain.CompactToBig(bits)

	// network PoW limit
	powLimitBigInt := params.PowLimit

	if bitsAsBigInt.Cmp(powLimitBigInt) > 0 {
		t.Fatalf("Genesis block fails the consensus:\n"+
			"genesis.Header.Bits:%x\n"+
			"                   :%064x\n"+
			"params.PowLimit    :%064x\n"+
			"genesis.Header.Bits "+
			"should respect network PoW limit",
			bits, bitsAsBigInt, powLimitBigInt)
	}
}

// TestDecredNetworkSettings checks Network-specific settings
func TestDecredNetworkSettings(t *testing.T) {
	checkPowLimitsAreConsistent(t, chaincfg.MainNetParams)
	checkPowLimitsAreConsistent(t, chaincfg.TestNet2Params)
	checkPowLimitsAreConsistent(t, chaincfg.SimNetParams)

	checkGenesisBlockRespectsNetworkPowLimit(t, chaincfg.MainNetParams)
	checkGenesisBlockRespectsNetworkPowLimit(t, chaincfg.TestNet2Params)
	checkGenesisBlockRespectsNetworkPowLimit(t, chaincfg.SimNetParams)
}
