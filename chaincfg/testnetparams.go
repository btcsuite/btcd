// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"time"

	"github.com/decred/dcrd/wire"
)

// TestNet2Params defines the network parameters for the test currency network.
// This network is sometimes simply called "testnet".
// This is the second public iteration of testnet.
var TestNet2Params = Params{
	Name:        "testnet2",
	Net:         wire.TestNet2,
	DefaultPort: "19108",
	DNSSeeds: []DNSSeed{
		{"testnet-seed.decred.mindcry.org", true},
		{"testnet-seed.decred.netpurgatory.com", true},
		{"testnet-seed.decred.org", true},
	},

	// Chain parameters
	GenesisBlock:             &testNet2GenesisBlock,
	GenesisHash:              &testNet2GenesisHash,
	PowLimit:                 testNetPowLimit,
	PowLimitBits:             0x1e00ffff,
	ReduceMinDifficulty:      false,
	MinDiffReductionTime:     0, // Does not apply since ReduceMinDifficulty false
	GenerateSupported:        true,
	MaximumBlockSizes:        []int{1310720},
	MaxTxSize:                1000000,
	TargetTimePerBlock:       time.Minute * 2,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       144,
	WorkDiffWindows:          20,
	TargetTimespan:           time.Minute * 2 * 144, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:              2500000000, // 25 Coin
	MulSubsidy:               100,
	DivSubsidy:               101,
	SubsidyReductionInterval: 2048,
	WorkRewardProportion:     6,
	StakeRewardProportion:    3,
	BlockTaxProportion:       1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: []Checkpoint{
		{12500, newHashFromStr("000000000046db2b18647632bac76577e418a5cdd8508a2f1cd82a6b30c3e854")},
		{25000, newHashFromStr("0000000000970b7f74178ba6bc3426cd2a65ab854c04e92f542567843f5612a2")},
		{37500, newHashFromStr("0000000000e5f9b3eb57259439694d3f12cd3b485cca54089fe3d4cc5c7c3e51")},
		{50000, newHashFromStr("0000000005bcc5dd36ba08523d32a3a581f1ef7376929f5b89757d1c9ced4154")},
		{62500, newHashFromStr("0000000003c0223971c732c49f019f449b494fdb822b67eb178fa4cf5d3b16ef")},
		{80000, newHashFromStr("0000000004239806fb02243757c0cd04f2103ad2c20d2afbdf21fafbd114ef60")},
		{97500, newHashFromStr("0000000003e41de65086786c253d2bf5259419cc15d1c1382b3d7bd69dcf7d45")},
		{110000, newHashFromStr("0000000003913d67af849f3dded4dd17038d366ff5c418be56f193ea574acf63")},
		{122500, newHashFromStr("0000000005db46602bc7146c87cd396db74696819c6685f0c61e9194e6278b07")},
		{140000, newHashFromStr("00000000015736a13fb25ef414947a8a7a4359ef5a00e3a03d6089f38f16f2de")},
		{157500, newHashFromStr("00000000052684525a3fedd619247b148eaa3ac38ab45781a1571099fd6036cf")},
		{175000, newHashFromStr("0000000000320a623fcc6453986ef13423f455ddf0f788ad1f2bda43858ccb8d")},
		{249802, newHashFromStr("0000000000153386623d86ce70cc9372fa000ac3b999eff11b9fc7a3ca0d072a")},
	},

	// Consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationQuorum:     2520, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
	RuleChangeActivationMultiplier: 3,    // 75%
	RuleChangeActivationDivisor:    4,
	RuleChangeActivationInterval:   5040, // 1 week
	Deployments: map[uint32][]ConsensusDeployment{
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
			StartTime:  1493164800, // Apr 26th, 2017
			ExpireTime: 1524700800, // Apr 26th, 2018
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
			StartTime:  1505260800, // Sep 13th, 2017
			ExpireTime: 1536796800, // Sep 13th, 2018
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
	MinimumStakeDiff:        20000000, // 0.2 Coin
	TicketPoolSize:          1024,
	TicketsPerBlock:         5,
	TicketMaturity:          16,
	TicketExpiry:            6144, // 6*TicketPoolSize
	CoinbaseMaturity:        16,
	SStxChangeMaturity:      1,
	TicketPoolSizeWeight:    4,
	StakeDiffAlpha:          1,
	StakeDiffWindowSize:     144,
	StakeDiffWindows:        20,
	StakeVersionInterval:    144 * 2 * 7, // ~1 week
	MaxFreshStakePerBlock:   20,          // 4*TicketsPerBlock
	StakeEnabledHeight:      16 + 16,     // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight:   768,         // Arbitrary
	StakeBaseSigScript:      []byte{0x00, 0x00},
	StakeMajorityMultiplier: 3,
	StakeMajorityDivisor:    4,

	// Decred organization related parameters.
	// Organization address is TccTkqj8wFqrUemmHMRSx8SYEueQYLmuuFk
	OrganizationPkScript:        hexDecode("4fa6cbd0dbe5ec407fe4c8ad374e667771fa0d44"),
	OrganizationPkScriptVersion: 0,
	BlockOneLedger:              BlockOneLedgerTestNet2,
}
