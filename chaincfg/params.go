// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"encoding/hex"
	"errors"
	"fmt"
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

	VoteBitsNotFound = fmt.Errorf("vote bits not found")
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

// Vote describes a voting instance.  It is self-describing so that the UI can
// be directly implemented using the fields.  Mask determines which bits can be
// used.  Bits are enumerated and must be consecutive.  Each vote requires one
// and only one abstain (bits = 0) and reject vote (IsNo = true).
//
// For example, change block height from int64 to uint64.
// Vote {
//	Id:          "blockheight",
//	Description: "Change block height from int64 to uint64"
//	Mask:        0x0006,
//	Choices:     []Choice{
//		{
//			Id:          "abstain",
//			Description: "abstain voting for change",
//			Bits:        0x0000,
//			IsAbstain:   true,
//			IsNo:        false,
//		},
//		{
//			Id:          "no",
//			Description: "reject changing block height to uint64",
//			Bits:        0x0002,
//			IsAbstain:   false,
//			IsNo:        false,
//		},
//		{
//			Id:          "yes",
//			Description: "accept changing block height to uint64",
//			Bits:        0x0004,
//			IsAbstain:   false,
//			IsNo:        true,
//		},
//	},
// }
//
type Vote struct {
	// Single unique word identifying the vote.
	Id string

	// Longer description of what the vote is about.
	Description string

	// Usable bits for this vote.
	Mask uint16

	Choices []Choice
}

// Choice defines one of the possible Choices that make up a vote. The 0 value
// in Bits indicates the default choice.  Care should be taken not to bias a
// vote with the default choice.
type Choice struct {
	// Single unique word identifying vote (e.g. yes)
	Id string

	// Longer description of the vote.
	Description string

	// Bits used for this vote.
	Bits uint16

	// This is the abstain choice.  By convention this must be the 0 vote
	// (abstain) and exist only once in the Vote.Choices array.
	IsAbstain bool

	// This coince indicates a hard No Vote.  By convention this must exist
	// only once in the Vote.Choices array.
	IsNo bool
}

// VoteIndex compares vote to Choice.Bits and returns the index into the
// Choices array.  If the vote is invalid it returns -1.
func (v *Vote) VoteIndex(vote uint16) int {
	vote &= v.Mask
	for k := range v.Choices {
		if vote == v.Choices[k].Bits {
			return k
		}
	}

	return -1
}

const (
	// VoteIDMaxBlockSize is the vote ID for the the maximum block size
	// increase agenda used for the hard fork demo.
	VoteIDMaxBlockSize = "maxblocksize"

	// VoteIDSDiffAlgorithm is the vote ID for the new stake difficulty
	// algorithm (aka ticket price) agenda defined by DCP0001.
	VoteIDSDiffAlgorithm = "sdiffalgorithm"

	// VoteIDLNSupport is the vote ID for determining if the developers
	// should work on integrating Lightning Network support.
	VoteIDLNSupport = "lnsupport"

	// VoteIDLNFeatures is the vote ID for the agenda that introduces
	// features useful for the Lightning Network (among other uses) defined
	// by DCP0002 and DCP0003.
	VoteIDLNFeatures = "lnfeatures"
)

// ConsensusDeployment defines details related to a specific consensus rule
// change that is voted in.  This is part of BIP0009.
type ConsensusDeployment struct {
	// Vote describes the what is being voted on and what the choices are.
	// This is sitting in a struct in order to make merging between btcd
	// easier.
	Vote Vote

	// StartTime is the median block time after which voting on the
	// deployment starts.
	StartTime uint64

	// ExpireTime is the median block time after which the attempted
	// deployment expires.
	ExpireTime uint64
}

// TokenPayout is a payout for block 1 which specifies an address and an amount
// to pay to that address in a transaction output.
type TokenPayout struct {
	Address string
	Amount  int64
}

// DNSSeed identifies a DNS seed.
type DNSSeed struct {
	// Host defines the hostname of the seed.
	Host string

	// HasFiltering defines whether the seed supports filtering
	// by service flags (wire.ServiceFlag).
	HasFiltering bool
}

// Params defines a Decred network by its parameters.  These parameters may be
// used by Decred applications to differentiate networks as well as addresses
// and keys for one network from those intended for use on another network.
type Params struct {
	// Name defines a human-readable identifier for the network.
	Name string

	// Net defines the magic bytes used to identify the network.
	Net wire.CurrencyNet

	// DefaultPort defines the default peer-to-peer port for the network.
	DefaultPort string

	// DNSSeeds defines a list of DNS seeds for the network that are used
	// as one method to discover peers.
	DNSSeeds []DNSSeed

	// GenesisBlock defines the first block of the chain.
	GenesisBlock *wire.MsgBlock

	// GenesisHash is the starting block hash.
	GenesisHash *chainhash.Hash

	// PowLimit defines the highest allowed proof of work value for a block
	// as a uint256.
	PowLimit *big.Int

	// PowLimitBits defines the highest allowed proof of work value for a
	// block in compact form.
	PowLimitBits uint32

	// ReduceMinDifficulty defines whether the network should reduce the
	// minimum required difficulty after a long enough period of time has
	// passed without finding a block.  This is really only useful for test
	// networks and should not be set on a main network.
	ReduceMinDifficulty bool

	// MinDiffReductionTime is the amount of time after which the minimum
	// required difficulty should be reduced when a block hasn't been found.
	//
	// NOTE: This only applies if ReduceMinDifficulty is true.
	MinDiffReductionTime time.Duration

	// GenerateSupported specifies whether or not CPU mining is allowed.
	GenerateSupported bool

	// MaximumBlockSizes are the maximum sizes of a block that can be
	// generated on the network.  It is an array because the max block size
	// can be different values depending on the results of a voting agenda.
	// The first entry is the initial block size for the network, while the
	// other entries are potential block size changes which take effect when
	// the vote for the associated agenda succeeds.
	MaximumBlockSizes []int

	// MaxTxSize is the maximum number of bytes a serialized transaction can
	// be in order to be considered valid by consensus.
	MaxTxSize int

	// TargetTimePerBlock is the desired amount of time to generate each
	// block.
	TargetTimePerBlock time.Duration

	// WorkDiffAlpha is the stake difficulty EMA calculation alpha (smoothing)
	// value. It is different from a normal EMA alpha. Closer to 1 --> smoother.
	WorkDiffAlpha int64

	// WorkDiffWindowSize is the number of windows (intervals) used for calculation
	// of the exponentially weighted average.
	WorkDiffWindowSize int64

	// WorkDiffWindows is the number of windows (intervals) used for calculation
	// of the exponentially weighted average.
	WorkDiffWindows int64

	// TargetTimespan is the desired amount of time that should elapse
	// before the block difficulty requirement is examined to determine how
	// it should be changed in order to maintain the desired block
	// generation rate.  This value should correspond to the product of
	// WorkDiffWindowSize and TimePerBlock above.
	TargetTimespan time.Duration

	// RetargetAdjustmentFactor is the adjustment factor used to limit
	// the minimum and maximum amount of adjustment that can occur between
	// difficulty retargets.
	RetargetAdjustmentFactor int64

	// Subsidy parameters.
	//
	// Subsidy calculation for exponential reductions:
	// 0 for i in range (0, height / SubsidyReductionInterval):
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

	// SubsidyReductionInterval is the reduction interval in blocks.
	SubsidyReductionInterval int64

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

	// These fields are related to voting on consensus rule changes as
	// defined by BIP0009.
	//
	// RuleChangeActivationQurom is the number of votes required for a vote
	// to take effect.
	//
	// RuleChangeActivationInterval is the number of blocks in each threshold
	// state retarget window.
	//
	// Deployments define the specific consensus rule changes to be voted
	// on for the stake version (the map key).
	RuleChangeActivationQuorum     uint32
	RuleChangeActivationMultiplier uint32
	RuleChangeActivationDivisor    uint32
	RuleChangeActivationInterval   uint32
	Deployments                    map[uint32][]ConsensusDeployment

	// Enforce current block version once network has upgraded.
	BlockEnforceNumRequired uint64

	// Reject previous block versions once network has upgraded.
	BlockRejectNumRequired uint64

	// The number of nodes to check.
	BlockUpgradeNumToCheck uint64

	// AcceptNonStdTxs is a mempool param to either accept and relay
	// non standard txs to the network or reject them
	AcceptNonStdTxs bool

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

	// SLIP-0044 registered coin type used for BIP44, used in the hierarchical
	// deterministic path for address generation.
	// All SLIP-0044 registered coin types are are defined here:
	// https://github.com/satoshilabs/slips/blob/master/slip-0044.md
	SLIP0044CoinType uint32

	// Legacy BIP44 coin type used in the hierarchical deterministic path for
	// address generation. Previous name was HDCoinType, the LegacyCoinType
	// was introduced for backwards compatibility. Usually, SLIP0044CoinType
	// should be used instead.
	LegacyCoinType uint32

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

	// CoinbaseMaturity is the number of blocks required before newly mined
	// coins (coinbase transactions) can be spent.
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

	// StakeVersionInterval determines the interval where the stake version
	// is calculated.
	StakeVersionInterval int64

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

	// StakeMajorityMultiplier and StakeMajorityDivisor are used
	// to calculate the super majority of stake votes using integer math as
	// such: X*StakeMajorityMultiplier/StakeMajorityDivisor
	StakeMajorityMultiplier int32
	StakeMajorityDivisor    int32

	// OrganizationPkScript is the output script for block taxes to be
	// distributed to in every block's coinbase. It should ideally be a P2SH
	// multisignature address.  OrganizationPkScriptVersion is the version
	// of the output script.  Until PoS hardforking is implemented, this
	// version must always match for a block to validate.
	OrganizationPkScript        []byte
	OrganizationPkScriptVersion uint16

	// BlockOneLedger specifies the list of payouts in the coinbase of
	// block height 1. If there are no payouts to be given, set this
	// to an empty slice.
	BlockOneLedger []*TokenPayout
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

// String returns the hostname of the DNS seed in human-readable form.
func (d DNSSeed) String() string {
	return d.Host
}

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
// chainhash.Hash.  It only differs from the one available in chainhash in that
// it panics on an error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(hexStr)
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
	return hash
}

func hexDecode(hexStr string) []byte {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return b
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

// LatestCheckpointHeight is the height of the latest checkpoint block in the
// parameters.
func (p *Params) LatestCheckpointHeight() int64 {
	if len(p.Checkpoints) == 0 {
		return 0
	}
	return p.Checkpoints[len(p.Checkpoints)-1].Height
}

func init() {
	// Register all default networks when the package is initialized.
	mustRegister(&MainNetParams)
	mustRegister(&TestNet3Params)
	mustRegister(&SimNetParams)
}
