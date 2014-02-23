// Copyright (c) 2013-2014 Conformal Systems LLC.
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

	// PowLimitBits is the highest proof of work value a bitcoin block can
	// have represented in compact form.  See CompactToBig for more details
	// on compact form.
	PowLimitBits uint32

	// SubsidyHalvingInterval is the interval of blocks at which the
	// baseSubsidy is continually halved.  Mathematically this is:
	// baseSubsidy / 2^(height/SubsidyHalvingInterval)
	SubsidyHalvingInterval int64
}

// mainPowLimit is the highest proof of work value a bitcoin block can have for
// the main network.  It is the value 2^224 - 1.
var mainPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne)

// mainNetParams contains parameters specific to the main network
// (btcwire.MainNet).
var mainNetParams = Params{
	GenesisBlock:           &btcwire.GenesisBlock,
	GenesisHash:            &btcwire.GenesisHash,
	PowLimit:               mainPowLimit,
	PowLimitBits:           BigToCompact(mainPowLimit),
	SubsidyHalvingInterval: 210000,
}

// regressionPowLimit is the highest proof of work value a bitcoin block can
// have for the regression test network.  It is the value 2^255 - 1.
var regressionPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)

// regressionParams contains parameters specific to the regression test network
// (btcwire.TestNet).
var regressionParams = Params{
	GenesisBlock:           &btcwire.TestNetGenesisBlock,
	GenesisHash:            &btcwire.TestNetGenesisHash,
	PowLimit:               regressionPowLimit,
	PowLimitBits:           BigToCompact(regressionPowLimit),
	SubsidyHalvingInterval: 150,
}

// testNetPowLimit is the highest proof of work value a bitcoin block can have
// for the test network (version 3).  It is the value 2^224 - 1.
var testNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne)

// testNet3Params contains parameters specific to the test network (version 3)
// (btcwire.TestNet3).
var testNet3Params = Params{
	GenesisBlock:           &btcwire.TestNet3GenesisBlock,
	GenesisHash:            &btcwire.TestNet3GenesisHash,
	PowLimit:               testNetPowLimit,
	PowLimitBits:           BigToCompact(testNetPowLimit),
	SubsidyHalvingInterval: 210000,
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
