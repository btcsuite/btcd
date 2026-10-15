// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/txscript/v2"
)

// MuSig2SigningInfo describes one MuSig2 signing context an input can be spent
// with, together with everything a participant needs to contribute a nonce and
// a partial signature to it.
//
// The info is derived from the same fields and with the same tweak inference
// the finalizer uses, so a signer that follows it produces partial signatures
// the finalizer is able to combine.
type MuSig2SigningInfo struct {
	// Participants is the PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS record the
	// context belongs to. Its keys are in the order required for key
	// aggregation, which is the order signers must pass to musig2.Sign
	// (without requesting sorting).
	Participants *MuSig2Participants

	// ContextKey is the aggregate key to record in the key data of the
	// PSBT_IN_MUSIG2_PUB_NONCE and PSBT_IN_MUSIG2_PARTIAL_SIG fields: the
	// taproot output key for a key spend, or the key found in the leaf
	// script for a script spend. It carries its real parity.
	ContextKey *btcec.PublicKey

	// TapLeafHash is the hash of the leaf script for a script spend and
	// nil for a key spend.
	TapLeafHash []byte

	// DerivationPath is the unhardened BIP-32 path from the bare aggregate
	// key to the key the context signs for (the internal key for a key
	// spend or the leaf key for a script spend). It is nil if the
	// aggregate key is used without derivation.
	DerivationPath []uint32

	// Tweaks is the ordered list of tweaks that turn the bare aggregate
	// key into the key the signature is checked against. It is to be
	// passed to musig2.Sign via musig2.WithTweaks.
	Tweaks []musig2.KeyTweakDesc

	// SigHash is the message to sign for this context.
	SigHash [32]byte
}

// MuSig2SigningInfos returns the MuSig2 signing contexts of the input at the
// given index that the given participant key is a member of. Key spend
// contexts are returned before script spend contexts.
//
// A context is only returned once the tweaks inferred for it are shown to
// reproduce the key the signature will be checked against. An error is
// returned if the participant is listed in a PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS
// record of the input but none of the input's spend paths can be matched to
// that record, since that means the PSBT is missing the fields a signer needs.
func MuSig2SigningInfos(p *Packet, inIndex int,
	participant *btcec.PublicKey) ([]*MuSig2SigningInfo, error) {

	if inIndex < 0 || inIndex >= len(p.Inputs) {
		return nil, ErrInvalidPsbtFormat
	}
	pInput := &p.Inputs[inIndex]

	var records []*MuSig2Participants
	for _, record := range pInput.MuSig2Participants {
		for _, key := range record.Keys {
			if key.IsEqual(participant) {
				records = append(records, record)
				break
			}
		}
	}
	if len(records) == 0 {
		return nil, nil
	}

	if pInput.WitnessUtxo == nil {
		return nil, fmt.Errorf("MuSig2 input %d has no witness UTXO",
			inIndex)
	}
	outputKey, err := taprootOutputKey(pInput.WitnessUtxo.PkScript)
	if err != nil {
		return nil, err
	}

	prevOutFetcher, err := PrevOutputFetcher(p)
	if err != nil {
		return nil, fmt.Errorf("error making prev out fetcher: %w", err)
	}
	sigHashes := txscript.NewTxSigHashes(p.UnsignedTx, prevOutFetcher)

	var infos []*MuSig2SigningInfo
	for _, record := range records {
		keySpend, err := muSig2KeySpendInfo(
			p, pInput, record, outputKey,
		)
		if err != nil {
			return nil, err
		}
		if keySpend != nil {
			sigHash, err := txscript.CalcTaprootSignatureHash(
				sigHashes, pInput.SighashType, p.UnsignedTx,
				inIndex, prevOutFetcher,
			)
			if err != nil {
				return nil, fmt.Errorf("error calculating "+
					"signature hash: %w", err)
			}
			copy(keySpend.SigHash[:], sigHash)

			infos = append(infos, keySpend)
		}
	}

	for _, record := range records {
		for _, leaf := range pInput.TaprootLeafScript {
			scriptSpends, err := muSig2ScriptSpendInfos(
				p, pInput, record, leaf,
			)
			if err != nil {
				return nil, err
			}

			tapLeaf := txscript.TapLeaf{
				LeafVersion: leaf.LeafVersion,
				Script:      leaf.Script,
			}
			for _, info := range scriptSpends {
				sigHash, err := txscript.CalcTapscriptSignaturehash(
					sigHashes, pInput.SighashType,
					p.UnsignedTx, inIndex, prevOutFetcher,
					tapLeaf,
				)
				if err != nil {
					return nil, fmt.Errorf("error " +
						"calculating tapscript " +
						"signature hash: %w",						err)
				}
				copy(info.SigHash[:], sigHash)

				infos = append(infos, info)
			}
		}
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("participant %x is listed in a MuSig2 "+
			"participants record of input %d, but no spend path " +
			"of the input matches the record",
			participant.SerializeCompressed(), inIndex)
	}

	return infos, nil
}

// muSig2KeySpendInfo returns the key spend signing context of the given
// participants record, or nil if the input's output key is not derived from
// the record's aggregate key.
func muSig2KeySpendInfo(p *Packet, pInput *PInput, record *MuSig2Participants,
	outputKey []byte) (*MuSig2SigningInfo, error) {

	bareAggregate := record.AggregateKey
	bareXOnly := schnorr.SerializePubKey(bareAggregate)

	var (
		tweaks []musig2.KeyTweakDesc
		path   []uint32
	)
	switch {
	// The output key is the bare aggregate, nothing was tweaked.
	case bytes.Equal(bareXOnly, outputKey):

	// Without an internal key we cannot tell how the output key was
	// derived from the aggregate.
	case pInput.TaprootInternalKey == nil:
		return nil, nil

	// The internal key is the bare aggregate, only the taproot tweak is
	// applied.
	case bytes.Equal(bareXOnly, pInput.TaprootInternalKey):
		tweaks = []musig2.KeyTweakDesc{
			taprootKeyTweak(
				bareAggregate, pInput.TaprootMerkleRoot,
			),
		}

	// The internal key was BIP-32 derived from the aggregate. A record
	// whose aggregate the internal key was not derived from simply does
	// not describe the key spend path, so a failed derivation is not an
	// error.
	default:
		derivationTweaks, internalKey, err := muSig2DerivationTweaks(
			p, pInput, bareAggregate, pInput.TaprootInternalKey,
		)
		if err != nil {
			return nil, nil
		}

		path, err = taprootDerivationPath(
			pInput, pInput.TaprootInternalKey,
		)
		if err != nil {
			return nil, err
		}

		tweaks = append(derivationTweaks, taprootKeyTweak(
			internalKey, pInput.TaprootMerkleRoot,
		))
	}

	finalKey, err := muSig2FinalKey(record, tweaks)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(schnorr.SerializePubKey(finalKey), outputKey) {
		return nil, nil
	}

	return &MuSig2SigningInfo{
		Participants:   record,
		ContextKey:     finalKey,
		DerivationPath: path,
		Tweaks:         tweaks,
	}, nil
}

// muSig2ScriptSpendInfos returns the script spend signing contexts of the given
// participants record in the given leaf: one for every key the leaf script
// pushes that is the record's aggregate key or a BIP-32 child of it.
func muSig2ScriptSpendInfos(p *Packet, pInput *PInput,
	record *MuSig2Participants,
	leaf *TaprootTapLeafScript) ([]*MuSig2SigningInfo, error) {

	bareAggregate := record.AggregateKey
	bareXOnly := schnorr.SerializePubKey(bareAggregate)
	leafHash := txscript.TapLeaf{
		LeafVersion: leaf.LeafVersion,
		Script:      leaf.Script,
	}.TapHash()

	var infos []*MuSig2SigningInfo
	tokenizer := txscript.MakeScriptTokenizer(0, leaf.Script)
	for tokenizer.Next() {
		leafKey := tokenizer.Data()
		if len(leafKey) != schnorr.PubKeyBytesLen {
			continue
		}

		var (
			tweaks []musig2.KeyTweakDesc
			path   []uint32
		)
		if !bytes.Equal(leafKey, bareXOnly) {
			var err error
			tweaks, _, err = muSig2DerivationTweaks(
				p, pInput, bareAggregate, leafKey,
			)
			if err != nil {
				continue
			}

			path, err = taprootDerivationPath(pInput, leafKey)
			if err != nil {
				return nil, err
			}
		}

		finalKey, err := muSig2FinalKey(record, tweaks)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(schnorr.SerializePubKey(finalKey), leafKey) {
			continue
		}

		infos = append(infos, &MuSig2SigningInfo{
			Participants:   record,
			ContextKey:     finalKey,
			TapLeafHash:    leafHash[:],
			DerivationPath: path,
			Tweaks:         tweaks,
		})
	}
	if err := tokenizer.Err(); err != nil {
		return nil, fmt.Errorf("error parsing tap leaf script: %w", err)
	}

	return infos, nil
}

// muSig2FinalKey aggregates the keys of the given participants record in their
// recorded order and applies the given tweaks.
func muSig2FinalKey(record *MuSig2Participants,
	tweaks []musig2.KeyTweakDesc) (*btcec.PublicKey, error) {

	keys := make([]*btcec.PublicKey, len(record.Keys))
	copy(keys, record.Keys)

	var opts []musig2.KeyAggOption
	if len(tweaks) > 0 {
		opts = append(opts, musig2.WithKeyTweaks(tweaks...))
	}

	aggKey, _, _, err := musig2.AggregateKeys(keys, false, opts...)
	if err != nil {
		return nil, fmt.Errorf("error aggregating keys: %w", err)
	}

	return aggKey.FinalKey, nil
}
