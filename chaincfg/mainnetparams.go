// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"time"

	"github.com/decred/dcrd/wire"
)

// MainNetParams defines the network parameters for the main Decred network.
var MainNetParams = Params{
	Name:        "mainnet",
	Net:         wire.MainNet,
	DefaultPort: "9108",
	DNSSeeds: []DNSSeed{
		{"mainnet-seed.decred.mindcry.org", true},
		{"mainnet-seed.decred.netpurgatory.com", true},
		{"mainnet-seed.decred.org", true},
	},

	// Chain parameters
	GenesisBlock:             &genesisBlock,
	GenesisHash:              &genesisHash,
	PowLimit:                 mainPowLimit,
	PowLimitBits:             0x1d00ffff,
	ReduceMinDifficulty:      false,
	MinDiffReductionTime:     0, // Does not apply since ReduceMinDifficulty false
	GenerateSupported:        false,
	MaximumBlockSizes:        []int{393216},
	MaxTxSize:                393216,
	TargetTimePerBlock:       time.Minute * 5,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       144,
	WorkDiffWindows:          20,
	TargetTimespan:           time.Minute * 5 * 144, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:              3119582664, // 21m
	MulSubsidy:               100,
	DivSubsidy:               101,
	SubsidyReductionInterval: 6144,
	WorkRewardProportion:     6,
	StakeRewardProportion:    3,
	BlockTaxProportion:       1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: []Checkpoint{
		{440, newHashFromStr("0000000000002203eb2c95ee96906730bb56b2985e174518f90eb4db29232d93")},
		{24480, newHashFromStr("0000000000000c9d4239c4ef7ef3fb5aaeed940244bc69c57c8c5e1f071b28a6")},
		{48590, newHashFromStr("0000000000000d5e0de21a96d3c965f5f2db2c82612acd7389c140c9afe92ba7")},
		{54770, newHashFromStr("00000000000009293d067b1126b7de07fc9b2b94ee50dfe0d48c239a7adb072c")},
		{60720, newHashFromStr("0000000000000a64475d68ffb9ad89a3d147c0f5138db26b40da9d19d0004117")},
		{65270, newHashFromStr("0000000000000021f107601962789b201f0a0cbb98ac5f8c12b93d94e795b441")},
		{75380, newHashFromStr("0000000000000e7d13cfc85806aa720fe3670980f5b7d33253e4f41985558372")},
		{85410, newHashFromStr("00000000000013ec928074bea6eac9754aa614c7acb20edf300f18b0cd122692")},
		{99880, newHashFromStr("0000000000000cb2a9a9ded647b9f78aae51ace32dd8913701d420ead272913c")},
		{123080, newHashFromStr("000000000000009ea6e02d0f0424f445ed50686f9ae4aecdf3b268e981114477")},
		{135960, newHashFromStr("00000000000001d2f9bbca9177972c0ba45acb40836b72945a75d73b99079498")},
		{139740, newHashFromStr("00000000000001397179ae1aff156fb1aea228938d06b83e43b78b1c44527b5b")},
		{155900, newHashFromStr("000000000000008557e37fb05177fc5a54e693de20689753639135f85a2dcb2e")},
		{164300, newHashFromStr("000000000000009ed067ff51cd5e15f3c786222a5183b20a991a80ce535907a9")},
		{181020, newHashFromStr("00000000000000b77d832cb2cbed02908d69323862a53e56345400ad81a6fb8f")},
		{189950, newHashFromStr("000000000000007341d8ae2ea7e41f25cee00e1a70a4a3dc1cb055d14ecb2e11")},
		{214672, newHashFromStr("0000000000000021d5cbeead55cb7fd659f07e8127358929ffc34cd362209758")},
		{259810, newHashFromStr("0000000000000000ee0fbf469a9f32477ffbb46ebd7a280a53c842ab4243f97c")},
	},

	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationQuorum:     4032, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
	RuleChangeActivationMultiplier: 3,    // 75%
	RuleChangeActivationDivisor:    4,
	RuleChangeActivationInterval:   2016 * 4, // 4 weeks
	Deployments: map[uint32][]ConsensusDeployment{
		4: {{
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
		}, {
			Vote: Vote{
				Id:          VoteIDLNSupport,
				Description: "Request developers begin work on Lightning Network (LN) integration",
				Mask:        0x0018, // Bits 3 and 4
				Choices: []Choice{{
					Id:          "abstain",
					Description: "abstain from voting",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "no, do not work on integrating LN support",
					Bits:        0x0008, // Bit 3
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "yes, begin work on integrating LN support",
					Bits:        0x0010, // Bit 4
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  1493164800, // Apr 26th, 2017
			ExpireTime: 1508976000, // Oct 26th, 2017
		}},
		5: {{
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
	// 75% (750 / 1000)
	// Reject previous block versions once a majority of the network has
	// upgraded.
	// 95% (950 / 1000)
	BlockEnforceNumRequired: 750,
	BlockRejectNumRequired:  950,
	BlockUpgradeNumToCheck:  1000,

	// AcceptNonStdTxs is a mempool param to either accept and relay
	// non standard txs to the network or reject them
	AcceptNonStdTxs: false,

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
	SLIP0044CoinType: 42, // SLIP0044, Decred
	LegacyCoinType:   20, // for backwards compatibility

	// Decred PoS parameters
	MinimumStakeDiff:        2 * 1e8, // 2 Coin
	TicketPoolSize:          8192,
	TicketsPerBlock:         5,
	TicketMaturity:          256,
	TicketExpiry:            40960, // 5*TicketPoolSize
	CoinbaseMaturity:        256,
	SStxChangeMaturity:      1,
	TicketPoolSizeWeight:    4,
	StakeDiffAlpha:          1, // Minimal
	StakeDiffWindowSize:     144,
	StakeDiffWindows:        20,
	StakeVersionInterval:    144 * 2 * 7, // ~1 week
	MaxFreshStakePerBlock:   20,          // 4*TicketsPerBlock
	StakeEnabledHeight:      256 + 256,   // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight:   4096,        // ~14 days
	StakeBaseSigScript:      []byte{0x00, 0x00},
	StakeMajorityMultiplier: 3,
	StakeMajorityDivisor:    4,

	// Decred organization related parameters
	// Organization address is Dcur2mcGjmENx4DhNqDctW5wJCVyT3Qeqkx
	OrganizationPkScript:        hexDecode("a914f5916158e3e2c4551c1796708db8367207ed13bb87"),
	OrganizationPkScriptVersion: 0,
	BlockOneLedger:              BlockOneLedgerMainNet,
}
