// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/wire/v2"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"
)

// bip341Vectors mirrors data/bip341_wallet_vectors.json, the BIP 341 wallet
// test vectors as shipped by Bitcoin Core.
type bip341Vectors struct {
	ScriptPubKey []struct {
		Given struct {
			InternalPubkey string          `json:"internalPubkey"`
			ScriptTree     json.RawMessage `json:"scriptTree"`
		} `json:"given"`
		Intermediary struct {
			LeafHashes    []string `json:"leafHashes"`
			MerkleRoot    *string  `json:"merkleRoot"`
			TweakedPubkey string   `json:"tweakedPubkey"`
		} `json:"intermediary"`
		Expected struct {
			ScriptPubKey            string   `json:"scriptPubKey"`
			Bip350Address           string   `json:"bip350Address"`
			ScriptPathControlBlocks []string `json:"scriptPathControlBlocks"`
		} `json:"expected"`
	} `json:"scriptPubKey"`

	KeyPathSpending []struct {
		Given struct {
			RawUnsignedTx string `json:"rawUnsignedTx"`
			UtxosSpent    []struct {
				ScriptPubKey string `json:"scriptPubKey"`
				AmountSats   int64  `json:"amountSats"`
			} `json:"utxosSpent"`
		} `json:"given"`
		InputSpending []struct {
			Given struct {
				TxinIndex       int     `json:"txinIndex"`
				InternalPrivkey string  `json:"internalPrivkey"`
				MerkleRoot      *string `json:"merkleRoot"`
				HashType        int     `json:"hashType"`
			} `json:"given"`
			Intermediary struct {
				TweakedPrivkey string `json:"tweakedPrivkey"`
				SigHash        string `json:"sigHash"`
			} `json:"intermediary"`
			Expected struct {
				Witness []string `json:"witness"`
			} `json:"expected"`
		} `json:"inputSpending"`
	} `json:"keyPathSpending"`
}

// bip341Leaf is a leaf of a script tree in the BIP 341 vectors.
type bip341Leaf struct {
	ID          int    `json:"id"`
	Script      string `json:"script"`
	LeafVersion int    `json:"leafVersion"`
}

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()

	b, err := hex.DecodeString(s)
	require.NoError(t, err)

	return b
}

// buildBIP341Tree converts a script tree from the vectors (either a leaf
// object or a two-element array) into a TapNode. For every leaf it records
// the leaf itself and its merkle path, ordered from the leaf to the root.
func buildBIP341Tree(t *testing.T, raw json.RawMessage,
	leaves map[int]TapLeaf, paths map[int][]byte) (TapNode, []int) {

	t.Helper()

	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		var children []json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &children))
		require.Len(t, children, 2)

		left, leftIDs := buildBIP341Tree(t, children[0], leaves, paths)
		right, rightIDs := buildBIP341Tree(
			t, children[1], leaves, paths,
		)

		leftHash, rightHash := left.TapHash(), right.TapHash()
		for _, id := range leftIDs {
			paths[id] = append(paths[id], rightHash[:]...)
		}
		for _, id := range rightIDs {
			paths[id] = append(paths[id], leftHash[:]...)
		}

		return NewTapBranch(left, right), append(leftIDs, rightIDs...)
	}

	var leaf bip341Leaf
	require.NoError(t, json.Unmarshal(raw, &leaf))

	tapLeaf := NewTapLeaf(
		TapscriptLeafVersion(leaf.LeafVersion),
		mustDecodeHex(t, leaf.Script),
	)
	leaves[leaf.ID] = tapLeaf

	return tapLeaf, []int{leaf.ID}
}

// TestBIP341WalletVectors checks btcd against the BIP 341 wallet test vectors:
// leaf and merkle root hashes, the tweaked output key, the scriptPubKey and
// bech32m address, the script path control blocks, and for key path spends
// the signature hash, the tweaked private key and the resulting witness.
func TestBIP341WalletVectors(t *testing.T) {
	t.Parallel()

	file, err := os.ReadFile("data/bip341_wallet_vectors.json")
	require.NoError(t, err)

	var vectors bip341Vectors
	require.NoError(t, json.Unmarshal(file, &vectors))

	for i, tc := range vectors.ScriptPubKey {
		internalKey, err := schnorr.ParsePubKey(
			mustDecodeHex(t, tc.Given.InternalPubkey),
		)
		require.NoError(t, err, "vector %d", i)

		var merkleRoot []byte
		leaves := make(map[int]TapLeaf)
		paths := make(map[int][]byte)
		if len(tc.Given.ScriptTree) > 0 &&
			string(tc.Given.ScriptTree) != "null" {

			root, _ := buildBIP341Tree(
				t, tc.Given.ScriptTree, leaves, paths,
			)
			rootHash := root.TapHash()
			merkleRoot = rootHash[:]

			for id, want := range tc.Intermediary.LeafHashes {
				leafHash := leaves[id].TapHash()
				require.Equal(
					t, want,
					hex.EncodeToString(leafHash[:]),
					"vector %d leaf %d", i, id,
				)
			}
		}
		if tc.Intermediary.MerkleRoot != nil {
			require.Equal(
				t, *tc.Intermediary.MerkleRoot,
				hex.EncodeToString(merkleRoot), "vector %d", i,
			)
		}

		outputKey := ComputeTaprootOutputKey(internalKey, merkleRoot)
		witnessProgram := schnorr.SerializePubKey(outputKey)
		require.Equal(
			t, tc.Intermediary.TweakedPubkey,
			hex.EncodeToString(witnessProgram), "vector %d", i,
		)

		pkScript, err := PayToTaprootScript(outputKey)
		require.NoError(t, err)
		require.Equal(
			t, tc.Expected.ScriptPubKey,
			hex.EncodeToString(pkScript),
			"vector %d", i,
		)

		addr, err := address.NewAddressTaproot(
			witnessProgram, &chaincfg.MainNetParams,
		)
		require.NoError(t, err)
		require.Equal(
			t, tc.Expected.Bip350Address, addr.EncodeAddress(),
			"vector %d", i,
		)

		yIsOdd := outputKey.SerializeCompressed()[0] ==
			secp.PubKeyFormatCompressedOdd
		for id, want := range tc.Expected.ScriptPathControlBlocks {
			leaf := leaves[id]
			ctrlBlock := ControlBlock{
				InternalKey:     internalKey,
				OutputKeyYIsOdd: yIsOdd,
				LeafVersion:     leaf.LeafVersion,
				InclusionProof:  paths[id],
			}
			ctrlBlockBytes, err := ctrlBlock.ToBytes()
			require.NoError(t, err)
			require.Equal(
				t, want, hex.EncodeToString(ctrlBlockBytes),
				"vector %d leaf %d", i, id,
			)

			// The control block from the vectors must also parse
			// and commit to the leaf script.
			parsed, err := ParseControlBlock(mustDecodeHex(t, want))
			require.NoError(t, err, "vector %d leaf %d", i, id)
			require.NoError(
				t, VerifyTaprootLeafCommitment(
					parsed, witnessProgram, leaf.Script,
				), "vector %d leaf %d", i, id,
			)
		}
	}

	for _, tc := range vectors.KeyPathSpending {
		var tx wire.MsgTx
		err := tx.Deserialize(bytes.NewReader(
			mustDecodeHex(t, tc.Given.RawUnsignedTx),
		))
		require.NoError(t, err)

		prevOuts := make(map[wire.OutPoint]*wire.TxOut)
		for i, utxo := range tc.Given.UtxosSpent {
			prevOuts[tx.TxIn[i].PreviousOutPoint] = wire.NewTxOut(
				utxo.AmountSats,
				mustDecodeHex(t, utxo.ScriptPubKey),
			)
		}
		fetcher := NewMultiPrevOutFetcher(prevOuts)
		sigHashes := NewTxSigHashes(&tx, fetcher)

		for _, input := range tc.InputSpending {
			given := input.Given
			hashType := SigHashType(given.HashType)

			sigHash, err := CalcTaprootSignatureHash(
				sigHashes, hashType, &tx, given.TxinIndex,
				fetcher,
			)
			require.NoError(t, err, "input %d", given.TxinIndex)
			require.Equal(
				t, input.Intermediary.SigHash,
				hex.EncodeToString(sigHash),
				"input %d", given.TxinIndex,
			)

			privKey, _ := btcec.PrivKeyFromBytes(
				mustDecodeHex(t, given.InternalPrivkey),
			)
			var merkleRoot []byte
			if given.MerkleRoot != nil {
				merkleRoot = mustDecodeHex(t, *given.MerkleRoot)
			}
			tweakedKey := TweakTaprootPrivKey(*privKey, merkleRoot)
			require.Equal(
				t, input.Intermediary.TweakedPrivkey,
				hex.EncodeToString(tweakedKey.Serialize()),
				"input %d", given.TxinIndex,
			)

			// The vectors are signed with all-zero auxiliary
			// randomness.
			sig, err := schnorr.Sign(
				tweakedKey, sigHash,
				schnorr.CustomNonce([32]byte{}),
			)
			require.NoError(t, err)

			witnessSig := sig.Serialize()
			if hashType != SigHashDefault {
				witnessSig = append(witnessSig, byte(hashType))
			}
			require.Equal(
				t, input.Expected.Witness[0],
				hex.EncodeToString(witnessSig),
				"input %d", given.TxinIndex,
			)
		}
	}
}
