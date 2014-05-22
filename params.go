package btcnet

import (
	"errors"
	"math/big"

	"github.com/conformal/btcwire"
)

// These variables are the chain proof-of-work limit parameters for each default
// network.
var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// mainPowLimit is the highest proof of work value a Bitcoin block can
	// have for the main network.  It is the value 2^224 - 1.
	mainPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne)

	// regressionPowLimit is the highest proof of work value a Bitcoin block
	// can have for the regression test network.  It is the value 2^255 - 1.
	regressionPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)

	// testNet3PowLimit is the highest proof of work value a Bitcoin block
	// can have for the test network (version 3).  It is the value
	// 2^224 - 1.
	testNet3PowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne)
)

// Params defines a Bitcoin network by its parameters.  These parameters may be
// used by Bitcoin applications to differentiate networks as well as addresses
// and keys for one network from those intended for use on another network.
type Params struct {
	Name string
	Net  btcwire.BitcoinNet

	// Chain parameters
	GenesisBlock           *btcwire.MsgBlock
	GenesisHash            *btcwire.ShaHash
	PowLimit               *big.Int
	PowLimitBits           uint32
	SubsidyHalvingInterval int32

	// Encoding magics
	PubKeyHashAddrID byte // First byte of a P2PKH address
	ScriptHashAddrID byte // First byte of a P2SH address
	PrivateKeyID     byte // First byte of a WIF private key
}

// MainNetParams defines the network parameters for the main Bitcoin network.
var MainNetParams = Params{
	Name: "mainnet",
	Net:  btcwire.MainNet,

	// Chain parameters
	GenesisBlock:           &btcwire.GenesisBlock,
	GenesisHash:            &btcwire.GenesisHash,
	PowLimit:               mainPowLimit,
	PowLimitBits:           0x1d00ffff,
	SubsidyHalvingInterval: 210000,

	// Encoding magics
	PubKeyHashAddrID: 0x00,
	ScriptHashAddrID: 0x05,
	PrivateKeyID:     0x80,
}

// RegressionNetParams defines the network parameters for the regression test
// Bitcoin network.  Not to be confused with the test Bitcoin network (version
// 3), this network is sometimes simply called "testnet".
var RegressionNetParams = Params{
	Name: "regtest",
	Net:  btcwire.TestNet,

	// Chain parameters
	GenesisBlock:           &btcwire.TestNetGenesisBlock,
	GenesisHash:            &btcwire.TestNetGenesisHash,
	PowLimit:               regressionPowLimit,
	PowLimitBits:           0x207fffff,
	SubsidyHalvingInterval: 150,

	// Encoding magics
	PubKeyHashAddrID: 0x6f,
	ScriptHashAddrID: 0xc4,
	PrivateKeyID:     0xef,
}

// TestNet3Params defines the network parameters for the test Bitcoin network
// (version 3).  Not to be confused with the regression test network, this
// network is sometimes simply called "testnet".
var TestNet3Params = Params{
	Name: "testnet",
	Net:  btcwire.TestNet3,

	// Chain parameters
	GenesisBlock:           &btcwire.TestNet3GenesisBlock,
	GenesisHash:            &btcwire.TestNet3GenesisHash,
	PowLimit:               testNet3PowLimit,
	PowLimitBits:           0x1d00ffff,
	SubsidyHalvingInterval: 210000,

	// Encoding magics
	PubKeyHashAddrID: 0x6f,
	ScriptHashAddrID: 0xc4,
	PrivateKeyID:     0xef,
}

var (
	// ErrUnknownNet describes an error where the network parameters for a
	// network cannot be looked up because the network is non-standard and
	// is not registered into this package.
	//
	// This will be removed when ParamsForNet is eventually removed.
	ErrUnknownNet = errors.New("unknown Bitcoin network")

	// ErrDuplicateNet describes an error where the parameters for a Bitcoin
	// network could not be set due to the network already being a standard
	// network or previously-registered into this package.
	//
	// This will be removed when Register is eventually removed.
	ErrDuplicateNet = errors.New("duplicate Bitcoin network")
)

var nets = map[btcwire.BitcoinNet]*Params{
	btcwire.MainNet:  &MainNetParams,
	btcwire.TestNet:  &RegressionNetParams,
	btcwire.TestNet3: &TestNet3Params,
}

// ParamsForNet returns the network parameters for a Bitcoin network, or
// ErrUnknownNet if the network is not a default network (mainnet, regtest,
// or testnet3) and not registered into the package with Register.
//
// This should be considered an unstable API and will be removed when all other
// Conformal btc* packages (btcwire not included) are updated from using
// btcwire.BitcoinNet to *Params.
func ParamsForNet(net btcwire.BitcoinNet) (*Params, error) {
	params, ok := nets[net]
	if !ok {
		return nil, ErrUnknownNet
	}
	return params, nil
}

// Register registers the network parameters for a Bitcoin network.  This may
// error with ErrDuplicateNet if the network is already registered.
//
// Network parameters should be registered into this package by a main package
// as early as possible.  Then, library packages may lookup networks or network
// parameters based on inputs and work regardless of the network being standard
// or not.
//
// This should be considered an unstable API and will be removed when all other
// Conformal btc* packages (btcwire not included) are updated from using
// btcwire.BitcoinNet to *Params.
func Register(params *Params) error {
	if _, ok := nets[params.Net]; ok {
		return ErrDuplicateNet
	}
	nets[params.Net] = params
	return nil
}
