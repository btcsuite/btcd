// Copyright (c) 2014-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"encoding/hex"
	"errors"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
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

	// simNetPowLimit is the highest proof of work value a Bitcoin block
	// can have for the simulation test network.  It is the value 2^255 - 1.
	simNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)
)

// Checkpoint identifies a known good point in the block chain.  Using
// checkpoints allows a few optimizations for old blocks during initial download
// and also prevents forks from old blocks.
//
// Each checkpoint is selected based upon several factors.  See the
// documentation for blockchain.IsCheckpointCandidate for details on the
// selection criteria.
type Checkpoint struct {
	Height int32
	Hash   *chainhash.Hash
}

type UtreexoCheckpoint struct {
	Height int32
	Roots  []*chainhash.Hash
}

func newLeafHashFromStr(src string) *chainhash.Hash {
	// Hex decoder expects the hash to be a multiple of two.  When not, pad
	// with a leading zero.
	var srcBytes []byte
	if len(src)%2 == 0 {
		srcBytes = []byte(src)
	} else {
		srcBytes = make([]byte, 1+len(src))
		srcBytes[0] = '0'
		copy(srcBytes[1:], src)
	}

	// Hex decode the source bytes to a temporary destination.
	var reversedHash [32]byte
	_, err := hex.Decode(reversedHash[32-hex.DecodedLen(len(srcBytes)):], srcBytes)
	if err != nil {
		panic(err)
	}

	hash := chainhash.Hash(reversedHash)
	return &hash
}

// DNSSeed identifies a DNS seed.
type DNSSeed struct {
	// Host defines the hostname of the seed.
	Host string

	// HasFiltering defines whether the seed supports filtering
	// by service flags (wire.ServiceFlag).
	HasFiltering bool
}

// ConsensusDeployment defines details related to a specific consensus rule
// change that is voted in.  This is part of BIP0009.
type ConsensusDeployment struct {
	// BitNumber defines the specific bit number within the block version
	// this particular soft-fork deployment refers to.
	BitNumber uint8

	// StartTime is the median block time after which voting on the
	// deployment starts.
	StartTime uint64

	// ExpireTime is the median block time after which the attempted
	// deployment expires.
	ExpireTime uint64
}

// Constants that define the deployment offset in the deployments field of the
// parameters for each deployment.  This is useful to be able to get the details
// of a specific deployment by name.
const (
	// DeploymentTestDummy defines the rule change deployment ID for testing
	// purposes.
	DeploymentTestDummy = iota

	// DeploymentCSV defines the rule change deployment ID for the CSV
	// soft-fork package. The CSV package includes the deployment of BIPS
	// 68, 112, and 113.
	DeploymentCSV

	// DeploymentSegwit defines the rule change deployment ID for the
	// Segregated Witness (segwit) soft-fork package. The segwit package
	// includes the deployment of BIPS 141, 142, 144, 145, 147 and 173.
	DeploymentSegwit

	// NOTE: DefinedDeployments must always come last since it is used to
	// determine how many defined deployments there currently are.

	// DefinedDeployments is the number of currently defined deployments.
	DefinedDeployments
)

// Params defines a Bitcoin network by its parameters.  These parameters may be
// used by Bitcoin applications to differentiate networks as well as addresses
// and keys for one network from those intended for use on another network.
type Params struct {
	// Name defines a human-readable identifier for the network.
	Name string

	// Net defines the magic bytes used to identify the network.
	Net wire.BitcoinNet

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

	// These fields define the block heights at which the specified softfork
	// BIP became active.
	BIP0034Height int32
	BIP0065Height int32
	BIP0066Height int32

	// CoinbaseMaturity is the number of blocks required before newly mined
	// coins (coinbase transactions) can be spent.
	CoinbaseMaturity uint16

	// SubsidyReductionInterval is the interval of blocks before the subsidy
	// is reduced.
	SubsidyReductionInterval int32

	// TargetTimespan is the desired amount of time that should elapse
	// before the block difficulty requirement is examined to determine how
	// it should be changed in order to maintain the desired block
	// generation rate.
	TargetTimespan time.Duration

	// TargetTimePerBlock is the desired amount of time to generate each
	// block.
	TargetTimePerBlock time.Duration

	// RetargetAdjustmentFactor is the adjustment factor used to limit
	// the minimum and maximum amount of adjustment that can occur between
	// difficulty retargets.
	RetargetAdjustmentFactor int64

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

	// AssumeValid specifies all blocks before this will not have the signatures
	// checked
	AssumeValid *chainhash.Hash

	// Checkpoints ordered from oldest to newest.
	Checkpoints []Checkpoint

	// UtreexoCheckpoints ordered from oldest to newest
	UtreexoCheckpoints []UtreexoCheckpoint

	// These fields are related to voting on consensus rule changes as
	// defined by BIP0009.
	//
	// RuleChangeActivationThreshold is the number of blocks in a threshold
	// state retarget window for which a positive vote for a rule change
	// must be cast in order to lock in a rule change. It should typically
	// be 95% for the main network and 75% for test networks.
	//
	// MinerConfirmationWindow is the number of blocks in each threshold
	// state retarget window.
	//
	// Deployments define the specific consensus rule changes to be voted
	// on.
	RuleChangeActivationThreshold uint32
	MinerConfirmationWindow       uint32
	Deployments                   [DefinedDeployments]ConsensusDeployment

	// Mempool parameters
	RelayNonStdTxs bool

	// Human-readable part for Bech32 encoded segwit addresses, as defined
	// in BIP 173.
	Bech32HRPSegwit string

	// Address encoding magics
	PubKeyHashAddrID        byte // First byte of a P2PKH address
	ScriptHashAddrID        byte // First byte of a P2SH address
	PrivateKeyID            byte // First byte of a WIF private key
	WitnessPubKeyHashAddrID byte // First byte of a P2WPKH address
	WitnessScriptHashAddrID byte // First byte of a P2WSH address

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID [4]byte
	HDPublicKeyID  [4]byte

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType uint32
}

// MainNetParams defines the network parameters for the main Bitcoin network.
var MainNetParams = Params{
	Name:        "mainnet",
	Net:         wire.MainNet,
	DefaultPort: "8333",
	DNSSeeds: []DNSSeed{
		{"seed.bitcoin.sipa.be", true},
		{"dnsseed.bluematt.me", true},
		{"dnsseed.bitcoin.dashjr.org", false},
		{"seed.bitcoinstats.com", true},
		{"seed.bitnodes.io", false},
		{"seed.bitcoin.jonasschnelli.ch", true},
	},

	// Chain parameters
	GenesisBlock:             &genesisBlock,
	GenesisHash:              &genesisHash,
	PowLimit:                 mainPowLimit,
	PowLimitBits:             0x1d00ffff,
	BIP0034Height:            227931, // 000000000000024b89b42a942fe0d9fea3bb44ab7bd1b19115dd6a759c0808b8
	BIP0065Height:            388381, // 000000000000000004c2b624ed5d7756c508d90fd0da2c7c679febfa6c4735f0
	BIP0066Height:            363725, // 00000000000000000379eaa19dce8c9b722d46ae6a57c2f1a988119488b50931
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,
	TargetTimespan:           time.Hour * 24 * 14, // 14 days
	TargetTimePerBlock:       time.Minute * 10,    // 10 minutes
	RetargetAdjustmentFactor: 4,                   // 25% less, 400% more
	ReduceMinDifficulty:      false,
	MinDiffReductionTime:     0,
	GenerateSupported:        false,

	AssumeValid: newHashFromStr("0000000000000000000b9d2ec5a352ecba0592946514a92f14319dc2b367fc72"), // 654683

	// Checkpoints ordered from oldest to newest.
	Checkpoints: []Checkpoint{
		{11111, newHashFromStr("0000000069e244f73d78e8fd29ba2fd2ed618bd6fa2ee92559f542fdb26e7c1d")},
		{33333, newHashFromStr("000000002dd5588a74784eaa7ab0507a18ad16a236e7b1ce69f00d7ddfb5d0a6")},
		{74000, newHashFromStr("0000000000573993a3c9e41ce34471c079dcf5f52a0e824a81e7f953b8661a20")},
		{105000, newHashFromStr("00000000000291ce28027faea320c8d2b054b2e0fe44a773f3eefb151d6bdc97")},
		{134444, newHashFromStr("00000000000005b12ffd4cd315cd34ffd4a594f430ac814c91184a0d42d2b0fe")},
		{168000, newHashFromStr("000000000000099e61ea72015e79632f216fe6cb33d7899acb35b75c8303b763")},
		{193000, newHashFromStr("000000000000059f452a5f7340de6682a977387c17010ff6e6c3bd83ca8b1317")},
		{210000, newHashFromStr("000000000000048b95347e83192f69cf0366076336c639f9b7228e9ba171342e")},
		{216116, newHashFromStr("00000000000001b4f4b433e81ee46494af945cf96014816a4e2370f11b23df4e")},
		{225430, newHashFromStr("00000000000001c108384350f74090433e7fcf79a606b8e797f065b130575932")},
		{250000, newHashFromStr("000000000000003887df1f29024b06fc2200b55f8af8f35453d7be294df2d214")},
		// NOTE The commented out bits are to match bitcoind
		//{267300, newHashFromStr("000000000000000a83fbd660e918f218bf37edd92b748ad940483c7c116179ac")},
		{295000, newHashFromStr("00000000000000004d9b4ef50f0f9d686fd69db2e03af35a100370c64632a983")},
		//{279000, newHashFromStr("0000000000000001ae8c72a0b0c301f67e3afca10e819efa9041e458e9bd7e40")},
		//{300255, newHashFromStr("0000000000000000162804527c6e9b9f0563a280525f9d08c12041def0a0f3b2")},
		//{319400, newHashFromStr("000000000000000021c6052e9becade189495d1c539aa37c58917305fd15f13b")},
		//{343185, newHashFromStr("0000000000000000072b8bf361d01a6ba7d445dd024203fafc78768ed4368554")},
		//{352940, newHashFromStr("000000000000000010755df42dba556bb72be6a32f3ce0b6941ce4430152c9ff")},
		//{382320, newHashFromStr("00000000000000000a8dc6ed5b133d0eb2fd6af56203e4159789b092defd8ab2")},
		//{400000, newHashFromStr("000000000000000004ec466ce4732fe6f1ed1cddc2ed4b328fff5224276e3f6f")},
		//{430000, newHashFromStr("000000000000000001868b2bb3a285f3cc6b33ea234eb70facf4dcdf22186b87")},
		//{460000, newHashFromStr("000000000000000000ef751bbce8e744ad303c47ece06c8d863e4d417efc258c")},
		//{490000, newHashFromStr("000000000000000000de069137b17b8d5a3dfbd5b145b2dcfb203f15d0c4de90")},
		//{520000, newHashFromStr("0000000000000000000d26984c0229c9f6962dc74db0a6d525f2f1640396f69c")},
		//{550000, newHashFromStr("000000000000000000223b7a2298fb1c6c75fb0efc28a4c56853ff4112ec6bc9")},
		//{560000, newHashFromStr("0000000000000000002c7b276daf6efb2b6aa68e2ce3be67ef925b3264ae7122")},
	},

	// Consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationThreshold: 1916, // 95% of MinerConfirmationWindow
	MinerConfirmationWindow:       2016, //
	Deployments: [DefinedDeployments]ConsensusDeployment{
		DeploymentTestDummy: {
			BitNumber:  28,
			StartTime:  1199145601, // January 1, 2008 UTC
			ExpireTime: 1230767999, // December 31, 2008 UTC
		},
		DeploymentCSV: {
			BitNumber:  0,
			StartTime:  1462060800, // May 1st, 2016
			ExpireTime: 1493596800, // May 1st, 2017
		},
		DeploymentSegwit: {
			BitNumber:  1,
			StartTime:  1479168000, // November 15, 2016 UTC
			ExpireTime: 1510704000, // November 15, 2017 UTC.
		},
	},

	// Mempool parameters
	RelayNonStdTxs: false,

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "bc", // always bc for main net

	// Address encoding magics
	PubKeyHashAddrID:        0x00, // starts with 1
	ScriptHashAddrID:        0x05, // starts with 3
	PrivateKeyID:            0x80, // starts with 5 (uncompressed) or K (compressed)
	WitnessPubKeyHashAddrID: 0x06, // starts with p2
	WitnessScriptHashAddrID: 0x0A, // starts with 7Xh

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x88, 0xad, 0xe4}, // starts with xprv
	HDPublicKeyID:  [4]byte{0x04, 0x88, 0xb2, 0x1e}, // starts with xpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 0,
}

// RegressionNetParams defines the network parameters for the regression test
// Bitcoin network.  Not to be confused with the test Bitcoin network (version
// 3), this network is sometimes simply called "testnet".
var RegressionNetParams = Params{
	Name:        "regtest",
	Net:         wire.TestNet,
	DefaultPort: "18444",
	DNSSeeds:    []DNSSeed{},

	// Chain parameters
	GenesisBlock:             &regTestGenesisBlock,
	GenesisHash:              &regTestGenesisHash,
	PowLimit:                 regressionPowLimit,
	PowLimitBits:             0x207fffff,
	CoinbaseMaturity:         100,
	BIP0034Height:            100000000, // Not active - Permit ver 1 blocks
	BIP0065Height:            1351,      // Used by regression tests
	BIP0066Height:            1251,      // Used by regression tests
	SubsidyReductionInterval: 150,
	TargetTimespan:           time.Hour * 24 * 14, // 14 days
	TargetTimePerBlock:       time.Minute * 10,    // 10 minutes
	RetargetAdjustmentFactor: 4,                   // 25% less, 400% more
	ReduceMinDifficulty:      true,
	MinDiffReductionTime:     time.Minute * 20, // TargetTimePerBlock * 2
	GenerateSupported:        true,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// Consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationThreshold: 108, // 75%  of MinerConfirmationWindow
	MinerConfirmationWindow:       144,
	Deployments: [DefinedDeployments]ConsensusDeployment{
		DeploymentTestDummy: {
			BitNumber:  28,
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		},
		DeploymentCSV: {
			BitNumber:  0,
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		},
		DeploymentSegwit: {
			BitNumber:  1,
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires.
		},
	},

	// Mempool parameters
	RelayNonStdTxs: true,

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "bcrt", // always bcrt for reg test net

	// Address encoding magics
	PubKeyHashAddrID: 0x6f, // starts with m or n
	ScriptHashAddrID: 0xc4, // starts with 2
	PrivateKeyID:     0xef, // starts with 9 (uncompressed) or c (compressed)

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // starts with tprv
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // starts with tpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 1,
}

// TestNet3Params defines the network parameters for the test Bitcoin network
// (version 3).  Not to be confused with the regression test network, this
// network is sometimes simply called "testnet".
var TestNet3Params = Params{
	Name:        "testnet3",
	Net:         wire.TestNet3,
	DefaultPort: "18333",
	DNSSeeds: []DNSSeed{
		{"testnet-seed.bitcoin.jonasschnelli.ch", true},
		{"testnet-seed.bitcoin.schildbach.de", false},
		{"seed.tbtc.petertodd.org", true},
		{"testnet-seed.bluematt.me", false},
	},

	// Chain parameters
	GenesisBlock:             &testNet3GenesisBlock,
	GenesisHash:              &testNet3GenesisHash,
	PowLimit:                 testNet3PowLimit,
	PowLimitBits:             0x1d00ffff,
	BIP0034Height:            21111,  // 0000000023b3a96d3484e5abb3755c413e7d41500f8e2a5c3f0dd01299cd8ef8
	BIP0065Height:            581885, // 00000000007f6655f22f98e72ed80d8b06dc761d5da09df0fa1dc4be4f861eb6
	BIP0066Height:            330776, // 000000002104c8c45e99a8853285a3b592602a3ccde2b832481da85e9e4ba182
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,
	TargetTimespan:           time.Hour * 24 * 14, // 14 days
	TargetTimePerBlock:       time.Minute * 10,    // 10 minutes
	RetargetAdjustmentFactor: 4,                   // 25% less, 400% more
	ReduceMinDifficulty:      true,
	MinDiffReductionTime:     time.Minute * 20, // TargetTimePerBlock * 2
	GenerateSupported:        false,

	AssumeValid: newHashFromStr("000000000000006433d1efec504c53ca332b64963c425395515b01977bd7b3b0"), // 1864000

	// Checkpoints ordered from oldest to newest.
	Checkpoints: []Checkpoint{
		{546, newHashFromStr("000000002a936ca763904c3c35fce2f3556c559c0214345d31b1bcebf76acb70")},
		{100000, newHashFromStr("00000000009e2958c15ff9290d571bf9459e93b19765c6801ddeccadbb160a1e")},
		{200000, newHashFromStr("0000000000287bffd321963ef05feab753ebe274e1d78b2fd4e2bfe9ad3aa6f2")},
		{300001, newHashFromStr("0000000000004829474748f3d1bc8fcf893c88be255e6d7f571c548aff57abf4")},
		{400002, newHashFromStr("0000000005e2c73b8ecb82ae2dbc2e8274614ebad7172b53528aba7501f5a089")},
		{500011, newHashFromStr("00000000000929f63977fbac92ff570a9bd9e7715401ee96f2848f7b07750b02")},
		{600002, newHashFromStr("000000000001f471389afd6ee94dcace5ccc44adc18e8bff402443f034b07240")},
		{700000, newHashFromStr("000000000000406178b12a4dea3b27e13b3c4fe4510994fd667d7c1e6a3f4dc1")},
		{800010, newHashFromStr("000000000017ed35296433190b6829db01e657d80631d43f5983fa403bfdb4c1")},
		{900000, newHashFromStr("0000000000356f8d8924556e765b7a94aaebc6b5c8685dcfa2b1ee8b41acd89b")},
		{1000007, newHashFromStr("00000000001ccb893d8a1f25b70ad173ce955e5f50124261bbbc50379a612ddf")},
		{1100007, newHashFromStr("00000000000abc7b2cd18768ab3dee20857326a818d1946ed6796f42d66dd1e8")},
		{1200007, newHashFromStr("00000000000004f2dc41845771909db57e04191714ed8c963f7e56713a7b6cea")},
		{1300007, newHashFromStr("0000000072eab69d54df75107c052b26b0395b44f77578184293bf1bb1dbd9fa")},
	},

	// UtreexoCheckpoints ordered from oldest to newest.
	UtreexoCheckpoints: []UtreexoCheckpoint{
		{546, []*chainhash.Hash{
			newLeafHashFromStr("d3dd9e244260a179b99d190a919b99a35a550460cbdd605522713cfdbb98da44"),
			newLeafHashFromStr("ba94eda4d73a26dcc36051c387d7b9cfb352f6e5b68172687a796cce0fc2f6f5"),
			newLeafHashFromStr("208d8e603f7bc0afc1e5b8162560715669852123417cde86407f67f6d0179653"),
			newLeafHashFromStr("cf594417f83db3cdecea39d0d0c7b355a10ce59dc618a2f0de83b2cc05745ece"),
			newLeafHashFromStr("1272e2efc68eef67113ae3de1aae6c8597f9b71c19d3fe5265c205f7d3dda3c6"),
			newLeafHashFromStr("8559c31701c90db56ce0bd351291c125224e871f948159dc17ce154b35e89641"),
			newLeafHashFromStr("1ede449941ab9909b00cc30bd459eca2964d6f1643bfd526bbf2eb0a109045ad"),
		},
		},
		{100000, []*chainhash.Hash{
			newLeafHashFromStr("a5022fb8133f0e368f3833464851376862ffac6d031dc0b0a3b9eac21fadaa2f"),
			newLeafHashFromStr("6f9a213c6b8cd49730e120125fd3232f159990ad543374dae3b95289122ea598"),
			newLeafHashFromStr("a3969c93b208b4cd427edeb204af8d44ed053b2f899c021f02a2a294f6b58b90"),
			newLeafHashFromStr("d2b2086ab2e712e5fb3d642b6c1fef7f14a9eec2352887d9bcdd84a40bf8d3ba"),
			newLeafHashFromStr("a1116a8fce189e9d33ee073102f15b6365e0bea565b61f7bb2dcb45b227087e2"),
			newLeafHashFromStr("b97079e56528cebb7d2935d7c16fc063ac74f5aa39729193ce125a32beb846e9"),
			newLeafHashFromStr("16b52a4afe5b166ad7f5040a1c878d367d4b348fc573692d39926fb4d56cc900"),
			newLeafHashFromStr("ee0a20088aca5ff9cec6d6af2e5bb8e63d0fa4d877e9202236bbf4753ce85957"),
			newLeafHashFromStr("1f27a5bde3eac5cd6e6c9a382727dc440bec8e2d9995a0a668a4b5ce3cff5ffa"),
		},
		},
		{200000, []*chainhash.Hash{
			newLeafHashFromStr("76a7459e52f8a57375249f9b676662df1714a3fb19a0d99ed22323d6e5639e16"),
			newLeafHashFromStr("0b322297cf836f7ada7f0808ba6e8ce64501e47a94b167ee23de752efff47213"),
			newLeafHashFromStr("62ebde24676f5a22ae70a98d03436462615239cbeb04908e6ad0ed0f453d2a5c"),
			newLeafHashFromStr("ac55e96ead0a80f525db68498e66f46a91cefd956fead05e25bf61605ddef63b"),
			newLeafHashFromStr("f48f5845c35d5c5f68d45f05d1944f28a488f996f7c0ea0628e316332d626c66"),
			newLeafHashFromStr("4f9203ec947a2c7ac87f61ce70be5a8fe5020bfccc29aa0a937d9216131bc117"),
			newLeafHashFromStr("1220773164e0022319cc070346cce8ba51ce04fd639f19fd29bf5407547455bb"),
			newLeafHashFromStr("867b10ef41843011e9b63784c19629c23cbc4e1f0b7c8ee790c50b34e1edf6e4"),
			newLeafHashFromStr("2260611466c393ed35ada0cff5bd9a76948aca67badeef0580c5424758bc521b"),
			newLeafHashFromStr("38aa3b1da8cb6b783114b127070b57e2c71bfb554edf5cbb1a8be02bef4cf4e7"),
			newLeafHashFromStr("7cf02b8071b8316d090c94e66969fd7c07ef1e09fe48fd3f903c1de4e35abb18"),
			newLeafHashFromStr("f216fe2ce7971420f5690b8044706ef6799ef6d3027e6908ba575d58af51b09a"),
		},
		},
		{300001, []*chainhash.Hash{
			newLeafHashFromStr("98102da3148a3fbe002a3f5936e6b2f016f70f50e0d5e1957d3b7d5c08424dc9"),
			newLeafHashFromStr("ab97796d0f2dedcb0b1ea2d1f5bd9748a1e37580ae2c3db93c55e578e42aa468"),
			newLeafHashFromStr("14b1d6d402d67455a90f03424af76d47a80cefbcaf6cd3953585466a534ec09a"),
			newLeafHashFromStr("611e66b27ceb8687353cb7bcd7bbf4374f694f8e9c6fbf295f64c710924c6ceb"),
			newLeafHashFromStr("1f709ba42178cda3c2dbe8b55225b79c910c5af4beca76523d59d90ece180974"),
			newLeafHashFromStr("dfb1a55d3f84b442168251d41c16ddfe9b0422c3f2c4ab2a845b71970d232b6e"),
			newLeafHashFromStr("0437cc9e4deffcfc43e8e13a2164bc5e05b6ed938e6b7397a14000a4b2cb8154"),
		},
		},
		{400002, []*chainhash.Hash{
			newLeafHashFromStr("07f6f3d36faa4149a54ba323c591e5a26a10dba860f99d146617ca172cb9b272"),
			newLeafHashFromStr("74c04f11550aac83210692b919e82ad337fbeb503d84841c5f0ff682047b58f5"),
			newLeafHashFromStr("7449d3c9a2a9232ce2008709fbd491703ce021cc98cb18386cf7cca31cacfbfd"),
			newLeafHashFromStr("b8f1a8210f3ca0187d94d06cb2292af1c2daf4c5aeab3f5f8e3164fa095ba912"),
			newLeafHashFromStr("e4d9ec14070b4d42d0e95b7ac64f22a469609c14248c4cddb58d8a377d6413a8"),
			newLeafHashFromStr("6530becd070316cc45a24e16e7a5fc18bb422ba16debf1eb717ee53738638284"),
			newLeafHashFromStr("1eec24192ed46f5157c218b7e639bbeb18a7bcf9cf855a1ab5281a6b2d50cba6"),
			newLeafHashFromStr("5f86a3172985ef1e4279fe94720198f0d9e96937b99fa25237de549525411a8e"),
			newLeafHashFromStr("cd63e75ffffefebdeb9e304ab10538a8c4fb633b088062ce829148ab0ce74bf0"),
			newLeafHashFromStr("9d3b3b5f2804405c8dfd8d09176ea67c18b59075d8630acfe1faba6d5151064a"),
			newLeafHashFromStr("9adc181336e61ac3c7284428a86318e13684d524df32db6ba4a77a96cc48f9c6"),
			newLeafHashFromStr("7e8fd69d384acf86d8b26e8ef6389c3d2a51a1ee802b1d4124b3f09a4a5ef9e0"),
			newLeafHashFromStr("4c1c0557e5a1227a34368fe495d3a6580790e363d80ea9e4b43d413613b86be2"),
		},
		},
		{500011, []*chainhash.Hash{
			newLeafHashFromStr("4a44382edeb7d5bf3a90dc3cdc5b72544c43f1b85c7c070601f2d11dd9702ef4"),
			newLeafHashFromStr("5981703a91a370805696a4bcd81e45337ced8f6b90a84f84f7acd5e56f1be4fe"),
			newLeafHashFromStr("6277bba84d97ea2d9df0c242c4c7f5f1da43cd29d326eb9a4cf745c44addb823"),
			newLeafHashFromStr("36bc8413c43127c3ffabb9ef6ca0cfd0c0affc22a296cc5488abaf303c40d354"),
			newLeafHashFromStr("c5632f0d2f126f3de784b9fa2846a11c8833ba02c820eafa268f83bf3f1d4812"),
			newLeafHashFromStr("bf3a2ff656e6d98b3d5efdb07561c2aa0ac913823d3ec74e50ef52831a8d73b7"),
			newLeafHashFromStr("564c27737949662f464d194821d7d024274862279fb66e80572fdb8dba87ba4d"),
			newLeafHashFromStr("f3ae5ac482b0f5a336e60eec546a86cd2ab0d7adb013026ae47920003ebd6b90"),
			newLeafHashFromStr("14e7815111fb9646f0c2d27125633b1032e528461bfeea56127009607d513263"),
			newLeafHashFromStr("093d329c1d035a67f3762aa679a1e6035142e01d5e148c4b072c214107109f90"),
			newLeafHashFromStr("9572b94d7a61d25ff21461d6017c21350e72d48754812405f65ffc8c1c589698"),
			newLeafHashFromStr("3d5d269bfacdd6d00bef0ade2591a053a51de58c7545828d97da326ebf0b5847"),
			newLeafHashFromStr("84431e17a7532f23aabaeb1b50fc3ee38374da20e850f895f83eedd9f60b8808"),
			newLeafHashFromStr("e42c0794eb82dde5d3176df23beabbd9850ee26548dc364c029db50cd0a22420"),
			newLeafHashFromStr("3dc2872e6d88c2644cb537d36ad20ae0de21a1a39c66363af52265bf818f4132"),
		},
		},
		{600002, []*chainhash.Hash{
			newLeafHashFromStr("08e77351f9054d9edb1565551edf6ea2a48e2dafe9bb02e784c829f667c2e353"),
			newLeafHashFromStr("db9b9004eb458af8b1d9af0309efc91353da9717afae69bcf6875cde6827ebe6"),
			newLeafHashFromStr("fb6ee7fd51f34bb54368c5b215a8e282af9584915cca51af65da110c67828eb5"),
			newLeafHashFromStr("7e0bbee0703a2d82cc768c9b2d93533ca6d7b4319e01512c922d79cae3642b9a"),
			newLeafHashFromStr("b3110d7cbee0970c72304e6b95416fa0a4ed84a7506b4d227e3ea941c1a8767b"),
			newLeafHashFromStr("584a07939ed538e97982c33f090f297fe04ed9ad727e22317e4d4b0f76e0ccc8"),
			newLeafHashFromStr("55310ea50c2361da6e844a5bf4904d8bc5d9eb667fd41c1f459e2879eb3b9383"),
			newLeafHashFromStr("b379701f8f70838676c21a357292d4d0eb2e9c835bb13f514a8abf5b17f42806"),
			newLeafHashFromStr("a2ce9f6088b40a67d727b3e010b43cd4579481ead0528f156fbb9ecceac45894"),
			newLeafHashFromStr("f7edecc27f7076022f62e8e93b9d67af2991f4313fd33db8cc4c56d262300aa1")},
		},
		{700000, []*chainhash.Hash{
			newLeafHashFromStr("e46f94d5e6575aa4160d780666d7adbcae783e84ccbf2c38e722c43613a7a221"),
			newLeafHashFromStr("814045182fefd402a7d5a01b918cce7ac9bf5eb97c2e60bf9459aae39179bbb2"),
			newLeafHashFromStr("ab8332cde688fd696609927543f82e75b3aec4159a864bb9448e8c6f626c6233"),
			newLeafHashFromStr("36464043ae2d3e67162a3053128fed403427e102786ff0da1f0f77c2e0aacaa7"),
			newLeafHashFromStr("e5974fb370453cda4fcbb83930a3383d9b6e325d7c313e0205afe0aed3085b21"),
			newLeafHashFromStr("c139bbdfd7fc8b8a09b8b7a552a3dcc34a1f88eaf80efb2f43bdbcb57d45226c"),
			newLeafHashFromStr("11df3ae9ad2ea67c5690008e312ace5b84bd52f29bef8c38bc85922d8fee73e2"),
			newLeafHashFromStr("cfd14fcdc0b77ad890801354c70cd45ffa562ce38821b194c34e67c41c3fd134"),
			newLeafHashFromStr("c8ff6964fc1c69a645f88ec5854b87b20ebba52e5765971de987325ca5c9244a"),
			newLeafHashFromStr("f11cdf30eaaeb1e0496d83be37073978e46ef733dfe42972abac8c8d9d50f191"),
			newLeafHashFromStr("2763b6bfe2e1c7c1475ee93f274233ab7c39bb70686741f3bd34bc9e5d64b65b"),
			newLeafHashFromStr("73ab0440b86180155341399fecf28b4192372a5e3b51fa99ff030613599062d4"),
			newLeafHashFromStr("c1c37e2946aaeacaae3962bc5f773f044d828cd1f2ef373dcf334ba67d5bf45b"),
			newLeafHashFromStr("77f077b0d5e2cf86c848f4edb44e890bba131969cc79aeb5ff3023adf064d413"),
			newLeafHashFromStr("1418d924740cace1f126b2e134ff7243ef7f4b3134a6b08787cc651d91fe103e"),
			newLeafHashFromStr("0a15659f5c1cee481ccd0ea2fbd7de8ac09d9686ec515023c5500e591863483f")},
		},
		{800010, []*chainhash.Hash{
			newLeafHashFromStr("426b44898b7a407d2335bfbe632c8713832219701fe17313c93f5654ad4b13a4"),
			newLeafHashFromStr("e466906ff6c2c8feb9f4bfc3c95bd06dd913786fa1d18dc0bdc6209ec1e8c526"),
			newLeafHashFromStr("7117325db9d84a81ce14c00c3a09a9ae7eaa9949de9f91c231b69ea26b454a9e"),
			newLeafHashFromStr("91e87483dde0425d97b3632ea055b15a722573c8fdd4c89f27ba9892a3464722"),
			newLeafHashFromStr("031b2172f8d43c54ab86af0b0ce1cd07c0dc63ad41f25caa9c48b49aa43c8583"),
			newLeafHashFromStr("fadf5577c58ae38dd93def7a2965d60902c26f811f56c8f503cb66c98483af0b"),
			newLeafHashFromStr("2a14e8272ea3800cecbf2253e52fefe55449ffb8d0b5d628bbf5b5b77da27a62"),
			newLeafHashFromStr("4d6053ab61928e59e6f9bf1988944ad1618c51cd0a7fab546e25ea6b5194f88a"),
			newLeafHashFromStr("37779fd07359552ad583ff4b8306f2a0698ff9327886ac55f89d19a3212b77d4"),
			newLeafHashFromStr("3ac4f6890e630c7e20cba28be86562428351003173b949ac9d3bdd1ecfa9d1e8"),
			newLeafHashFromStr("a991203d74f53a0f73268b9ea339196efd59f4383d15c21b840f298e9e586a6b"),
			newLeafHashFromStr("e3fdc3f844a41c4d147e0f1cf913230e6f47c7db3518f39bc7ca088991fd9a19")},
		},
		{900000, []*chainhash.Hash{
			newLeafHashFromStr("f245f9a6e5721fc629b62bbf6071579217441270fdf799e9ee1a100823e42932"),
			newLeafHashFromStr("d20cce9e7e0d39854f4a315ec72e5278772da80385b55c4bad746984c3f2de33"),
			newLeafHashFromStr("4ce9431d73493546942e4488c72feebde655de9e7797344ba7c7019659d2cc1b"),
			newLeafHashFromStr("5a01e9605a76dcda76df762638cd2df2ed7c6926ade46c032eafb1645e018459"),
			newLeafHashFromStr("b4be81279c93f1ed0f5d9e30cbfb669d84ce05a6ed76f77850229ffcbb3c3728"),
			newLeafHashFromStr("2cc2c800fe82d344ecf81de21bc79f627503278392b9c167378cbb99efea0104"),
			newLeafHashFromStr("bb31daa7453395710fa6a94de538e905610af5cfa58640b1670c10faffd9b313"),
			newLeafHashFromStr("e7204406399929d0a63e625d447ca0adb53643e4f0c658e407f049b89aef2695"),
			newLeafHashFromStr("2b7ad04f3ec2ac6752e6c9b26e9afd38f974c21d00d982471425a12392582c69"),
			newLeafHashFromStr("75e728fc005dbf8e3b5594354c82163f958b89bf9c41cbba84fc5671aa1cac90"),
			newLeafHashFromStr("80fe0d34821964812cd003d116d28a671d8a9b3243412a76c6ac79bbe65bf1c0"),
			newLeafHashFromStr("45b28726191279700fd3afcdfba2842dc16aa9731ba84346893195901d41530a"),
			newLeafHashFromStr("477d96fb9eecbbad75f3d492d730fe49e95f5309e8e977cdcf1a3808919101dd"),
			newLeafHashFromStr("07c09cf14d25281e4b4e11b01886ef8a1edf264a8ba8c1daf41116f48efad173"),
			newLeafHashFromStr("6bba999feb2d9bcf660c10410891912e4175bab789401694bac87d0dd3bb9e31"),
			newLeafHashFromStr("a02a925be262d32276fb6b63300a3b71341386d8ca7fb39a4715e29cc52966e9"),
			newLeafHashFromStr("21c47be845dafdfd0d5b60c2c1cf548bc396dbf710e61949a2392e4573af3100"),
			newLeafHashFromStr("ee6941332c286a102a420add2502adb7851beb7d5d55b655847c8465cd4f76e5")},
		},
		{1000007, []*chainhash.Hash{
			newLeafHashFromStr("5f71991360396d091c8a516429436d7336d361e35c7c581a115cfba86a7f2cc9"),
			newLeafHashFromStr("7f2798436c4d5c3c58a6ffbd3537fd5d9e5e9c10eba4b1055a251ade69068210"),
			newLeafHashFromStr("91e19af265c809dd079bd33e5e8c091fe504b7ad65e4b978d2ea14b5fc2a95cb"),
			newLeafHashFromStr("9ffefa0896eeca6da51cdffb4581efa9ae3ef7be7d3537ec3f3c40f8df1317af"),
			newLeafHashFromStr("ea829a92b6659eef22f4b47de0b6b61e4e74dbe2adbb6e63f12558026e1cface"),
			newLeafHashFromStr("9299a3464df3904a4ea9bad038b8d7e54f9c1743160801c8af86deb98528a124"),
			newLeafHashFromStr("845a2c4104b8166f7c3f1cc485ede25c81153a4def06927576ff51a6f0b72fad"),
			newLeafHashFromStr("39fa3ff40fc3b99738e371e2729542de3b16409f5fc7c77490c58e68185dff5e"),
			newLeafHashFromStr("e6031dc95a1e611b681d130843c54d4a6d814159005e6d401cc804ec7d65315f"),
			newLeafHashFromStr("d223e3017fd40a4fc31d0d7cd3ec7b22fdeb16e200abbf9a97aa171108caab3e"),
			newLeafHashFromStr("cac25a0f606b5021b8ede6b7fe640ce787d3f2902c1b31d082a5ad54eebf6389"),
			newLeafHashFromStr("014c445e0f662ceebf7d765cadf4ac512280c1173066b6729bf838c054eaccc7"),
			newLeafHashFromStr("50b03d53dd5ca83fd032f2eca88d8eeca1cf8df56ec5adc28e37701d3acc981e")},
		},
		{1100007, []*chainhash.Hash{
			newLeafHashFromStr("327aa52bc72d9be9664d1642ae34fac76edb7eb29da06701f3ce89ee917b2376"),
			newLeafHashFromStr("e22380ee9b87e7f00bffe940a25dddcd33598968d8293fd6449c6739beb7e218"),
			newLeafHashFromStr("278d0307b759b937f93d93c6403df4e3d58ef78d8424e97dbf1f07c9d0391934"),
			newLeafHashFromStr("7cce6c06fb2e1c1c9d3d891feb7bea33fe0b8d08c0569cb09bad0e41d1dd65c9"),
			newLeafHashFromStr("fddaebee2b76df65a18f194197019a4981882aca488560853a11d1fb2fd4a243"),
			newLeafHashFromStr("84320c1e886b54c027f41be79ac064e2b0b38ca009b1abc1e6d60bf9edd54640"),
			newLeafHashFromStr("d5f0f577e356bf36ce6763243869a9708832d8c67a278b74d0dba8837169a89a"),
			newLeafHashFromStr("c210af493c08635dd3dca6627c78c7ccf77d64d81e6d2058fe5f3c056cdab23f"),
			newLeafHashFromStr("f15d810e31fe48977f1b126a660b390db1e03b0146ec71ade75ab19c17b05d37"),
			newLeafHashFromStr("eb100b59696b66043b49638fdfa327f6e6d511078e013b362028d38c2913f49f"),
			newLeafHashFromStr("662956d1aa78a9920715f209bd506fd84cad36c9435bbfba2d2e51de5c55d4d1"),
			newLeafHashFromStr("9623945850a87acc64a6263d22d0e11c2cf0ab91c3f1b7f8afdc6f385ff86084"),
			newLeafHashFromStr("14f3f3ffd9f619c4d5fe25a41085aed33bca35a45cef8ca3ca702cd64049c5dc"),
			newLeafHashFromStr("87357b4f8364addea212c27554b34d65e7d35204f6bf6837932060032869676e"),
			newLeafHashFromStr("7bb449e2c30b798db6243483b61c91b6f69475aa28b7187a83fa20c643a5e940"),
			newLeafHashFromStr("e8a03e1996583a20c9a76ab0aaec493200b7a2cebb0109606adf30cabf398494"),
			newLeafHashFromStr("7938b7dc6f6f85a9a0fdec0501f0624d96f8362a213d1f074fbeae196d02802a"),
			newLeafHashFromStr("a03ebac79ddefccb3e0b060e4da0e988c69bec51756ba133d26ac5fa3e95b94d")},
		},
		{1200007, []*chainhash.Hash{
			newLeafHashFromStr("d622e48ae4f833ff6ae2ad539169f256c675f6416f210d00f66e711f589c0d74"),
			newLeafHashFromStr("c957e0328a0ea57fffed0b683c7a2f1eb8527e7034063e8d1977b64286427ec3"),
			newLeafHashFromStr("82385f5d0b70c434a4cb21d187bf98330929e120cbaa2fba0419a2a105000522"),
			newLeafHashFromStr("5fae0ee12d9c0ff42f9b2681bd8219cae7a61cc9c1c07b3714a1b3ee6eef5ac3"),
			newLeafHashFromStr("88e734bb9cd1e925518264993c8c4c85c43fa2fd532b81afc864afb0059b0681"),
			newLeafHashFromStr("7197456c1da8d7f8e251f3f3ca291201c78332e051fe1d320a886b061b144d2b"),
			newLeafHashFromStr("52aeb17e5c510046f969b636f024c9306261f68c7300b0b45bf1352f3c6bb301"),
			newLeafHashFromStr("6bd2ab1a00692bff2a53f9a2031a671e3a0d8ac55bd49a99e9d537ebacb035aa"),
			newLeafHashFromStr("b423daea8cf00e7de881edd71a4eb70ee3579eac7af61c070c87b06f9438936d"),
			newLeafHashFromStr("59617bf39c97a4607dc7530983727cfd124a878dd1ccbf23645a9d6b949d4c5c"),
			newLeafHashFromStr("35bd621f0eda28cb97bf34ee58d1bcfea1b3ac56a0e9344f6c66ec3cb039fa14"),
			newLeafHashFromStr("9b5ce9c13ad0d5a8650652450091b763b1f172d29275d29853b92940b1e305c4"),
			newLeafHashFromStr("434aceb53e78fbf3d450587347321ef4916d5fa910499f183616e096eb48f718"),
			newLeafHashFromStr("eb48720e3c71aa4b9bd121f5c59beaeb12a089990eecfd7190e880d0525d8c8c")},
		},
		{1300007, []*chainhash.Hash{
			newLeafHashFromStr("c34c928ea62b76fc2bdab5601ea3f12f9df4be5fd8bde3f4438156c10d11ad43"),
			newLeafHashFromStr("17ad4b919d18dd7ca7c89ebed959645180bbb0ddd1e5d25fadcd61fe64b331d0"),
			newLeafHashFromStr("c5d4b0e399a7143d817c255763b1e6843f6973704cda645f0d8e65f5c5f43219"),
			newLeafHashFromStr("72b7bbe46fe5dee8d571e377c72e98e6feb5329ec6ad0964a2a54d0955301b30"),
			newLeafHashFromStr("38e912b09e7336df4a3382f00d795794963e18d8ba61d94c5e674471704297df"),
			newLeafHashFromStr("df938a68ffa902cbb86ea56c58780ceb9aff9546fab17ff5b97327866b6eea60"),
			newLeafHashFromStr("572046b8b80a35c7198a1bb5e18597d47359890351d070476b4eb352a113daff"),
			newLeafHashFromStr("78c49f7d9fb05a8be6abb2abe7e93c7ed32ac537a3029a7a033709455d9908f5"),
			newLeafHashFromStr("ef287ac9698077bcefd4ba727ee2c7d91b0b9479377c09f047f0907bdaac0f8f"),
			newLeafHashFromStr("20a869f0b6d1156fc3d74229190722a72328edf813f93b2483021dfe6e231d2f"),
			newLeafHashFromStr("ef97f96c4ba864dff96193cee5ef37e391a9b324b7983e186dc4eed3a6059b41"),
			newLeafHashFromStr("41bf2e7fe8bdc7707ecdb43d03f81747761303fb3f6b905756e279f1e91c07fa"),
			newLeafHashFromStr("753bd25a677efe322cbb6ab590f49392fe75f57527ba7e22210643c4781cfcd9"),
			newLeafHashFromStr("4c8e7547caf753d663b91927859bba816fca871a164e260ba4597b4e8bb87b7f"),
			newLeafHashFromStr("703cc23bcd356694a6767c94e0169d29d47ec6a576e70bc52ac88b0cbdecb03b")},
		},
	},

	// Consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationThreshold: 1512, // 75% of MinerConfirmationWindow
	MinerConfirmationWindow:       2016,
	Deployments: [DefinedDeployments]ConsensusDeployment{
		DeploymentTestDummy: {
			BitNumber:  28,
			StartTime:  1199145601, // January 1, 2008 UTC
			ExpireTime: 1230767999, // December 31, 2008 UTC
		},
		DeploymentCSV: {
			BitNumber:  0,
			StartTime:  1456790400, // March 1st, 2016
			ExpireTime: 1493596800, // May 1st, 2017
		},
		DeploymentSegwit: {
			BitNumber:  1,
			StartTime:  1462060800, // May 1, 2016 UTC
			ExpireTime: 1493596800, // May 1, 2017 UTC.
		},
	},

	// Mempool parameters
	RelayNonStdTxs: true,

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "tb", // always tb for test net

	// Address encoding magics
	PubKeyHashAddrID:        0x6f, // starts with m or n
	ScriptHashAddrID:        0xc4, // starts with 2
	WitnessPubKeyHashAddrID: 0x03, // starts with QW
	WitnessScriptHashAddrID: 0x28, // starts with T7n
	PrivateKeyID:            0xef, // starts with 9 (uncompressed) or c (compressed)

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // starts with tprv
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // starts with tpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 1,
}

// SimNetParams defines the network parameters for the simulation test Bitcoin
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
	BIP0034Height:            0, // Always active on simnet
	BIP0065Height:            0, // Always active on simnet
	BIP0066Height:            0, // Always active on simnet
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,
	TargetTimespan:           time.Hour * 24 * 14, // 14 days
	TargetTimePerBlock:       time.Minute * 10,    // 10 minutes
	RetargetAdjustmentFactor: 4,                   // 25% less, 400% more
	ReduceMinDifficulty:      true,
	MinDiffReductionTime:     time.Minute * 20, // TargetTimePerBlock * 2
	GenerateSupported:        true,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// Consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationThreshold: 75, // 75% of MinerConfirmationWindow
	MinerConfirmationWindow:       100,
	Deployments: [DefinedDeployments]ConsensusDeployment{
		DeploymentTestDummy: {
			BitNumber:  28,
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		},
		DeploymentCSV: {
			BitNumber:  0,
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		},
		DeploymentSegwit: {
			BitNumber:  1,
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires.
		},
	},

	// Mempool parameters
	RelayNonStdTxs: true,

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "sb", // always sb for sim net

	// Address encoding magics
	PubKeyHashAddrID:        0x3f, // starts with S
	ScriptHashAddrID:        0x7b, // starts with s
	PrivateKeyID:            0x64, // starts with 4 (uncompressed) or F (compressed)
	WitnessPubKeyHashAddrID: 0x19, // starts with Gg
	WitnessScriptHashAddrID: 0x28, // starts with ?

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x20, 0xb9, 0x00}, // starts with sprv
	HDPublicKeyID:  [4]byte{0x04, 0x20, 0xbd, 0x3a}, // starts with spub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 115, // ASCII for s
}

var (
	// ErrDuplicateNet describes an error where the parameters for a Bitcoin
	// network could not be set due to the network already being a standard
	// network or previously-registered into this package.
	ErrDuplicateNet = errors.New("duplicate Bitcoin network")

	// ErrUnknownHDKeyID describes an error where the provided id which
	// is intended to identify the network for a hierarchical deterministic
	// private extended key is not registered.
	ErrUnknownHDKeyID = errors.New("unknown hd private extended key bytes")

	// ErrInvalidHDKeyID describes an error where the provided hierarchical
	// deterministic version bytes, or hd key id, is malformed.
	ErrInvalidHDKeyID = errors.New("invalid hd extended key version bytes")
)

var (
	registeredNets       = make(map[wire.BitcoinNet]struct{})
	pubKeyHashAddrIDs    = make(map[byte]struct{})
	scriptHashAddrIDs    = make(map[byte]struct{})
	bech32SegwitPrefixes = make(map[string]struct{})
	hdPrivToPubKeyIDs    = make(map[[4]byte][]byte)
)

// String returns the hostname of the DNS seed in human-readable form.
func (d DNSSeed) String() string {
	return d.Host
}

// Register registers the network parameters for a Bitcoin network.  This may
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
	pubKeyHashAddrIDs[params.PubKeyHashAddrID] = struct{}{}
	scriptHashAddrIDs[params.ScriptHashAddrID] = struct{}{}

	err := RegisterHDKeyID(params.HDPublicKeyID[:], params.HDPrivateKeyID[:])
	if err != nil {
		return err
	}

	// A valid Bech32 encoded segwit address always has as prefix the
	// human-readable part for the given net followed by '1'.
	bech32SegwitPrefixes[params.Bech32HRPSegwit+"1"] = struct{}{}
	return nil
}

// mustRegister performs the same function as Register except it panics if there
// is an error.  This should only be called from package init functions.
func mustRegister(params *Params) {
	if err := Register(params); err != nil {
		panic("failed to register network: " + err.Error())
	}
}

// IsPubKeyHashAddrID returns whether the id is an identifier known to prefix a
// pay-to-pubkey-hash address on any default or registered network.  This is
// used when decoding an address string into a specific address type.  It is up
// to the caller to check both this and IsScriptHashAddrID and decide whether an
// address is a pubkey hash address, script hash address, neither, or
// undeterminable (if both return true).
func IsPubKeyHashAddrID(id byte) bool {
	_, ok := pubKeyHashAddrIDs[id]
	return ok
}

// IsScriptHashAddrID returns whether the id is an identifier known to prefix a
// pay-to-script-hash address on any default or registered network.  This is
// used when decoding an address string into a specific address type.  It is up
// to the caller to check both this and IsPubKeyHashAddrID and decide whether an
// address is a pubkey hash address, script hash address, neither, or
// undeterminable (if both return true).
func IsScriptHashAddrID(id byte) bool {
	_, ok := scriptHashAddrIDs[id]
	return ok
}

// IsBech32SegwitPrefix returns whether the prefix is a known prefix for segwit
// addresses on any default or registered network.  This is used when decoding
// an address string into a specific address type.
func IsBech32SegwitPrefix(prefix string) bool {
	prefix = strings.ToLower(prefix)
	_, ok := bech32SegwitPrefixes[prefix]
	return ok
}

// RegisterHDKeyID registers a public and private hierarchical deterministic
// extended key ID pair.
//
// Non-standard HD version bytes, such as the ones documented in SLIP-0132,
// should be registered using this method for library packages to lookup key
// IDs (aka HD version bytes). When the provided key IDs are invalid, the
// ErrInvalidHDKeyID error will be returned.
//
// Reference:
//   SLIP-0132 : Registered HD version bytes for BIP-0032
//   https://github.com/satoshilabs/slips/blob/master/slip-0132.md
func RegisterHDKeyID(hdPublicKeyID []byte, hdPrivateKeyID []byte) error {
	if len(hdPublicKeyID) != 4 || len(hdPrivateKeyID) != 4 {
		return ErrInvalidHDKeyID
	}

	var keyID [4]byte
	copy(keyID[:], hdPrivateKeyID)
	hdPrivToPubKeyIDs[keyID] = hdPublicKeyID

	return nil
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

func init() {
	// Register all default networks when the package is initialized.
	mustRegister(&MainNetParams)
	mustRegister(&TestNet3Params)
	mustRegister(&RegressionNetParams)
	mustRegister(&SimNetParams)
}
