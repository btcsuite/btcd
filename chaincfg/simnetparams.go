// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"math"
	"time"

	"github.com/decred/dcrd/wire"
)

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
	DNSSeeds:    []DNSSeed{}, // NOTE: There must NOT be any seeds.

	// Chain parameters
	GenesisBlock:             &simNetGenesisBlock,
	GenesisHash:              &simNetGenesisHash,
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

	// Consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationQuorum:     160, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
	RuleChangeActivationMultiplier: 3,   // 75%
	RuleChangeActivationDivisor:    4,
	RuleChangeActivationInterval:   320, // 320 seconds
	Deployments: map[uint32][]ConsensusDeployment{
		4: {{
			Vote: Vote{
				Id:          VoteIDMaxBlockSize,
				Description: "Change maximum allowed block size from 1MiB to 1.25MB",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []Choice{{
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
			Vote: Vote{
				Id:          VoteIDSDiffAlgorithm,
				Description: "Change stake difficulty algorithm as defined in DCP0001",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []Choice{{
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
			Vote: Vote{
				Id:          VoteIDLNFeatures,
				Description: "Enable features defined in DCP0002 and DCP0003 necessary to support Lightning Network (LN)",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []Choice{{
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

	// AcceptNonStdTxs is a mempool param to either accept and relay
	// non standard txs to the network or reject them
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
	SLIP0044CoinType: 1,   // SLIP0044, Testnet (all coins)
	LegacyCoinType:   115, // ASCII for s, for backwards compatibility

	// Decred PoS parameters
	MinimumStakeDiff:        20000,
	TicketPoolSize:          64,
	TicketsPerBlock:         5,
	TicketMaturity:          16,
	TicketExpiry:            384, // 6*TicketPoolSize
	CoinbaseMaturity:        16,
	SStxChangeMaturity:      1,
	TicketPoolSizeWeight:    4,
	StakeDiffAlpha:          1,
	StakeDiffWindowSize:     8,
	StakeDiffWindows:        8,
	StakeVersionInterval:    8 * 2 * 7,
	MaxFreshStakePerBlock:   20,            // 4*TicketsPerBlock
	StakeEnabledHeight:      16 + 16,       // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight:   16 + (64 * 2), // CoinbaseMaturity + TicketPoolSize*2
	StakeBaseSigScript:      []byte{0xDE, 0xAD, 0xBE, 0xEF},
	StakeMajorityMultiplier: 3,
	StakeMajorityDivisor:    4,

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
	// Organization address is ScuQxvveKGfpG1ypt6u27F99Anf7EW3cqhq
	OrganizationPkScript:        hexDecode("a914cbb08d6ca783b533b2c7d24a51fbca92d937bf9987"),
	OrganizationPkScriptVersion: 0,
	BlockOneLedger:              BlockOneLedgerSimNet,
}
