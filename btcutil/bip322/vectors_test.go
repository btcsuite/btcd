package bip322

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stretchr/testify/require"
)

// TestBIP322CrossLibraryVectors verifies known-good BIP-322 signatures from
// the BIP spec against the btcd unified Verify function.
func TestBIP322CrossLibraryVectors(t *testing.T) {
	type vector struct {
		name      string
		address   string
		message   string
		signature string // base64
		valid     bool
	}

	vectors := []vector{
		// BIP-322 official spec vectors for P2WPKH
		{
			name:    "bip322-spec: p2wpkh empty message sig1",
			address: "bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
			message: "",
			signature: "AkcwRAIgM2gBAQqvZX15ZiysmKmQpDrG83avLIT492QBzLnQIxYCIBaTpOaD" +
				"20qRlEylyxFSeEA2ba9YOixpX8z46TSDtS40ASECx/EgAxlkQpQ9hYjgGu6E" +
				"BCPMVPwVIVJqO4XCsMvViHI=",
			valid: true,
		},
		{
			name:    "bip322-spec: p2wpkh hello world sig1",
			address: "bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
			message: "Hello World",
			signature: "AkcwRAIgZRfIY3p7/DoVTty6YZbWS71bc5Vct9p9Fia83eRmw2QCICK/ENGf" +
				"wLtptFluMGs2KsqoNSk89pO7F29zJLUx9a/sASECx/EgAxlkQpQ9hYjgGu6E" +
				"BCPMVPwVIVJqO4XCsMvViHI=",
			valid: true,
		},
		// Invalid: signature for "Hello World" verified against different message
		{
			name:    "wrong message",
			address: "bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
			message: "Different Message",
			signature: "AkcwRAIgZRfIY3p7/DoVTty6YZbWS71bc5Vct9p9Fia83eRmw2QCICK/ENGf" +
				"wLtptFluMGs2KsqoNSk89pO7F29zJLUx9a/sASECx/EgAxlkQpQ9hYjgGu6E" +
				"BCPMVPwVIVJqO4XCsMvViHI=",
			valid: false,
		},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			addr, err := btcutil.DecodeAddress(v.address, &chaincfg.MainNetParams)
			require.NoError(t, err)
			valid, err := Verify(addr, v.message, v.signature)
			if v.valid {
				require.NoError(t, err)
				require.True(t, valid, "expected valid signature")
			} else {
				require.False(t, valid || (err != nil && err == ErrInconclusive))
			}
		})
	}
}

// TestBIP322EdgeCases covers edge cases including empty messages, UTF-8,
// binary data, long messages, tampered signatures, wrong addresses, malformed
// input, P2TR, and transaction version validation.
func TestBIP322EdgeCases(t *testing.T) {
	wif, err := btcutil.DecodeWIF("L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k")
	require.NoError(t, err)
	privKey := wif.PrivKey

	p2wpkhAddr, err := btcutil.DecodeAddress(
		"bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	t.Run("empty message", func(t *testing.T) {
		sig, err := Sign(privKey, p2wpkhAddr, "")
		require.NoError(t, err)
		valid, err := Verify(p2wpkhAddr, "", sig)
		require.NoError(t, err)
		require.True(t, valid)
	})

	t.Run("utf8 emoji message", func(t *testing.T) {
		msg := "Hello 🌍 World 你好 世界"
		require.True(t, utf8.ValidString(msg))
		sig, err := Sign(privKey, p2wpkhAddr, msg)
		require.NoError(t, err)
		valid, err := Verify(p2wpkhAddr, msg, sig)
		require.NoError(t, err)
		require.True(t, valid)
	})

	t.Run("binary message with null bytes", func(t *testing.T) {
		msg := "hello\x00world\x00\xff"
		sig, err := Sign(privKey, p2wpkhAddr, msg)
		require.NoError(t, err)
		valid, err := Verify(p2wpkhAddr, msg, sig)
		require.NoError(t, err)
		require.True(t, valid)
	})

	t.Run("long message 64KB", func(t *testing.T) {
		msg := strings.Repeat("a", 65536)
		sig, err := Sign(privKey, p2wpkhAddr, msg)
		require.NoError(t, err)
		valid, err := Verify(p2wpkhAddr, msg, sig)
		require.NoError(t, err)
		require.True(t, valid)
	})

	t.Run("tampered signature invalid", func(t *testing.T) {
		sig, err := Sign(privKey, p2wpkhAddr, "Hello World")
		require.NoError(t, err)
		decoded, err := base64.StdEncoding.DecodeString(sig)
		require.NoError(t, err)
		if len(decoded) > 5 {
			decoded[5] ^= 0xFF
		}
		tampered := base64.StdEncoding.EncodeToString(decoded)
		valid, _ := Verify(p2wpkhAddr, "Hello World", tampered)
		require.False(t, valid, "tampered signature should be invalid")
	})

	t.Run("wrong address returns invalid", func(t *testing.T) {
		sig, err := Sign(privKey, p2wpkhAddr, "Hello World")
		require.NoError(t, err)
		otherAddr, err := btcutil.DecodeAddress(
			"bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq",
			&chaincfg.MainNetParams,
		)
		require.NoError(t, err)
		valid, _ := Verify(otherAddr, "Hello World", sig)
		require.False(t, valid)
	})

	t.Run("malformed base64 returns error", func(t *testing.T) {
		_, err := Verify(p2wpkhAddr, "Hello World", "not-valid-base64!!!")
		require.Error(t, err)
	})

	t.Run("p2tr empty message", func(t *testing.T) {
		taprootKey := txscript.ComputeTaprootKeyNoScript(privKey.PubKey())
		p2trAddr, err := btcutil.NewAddressTaproot(
			schnorr.SerializePubKey(taprootKey),
			&chaincfg.MainNetParams,
		)
		require.NoError(t, err)
		sig, err := Sign(privKey, p2trAddr, "")
		require.NoError(t, err)
		valid, err := Verify(p2trAddr, "", sig)
		require.NoError(t, err)
		require.True(t, valid)
	})

	t.Run("to_spend version is 0", func(t *testing.T) {
		scriptPubKey, err := txscript.PayToAddrScript(p2wpkhAddr)
		require.NoError(t, err)
		toSpend, err := BuildToSpendTx("", scriptPubKey)
		require.NoError(t, err)
		require.Equal(t, int32(0), toSpend.Version, "to_spend must be version 0")
	})

	t.Run("to_sign version is 0", func(t *testing.T) {
		scriptPubKey, err := txscript.PayToAddrScript(p2wpkhAddr)
		require.NoError(t, err)
		toSpend, err := BuildToSpendTx("test", scriptPubKey)
		require.NoError(t, err)
		toSign, err := BuildToSignTx(toSpend.TxHash(), scriptPubKey)
		require.NoError(t, err)
		require.Equal(t, int32(0), toSign.Version, "to_sign must be version 0")
	})
}
