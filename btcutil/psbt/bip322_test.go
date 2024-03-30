package psbt

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stretchr/testify/require"
)

// TestBuildToSpendTx tests that the BuildToSpendTx function works as
// expected on the passed test vector(s) as mentioned in BIP-322:
// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func TestBuildToSpendTx(t *testing.T) {
	SegWitAddress := "bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l"

	addr, err := btcutil.DecodeAddress(
		SegWitAddress, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	scriptPubKey, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	emptyStringToSpendTx, err := BuildToSpendTx("", scriptPubKey)
	require.NoError(t, err)

	// Create to_spend transaction for the empty message.
	EmptyStringToSpendTxExpected := "c5680aa69bb8d860bf82d4e9cd3504b55dd" +
		"e018de765a91bb566283c545a99a7"
	require.Equal(t, EmptyStringToSpendTxExpected,
		emptyStringToSpendTx.TxHash().String(),
	)

	// Create to_spend transaction for the "Hello World" message.
	helloWorldToSpendTx, err := BuildToSpendTx(
		"Hello World", scriptPubKey,
	)
	require.NoError(t, err)

	HelloWorldToSpendTxExpected := "b79d196740ad5217771c1098fc4a4b51e053" +
		"5c32236c71f1ea4d61a2d603352b"
	require.Equal(t, HelloWorldToSpendTxExpected,
		helloWorldToSpendTx.TxHash().String(),
	)
}
