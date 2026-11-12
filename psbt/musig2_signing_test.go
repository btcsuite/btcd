// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// muSig2DerivedKey derives the key at the given path from the BIP-328
// synthetic xpub of the aggregate of the given participants.
func muSig2DerivedKey(t *testing.T, pubs []*btcec.PublicKey,
	path []uint32) (*btcec.PublicKey, *btcec.PublicKey) {

	t.Helper()

	aggKey, _, _, err := musig2.AggregateKeys(copyKeys(pubs), false)
	require.NoError(t, err)
	bareAggregate := aggKey.PreTweakedKey

	current := bip328SyntheticXpub(bareAggregate)
	for _, idx := range path {
		current, err = current.Derive(idx)
		require.NoError(t, err)
	}

	derived, err := current.ECPubKey()
	require.NoError(t, err)

	return bareAggregate, derived
}

// signWithMuSig2Infos runs a complete MuSig2 session driven solely by the
// signing infos MuSig2SigningInfos returns for every participant, and records
// the nonces and partial signatures on the input the way a signer would.
func signWithMuSig2Infos(t *testing.T, p *Packet,
	privs []*btcec.PrivateKey) {

	t.Helper()

	infos := make([]*MuSig2SigningInfo, len(privs))
	secNonces := make([][musig2.SecNonceSize]byte, len(privs))
	pubNonces := make([][musig2.PubNonceSize]byte, len(privs))
	for idx, priv := range privs {
		participantInfos, err := MuSig2SigningInfos(p, 0, priv.PubKey())
		require.NoError(t, err)
		require.Len(t, participantInfos, 1)
		infos[idx] = participantInfos[0]

		nonces, err := musig2.GenNonces(
			musig2.WithPublicKey(priv.PubKey()),
		)
		require.NoError(t, err)
		secNonces[idx] = nonces.SecNonce
		pubNonces[idx] = nonces.PubNonce
	}

	combinedNonce, err := musig2.AggregateNonces(pubNonces)
	require.NoError(t, err)

	updater, err := NewUpdater(p)
	require.NoError(t, err)
	for idx, priv := range privs {
		info := infos[idx]
		require.NoError(t, updater.AddInMuSig2PubNonce(
			0, &MuSig2PubNonce{
				PubKey:       priv.PubKey(),
				AggregateKey: info.ContextKey,
				TapLeafHash:  info.TapLeafHash,
				PubNonce:     pubNonces[idx],
			},
		))
	}

	for idx, priv := range privs {
		info := infos[idx]

		var opts []musig2.SignOption
		if len(info.Tweaks) > 0 {
			opts = append(opts, musig2.WithTweaks(info.Tweaks...))
		}
		partialSig, err := musig2.Sign(
			secNonces[idx], priv, combinedNonce,
			copyKeys(info.Participants.Keys), info.SigHash, opts...,
		)
		require.NoError(t, err)

		outcome, err := updater.SignMuSig2(0, &MuSig2PartialSig{
			PubKey:       priv.PubKey(),
			AggregateKey: info.ContextKey,
			TapLeafHash:  info.TapLeafHash,
			PartialSig:   *partialSig,
		})
		require.NoError(t, err)
		require.Equal(t, SignOutcome(SignSuccesful), outcome)
	}
}

// TestMuSig2SigningInfosKeySpendDerived asserts that the signing info of a key
// spend whose internal key is a BIP-328 child of the aggregate (the shape of a
// tr(musig(...)/<0;1>/*) descriptor) leads to a finalizable, consensus-valid
// witness.
func TestMuSig2SigningInfosKeySpendDerived(t *testing.T) {
	privs, pubs := makeTestParticipants(t)
	privs, pubs = privs[:2], pubs[:2]

	path := []uint32{1, 7}
	bareAggregate, internalKey := muSig2DerivedKey(t, pubs, path)
	outputKey := txscript.ComputeTaprootKeyNoScript(internalKey)
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	tx, _ := muSig2TestTx(pkScript)
	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)
	updater, err := NewUpdater(p)
	require.NoError(t, err)
	require.NoError(t, updater.AddInWitnessUtxo(&wire.TxOut{
		Value: muSig2TestInputAmount, PkScript: pkScript,
	}, 0))
	require.NoError(t, updater.AddInMuSig2Participants(
		0, &MuSig2Participants{
			AggregateKey: bareAggregate,
			Keys:         pubs,
		},
	))
	p.Inputs[0].TaprootInternalKey = schnorr.SerializePubKey(internalKey)
	p.Inputs[0].TaprootBip32Derivation = []*TaprootBip32Derivation{{
		XOnlyPubKey: schnorr.SerializePubKey(internalKey),
		Bip32Path:   path,
	}}

	infos, err := MuSig2SigningInfos(p, 0, pubs[0])
	require.NoError(t, err)
	require.Len(t, infos, 1)
	require.Nil(t, infos[0].TapLeafHash)
	require.Equal(t, path, infos[0].DerivationPath)
	require.Len(t, infos[0].Tweaks, 3)
	require.Equal(
		t, schnorr.SerializePubKey(outputKey),
		schnorr.SerializePubKey(infos[0].ContextKey),
	)

	signWithMuSig2Infos(t, p, privs)

	parsed := serializeAndParse(t, p)
	require.NoError(t, MaybeFinalizeAll(parsed))
	verifyFinalized(t, parsed)
}

// TestMuSig2SigningInfosKeySpendBare asserts that a key spend whose internal
// key is the bare aggregate only carries the taproot tweak.
func TestMuSig2SigningInfosKeySpendBare(t *testing.T) {
	privs, pubs := makeTestParticipants(t)

	aggKey, _, _, err := musig2.AggregateKeys(copyKeys(pubs), false)
	require.NoError(t, err)
	bareAggregate := aggKey.PreTweakedKey
	outputKey := txscript.ComputeTaprootKeyNoScript(bareAggregate)
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	tx, _ := muSig2TestTx(pkScript)
	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)
	updater, err := NewUpdater(p)
	require.NoError(t, err)
	require.NoError(t, updater.AddInWitnessUtxo(&wire.TxOut{
		Value: muSig2TestInputAmount, PkScript: pkScript,
	}, 0))
	require.NoError(t, updater.AddInMuSig2Participants(
		0, &MuSig2Participants{
			AggregateKey: bareAggregate,
			Keys:         pubs,
		},
	))
	p.Inputs[0].TaprootInternalKey = schnorr.SerializePubKey(bareAggregate)

	infos, err := MuSig2SigningInfos(p, 0, pubs[1])
	require.NoError(t, err)
	require.Len(t, infos, 1)
	require.Nil(t, infos[0].DerivationPath)
	require.Len(t, infos[0].Tweaks, 1)
	require.True(t, infos[0].Tweaks[0].IsXOnly)

	signWithMuSig2Infos(t, p, privs)

	parsed := serializeAndParse(t, p)
	require.NoError(t, MaybeFinalizeAll(parsed))
	verifyFinalized(t, parsed)
}

// TestMuSig2SigningInfosScriptSpendDerived asserts that a leaf committing to a
// BIP-328 child of the aggregate yields a script spend signing info that leads
// to a consensus-valid script path witness, and that a participant that is not
// part of the leaf's aggregate gets no signing info.
func TestMuSig2SigningInfosScriptSpendDerived(t *testing.T) {
	allPrivs, allPubs := makeTestParticipants(t)
	privs := []*btcec.PrivateKey{allPrivs[0], allPrivs[2]}
	pubs := []*btcec.PublicKey{allPubs[0], allPubs[2]}

	path := []uint32{0, 3}
	bareAggregate, leafKey := muSig2DerivedKey(t, pubs, path)

	leafScript, err := txscript.NewScriptBuilder().
		AddData(schnorr.SerializePubKey(leafKey)).
		AddOp(txscript.OP_CHECKSIG).Script()
	require.NoError(t, err)
	tapLeaf := txscript.NewBaseTapLeaf(leafScript)
	leafHash := tapLeaf.TapHash()
	tree := txscript.AssembleTaprootScriptTree(tapLeaf)
	rootHash := tree.RootNode.TapHash()

	// The internal key is unrelated to the leaf's aggregate.
	internalKey := allPubs[1]
	outputKey := txscript.ComputeTaprootOutputKey(internalKey, rootHash[:])
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)
	controlBlock := tree.LeafMerkleProofs[0].ToControlBlock(internalKey)
	controlBlockBytes, err := controlBlock.ToBytes()
	require.NoError(t, err)

	tx, _ := muSig2TestTx(pkScript)
	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)
	updater, err := NewUpdater(p)
	require.NoError(t, err)
	require.NoError(t, updater.AddInWitnessUtxo(&wire.TxOut{
		Value: muSig2TestInputAmount, PkScript: pkScript,
	}, 0))
	require.NoError(t, updater.AddInMuSig2Participants(
		0, &MuSig2Participants{
			AggregateKey: bareAggregate,
			Keys:         pubs,
		},
	))
	pInput := &p.Inputs[0]
	pInput.TaprootInternalKey = schnorr.SerializePubKey(internalKey)
	pInput.TaprootMerkleRoot = rootHash[:]
	pInput.TaprootLeafScript = []*TaprootTapLeafScript{{
		ControlBlock: controlBlockBytes,
		Script:       leafScript,
		LeafVersion:  txscript.BaseLeafVersion,
	}}
	pInput.TaprootBip32Derivation = []*TaprootBip32Derivation{{
		XOnlyPubKey: schnorr.SerializePubKey(leafKey),
		Bip32Path:   path,
		LeafHashes:  [][]byte{leafHash[:]},
	}}

	// The participant outside of the leaf's aggregate has nothing to
	// sign.
	infos, err := MuSig2SigningInfos(p, 0, allPubs[1])
	require.NoError(t, err)
	require.Empty(t, infos)

	infos, err = MuSig2SigningInfos(p, 0, pubs[0])
	require.NoError(t, err)
	require.Len(t, infos, 1)
	require.Equal(t, leafHash[:], infos[0].TapLeafHash)
	require.Equal(t, path, infos[0].DerivationPath)
	require.True(t, leafKey.IsEqual(infos[0].ContextKey))

	signWithMuSig2Infos(t, p, privs)

	parsed := serializeAndParse(t, p)
	require.NoError(t, MaybeFinalizeAll(parsed))
	verifyFinalized(t, parsed)
}

// TestMuSig2SigningInfosNoMatchingPath asserts that a participants record
// without any spend path of the input it could belong to is reported as an
// error rather than silently ignored.
func TestMuSig2SigningInfosNoMatchingPath(t *testing.T) {
	_, pubs := makeTestParticipants(t)

	aggKey, _, _, err := musig2.AggregateKeys(copyKeys(pubs[:2]), false)
	require.NoError(t, err)

	// The output key belongs to an unrelated key.
	outputKey := txscript.ComputeTaprootKeyNoScript(pubs[2])
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	tx, _ := muSig2TestTx(pkScript)
	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)
	updater, err := NewUpdater(p)
	require.NoError(t, err)
	require.NoError(t, updater.AddInWitnessUtxo(&wire.TxOut{
		Value: muSig2TestInputAmount, PkScript: pkScript,
	}, 0))
	require.NoError(t, updater.AddInMuSig2Participants(
		0, &MuSig2Participants{
			AggregateKey: aggKey.PreTweakedKey,
			Keys:         pubs[:2],
		},
	))
	p.Inputs[0].TaprootInternalKey = schnorr.SerializePubKey(pubs[2])

	_, err = MuSig2SigningInfos(p, 0, pubs[0])
	require.ErrorContains(t, err, "no spend path")

	_, err = MuSig2SigningInfos(p, 1, pubs[0])
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)
}
