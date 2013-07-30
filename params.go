// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"github.com/conformal/btcwire"
	"math/big"
)

// Params houses parameters unique to various bitcoin networks such as the main
// network and test networks.  See ChainParams.
type Params struct {
	// GenesisBlock is the genesis block for the specific network.
	GenesisBlock *btcwire.MsgBlock

	// GenesisHash is the genesis block hash for the specific network.
	GenesisHash *btcwire.ShaHash

	// PowLimit is the highest proof of work value a bitcoin block can have
	// for the specific network.
	PowLimit *big.Int
}

// mainNetParams contains parameters specific to the main network
// (btcwire.MainNet).
var mainNetParams = Params{
	GenesisBlock: &btcwire.GenesisBlock,
	GenesisHash:  &btcwire.GenesisHash,

	// PowLimit is the highest proof of work value a bitcoin block can have.
	// It is the value 2^224 - 1 for the main network.
	PowLimit: new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne),
}

// regressionParams contains parameters specific to the regression test network
// (btcwire.TestNet).
var regressionParams = Params{
	GenesisBlock: &btcwire.TestNetGenesisBlock,
	GenesisHash:  &btcwire.TestNetGenesisHash,

	// PowLimit is the highest proof of work value a bitcoin block can have.
	// It is the value 2^256 - 1 for the regression test network.
	PowLimit: new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne),
}

// testNet3Params contains parameters specific to the test network (version 3)
// (btcwire.TestNet3).
var testNet3Params = Params{
	GenesisBlock: &btcwire.TestNet3GenesisBlock,
	GenesisHash:  &btcwire.TestNet3GenesisHash,

	// PowLimit is the highest proof of work value a bitcoin block can have.
	// It is the value 2^224 - 1 for the test network (version 3).
	PowLimit: new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne),
}

// chainParams returns chain parameters specific to the bitcoin network
// associated with the BlockChain instance.
func (b *BlockChain) chainParams() *Params {
	return ChainParams(b.btcnet)
}

// ChainParams returns chain parameters specific to the passed bitcoin network.
// It returns the parameters for btcwire.MainNet if the passed network is not
// supported.
func ChainParams(btcnet btcwire.BitcoinNet) *Params {
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
