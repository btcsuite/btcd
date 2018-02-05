// Copyright (c) 2016-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fullblocktests

import (
	"encoding/hex"
	"math"
	"math/big"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// newHashFromStr converts the passed big-endian hex string into a
// wire.Hash.  It only differs from the one available in chainhash in that
// it panics on an error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(hexStr)
	if err != nil {
		panic(err)
	}
	return hash
}

// fromHex converts the passed hex string into a byte slice and will panic if
// there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can be detected. It will only (and must only) be
// called for initialization purposes.
func fromHex(s string) []byte {
	r, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return r
}

var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// simNetPowLimit is the highest proof of work value a Decred block
	// can have for the simulation test network.  It is the value 2^255 - 1.
	simNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)

	// simNetGenesisBlock defines the genesis block of the block chain which serves
	// as the public transaction ledger for the simulation test network.
	simNetGenesisBlock = wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:     1,
			PrevBlock:   *newHashFromStr("0000000000000000000000000000000000000000000000000000000000000000"),
			MerkleRoot:  *newHashFromStr("66aa7491b9adce110585ccab7e3fb5fe280de174530cca10eba2c6c3df01c10d"),
			StakeRoot:   *newHashFromStr("0000000000000000000000000000000000000000000000000000000000000000"),
			VoteBits:    uint16(0x0000),
			FinalState:  [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			Voters:      uint16(0x0000),
			FreshStake:  uint8(0x00),
			Revocations: uint8(0x00),
			Timestamp:   time.Unix(1401292357, 0), // 2009-01-08 20:54:25 -0600 CST
			PoolSize:    uint32(0),
			Bits:        0x207fffff, // 545259519
			SBits:       int64(0x0000000000000000),
			Nonce:       0x00000000,
			Height:      uint32(0),
		},
		Transactions: []*wire.MsgTx{{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: []*wire.TxIn{{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0xffffffff,
				},
				SignatureScript: fromHex("04ffff001d010445" +
					"5468652054696d65732030332f4a616e2f" +
					"32303039204368616e63656c6c6f72206f" +
					"6e206272696e6b206f66207365636f6e64" +
					"206261696c6f757420666f72206261686b73"),
				Sequence: 0xffffffff,
			}},
			TxOut: []*wire.TxOut{{
				Value: 0,
				PkScript: fromHex("4104678afdb0fe5548271967f1" +
					"a67130b7105cd6a828e03909a67962e0ea1f" +
					"61deb649f6bc3f4cef38c4f35504e51ec138" +
					"c4f35504e51ec112de5c384df7ba0b8d578a" +
					"4c702b6bf11d5fac"),
			}},
			LockTime: 0,
			Expiry:   0,
		}},
		STransactions: nil,
	}
)

// simNetParams defines the network parameters for the simulation test Decred
// network.
//
// NOTE: The test generator intentionally does not use the existing definitions
// in the chaincfg package since the intent is to be able to generate known
// good tests which exercise that code.  Using the chaincfg parameters would
// allow them to change without the tests failing as desired.
var simNetParams = &chaincfg.Params{
	Name:        "simnet",
	Net:         wire.SimNet,
	DefaultPort: "18555",
	DNSSeeds:    nil, // NOTE: There must NOT be any seeds.

	// Chain parameters
	GenesisBlock:             &simNetGenesisBlock,
	GenesisHash:              newHashFromStr("5bec7567af40504e0994db3b573c186fffcc4edefe096ff2e58d00523bd7e8a6"),
	PowLimit:                 simNetPowLimit,
	PowLimitBits:             0x207fffff,
	ReduceMinDifficulty:      false,
	MinDiffReductionTime:     0, // Does not apply since ReduceMinDifficulty false
	GenerateSupported:        true,
	MaximumBlockSizes:        []int{1000000, 1310720},
	MaxTxSize:                1000000,
	TargetTimePerBlock:       time.Second,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       8,
	WorkDiffWindows:          4,
	TargetTimespan:           time.Second * 8, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:              50000000000,
	MulSubsidy:               100,
	DivSubsidy:               101,
	SubsidyReductionInterval: 128,
	WorkRewardProportion:     6,
	StakeRewardProportion:    3,
	BlockTaxProportion:       1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// BIP0009 consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationQuorum:     160, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
	RuleChangeActivationMultiplier: 3,   // 75%
	RuleChangeActivationDivisor:    4,
	RuleChangeActivationInterval:   320, // 320 seconds
	Deployments: map[uint32][]chaincfg.ConsensusDeployment{
		4: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDMaxBlockSize,
				Description: "Change maximum allowed block size from 1MiB to 1.25MB",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "reject changing max allowed block size",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "accept changing max allowed block size",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
		5: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDSDiffAlgorithm,
				Description: "Change stake difficulty algorithm as defined in DCP0001",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing algorithm",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new algorithm",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
		6: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDLNFeatures,
				Description: "Enable features defined in DCP0002 and DCP0003 necessary to support Lightning Network (LN)",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
	},

	// Enforce current block version once majority of the network has
	// upgraded.
	// 51% (51 / 100)
	// Reject previous block versions once a majority of the network has
	// upgraded.
	// 75% (75 / 100)
	BlockEnforceNumRequired: 51,
	BlockRejectNumRequired:  75,
	BlockUpgradeNumToCheck:  100,

	// AcceptNonStdTxs is a Mempool param to accept and relay non standard
	// txs to the network or reject them
	AcceptNonStdTxs: true,

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
	StakeBaseSigScript:    []byte{0xde, 0xad, 0xbe, 0xef},

	// Decred organization related parameters
	OrganizationPkScript:        fromHex("a914cbb08d6ca783b533b2c7d24a51fbca92d937bf9987"),
	OrganizationPkScriptVersion: 0,
	BlockOneLedger: []*chaincfg.TokenPayout{
		{Address: "Sshw6S86G2bV6W32cbc7EhtFy8f93rU6pae", Amount: 100000 * 1e8},
		{Address: "SsjXRK6Xz6CFuBt6PugBvrkdAa4xGbcZ18w", Amount: 100000 * 1e8},
		{Address: "SsfXiYkYkCoo31CuVQw428N6wWKus2ZEw5X", Amount: 100000 * 1e8},
	},
}
