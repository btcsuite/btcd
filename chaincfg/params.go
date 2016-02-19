// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"errors"
	"math/big"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// These variables are the chain proof-of-work limit parameters for each default
// network.
var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// mainPowLimit is the highest proof of work value a Decred block can
	// have for the main network.  It is the value 2^224 - 1.
	mainPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne)

	// testNetPowLimit is the highest proof of work value a Decred block
	// can have for the test network.  It is the value 2^232 - 1.
	testNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 232), bigOne)

	// simNetPowLimit is the highest proof of work value a Decred block
	// can have for the simulation test network.  It is the value 2^255 - 1.
	simNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)
)

// SigHashOptimization is an optimization for verification of transactions that
// do CHECKSIG operations with hashType SIGHASH_ALL. Although there should be no
// consequences to daemons that are simply running a node, it may be the case
// that you could cause database corruption if you turn this code on, create and
// manipulate your own MsgTx, then include them in blocks. For safety, if you're
// using the daemon with wallet or mining with the daemon this should be disabled.
// If you believe that any MsgTxs in your daemon will be used mutably, do NOT
// turn on this feature. It is disabled by default.
// This feature is considered EXPERIMENTAL, enable at your own risk!
var SigHashOptimization = false

// CheckForDuplicateHashes checks for duplicate hashes when validating blocks.
// Because of the rule inserting the height into the second (nonce) txOut, there
// should never be a duplicate transaction hash that overwrites another. However,
// because there is a 2^128 chance of a collision, the paranoid user may wish to
// turn this feature on.
var CheckForDuplicateHashes = false

// CPUMinerThreads is the default number of threads to utilize with the
// CPUMiner when mining.
var CPUMinerThreads = 1

// Checkpoint identifies a known good point in the block chain.  Using
// checkpoints allows a few optimizations for old blocks during initial download
// and also prevents forks from old blocks.
//
// Each checkpoint is selected based upon several factors.  See the
// documentation for chain.IsCheckpointCandidate for details on the selection
// criteria.
type Checkpoint struct {
	Height int64
	Hash   *chainhash.Hash
}

// TokenPayout is a payout for block 1 which specifies an address and an amount
// to pay to that address in a transaction output.
type TokenPayout struct {
	Address string
	Amount  int64
}

// Params defines a Decred network by its parameters.  These parameters may be
// used by Decred applications to differentiate networks as well as addresses
// and keys for one network from those intended for use on another network.
type Params struct {
	Name        string
	Net         wire.CurrencyNet
	DefaultPort string
	DNSSeeds    []string

	// Starting block for the network (block 0).
	GenesisBlock *wire.MsgBlock

	// Starting block hash.
	GenesisHash *chainhash.Hash

	// The version of the block that the majority of the network is currently
	// on.
	CurrentBlockVersion int32

	// Maximum value for nbits (minimum Proof of Work) as a uint256.
	PowLimit *big.Int

	// Maximum value for nbits (minimum Proof of Work) in compact form.
	PowLimitBits uint32

	// Testnet difficulty reset flag.
	ResetMinDifficulty bool

	// MinDiffResetTimeFactor is the amount to multiply TimePerBlock by to
	// reset the difficulty to the minimum network factor.
	MinDiffResetTimeFactor time.Duration

	// GenerateSupported specifies whether or not CPU mining is allowed.
	GenerateSupported bool

	// MaximumBlockSize is the maximum size of a block that can be generated
	// on the network.
	MaximumBlockSize int

	// TimePerBlock is the desired amount of time to generate each block in
	// minutes.
	TimePerBlock time.Duration

	// WorkDiffAlpha is the stake difficulty EMA calculation alpha (smoothing)
	// value. It is different from a normal EMA alpha. Closer to 1 --> smoother.
	WorkDiffAlpha int64

	// WorkDiffWindowSize is the number of windows (intervals) used for calculation
	// of the exponentially weighted average.
	WorkDiffWindowSize int64

	// WorkDiffWindows is the number of windows (intervals) used for calculation
	// of the exponentially weighted average.
	WorkDiffWindows int64

	// targetTimespan is the desired amount of time that should elapse
	// before block difficulty requirement is examined to determine how
	// it should be changed in order to maintain the desired block
	// generation rate. This value should correspond to the product of
	// WorkDiffWindowSize and TimePerBlock above.
	TargetTimespan time.Duration

	// RetargetAdjustmentFactor is the adjustment factor used to limit
	// the minimum and maximum amount of adjustment that can occur between
	// difficulty retargets.
	RetargetAdjustmentFactor int64

	// Subsidy parameters.
	//
	// Subsidy calculation for exponential reductions:
	// 0 for i in range (0, height / ReductionInterval):
	// 1     subsidy *= MulSubsidy
	// 2     subsidy /= DivSubsidy
	//
	// Caveat: Don't overflow the int64 register!!

	// BaseSubsidy is the starting subsidy amount for mined blocks.
	BaseSubsidy int64

	// Subsidy reduction multiplier.
	MulSubsidy int64

	// Subsidy reduction divisor.
	DivSubsidy int64

	// Reduction interval in blocks.
	ReductionInterval int64

	// WorkRewardProportion is the comparative amount of the subsidy given for
	// creating a block.
	WorkRewardProportion uint16

	// StakeRewardProportion is the comparative amount of the subsidy given for
	// casting stake votes (collectively, per block).
	StakeRewardProportion uint16

	// BlockTaxProportion is the inverse of the percentage of funds for each
	// block to allocate to the developer organization.
	// e.g. 10% --> 10 (or 1 / (1/10))
	// Special case: disable taxes with a value of 0
	BlockTaxProportion uint16

	// Checkpoints ordered from oldest to newest.
	Checkpoints []Checkpoint

	// Mempool parameters
	RelayNonStdTxs bool

	// NetworkAddressPrefix is the first letter of the network
	// for any given address encoded as a string.
	NetworkAddressPrefix string

	// Address encoding magics
	PubKeyAddrID     [2]byte // First 2 bytes of a P2PK address
	PubKeyHashAddrID [2]byte // First 2 bytes of a P2PKH address
	PKHEdwardsAddrID [2]byte // First 2 bytes of an Edwards P2PKH address
	PKHSchnorrAddrID [2]byte // First 2 bytes of a secp256k1 Schnorr P2PKH address
	ScriptHashAddrID [2]byte // First 2 bytes of a P2SH address
	PrivateKeyID     [2]byte // First 2 bytes of a WIF private key

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID [4]byte
	HDPublicKeyID  [4]byte

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType uint32

	// MinimumStakeDiff if the minimum amount of Atoms required to purchase a
	// stake ticket.
	MinimumStakeDiff int64

	// Ticket pool sizes for Decred PoS. This denotes the number of possible
	// buckets/number of different ticket numbers. It is also the number of
	// possible winner numbers there are.
	TicketPoolSize uint16

	// Average number of tickets per block for Decred PoS.
	TicketsPerBlock uint16

	// Number of blocks for tickets to mature (spendable at TicketMaturity+1).
	TicketMaturity uint16

	// Number of blocks for tickets to expire after they have matured. This MUST
	// be >= (StakeEnabledHeight + StakeValidationHeight).
	TicketExpiry uint32

	// Maturity for spending coinbase tx.
	CoinbaseMaturity uint16

	// Maturity for spending SStx change outputs.
	SStxChangeMaturity uint16

	// TicketPoolSizeWeight is the multiplicative weight applied to the
	// ticket pool size difference between a window period and its target
	// when determining the stake system.
	TicketPoolSizeWeight uint16

	// StakeDiffAlpha is the stake difficulty EMA calculation alpha (smoothing)
	// value. It is different from a normal EMA alpha. Closer to 1 --> smoother.
	StakeDiffAlpha int64

	// StakeDiffWindowSize is the number of blocks used for each interval in
	// exponentially weighted average.
	StakeDiffWindowSize int64

	// StakeDiffWindows is the number of windows (intervals) used for calculation
	// of the exponentially weighted average.
	StakeDiffWindows int64

	// MaxFreshStakePerBlock is the maximum number of new tickets that may be
	// submitted per block.
	MaxFreshStakePerBlock uint8

	// StakeEnabledHeight is the height in which the first ticket could possibly
	// mature.
	StakeEnabledHeight int64

	// StakeValidationHeight is the height at which votes (SSGen) are required
	// to add a new block to the top of the blockchain. This height is the
	// first block that will be voted on, but will include in itself no votes.
	StakeValidationHeight int64

	// StakeBaseSigScript is the consensus stakebase signature script for all
	// votes on the network. This isn't signed in any way, so without forcing
	// it to be this value miners/daemons could freely change it.
	StakeBaseSigScript []byte

	// OrganizationAddress is the static address for block taxes to be
	// distributed to in every block's coinbase. It should ideally be
	// a P2SH multisignature address.
	OrganizationAddress string

	// BlockOneLedger specifies the list of payouts in the coinbase of
	// block height 1. If there are no payouts to be given, set this
	// to an empty slice.
	BlockOneLedger []*TokenPayout
}

// MainNetParams defines the network parameters for the main Decred network.
var MainNetParams = Params{
	Name:        "mainnet",
	Net:         wire.MainNet,
	DefaultPort: "9108",
	DNSSeeds: []string{
		"mainnet-seed.decred.mindcry.org",
		"mainnet-seed.decred.netpurgatory.com",
		"mainnet.decredseed.org",
		"mainnet-seed.decred.org",
	},

	// Chain parameters
	GenesisBlock:             &genesisBlock,
	GenesisHash:              &genesisHash,
	CurrentBlockVersion:      1,
	PowLimit:                 mainPowLimit,
	PowLimitBits:             0x1d00ffff,
	ResetMinDifficulty:       false,
	MinDiffResetTimeFactor:   0x7FFFFFFF,
	GenerateSupported:        false,
	MaximumBlockSize:         393216,
	TimePerBlock:             time.Minute * 5,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       144,
	WorkDiffWindows:          20,
	TargetTimespan:           time.Minute * 5 * 144, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:           3119582664, // 21m
	MulSubsidy:            100,
	DivSubsidy:            101,
	ReductionInterval:     6144,
	WorkRewardProportion:  6,
	StakeRewardProportion: 3,
	BlockTaxProportion:    1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: []Checkpoint{
		{440, newHashFromStr("0000000000002203eb2c95ee96906730bb56b2985e174518f90eb4db29232d93")},
		{24480, newHashFromStr("0000000000000c9d4239c4ef7ef3fb5aaeed940244bc69c57c8c5e1f071b28a6")},
		{48590, newHashFromStr("0000000000000d5e0de21a96d3c965f5f2db2c82612acd7389c140c9afe92ba7")},
	},

	// Mempool parameters
	RelayNonStdTxs: false,

	// Address encoding magics
	NetworkAddressPrefix: "D",
	PubKeyAddrID:         [2]byte{0x13, 0x86}, // starts with Dk
	PubKeyHashAddrID:     [2]byte{0x07, 0x3f}, // starts with Ds
	PKHEdwardsAddrID:     [2]byte{0x07, 0x1f}, // starts with De
	PKHSchnorrAddrID:     [2]byte{0x07, 0x01}, // starts with DS
	ScriptHashAddrID:     [2]byte{0x07, 0x1a}, // starts with Dc
	PrivateKeyID:         [2]byte{0x22, 0xde}, // starts with Pm

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x02, 0xfd, 0xa4, 0xe8}, // starts with dprv
	HDPublicKeyID:  [4]byte{0x02, 0xfd, 0xa9, 0x26}, // starts with dpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 20,

	// Decred PoS parameters
	MinimumStakeDiff:      2 * 1e8, // 2 Coin
	TicketPoolSize:        8192,
	TicketsPerBlock:       5,
	TicketMaturity:        256,
	TicketExpiry:          40960, // 5*TicketPoolSize
	CoinbaseMaturity:      256,
	SStxChangeMaturity:    1,
	TicketPoolSizeWeight:  4,
	StakeDiffAlpha:        1, // Minimal
	StakeDiffWindowSize:   144,
	StakeDiffWindows:      20,
	MaxFreshStakePerBlock: 20,        // 4*TicketsPerBlock
	StakeEnabledHeight:    256 + 256, // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight: 4096,      // ~14 days
	StakeBaseSigScript:    []byte{0x00, 0x00},

	// Decred organization related parameters
	OrganizationAddress: "Dcur2mcGjmENx4DhNqDctW5wJCVyT3Qeqkx",
	BlockOneLedger:      BlockOneLedgerMainNet,
}

// TestNetParams defines the network parameters for the test currency network.
// This network is sometimes simply called "testnet".
var TestNetParams = Params{
	Name:        "testnet",
	Net:         wire.TestNet,
	DefaultPort: "19108",
	DNSSeeds: []string{
		"testnet-seed.decred.mindcry.org",
		"testnet-seed.decred.netpurgatory.org",
		"testnet.decredseed.org",
		"testnet-seed.decred.org",
	},

	// Chain parameters
	GenesisBlock:             &testNetGenesisBlock,
	GenesisHash:              &testNetGenesisHash,
	CurrentBlockVersion:      0,
	PowLimit:                 testNetPowLimit,
	PowLimitBits:             0x1e00ffff,
	ResetMinDifficulty:       false,
	MinDiffResetTimeFactor:   0x7FFFFFFF,
	GenerateSupported:        true,
	MaximumBlockSize:         1000000,
	TimePerBlock:             time.Minute * 2,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       144,
	WorkDiffWindows:          20,
	TargetTimespan:           time.Minute * 2 * 144, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:           2500000000, // 25 Coin
	MulSubsidy:            100,
	DivSubsidy:            101,
	ReductionInterval:     2048,
	WorkRewardProportion:  6,
	StakeRewardProportion: 3,
	BlockTaxProportion:    1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: []Checkpoint{
		{2900, newHashFromStr("000000000012475b579cd16d670f42982648e5dbf81d84960c83fd1990d84cbe")},
		{27550, newHashFromStr("00000000183a9f9b8fa6accc32c7abb2456a8e0e12baf045788a74114dbbebe3")},
		{61540, newHashFromStr("0000000003bc7461b7cd821fb258de72b9b0c7604b1a5346d29eb132d846ccb0")},
		{97110, newHashFromStr("0000000006ae0065cf97102844854966d907d6e50ab18894679fbad1dfc017b4")},
		{132220, newHashFromStr("000000000372a1bf5ad5c42361672bfeae6a513ad25ab9eaf4590f890e090c36")},
	},

	// Mempool parameters
	RelayNonStdTxs: true,

	// Address encoding magics
	NetworkAddressPrefix: "T",
	PubKeyAddrID:         [2]byte{0x28, 0xf7}, // starts with Tk
	PubKeyHashAddrID:     [2]byte{0x0f, 0x21}, // starts with Ts
	PKHEdwardsAddrID:     [2]byte{0x0f, 0x01}, // starts with Te
	PKHSchnorrAddrID:     [2]byte{0x0e, 0xe3}, // starts with TS
	ScriptHashAddrID:     [2]byte{0x0e, 0xfc}, // starts with Tc
	PrivateKeyID:         [2]byte{0x23, 0x0e}, // starts with Pt

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x97}, // starts with tprv
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xd1}, // starts with tpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 11,

	// Decred PoS parameters
	MinimumStakeDiff:      20000000, // 0.2 Coin
	TicketPoolSize:        1024,
	TicketsPerBlock:       5,
	TicketMaturity:        16,
	TicketExpiry:          6144, // 6*TicketPoolSize
	CoinbaseMaturity:      16,
	SStxChangeMaturity:    1,
	TicketPoolSizeWeight:  4,
	StakeDiffAlpha:        1,
	StakeDiffWindowSize:   144,
	StakeDiffWindows:      20,
	MaxFreshStakePerBlock: 20,      // 4*TicketsPerBlock
	StakeEnabledHeight:    16 + 16, // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight: 768,     // Arbitrary
	StakeBaseSigScript:    []byte{0xDE, 0xAD, 0xBE, 0xEF},

	// Decred organization related parameters
	OrganizationAddress: "TcemyEtyHSg9L7jww7uihv9BJfKL6YGiZYn",
	BlockOneLedger:      BlockOneLedgerTestNet,
}

// SimNetParams defines the network parameters for the simulation test Decred
// network.  This network is similar to the normal test network except it is
// intended for private use within a group of individuals doing simulation
// testing.  The functionality is intended to differ in that the only nodes
// which are specifically specified are used to create the network rather than
// following normal discovery rules.  This is important as otherwise it would
// just turn into another public testnet.
var SimNetParams = Params{
	Name:        "simnet",
	Net:         wire.SimNet,
	DefaultPort: "18555",
	DNSSeeds:    []string{}, // NOTE: There must NOT be any seeds.

	// Chain parameters
	GenesisBlock:             &simNetGenesisBlock,
	GenesisHash:              &simNetGenesisHash,
	CurrentBlockVersion:      0,
	PowLimit:                 simNetPowLimit,
	PowLimitBits:             0x207fffff,
	ResetMinDifficulty:       false,
	MinDiffResetTimeFactor:   0x7FFFFFFF,
	GenerateSupported:        true,
	MaximumBlockSize:         1000000,
	TimePerBlock:             time.Second * 1,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       8,
	WorkDiffWindows:          4,
	TargetTimespan:           time.Second * 1 * 8, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:           50000000000,
	MulSubsidy:            100,
	DivSubsidy:            101,
	ReductionInterval:     128,
	WorkRewardProportion:  6,
	StakeRewardProportion: 3,
	BlockTaxProportion:    1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// Mempool parameters
	RelayNonStdTxs: true,

	// Address encoding magics
	NetworkAddressPrefix: "S",
	PubKeyAddrID:         [2]byte{0x27, 0x6f}, // starts with Sk
	PubKeyHashAddrID:     [2]byte{0x0e, 0x91}, // starts with Ss
	PKHEdwardsAddrID:     [2]byte{0x0e, 0x71}, // starts with Se
	PKHSchnorrAddrID:     [2]byte{0x0e, 0x53}, // starts with SS
	ScriptHashAddrID:     [2]byte{0x0e, 0x6c}, // starts with Sc
	PrivateKeyID:         [2]byte{0x23, 0x07}, // starts with Ps

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x20, 0xb9, 0x03}, // starts with sprv
	HDPublicKeyID:  [4]byte{0x04, 0x20, 0xbd, 0x3d}, // starts with spub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 115, // ASCII for s

	// Decred PoS parameters
	MinimumStakeDiff:      20000,
	TicketPoolSize:        64,
	TicketsPerBlock:       5,
	TicketMaturity:        16,
	TicketExpiry:          384, // 6*TicketPoolSize
	CoinbaseMaturity:      16,
	SStxChangeMaturity:    1,
	TicketPoolSizeWeight:  4,
	StakeDiffAlpha:        1,
	StakeDiffWindowSize:   8,
	StakeDiffWindows:      8,
	MaxFreshStakePerBlock: 20,            // 4*TicketsPerBlock
	StakeEnabledHeight:    16 + 16,       // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight: 16 + (64 * 2), // CoinbaseMaturity + TicketPoolSize*2
	StakeBaseSigScript:    []byte{0xDE, 0xAD, 0xBE, 0xEF},

	// Decred organization related parameters
	//
	// "Dev org" address is a 3-of-3 P2SH going to wallet:
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// briefcase
	// (seed 0x00000000000000000000000000000000000000000000000000000000000000)
	//
	// This same wallet owns the three ledger outputs for simnet.
	//
	// P2SH details for simnet dev org is below.
	//
	// address: Scc4ZC844nzuZCXsCFXUBXTLks2mD6psWom
	// redeemScript: 532103e8c60c7336744c8dcc7b85c27789950fc52aa4e48f895ebbfb
	// ac383ab893fc4c2103ff9afc246e0921e37d12e17d8296ca06a8f92a07fbe7857ed1d4
	// f0f5d94e988f21033ed09c7fa8b83ed53e6f2c57c5fa99ed2230c0d38edf53c0340d0f
	// c2e79c725a53ae
	//   (3-of-3 multisig)
	// Pubkeys used:
	//   SkQmxbeuEFDByPoTj41TtXat8tWySVuYUQpd4fuNNyUx51tF1csSs
	//   SkQn8ervNvAUEX5Ua3Lwjc6BAuTXRznDoDzsyxgjYqX58znY7w9e4
	//   SkQkfkHZeBbMW8129tZ3KspEh1XBFC1btbkgzs6cjSyPbrgxzsKqk
	//
	OrganizationAddress: "ScuQxvveKGfpG1ypt6u27F99Anf7EW3cqhq",
	BlockOneLedger:      BlockOneLedgerSimNet,
}

var (
	// ErrDuplicateNet describes an error where the parameters for a Decred
	// network could not be set due to the network already being a standard
	// network or previously-registered into this package.
	ErrDuplicateNet = errors.New("duplicate Decred network")

	// ErrUnknownHDKeyID describes an error where the provided id which
	// is intended to identify the network for a hierarchical deterministic
	// private extended key is not registered.
	ErrUnknownHDKeyID = errors.New("unknown hd private extended key bytes")
)

var (
	registeredNets    = make(map[wire.CurrencyNet]struct{})
	pubKeyAddrIDs     = make(map[[2]byte]struct{})
	pubKeyHashAddrIDs = make(map[[2]byte]struct{})
	pkhEdwardsAddrIDs = make(map[[2]byte]struct{})
	pkhSchnorrAddrIDs = make(map[[2]byte]struct{})
	scriptHashAddrIDs = make(map[[2]byte]struct{})
	hdPrivToPubKeyIDs = make(map[[4]byte][]byte)
)

// Register registers the network parameters for a Decred network.  This may
// error with ErrDuplicateNet if the network is already registered (either
// due to a previous Register call, or the network being one of the default
// networks).
//
// Network parameters should be registered into this package by a main package
// as early as possible.  Then, library packages may lookup networks or network
// parameters based on inputs and work regardless of the network being standard
// or not.
func Register(params *Params) error {
	if _, ok := registeredNets[params.Net]; ok {
		return ErrDuplicateNet
	}
	registeredNets[params.Net] = struct{}{}
	pubKeyAddrIDs[params.PubKeyAddrID] = struct{}{}
	pubKeyHashAddrIDs[params.PubKeyHashAddrID] = struct{}{}
	scriptHashAddrIDs[params.ScriptHashAddrID] = struct{}{}
	hdPrivToPubKeyIDs[params.HDPrivateKeyID] = params.HDPublicKeyID[:]
	return nil
}

// mustRegister performs the same function as Register except it panics if there
// is an error.  This should only be called from package init functions.
func mustRegister(params *Params) {
	if err := Register(params); err != nil {
		panic("failed to register network: " + err.Error())
	}
}

// IsPubKeyAddrID returns whether the id is an identifier known to prefix a
// pay-to-pubkey address on any default or registered network.
func IsPubKeyAddrID(id [2]byte) bool {
	_, ok := pubKeyHashAddrIDs[id]
	return ok
}

// IsPubKeyHashAddrID returns whether the id is an identifier known to prefix a
// pay-to-pubkey-hash address on any default or registered network.  This is
// used when decoding an address string into a specific address type.  It is up
// to the caller to check both this and IsScriptHashAddrID and decide whether an
// address is a pubkey hash address, script hash address, neither, or
// undeterminable (if both return true).
func IsPubKeyHashAddrID(id [2]byte) bool {
	_, ok := pubKeyHashAddrIDs[id]
	return ok
}

// IsPKHEdwardsAddrID returns whether the id is an identifier know to prefix a
// pay-to-pubkey-hash Edwards address.
func IsPKHEdwardsAddrID(id [2]byte) bool {
	_, ok := pkhEdwardsAddrIDs[id]
	return ok
}

// IsPKHSchnorrAddrID returns whether the id is an identifier know to prefix a
// pay-to-pubkey-hash secp256k1 Schnorr address.
func IsPKHSchnorrAddrID(id [2]byte) bool {
	_, ok := pkhSchnorrAddrIDs[id]
	return ok
}

// IsScriptHashAddrID returns whether the id is an identifier known to prefix a
// pay-to-script-hash address on any default or registered network.  This is
// used when decoding an address string into a specific address type.  It is up
// to the caller to check both this and IsPubKeyHashAddrID and decide whether an
// address is a pubkey hash address, script hash address, neither, or
// undeterminable (if both return true).
func IsScriptHashAddrID(id [2]byte) bool {
	_, ok := scriptHashAddrIDs[id]
	return ok
}

// HDPrivateKeyToPublicKeyID accepts a private hierarchical deterministic
// extended key id and returns the associated public key id.  When the provided
// id is not registered, the ErrUnknownHDKeyID error will be returned.
func HDPrivateKeyToPublicKeyID(id []byte) ([]byte, error) {
	if len(id) != 4 {
		return nil, ErrUnknownHDKeyID
	}

	var key [4]byte
	copy(key[:], id)
	pubBytes, ok := hdPrivToPubKeyIDs[key]
	if !ok {
		return nil, ErrUnknownHDKeyID
	}

	return pubBytes, nil
}

// newHashFromStr converts the passed big-endian hex string into a
// wire.Hash.  It only differs from the one available in wire in that
// it panics on an error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *chainhash.Hash {
	sha, err := chainhash.NewHashFromStr(hexStr)
	if err != nil {
		// Ordinarily I don't like panics in library code since it
		// can take applications down without them having a chance to
		// recover which is extremely annoying, however an exception is
		// being made in this case because the only way this can panic
		// is if there is an error in the hard-coded hashes.  Thus it
		// will only ever potentially panic on init and therefore is
		// 100% predictable.
		panic(err)
	}
	return sha
}

// BlockOneSubsidy returns the total subsidy of block height 1 for the
// network.
func (p *Params) BlockOneSubsidy() int64 {
	if len(p.BlockOneLedger) == 0 {
		return 0
	}

	sum := int64(0)
	for _, output := range p.BlockOneLedger {
		sum += output.Amount
	}

	return sum
}

// TotalSubsidyProportions is the sum of WorkReward, StakeReward, and BlockTax
// proportions.
func (p *Params) TotalSubsidyProportions() uint16 {
	return p.WorkRewardProportion + p.StakeRewardProportion + p.BlockTaxProportion
}

func init() {
	// Register all default networks when the package is initialized.
	mustRegister(&MainNetParams)
	mustRegister(&TestNetParams)
	mustRegister(&SimNetParams)
}
