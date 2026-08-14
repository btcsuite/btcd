// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"sort"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// muSig2TestInputAmount is the value of the single input every synthetic
// MuSig2 spending transaction below spends.
const muSig2TestInputAmount = int64(100_000_000)

// muSig2TestTx builds a dummy single-input, single-output transaction spending
// the given pkScript, and returns it along with a fetcher for its prevout.
func muSig2TestTx(pkScript []byte) (*wire.MsgTx,
	txscript.PrevOutputFetcher) {

	prevHash := chainhash.Hash{0xde, 0xad, 0xbe, 0xef}

	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Hash: prevHash, Index: 0},
		Sequence:         0xfffffffd,
	})
	dummyOutScript := append(
		[]byte{0x00, 0x14}, bytes.Repeat([]byte{1}, 20)...,
	)
	tx.AddTxOut(wire.NewTxOut(muSig2TestInputAmount-1000, dummyOutScript))

	fetcher := txscript.NewCannedPrevOutputFetcher(
		pkScript, muSig2TestInputAmount,
	)

	return tx, fetcher
}

// signMuSig2Session runs a complete MuSig2 signing session over the given
// message: every participant generates a nonce, the nonces are aggregated, and
// every participant produces a partial signature under the given tweaks. The
// returned public nonces and partial signatures are aligned with privs.
//
// The pubs slice is passed to Sign as-is, so when sortKeys is false its order is
// the order the session aggregates in.
func signMuSig2Session(t *testing.T, privs []*btcec.PrivateKey,
	pubs []*btcec.PublicKey, sortKeys bool, tweaks []musig2.KeyTweakDesc,
	msg [32]byte) ([][musig2.PubNonceSize]byte,
	[]*musig2.PartialSignature) {

	t.Helper()

	secNonces := make([][musig2.SecNonceSize]byte, len(privs))
	pubNonces := make([][musig2.PubNonceSize]byte, len(privs))
	for idx, priv := range privs {
		nonces, err := musig2.GenNonces(
			musig2.WithPublicKey(priv.PubKey()),
		)
		require.NoError(t, err)

		secNonces[idx] = nonces.SecNonce
		pubNonces[idx] = nonces.PubNonce
	}

	combinedNonce, err := musig2.AggregateNonces(pubNonces)
	require.NoError(t, err)

	var signOpts []musig2.SignOption
	if sortKeys {
		signOpts = append(signOpts, musig2.WithSortedKeys())
	}
	if len(tweaks) > 0 {
		signOpts = append(signOpts, musig2.WithTweaks(tweaks...))
	}

	partialSigs := make([]*musig2.PartialSignature, len(privs))
	for idx, priv := range privs {
		partialSig, err := musig2.Sign(
			secNonces[idx], priv, combinedNonce, pubs, msg,
			signOpts...,
		)
		require.NoError(t, err)

		partialSigs[idx] = partialSig
	}

	return pubNonces, partialSigs
}

// copyKeys returns a copy of the given key slice.
//
// NOTE: musig2.AggregateKeys sorts the slice it is given in place when sorting is
// requested, so a caller that needs to keep its own order (or its alignment with
// a parallel slice of private keys) must hand it a copy.
func copyKeys(keys []*btcec.PublicKey) []*btcec.PublicKey {
	out := make([]*btcec.PublicKey, len(keys))
	copy(out, keys)

	return out
}

// descendingKeyOrder returns the participants ordered by descending serialized
// public key. BIP-327's KeySort orders ascending, so for a set of distinct keys
// this is guaranteed to be a different order. The private keys stay aligned with
// their public keys.
func descendingKeyOrder(privs []*btcec.PrivateKey,
	pubs []*btcec.PublicKey) ([]*btcec.PrivateKey, []*btcec.PublicKey) {

	order := make([]int, len(pubs))
	for idx := range order {
		order[idx] = idx
	}
	sort.Slice(order, func(i, j int) bool {
		return bytes.Compare(
			pubs[order[i]].SerializeCompressed(),
			pubs[order[j]].SerializeCompressed(),
		) > 0
	})

	outPrivs := make([]*btcec.PrivateKey, len(privs))
	outPubs := make([]*btcec.PublicKey, len(pubs))
	for idx, from := range order {
		outPrivs[idx] = privs[from]
		outPubs[idx] = pubs[from]
	}

	return outPrivs, outPubs
}

// addMuSig2SigningFields adds the nonces and partial signatures of a signing
// session to the given input, all referencing the same aggregate key and
// optional tap leaf hash.
func addMuSig2SigningFields(t *testing.T, updater *Updater,
	pubs []*btcec.PublicKey, aggregateKey *btcec.PublicKey,
	tapLeafHash []byte, pubNonces [][musig2.PubNonceSize]byte,
	partialSigs []*musig2.PartialSignature) {

	t.Helper()

	for idx, pub := range pubs {
		require.NoError(t, updater.AddInMuSig2PubNonce(
			0, &MuSig2PubNonce{
				PubKey:       pub,
				AggregateKey: aggregateKey,
				TapLeafHash:  tapLeafHash,
				PubNonce:     pubNonces[idx],
			},
		))
		require.NoError(t, updater.AddInMuSig2PartialSig(
			0, &MuSig2PartialSig{
				PubKey:       pub,
				AggregateKey: aggregateKey,
				TapLeafHash:  tapLeafHash,
				PartialSig:   *partialSigs[idx],
			},
		))
	}
}

// serializeAndParse round-trips the packet through its wire encoding, so the
// tests below finalize a packet that really came off the wire.
func serializeAndParse(t *testing.T, p *Packet) *Packet {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	parsed, err := NewFromRawBytes(bytes.NewReader(buf.Bytes()), false)
	require.NoError(t, err)

	return parsed
}

// TestFinalize_MuSig2_UnsortedParticipants asserts that an input whose signing
// session aggregated its keys in an explicitly unsorted order finalizes to a
// consensus-valid witness.
//
// BIP-327 makes KeySort optional and BIP-373 stores the participants of a
// PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS record "in the order required for
// aggregation", so the finalizer must aggregate in the recorded order rather
// than sorting unconditionally. The order used here is the reverse of the sorted
// order, which yields a different aggregate key, so sorting anyway cannot
// produce a valid signature.
func TestFinalize_MuSig2_UnsortedParticipants(t *testing.T) {
	sortedPrivs, sortedPubs := makeTestParticipants(t)
	privs, pubs := descendingKeyOrder(sortedPrivs, sortedPubs)

	// The session aggregates in the given order, without sorting.
	aggKey, _, _, err := musig2.AggregateKeys(copyKeys(pubs), false)
	require.NoError(t, err)

	// Confirm the premise of this test: sorting the very same keys yields a
	// different aggregate key, so the recorded order genuinely matters.
	sortedAgg, _, _, err := musig2.AggregateKeys(copyKeys(pubs), true)
	require.NoError(t, err)
	require.NotEqual(
		t, aggKey.FinalKey.SerializeCompressed(),
		sortedAgg.FinalKey.SerializeCompressed(),
	)

	// BIP-373 test vector case 1 shape: the taproot output key IS the
	// aggregate, so no tweak is applied at sign time.
	outputKey := aggKey.FinalKey
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	tx, prevFetcher := muSig2TestTx(pkScript)
	sigHashes := txscript.NewTxSigHashes(tx, prevFetcher)
	sigHash, err := txscript.CalcTaprootSignatureHash(
		sigHashes, txscript.SigHashDefault, tx, 0, prevFetcher,
	)
	require.NoError(t, err)

	var sigHashMsg [32]byte
	copy(sigHashMsg[:], sigHash)

	pubNonces, partialSigs := signMuSig2Session(
		t, privs, pubs, false, nil, sigHashMsg,
	)

	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	require.NoError(t, updater.AddInWitnessUtxo(&wire.TxOut{
		Value: muSig2TestInputAmount, PkScript: pkScript,
	}, 0))

	// The record stores the keys in the unsorted order the session used.
	require.NoError(t, updater.AddInMuSig2Participants(
		0, &MuSig2Participants{
			AggregateKey: aggKey.PreTweakedKey,
			Keys:         pubs,
		},
	))
	addMuSig2SigningFields(
		t, updater, pubs, outputKey, nil, pubNonces, partialSigs,
	)

	parsed := serializeAndParse(t, p)

	// The unsorted order must survive the wire encoding, otherwise the
	// finalizer could never recover it.
	require.Len(t, parsed.Inputs[0].MuSig2Participants, 1)
	for idx, key := range pubs {
		require.True(t, key.IsEqual(
			parsed.Inputs[0].MuSig2Participants[0].Keys[idx],
		))
	}

	require.NoError(t, MaybeFinalizeAll(parsed))
	require.Len(t, parsed.Inputs[0].FinalScriptWitness, 66)

	verifyFinalized(t, parsed)
}

// TestFinalize_MuSig2_ScriptSpendDerivedKey asserts that a tapscript leaf spend
// whose leaf key is a BIP-32 child of a parent MuSig2 aggregate finalizes to a
// consensus-valid witness.
//
// BIP-373 permits the key inside a leaf to be derived from a parent aggregate,
// in which case the signers apply the BIP-32 derivation tweaks and the finalizer
// has to apply the same ones. There is no taproot tweak here: the leaf script
// commits to the derived key directly.
//
// No PSBT_GLOBAL_XPUB is present, so the derivation also goes through the
// BIP-328 synthetic xpub fallback.
func TestFinalize_MuSig2_ScriptSpendDerivedKey(t *testing.T) {
	privs, pubs := makeTestParticipants(t)

	// The parent aggregate, from which the key in the leaf is derived.
	bareAgg, _, _, err := musig2.AggregateKeys(copyKeys(pubs), true)
	require.NoError(t, err)
	parentKey := bareAgg.PreTweakedKey

	// Derive the leaf key at path 3/7 off the BIP-328 synthetic xpub, and
	// collect the per-step tweaks the signers have to apply.
	derivPath := []uint32{3, 7}
	syntheticXpub := bip328SyntheticXpub(parentKey)

	tweaks := make([]musig2.KeyTweakDesc, 0, len(derivPath))
	current := syntheticXpub
	for _, idx := range derivPath {
		tweaks = append(tweaks, musig2.KeyTweakDesc{
			Tweak:   bip32ChildTweak(t, current, idx),
			IsXOnly: false,
		})

		current, err = current.Derive(idx)
		require.NoError(t, err)
	}

	leafKey, err := current.ECPubKey()
	require.NoError(t, err)

	// Aggregating under the derivation tweaks must reproduce the leaf key,
	// which is what makes the leaf's CHECKSIG satisfiable by the session.
	derivedAgg, _, _, err := musig2.AggregateKeys(
		copyKeys(pubs), true, musig2.WithKeyTweaks(tweaks...),
	)
	require.NoError(t, err)
	require.Equal(
		t, schnorr.SerializePubKey(leafKey),
		schnorr.SerializePubKey(derivedAgg.FinalKey),
	)

	// A single-leaf tapscript tree committing to the derived key, under an
	// unrelated internal key.
	leafScript, err := txscript.NewScriptBuilder().
		AddData(schnorr.SerializePubKey(leafKey)).
		AddOp(txscript.OP_CHECKSIG).Script()
	require.NoError(t, err)

	tapLeaf := txscript.NewBaseTapLeaf(leafScript)
	leafHash := tapLeaf.TapHash()
	tree := txscript.AssembleTaprootScriptTree(tapLeaf)
	rootHash := tree.RootNode.TapHash()

	internalKey := mustParsePubKey(t, bip373ParticipantHex[0])
	outputKey := txscript.ComputeTaprootOutputKey(internalKey, rootHash[:])
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	controlBlock := tree.LeafMerkleProofs[0].ToControlBlock(internalKey)
	controlBlockBytes, err := controlBlock.ToBytes()
	require.NoError(t, err)

	tx, prevFetcher := muSig2TestTx(pkScript)
	sigHashes := txscript.NewTxSigHashes(tx, prevFetcher)
	sigHash, err := txscript.CalcTapscriptSignaturehash(
		sigHashes, txscript.SigHashDefault, tx, 0, prevFetcher, tapLeaf,
	)
	require.NoError(t, err)

	var sigHashMsg [32]byte
	copy(sigHashMsg[:], sigHash)

	pubNonces, partialSigs := signMuSig2Session(
		t, privs, pubs, true, tweaks, sigHashMsg,
	)

	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	require.NoError(t, updater.AddInWitnessUtxo(&wire.TxOut{
		Value: muSig2TestInputAmount, PkScript: pkScript,
	}, 0))

	pInput := &p.Inputs[0]
	pInput.TaprootInternalKey = schnorr.SerializePubKey(internalKey)
	pInput.TaprootMerkleRoot = rootHash[:]
	pInput.TaprootLeafScript = []*TaprootTapLeafScript{{
		ControlBlock: controlBlockBytes,
		Script:       leafScript,
		LeafVersion:  txscript.BaseLeafVersion,
	}}

	// The derivation path of the key in the leaf, which is what tells the
	// finalizer which tweaks to reconstruct.
	pInput.TaprootBip32Derivation = []*TaprootBip32Derivation{{
		XOnlyPubKey:          schnorr.SerializePubKey(leafKey),
		MasterKeyFingerprint: 0,
		Bip32Path:            derivPath,
		LeafHashes:           [][]byte{leafHash[:]},
	}}

	// The record names the parent aggregate, while the nonces and partial
	// signatures name the derived key that is in the leaf script.
	require.NoError(t, updater.AddInMuSig2Participants(
		0, &MuSig2Participants{
			AggregateKey: parentKey,
			Keys:         pubs,
		},
	))
	addMuSig2SigningFields(
		t, updater, pubs, leafKey, leafHash[:], pubNonces, partialSigs,
	)

	parsed := serializeAndParse(t, p)

	require.NoError(t, MaybeFinalizeAll(parsed))

	finalTx, err := Extract(parsed)
	require.NoError(t, err)

	// Witness shape for a script spend: [sig, leafScript, controlBlock].
	require.Len(t, finalTx.TxIn[0].Witness, 3)
	require.Len(t, finalTx.TxIn[0].Witness[0], 64)
	require.Equal(t, leafScript, finalTx.TxIn[0].Witness[1])
	require.Equal(t, controlBlockBytes, finalTx.TxIn[0].Witness[2])

	verifyFinalized(t, parsed)
}
