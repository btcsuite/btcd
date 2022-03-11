// Copyright 2013-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package musig2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// TestMuSig2SgnTestVectors tests that this implementation of musig2 matches
// the secp256k1-zkp test vectors.
func TestMuSig2SignTestVectors(t *testing.T) {
	t.Parallel()
}

var (
	key1Bytes, _ = hex.DecodeString("F9308A019258C31049344F85F89D5229B53" +
		"1C845836F99B08601F113BCE036F9")
	key2Bytes, _ = hex.DecodeString("DFF1D77F2A671C5F36183726DB2341BE58F" +
		"EAE1DA2DECED843240F7B502BA659")
	key3Bytes, _ = hex.DecodeString("3590A94E768F8E1815C2F24B4D80A8E3149" +
		"316C3518CE7B7AD338368D038CA66")

	testKeys = [][]byte{key1Bytes, key2Bytes, key3Bytes}

	keyCombo1, _ = hex.DecodeString("E5830140512195D74C8307E39637CBE5FB730EBEAB80EC514CF88A877CEEEE0B")
	keyCombo2, _ = hex.DecodeString("D70CD69A2647F7390973DF48CBFA2CCC407B8B2D60B08C5F1641185C7998A290")
	keyCombo3, _ = hex.DecodeString("81A8B093912C9E481408D09776CEFB48AEB8B65481B6BAAFB3C5810106717BEB")
	keyCombo4, _ = hex.DecodeString("2EB18851887E7BDC5E830E89B19DDBC28078F1FA88AAD0AD01CA06FE4F80210B")
)

// TestMuSig2KeyAggTestVectors tests that this implementation of musig2 key
// aggregation lines up with the secp256k1-zkp test vectors.
func TestMuSig2KeyAggTestVectors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		keyOrder    []int
		expectedKey []byte
	}{
		// Keys in backwards lexicographical order.
		{
			keyOrder:    []int{0, 1, 2},
			expectedKey: keyCombo1,
		},

		// Keys in sorted order.
		{
			keyOrder:    []int{2, 1, 0},
			expectedKey: keyCombo2,
		},

		// Only the first key.
		{
			keyOrder:    []int{0, 0, 0},
			expectedKey: keyCombo3,
		},

		// Duplicate the first key and second keys.
		{
			keyOrder:    []int{0, 0, 1, 1},
			expectedKey: keyCombo4,
		},
	}
	for i, testCase := range testCases {
		testName := fmt.Sprintf("%v", testCase.keyOrder)
		t.Run(testName, func(t *testing.T) {
			var keys []*btcec.PublicKey
			for _, keyIndex := range testCase.keyOrder {
				keyBytes := testKeys[keyIndex]
				pub, err := schnorr.ParsePubKey(keyBytes)
				if err != nil {
					t.Fatalf("unable to parse pubkeys: %v", err)
				}

				keys = append(keys, pub)
			}

			uniqueKeyIndex := secondUniqueKeyIndex(keys)
			combinedKey := AggregateKeys(
				keys, false, WithUniqueKeyIndex(uniqueKeyIndex),
			)
			combinedKeyBytes := schnorr.SerializePubKey(combinedKey)
			if !bytes.Equal(combinedKeyBytes, testCase.expectedKey) {
				t.Fatalf("case: #%v, invalid aggregation: "+
					"expected %x, got %x", i, testCase.expectedKey,
					combinedKeyBytes)
			}
		})
	}
}

func mustParseHex(str string) []byte {
	b, err := hex.DecodeString(str)
	if err != nil {
		panic(fmt.Errorf("unable to parse hex: %v", err))
	}

	return b
}

func parseKey(xHex string) *btcec.PublicKey {
	xB, err := hex.DecodeString(xHex)
	if err != nil {
		panic(err)
	}

	var x, y btcec.FieldVal
	x.SetByteSlice(xB)
	if !btcec.DecompressY(&x, false, &y) {
		panic("x not on curve")
	}
	y.Normalize()

	return btcec.NewPublicKey(&x, &y)
}

var (
	signSetPrivKey, _ = btcec.PrivKeyFromBytes(
		mustParseHex("7FB9E0E687ADA1EEBF7ECFE2F21E73EBDB51A7D450948DFE8D76D7F2D1007671"),
	)
	signSetPubKey, _ = schnorr.ParsePubKey(schnorr.SerializePubKey(signSetPrivKey.PubKey()))

	signTestMsg = mustParseHex("F95466D086770E689964664219266FE5ED215C92AE20BAB5C9D79ADDDDF3C0CF")

	signSetKey2, _ = schnorr.ParsePubKey(
		mustParseHex("F9308A019258C31049344F85F89D5229B531C845836F99B086" +
			"01F113BCE036F9"),
	)
	signSetKey3, _ = schnorr.ParsePubKey(
		mustParseHex("DFF1D77F2A671C5F36183726DB2341BE58FEAE1DA2DECED843" +
			"240F7B502BA659"),
	)

	signSetKeys = []*btcec.PublicKey{signSetPubKey, signSetKey2, signSetKey3}
)

// TestMuSig2SigningTestVectors tests that the musig2 implementation produces
// the same set of signatures.
func TestMuSig2SigningTestVectors(t *testing.T) {
	t.Parallel()

	var aggregatedNonce [PubNonceSize]byte
	copy(
		aggregatedNonce[:],
		mustParseHex("028465FCF0BBDBCF443AABCCE533D42B4B5A10966AC09A49655E8C42DAAB8FCD61"),
	)
	copy(
		aggregatedNonce[33:],
		mustParseHex("037496A3CC86926D452CAFCFD55D25972CA1675D549310DE296BFF42F72EEEA8C9"),
	)

	var secNonce [SecNonceSize]byte
	copy(secNonce[:], mustParseHex("508B81A611F100A6B2B6B29656590898AF488BCF2E1F55CF22E5CFB84421FE61"))
	copy(secNonce[32:], mustParseHex("FA27FD49B1D50085B481285E1CA205D55C82CC1B31FF5CD54A489829355901F7"))

	testCases := []struct {
		keyOrder           []int
		expectedPartialSig []byte
	}{
		{
			keyOrder:           []int{0, 1, 2},
			expectedPartialSig: mustParseHex("68537CC5234E505BD14061F8DA9E90C220A181855FD8BDB7F127BB12403B4D3B"),
		},
		{
			keyOrder:           []int{1, 0, 2},
			expectedPartialSig: mustParseHex("2DF67BFFF18E3DE797E13C6475C963048138DAEC5CB20A357CECA7C8424295EA"),
		},

		{
			keyOrder:           []int{1, 2, 0},
			expectedPartialSig: mustParseHex("0D5B651E6DE34A29A12DE7A8B4183B4AE6A7F7FBE15CDCAFA4A3D1BCAABC7517"),
		},
	}

	var msg [32]byte
	copy(msg[:], signTestMsg)

	for _, testCase := range testCases {
		testName := fmt.Sprintf("%v", testCase.keyOrder)
		t.Run(testName, func(t *testing.T) {
			keySet := make([]*btcec.PublicKey, 0, len(testCase.keyOrder))
			for _, keyIndex := range testCase.keyOrder {
				keySet = append(keySet, signSetKeys[keyIndex])
			}

			partialSig, err := Sign(
				secNonce, signSetPrivKey, aggregatedNonce, keySet, msg,
			)
			if err != nil {
				t.Fatalf("unable to generate partial sig: %v", err)
			}

			var partialSigBytes [32]byte
			partialSig.S.PutBytesUnchecked(partialSigBytes[:])

			if !bytes.Equal(partialSigBytes[:], testCase.expectedPartialSig) {
				t.Fatalf("sigs don't match: expected %x, got %x",
					testCase.expectedPartialSig, partialSigBytes,
				)
			}
		})
	}
}

type signer struct {
	privKey *btcec.PrivateKey
	pubKey  *btcec.PublicKey

	nonces *Nonces

	partialSig *PartialSignature
}

type signerSet []signer

func (s signerSet) keys() []*btcec.PublicKey {
	keys := make([]*btcec.PublicKey, len(s))
	for i := 0; i < len(s); i++ {
		keys[i] = s[i].pubKey
	}

	return keys
}

func (s signerSet) partialSigs() []*PartialSignature {
	sigs := make([]*PartialSignature, len(s))
	for i := 0; i < len(s); i++ {
		sigs[i] = s[i].partialSig
	}

	return sigs
}

func (s signerSet) pubNonces() [][PubNonceSize]byte {
	nonces := make([][PubNonceSize]byte, len(s))
	for i := 0; i < len(s); i++ {
		nonces[i] = s[i].nonces.PubNonce
	}

	return nonces
}

func (s signerSet) combinedKey() *btcec.PublicKey {
	uniqueKeyIndex := secondUniqueKeyIndex(s.keys())
	return AggregateKeys(
		s.keys(), false, WithUniqueKeyIndex(uniqueKeyIndex),
	)
}

// TestMuSigMultiParty tests that for a given set of 100 signers, we're able to
// properly generate valid sub signatures, which ultimately can be combined
// into a single valid signature.
func TestMuSigMultiParty(t *testing.T) {
	t.Parallel()

	// First generate the private key, public key, and nonce for each
	// signer.
	numSigners := 100
	signers := make(signerSet, 0, numSigners)
	for i := 0; i < numSigners; i++ {
		privKey, err := btcec.NewPrivateKey()
		if err != nil {
			t.Fatalf("unable to gen priv key: %v", err)
		}

		pubKey, err := schnorr.ParsePubKey(
			schnorr.SerializePubKey(privKey.PubKey()),
		)
		if err != nil {
			t.Fatalf("unable to gen key: %v", err)
		}

		nonces, err := GenNonces()
		if err != nil {
			t.Fatalf("unable to gen nonces: %v", err)
		}

		signers = append(signers, signer{
			privKey: privKey,
			pubKey:  pubKey,
			nonces:  nonces,
		})
	}

	msg := sha256.Sum256([]byte("let's get taprooty"))

	// With the set of nonces generated, we can now generate our aggregate
	// nonce.
	combinedNonce, err := AggregateNonces(signers.pubNonces())
	if err != nil {
		t.Fatalf("unable to gen nonces: %v", err)
	}

	signerKeys := signers.keys()
	combinedKey := signers.combinedKey()

	// Now for the signing phase: each signer will generate their partial
	// signature and stash that away. We'll also store the final nonce as
	// well, since we'll need that to validate the signature.
	var finalNonce *btcec.PublicKey
	for i := range signers {
		signer := signers[i]
		partialSig, err := Sign(
			signer.nonces.SecNonce, signer.privKey, combinedNonce,
			signerKeys, msg,
		)
		if err != nil {
			t.Fatalf("unable to generate partial sig: %v", err)
		}

		signers[i].partialSig = partialSig

		if finalNonce == nil {
			finalNonce = partialSig.R
		}
	}

	// Finally we'll combined all the nonces, and ensure that it validates
	// as a single schnorr signature.
	finalSig := CombineSigs(finalNonce, signers.partialSigs())
	if !finalSig.Verify(msg[:], combinedKey) {
		t.Fatalf("final sig is invalid!")
	}
}
