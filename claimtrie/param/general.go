package param

import "github.com/lbryio/lbcd/wire"

type ClaimTrieParams struct {
	MaxActiveDelay    int32
	ActiveDelayFactor int32

	MaxNodeManagerCacheSize int

	OriginalClaimExpirationTime       int32
	ExtendedClaimExpirationTime       int32
	ExtendedClaimExpirationForkHeight int32

	MaxRemovalWorkaroundHeight int32

	NormalizedNameForkHeight    int32
	AllClaimsInMerkleForkHeight int32
}

var (
	ActiveParams = MainNet

	MainNet = ClaimTrieParams{
		MaxActiveDelay:          4032,
		ActiveDelayFactor:       32,
		MaxNodeManagerCacheSize: 32000,

		OriginalClaimExpirationTime:       262974,
		ExtendedClaimExpirationTime:       2102400,
		ExtendedClaimExpirationForkHeight: 400155, // https://lbry.io/news/hf1807
		MaxRemovalWorkaroundHeight:        658300,
		NormalizedNameForkHeight:          539940, // targeting 21 March 2019}, https://lbry.com/news/hf1903
		AllClaimsInMerkleForkHeight:       658309, // targeting 30 Oct 2019}, https://lbry.com/news/hf1910
	}

	TestNet = ClaimTrieParams{
		MaxActiveDelay:          4032,
		ActiveDelayFactor:       32,
		MaxNodeManagerCacheSize: 32000,

		OriginalClaimExpirationTime:       262974,
		ExtendedClaimExpirationTime:       2102400,
		ExtendedClaimExpirationForkHeight: 278160,
		MaxRemovalWorkaroundHeight:        1, // if you get a hash mismatch, come back to this
		NormalizedNameForkHeight:          993380,
		AllClaimsInMerkleForkHeight:       1198559,
	}

	Regtest = ClaimTrieParams{
		MaxActiveDelay:          4032,
		ActiveDelayFactor:       32,
		MaxNodeManagerCacheSize: 32000,

		OriginalClaimExpirationTime:       500,
		ExtendedClaimExpirationTime:       600,
		ExtendedClaimExpirationForkHeight: 800,
		MaxRemovalWorkaroundHeight:        -1,
		NormalizedNameForkHeight:          250,
		AllClaimsInMerkleForkHeight:       349,
	}
)

func SetNetwork(net wire.BitcoinNet) {

	switch net {
	case wire.MainNet:
		ActiveParams = MainNet
	case wire.TestNet3:
		ActiveParams = TestNet
	case wire.TestNet, wire.SimNet: // "regtest"
		ActiveParams = Regtest
	}
}
