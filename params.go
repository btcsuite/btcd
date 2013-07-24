// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"github.com/conformal/btcwire"
	"math/big"
)

// params is used to group parameters for various networks such as the main
// network and test networks.
type params struct {
	genesisBlock *btcwire.MsgBlock
	genesisHash  *btcwire.ShaHash
	powLimit     *big.Int
}

// mainNetParams contains parameters specific to the main network
// (btcwire.MainNet).
var mainNetParams = params{
	genesisBlock: &btcwire.GenesisBlock,
	genesisHash:  &btcwire.GenesisHash,

	// powLimit is the highest proof of work value a bitcoin block can have.
	// It is the value 2^224 - 1 for the main network.
	powLimit: new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne),
}

// regressionParams contains parameters specific to the regression test network
// (btcwire.TestNet).
var regressionParams = params{
	genesisBlock: &btcwire.TestNetGenesisBlock,
	genesisHash:  &btcwire.TestNetGenesisHash,

	// powLimit is the highest proof of work value a bitcoin block can have.
	// It is the value 2^256 - 1 for the regression test network.
	powLimit: new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne),
}

// testNet3Params contains parameters specific to the test network (version 3)
// (btcwire.TestNet3).
var testNet3Params = params{
	genesisBlock: &btcwire.TestNet3GenesisBlock,
	genesisHash:  &btcwire.TestNet3GenesisHash,

	// powLimit is the highest proof of work value a bitcoin block can have.
	// It is the value 2^224 - 1 for the test network (version 3).
	powLimit: new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne),
}

// netParams returns parameters specific to the passed bitcoin network.
func (b *BlockChain) netParams() *params {
	switch b.btcnet {
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
