// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// makeTestParticipants returns three deterministic (priv, pub) pairs to use
// as the MuSig2 signing set. The test only needs a self-consistent 3-party
// setup; it does not depend on the specific keys matching any external
// vector.
func makeTestParticipants(t *testing.T) (
	[]*btcec.PrivateKey, []*btcec.PublicKey) {

	t.Helper()

	scalars := []string{
		"f5dd1de7b85c0e8c1ada7c0c95eaa42d2bcb29ee71f5e0e63d8df1eb9e3a0e75",
		"7da2bf6e2e09f9da7e1f60af26b0e94649ada55da00bdc8a7d8b4eaf72fe69bb",
		"0000000000000000000000000000000000000000000000000000000000000003",
	}

	privs := make([]*btcec.PrivateKey, len(scalars))
	pubs := make([]*btcec.PublicKey, len(scalars))
	for i, hexStr := range scalars {
		raw, err := hex.DecodeString(hexStr)
		require.NoError(t, err)

		priv, pub := btcec.PrivKeyFromBytes(raw)
		privs[i] = priv
		pubs[i] = pub
	}

	return privs, pubs
}

// bip32ChildTweak computes the per-step BIP-32 tweak (the IL half of the
// HMAC-SHA512) for an unhardened child derivation.
func bip32ChildTweak(t *testing.T, parent *hdkeychain.ExtendedKey,
	idx uint32) [32]byte {

	t.Helper()

	pub, err := parent.ECPubKey()
	require.NoError(t, err)

	var idxBytes [4]byte
	binary.BigEndian.PutUint32(idxBytes[:], idx)

	h := hmac.New(sha512.New, parent.ChainCode())
	h.Write(pub.SerializeCompressed())
	h.Write(idxBytes[:])
	ilr := h.Sum(nil)

	var tweak [32]byte
	copy(tweak[:], ilr[:32])
	return tweak
}

// computeTaprootTweak computes BIP-86 (when merkleRoot is nil) or
// taproot-with-merkle-root tap tweak for the given x-only key.
func computeTaprootTweak(t *testing.T, xOnlyKey []byte,
	merkleRoot []byte) [32]byte {

	t.Helper()

	hashedTweak := chainhash.TaggedHash(
		chainhash.TagTapTweak, xOnlyKey, merkleRoot,
	)
	var out [32]byte
	copy(out[:], hashedTweak[:])
	return out
}

// TestFinalize_MuSig2_BIP32Derived_WithGlobalXpub builds a synthetic PSBT
// that exercises the BIP-373 case 4 finalize path: the taproot internal
// key is an unhardened BIP-32 child of the bare MuSig2 aggregate, and the
// PSBT carries the synthetic aggregate xpub via PSBT_GLOBAL_XPUB. The
// test generates fresh nonces and partial signatures programmatically,
// then runs Finalize and verifies the produced witness with the script
// engine.
func TestFinalize_MuSig2_BIP32Derived_WithGlobalXpub(t *testing.T) {
	privs, pubs := makeTestParticipants(t)

	// Bare aggregate is the KeyAgg of the three participant keys.
	bareAgg, _, _, err := musig2.AggregateKeys(pubs, true)
	require.NoError(t, err)

	// Synthetic aggregate xpub: pick a deterministic 32-byte chain code.
	chainCode := bytes.Repeat([]byte{0xa5}, 32)
	parentFP := []byte{0, 0, 0, 0}
	xpubAgg := hdkeychain.NewExtendedKey(
		chaincfg.MainNetParams.HDPublicKeyID[:],
		bareAgg.PreTweakedKey.SerializeCompressed(),
		chainCode, parentFP, 0, 0, false,
	)

	// Derive the internal key at path 1/2.
	derivPath := []uint32{1, 2}

	bip32T1 := bip32ChildTweak(t, xpubAgg, derivPath[0])
	xpubChild1, err := xpubAgg.Derive(derivPath[0])
	require.NoError(t, err)

	bip32T2 := bip32ChildTweak(t, xpubChild1, derivPath[1])
	xpubChild2, err := xpubChild1.Derive(derivPath[1])
	require.NoError(t, err)

	internalKey, err := xpubChild2.ECPubKey()
	require.NoError(t, err)

	// BIP-86: output key = internal_key + tap_tweak * G, where
	// tap_tweak = TaggedHash("TapTweak", x_only(internal_key)).
	internalKeyXOnly := schnorr.SerializePubKey(internalKey)
	tapTweak := computeTaprootTweak(t, internalKeyXOnly, nil)

	allTweaks := []musig2.KeyTweakDesc{
		{Tweak: bip32T1, IsXOnly: false},
		{Tweak: bip32T2, IsXOnly: false},
		{Tweak: tapTweak, IsXOnly: true},
	}

	// FinalKey of the full tweak chain = the taproot output key.
	fullAgg, _, _, err := musig2.AggregateKeys(
		pubs, true, musig2.WithKeyTweaks(allTweaks...),
	)
	require.NoError(t, err)
	outputKey := fullAgg.FinalKey

	// Build a P2TR pkScript for the output key and a dummy spending
	// transaction. The transaction sends a single input (referencing an
	// arbitrary outpoint) to a single dummy P2WPKH output.
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	const inputAmount = int64(100_000_000)
	prevHash := chainhash.Hash{0xde, 0xad, 0xbe, 0xef}

	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Hash: prevHash, Index: 0},
		Sequence:         0xfffffffd,
	})
	dummyOutScript := append(
		[]byte{0x00, 0x14}, bytes.Repeat([]byte{1}, 20)...,
	)
	tx.AddTxOut(wire.NewTxOut(inputAmount-1000, dummyOutScript))

	// Compute the sighash that the participants will sign over.
	prevFetcher := txscript.NewCannedPrevOutputFetcher(
		pkScript, inputAmount,
	)
	sigHashes := txscript.NewTxSigHashes(tx, prevFetcher)
	sigHash, err := txscript.CalcTaprootSignatureHash(
		sigHashes, txscript.SigHashDefault, tx, 0, prevFetcher,
	)
	require.NoError(t, err)
	var sigHashMsg [32]byte
	copy(sigHashMsg[:], sigHash)

	// Each participant generates a (sec, pub) nonce pair.
	type nonceEntry struct {
		sec [musig2.SecNonceSize]byte
		pub [musig2.PubNonceSize]byte
	}
	nonces := make([]nonceEntry, len(privs))
	for i, priv := range privs {
		n, err := musig2.GenNonces(
			musig2.WithPublicKey(priv.PubKey()),
			musig2.WithNonceCombinedKeyAux(outputKey),
		)
		require.NoError(t, err)
		nonces[i] = nonceEntry{sec: n.SecNonce, pub: n.PubNonce}
	}

	pubNonces := make([][musig2.PubNonceSize]byte, len(nonces))
	for i, n := range nonces {
		pubNonces[i] = n.pub
	}
	combinedNonce, err := musig2.AggregateNonces(pubNonces)
	require.NoError(t, err)

	// Each participant computes a partial signature using the full tweak
	// chain so the resulting sigs combine under the post-tweak output
	// key.
	partialSigs := make([]*musig2.PartialSignature, len(privs))
	for i, priv := range privs {
		ps, err := musig2.Sign(
			nonces[i].sec, priv, combinedNonce, pubs, sigHashMsg,
			musig2.WithSortedKeys(),
			musig2.WithTweaks(allTweaks...),
		)
		require.NoError(t, err)
		partialSigs[i] = ps
	}

	// Construct the PSBT.
	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	require.NoError(t, updater.AddInWitnessUtxo(
		&wire.TxOut{Value: inputAmount, PkScript: pkScript}, 0,
	))

	// Add the synthetic aggregate xpub to PSBT_GLOBAL_XPUB. We use a
	// zero master fingerprint and an empty path because the xpub *is*
	// the master in this synthetic setup.
	p.XPubs = append(p.XPubs, XPub{
		ExtendedKey:          EncodeExtendedKey(xpubAgg),
		MasterKeyFingerprint: 0,
		Bip32Path:            nil,
	})

	// Internal key + its derivation entry pinning the BIP-32 path.
	p.Inputs[0].TaprootInternalKey = internalKeyXOnly
	p.Inputs[0].TaprootBip32Derivation = append(
		p.Inputs[0].TaprootBip32Derivation,
		&TaprootBip32Derivation{
			XOnlyPubKey:          internalKeyXOnly,
			MasterKeyFingerprint: 0,
			Bip32Path:            derivPath,
		},
	)

	// MuSig2 fields.
	require.NoError(t, updater.AddInMuSig2Participants(
		0, &MuSig2Participants{
			AggregateKey: bareAgg.PreTweakedKey,
			Keys:         pubs,
		},
	))

	for i, priv := range privs {
		require.NoError(t, updater.AddInMuSig2PubNonce(
			0, &MuSig2PubNonce{
				PubKey:       priv.PubKey(),
				AggregateKey: outputKey,
				PubNonce:     nonces[i].pub,
			},
		))
		require.NoError(t, updater.AddInMuSig2PartialSig(
			0, &MuSig2PartialSig{
				PubKey:       priv.PubKey(),
				AggregateKey: outputKey,
				PartialSig:   *partialSigs[i],
			},
		))
	}

	// Round-trip through serialize/deserialize to make sure the wire
	// form survives, then finalize and verify.
	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	parsed, err := NewFromRawBytes(bytes.NewReader(buf.Bytes()), false)
	require.NoError(t, err)

	require.NoError(t, MaybeFinalizeAll(parsed))
	require.NotNil(t, parsed.Inputs[0].FinalScriptWitness)

	verifyFinalized(t, parsed)
}
