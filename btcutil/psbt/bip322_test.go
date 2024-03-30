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

// TestBuildToSignTx tests that the BuildToSignTx function works as
// expected on the passed test vector(s) as mentioned in BIP-322:
// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func TestBuildToSignTx(t *testing.T) {
	SegWitAddress := "bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l"

	addr, err := btcutil.DecodeAddress(
		SegWitAddress, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	scriptPubKey, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	TestPrivateKey := "L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k"

	wif, err := btcutil.DecodeWIF(TestPrivateKey)
	require.NoError(t, err)

	// Create to_sign transaction for the empty message.
	EmptyStringToSpendTxId := "c5680aa69bb8d860bf82d4e9cd3504b55dd" +
		"e018de765a91bb566283c545a99a7"
	emptyStringToSignTx, err := BuildToSignTx(
		EmptyStringToSpendTxId, scriptPubKey, false, nil,
	)
	require.NoError(t, err)

	sig1, err := txscript.RawTxInSignature(
		emptyStringToSignTx.UnsignedTx, 0, scriptPubKey, txscript.SigHashAll, wif.PrivKey,
	)
	require.NoError(t, err)

	emptyStringToSignTxUpdater, err := NewUpdater(emptyStringToSignTx)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	_, err = emptyStringToSignTxUpdater.Sign(
		0, sig1, wif.SerializePubKey(), nil, nil,
	)
	require.NoError(t, err)

	err = Finalize(emptyStringToSignTx, 0)
	require.NoError(t, err)

	emptyStringToSignTxSigned, err := Extract(emptyStringToSignTx)
	require.NoError(t, err)

	EmptyStringToSignTxExpected := "1e9654e951a5ba44c8604c4de6c67fd78" +
		"a27e81dcadcfe1edf638ba3aaebaed6"
	require.Equal(t, EmptyStringToSignTxExpected,
		emptyStringToSignTxSigned.TxHash().String(),
	)

	// Create to_sign transaction for the "Hello World" message.
	HelloWorldToSpendTxId := "b79d196740ad5217771c1098fc4a4b51e053" +
		"5c32236c71f1ea4d61a2d603352b"
	helloWorldToSignTx, err := BuildToSignTx(
		HelloWorldToSpendTxId, scriptPubKey, false, nil,
	)
	require.NoError(t, err)

	sig2, err := txscript.RawTxInSignature(
		helloWorldToSignTx.UnsignedTx, 0, scriptPubKey,
		txscript.SigHashAll, wif.PrivKey,
	)
	require.NoError(t, err)

	helloWorldToSignTxUpdater, err := NewUpdater(helloWorldToSignTx)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	_, err = helloWorldToSignTxUpdater.Sign(
		0, sig2, wif.SerializePubKey(), nil, nil,
	)
	require.NoError(t, err)

	err = Finalize(helloWorldToSignTx, 0)
	require.NoError(t, err)

	helloWorldToSignTxSigned, err := Extract(helloWorldToSignTx)
	require.NoError(t, err)

	HelloWorldToSignTxExpected := "88737ae86f2077145f93cc4b153ae9a1cb8d5" +
		"6afa511988c149c5c8c9d93bddf"
	require.Equal(t, HelloWorldToSignTxExpected,
		helloWorldToSignTxSigned.TxHash().String(),
	)
}
