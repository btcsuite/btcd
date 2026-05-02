package bip322

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
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
		// P2TR. Key: L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k
		{
			name:      "p2tr empty message",
			address:   "bc1ppv609nr0vr25u07u95waq5lucwfm6tde4nydujnu8npg4q75mr5sxq8lt3",
			message:   "",
			signature: "AUENaEN08MvLbhHu5+JxE0z2cwjClb3Mt3Nv9IT4/4CLdMYMpIhLQlTTAkYK6WO/Ew4yC05Z9WHCipKnl2SfoengAQ==",
			valid:     true,
		},
		{
			name:      "p2tr hello world",
			address:   "bc1ppv609nr0vr25u07u95waq5lucwfm6tde4nydujnu8npg4q75mr5sxq8lt3",
			message:   "Hello World",
			signature: "AUFylEJD/8Arqu9J/uTde49VdZDCP/Bv9J9XDdmJc9Tdy/83yvVggexMchQJ+9teYj5bo+gdW8l98PO+JZ1mS1WLAQ==",
			valid:     true,
		},
		// P2SH-P2WPKH. Key: L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k
		{
			name:    "p2sh-p2wpkh empty message (key1)",
			address: "37qyp7jQAzqb2rCBpMvVtLDuuzKAUCVnJb",
			message: "",
			signature: "AkgwRQIhAJFXl+/g7w+UDxvGex2KulmFJshHi02gQZXGRpZ+9bapAiB5Q/ik" +
				"azTS7FMewqVFil3RLkEErUZ25k6f/xYThoQXogEhAsfxIAMZZEKUPYWI4Bru" +
				"hAQjzFT8FSFSajuFwrDL1Yhy",
			valid: true,
		},
		{
			name:    "p2sh-p2wpkh hello world (key1)",
			address: "37qyp7jQAzqb2rCBpMvVtLDuuzKAUCVnJb",
			message: "Hello World",
			signature: "AkcwRAIgRfq2Gfv9guFcXAf2vQEJSHX8FP5OuiHs1DSK1I0wk/0CIGPzqm6Q" +
				"NPTDJuki148OQ2DbJtXyrr71s4xYPwogQUupASECx/EgAxlkQpQ9hYjgGu6E" +
				"BCPMVPwVIVJqO4XCsMvViHI=",
			valid: true,
		},
		// P2SH-P2WPKH. Key: KwTbAxmBXjoZM3bzbXixEr9nxLhyYSM4vp2swet58i19bw9sqk5z
		{
			name:    "p2sh-p2wpkh hello world (key2)",
			address: "3HSVzEhCFuH9Z3wvoWTexy7BMVVp3PjS6f",
			message: "Hello World",
			signature: "AkgwRQIhAMd2wZSY3x0V9Kr/NClochoTXcgDaGl3OObOR17yx3QQAiBV" +
				"WxqNSS+CKen7bmJTG6YfJjsggQ4Fa2RHKgBKrdQQ+gEhAxa5UDdQCHSQ" +
				"HfKQv14ybcYm1C9y6b12xAuukWzSnS+w",
			valid: true,
		},
		// P2TR SIGHASH_ALL (65-byte Schnorr sig). Key: L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k
		{
			name:    "p2tr hello world sighash_all",
			address: "bc1ppv609nr0vr25u07u95waq5lucwfm6tde4nydujnu8npg4q75mr5sxq8lt3",
			message: "Hello World",
			signature: "AUHd69PrJQEv+oKTfZ8l+WROBHuy9HKrbFCJu7U1iK2iiEy1vMU5EfMtjc+V" +
				"SHM7aU0SDbak5IUZRVno2P5mjSafAQ==",
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

func TestBIP322NegativeVerification(t *testing.T) {
	wif, err := btcutil.DecodeWIF("L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k")
	require.NoError(t, err)
	privKey := wif.PrivKey
	pubKeyBytes := privKey.PubKey().SerializeCompressed()

	// Derive from the first key.
	p2wpkhAddr, err := btcutil.DecodeAddress(
		"bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	taprootKey := txscript.ComputeTaprootKeyNoScript(privKey.PubKey())
	p2trAddr, err := btcutil.NewAddressTaproot(
		schnorr.SerializePubKey(taprootKey), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	redeemScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(btcutil.Hash160(pubKeyBytes)).
		Script()
	require.NoError(t, err)
	p2shAddr, err := btcutil.NewAddressScriptHash(
		redeemScript, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	p2pkhAddr, err := btcutil.NewAddressPubKeyHash(
		btcutil.Hash160(pubKeyBytes), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	// Derive from the second key.
	otherWif, err := btcutil.DecodeWIF(
		"KwTbAxmBXjoZM3bzbXixEr9nxLhyYSM4vp2swet58i19bw9sqk5z",
	)
	require.NoError(t, err)
	otherPub := otherWif.PrivKey.PubKey().SerializeCompressed()

	otherP2wpkhAddr, err := btcutil.NewAddressWitnessPubKeyHash(
		btcutil.Hash160(otherPub), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	otherTaprootKey := txscript.ComputeTaprootKeyNoScript(otherWif.PrivKey.PubKey())
	otherP2trAddr, err := btcutil.NewAddressTaproot(
		schnorr.SerializePubKey(otherTaprootKey), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	otherRedeemScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(btcutil.Hash160(otherPub)).
		Script()
	require.NoError(t, err)
	otherP2shAddr, err := btcutil.NewAddressScriptHash(
		otherRedeemScript, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	otherP2pkhAddr, err := btcutil.NewAddressPubKeyHash(
		btcutil.Hash160(otherPub), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	cases := []struct {
		name      string
		addr      btcutil.Address
		otherAddr btcutil.Address
	}{
		{"p2wpkh", p2wpkhAddr, otherP2wpkhAddr},
		{"p2tr", p2trAddr, otherP2trAddr},
		{"p2sh-p2wpkh", p2shAddr, otherP2shAddr},
		{"p2pkh", p2pkhAddr, otherP2pkhAddr},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/wrong_message", func(t *testing.T) {
			sig, err := Sign(privKey, tc.addr, "Hello World")
			require.NoError(t, err)
			valid, _ := Verify(tc.addr, "Different Message", sig)
			require.False(t, valid, "signature must not verify against wrong message")
		})

		t.Run(tc.name+"/wrong_address", func(t *testing.T) {
			sig, err := Sign(privKey, tc.addr, "Hello World")
			require.NoError(t, err)
			valid, _ := Verify(tc.otherAddr, "Hello World", sig)
			require.False(t, valid, "signature must not verify against wrong address")
		})
	}
}

func TestBIP322ScriptSpendP2TRRejection(t *testing.T) {
	addr, wif := deriveTaprootAddr(t, testPrivKeyWIF)

	sig, err := SignP2TR(wif.PrivKey, addr, "Hello World")
	require.NoError(t, err)

	witness, err := DecodeSimple(sig)
	require.NoError(t, err)
	require.Len(t, witness, 1, "key-path P2TR witness must have exactly 1 element")

	scriptPathWitness := wire.TxWitness{
		witness[0],
		{txscript.OP_TRUE},
		make([]byte, 33),
	}

	encoded, err := EncodeSimple(scriptPathWitness)
	require.NoError(t, err)

	valid, _ := VerifyP2TR(addr, "Hello World", encoded)
	require.False(t, valid, "script-path spend must be rejected for key-path-only address")
}

func TestBIP322TamperedSignatures(t *testing.T) {
	wif, err := btcutil.DecodeWIF("L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k")
	require.NoError(t, err)
	privKey := wif.PrivKey
	pubKeyBytes := privKey.PubKey().SerializeCompressed()

	taprootKey := txscript.ComputeTaprootKeyNoScript(privKey.PubKey())
	p2trAddr, err := btcutil.NewAddressTaproot(
		schnorr.SerializePubKey(taprootKey), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	redeemScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(btcutil.Hash160(pubKeyBytes)).
		Script()
	require.NoError(t, err)
	p2shAddr, err := btcutil.NewAddressScriptHash(
		redeemScript, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	p2pkhAddr, err := btcutil.NewAddressPubKeyHash(
		btcutil.Hash160(pubKeyBytes), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	cases := []struct {
		name string
		addr btcutil.Address
	}{
		{"p2tr", p2trAddr},
		{"p2sh-p2wpkh", p2shAddr},
		{"p2pkh", p2pkhAddr},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, err := Sign(privKey, tc.addr, "Hello World")
			require.NoError(t, err)
			decoded, err := base64.StdEncoding.DecodeString(sig)
			require.NoError(t, err)
			require.True(t, len(decoded) > 5)
			decoded[5] ^= 0xFF
			tampered := base64.StdEncoding.EncodeToString(decoded)
			valid, _ := Verify(tc.addr, "Hello World", tampered)
			require.False(t, valid, "tampered %s signature must be invalid", tc.name)
		})
	}
}

func TestBIP322P2WSHNegative(t *testing.T) {
	wif, err := btcutil.DecodeWIF("L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k")
	require.NoError(t, err)

	pubKey := wif.PrivKey.PubKey()
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())

	witnessScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(pubKeyHash).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	require.NoError(t, err)

	// Builds a BIP-322 P2WSH witness since Sign does not support P2WSH
	buildP2WSHSig := func(msg string) (btcutil.Address, string) {
		t.Helper()
		scriptHash := sha256.Sum256(witnessScript)
		addr, err := btcutil.NewAddressWitnessScriptHash(
			scriptHash[:], &chaincfg.MainNetParams,
		)
		require.NoError(t, err)

		scriptPubKey, err := txscript.PayToAddrScript(addr)
		require.NoError(t, err)
		toSpend, err := BuildToSpendTx(msg, scriptPubKey)
		require.NoError(t, err)
		toSign, err := BuildToSignTx(toSpend.TxHash(), scriptPubKey)
		require.NoError(t, err)

		prevFetcher := txscript.NewCannedPrevOutputFetcher(scriptPubKey, 0)
		sigHashes := txscript.NewTxSigHashes(toSign, prevFetcher)
		sig, err := txscript.RawTxInWitnessSignature(
			toSign, sigHashes, 0, 0, witnessScript,
			txscript.SigHashAll, wif.PrivKey,
		)
		require.NoError(t, err)

		witness := wire.TxWitness{sig, pubKey.SerializeCompressed(), witnessScript}
		encoded, err := EncodeSimple(witness)
		require.NoError(t, err)
		return addr, encoded
	}

	t.Run("wrong message", func(t *testing.T) {
		addr, sig := buildP2WSHSig("Hello World")
		valid, err := VerifyP2WSH(addr, "Different", sig)
		require.NoError(t, err)
		require.False(t, valid)
	})

	t.Run("wrong address", func(t *testing.T) {
		_, sig := buildP2WSHSig("Hello World")

		otherWif, err := btcutil.DecodeWIF(
			"KwTbAxmBXjoZM3bzbXixEr9nxLhyYSM4vp2swet58i19bw9sqk5z",
		)
		require.NoError(t, err)
		otherPubHash := btcutil.Hash160(
			otherWif.PrivKey.PubKey().SerializeCompressed(),
		)
		otherWitnessScript, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).
			AddData(otherPubHash).
			AddOp(txscript.OP_EQUALVERIFY).
			AddOp(txscript.OP_CHECKSIG).
			Script()
		require.NoError(t, err)
		otherHash := sha256.Sum256(otherWitnessScript)
		otherAddr, err := btcutil.NewAddressWitnessScriptHash(
			otherHash[:], &chaincfg.MainNetParams,
		)
		require.NoError(t, err)

		valid, err := VerifyP2WSH(otherAddr, "Hello World", sig)
		require.NoError(t, err)
		require.False(t, valid)
	})

	t.Run("tampered", func(t *testing.T) {
		addr, sig := buildP2WSHSig("Hello World")
		decoded, err := base64.StdEncoding.DecodeString(sig)
		require.NoError(t, err)
		decoded[5] ^= 0xFF
		tampered := base64.StdEncoding.EncodeToString(decoded)
		valid, _ := VerifyP2WSH(addr, "Hello World", tampered)
		require.False(t, valid)
	})
}

func TestBIP322P2PKHStructureValidation(t *testing.T) {
	wif, err := btcutil.DecodeWIF("L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k")
	require.NoError(t, err)
	pubKeyBytes := wif.PrivKey.PubKey().SerializeCompressed()
	p2pkhAddr, err := btcutil.NewAddressPubKeyHash(
		btcutil.Hash160(pubKeyBytes), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	sig, err := SignP2PKH(wif.PrivKey, p2pkhAddr, "Hello World")
	require.NoError(t, err)

	toSign, err := DecodeFull(sig)
	require.NoError(t, err)

	t.Run("version not zero", func(t *testing.T) {
		tx := toSign.Copy()
		tx.Version = 2
		encoded, err := EncodeFull(tx)
		require.NoError(t, err)
		valid, _ := VerifyP2PKH(p2pkhAddr, "Hello World", encoded)
		require.False(t, valid)
	})

	t.Run("locktime not zero", func(t *testing.T) {
		tx := toSign.Copy()
		tx.LockTime = 500000
		encoded, err := EncodeFull(tx)
		require.NoError(t, err)
		valid, _ := VerifyP2PKH(p2pkhAddr, "Hello World", encoded)
		require.False(t, valid)
	})

	t.Run("output value not zero", func(t *testing.T) {
		tx := toSign.Copy()
		tx.TxOut[0].Value = 1000
		encoded, err := EncodeFull(tx)
		require.NoError(t, err)
		valid, _ := VerifyP2PKH(p2pkhAddr, "Hello World", encoded)
		require.False(t, valid)
	})
}

func TestBIP322MalformedSignatures(t *testing.T) {
	p2wpkhAddr, err := btcutil.DecodeAddress(
		"bc1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l",
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	t.Run("empty string", func(t *testing.T) {
		_, err := Verify(p2wpkhAddr, "Hello World", "")
		require.Error(t, err)
	})

	t.Run("valid base64 but garbage content", func(t *testing.T) {
		garbage := base64.StdEncoding.EncodeToString([]byte("not a real signature"))
		valid, _ := Verify(p2wpkhAddr, "Hello World", garbage)
		require.False(t, valid)
	})

	t.Run("truncated witness", func(t *testing.T) {
		wif, err := btcutil.DecodeWIF("L3VFeEujGtevx9w18HD1fhRbCH67Az2dpCymeRE1SoPK6XQtaN2k")
		require.NoError(t, err)
		sig, err := Sign(wif.PrivKey, p2wpkhAddr, "Hello World")
		require.NoError(t, err)
		decoded, err := base64.StdEncoding.DecodeString(sig)
		require.NoError(t, err)
		truncated := base64.StdEncoding.EncodeToString(decoded[:len(decoded)/2])
		valid, _ := Verify(p2wpkhAddr, "Hello World", truncated)
		require.False(t, valid)
	})
}
