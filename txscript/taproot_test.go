// Copyright (c) 2013-2022 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/hex"
	"fmt"
	prand "math/rand"
	"testing"
	"testing/quick"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"
)

var (
	testPubBytes, _ = hex.DecodeString("F9308A019258C31049344F85F89D5229B" +
		"531C845836F99B08601F113BCE036F9")

	// rootKey is the test root key defined in the test vectors:
	// https://github.com/bitcoin/bips/blob/master/bip-0086.mediawiki
	rootKey, _ = hdkeychain.NewKeyFromString(
		"xprv9s21ZrQH143K3GJpoapnV8SFfukcVBSfeCficPSGfubmSFDxo1kuHnLi" +
			"sriDvSnRRuL2Qrg5ggqHKNVpxR86QEC8w35uxmGoggxtQTPvfUu",
	)

	// accountPath is the base path for BIP86 (m/86'/0'/0').
	accountPath = []uint32{
		86 + hdkeychain.HardenedKeyStart, hdkeychain.HardenedKeyStart,
		hdkeychain.HardenedKeyStart,
	}
	expectedExternalAddresses = []string{
		"bc1p5cyxnuxmeuwuvkwfem96lqzszd02n6xdcjrs20cac6yqjjwudpxqkedrcr",
		"bc1p4qhjn9zdvkux4e44uhx8tc55attvtyu358kutcqkudyccelu0was9fqzwh",
	}
	expectedInternalAddresses = []string{
		"bc1p3qkhfews2uk44qtvauqyr2ttdsw7svhkl9nkm9s9c3x4ax5h60wqwruhk7",
	}
)

// TestControlBlockParsing tests that we're able to generate and parse a valid
// control block.
func TestControlBlockParsing(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		controlBlockGen func() []byte
		valid           bool
	}{
		// An invalid control block, it's only 5 bytes and needs to be
		// at least 33 bytes.
		{
			controlBlockGen: func() []byte {
				return bytes.Repeat([]byte{0x00}, 5)
			},
			valid: false,
		},

		// An invalid control block, it's greater than the largest
		// accepted control block.
		{
			controlBlockGen: func() []byte {
				return bytes.Repeat([]byte{0x00}, ControlBlockMaxSize+1)
			},
			valid: false,
		},

		// An invalid control block, it isn't a multiple of 32 bytes
		// enough though it has a valid starting byte length.
		{
			controlBlockGen: func() []byte {
				return bytes.Repeat([]byte{0x00}, ControlBlockBaseSize+34)
			},
			valid: false,
		},

		// A valid control block, of the largest possible size.
		{
			controlBlockGen: func() []byte {
				privKey, _ := btcec.NewPrivateKey()
				pubKey := privKey.PubKey()

				yIsOdd := (pubKey.SerializeCompressed()[0] ==
					secp.PubKeyFormatCompressedOdd)

				ctrl := ControlBlock{
					InternalKey:     pubKey,
					OutputKeyYIsOdd: yIsOdd,
					LeafVersion:     BaseLeafVersion,
					InclusionProof: bytes.Repeat(
						[]byte{0x00},
						ControlBlockMaxSize-ControlBlockBaseSize,
					),
				}

				ctrlBytes, _ := ctrl.ToBytes()
				return ctrlBytes
			},
			valid: true,
		},

		// A valid control block, only has a single element in the
		// proof as the tree only has a single element.
		{
			controlBlockGen: func() []byte {
				privKey, _ := btcec.NewPrivateKey()
				pubKey := privKey.PubKey()

				yIsOdd := (pubKey.SerializeCompressed()[0] ==
					secp.PubKeyFormatCompressedOdd)

				ctrl := ControlBlock{
					InternalKey:     pubKey,
					OutputKeyYIsOdd: yIsOdd,
					LeafVersion:     BaseLeafVersion,
					InclusionProof: bytes.Repeat(
						[]byte{0x00}, ControlBlockNodeSize,
					),
				}

				ctrlBytes, _ := ctrl.ToBytes()
				return ctrlBytes
			},
			valid: true,
		},
	}
	for i, testCase := range testCases {
		ctrlBlockBytes := testCase.controlBlockGen()

		ctrlBlock, err := ParseControlBlock(ctrlBlockBytes)
		switch {
		case testCase.valid && err != nil:
			t.Fatalf("#%v: unable to parse valid control block: %v", i, err)

		case !testCase.valid && err == nil:
			t.Fatalf("#%v: invalid control block should have failed: %v", i, err)
		}

		if !testCase.valid {
			continue
		}

		// If we serialize the control block, we should get the exact same
		// set of bytes as the input.
		ctrlBytes, err := ctrlBlock.ToBytes()
		if err != nil {
			t.Fatalf("#%v: unable to encode bytes: %v", i, err)
		}
		if !bytes.Equal(ctrlBytes, ctrlBlockBytes) {
			t.Fatalf("#%v: encoding mismatch: expected %x, "+
				"got %x", i, ctrlBlockBytes, ctrlBytes)
		}
	}
}

// TestTaprootScriptSpendTweak tests that for any 32-byte hypothetical script
// root, the resulting tweaked public key is the same as tweaking the private
// key, then generating a public key from that. This test a quickcheck test to
// assert the following invariant:
//
//   - taproot_tweak_pubkey(pubkey_gen(seckey), h)[1] ==
//     pubkey_gen(taproot_tweak_seckey(seckey, h))
func TestTaprootScriptSpendTweak(t *testing.T) {
	t.Parallel()

	// Assert that if we use this x value as the hash of the script root,
	// then if we generate a tweaked public key, it's the same key as if we
	// used that key to generate the tweaked
	// private key, and then generated the public key from that.
	f := func(x [32]byte) bool {
		privKey, err := btcec.NewPrivateKey()
		if err != nil {
			return false
		}

		// Generate the tweaked public key using the x value as the
		// script root.
		tweakedPub := ComputeTaprootOutputKey(privKey.PubKey(), x[:])

		// Now we'll generate the corresponding tweaked private key.
		tweakedPriv := TweakTaprootPrivKey(*privKey, x[:])

		// The public key for this private key should be the same as
		// the tweaked public key we generate above.
		return tweakedPub.IsEqual(tweakedPriv.PubKey()) &&
			bytes.Equal(
				schnorr.SerializePubKey(tweakedPub),
				schnorr.SerializePubKey(tweakedPriv.PubKey()),
			)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("tweaked public/private key mapping is "+
			"incorrect: %v", err)
	}

}

// TestTaprootTweakNoMutation tests that the underlying private key passed into
// TweakTaprootPrivKey is never mutated.
func TestTaprootTweakNoMutation(t *testing.T) {
	t.Parallel()

	// Assert that given a random tweak, and a random private key, that if
	// we tweak the private key it remains unaffected.
	f := func(privBytes, tweak [32]byte) bool {
		privKey, _ := btcec.PrivKeyFromBytes(privBytes[:])

		// Now we'll generate the corresponding tweaked private key.
		tweakedPriv := TweakTaprootPrivKey(*privKey, tweak[:])

		// The tweaked private key and the original private key should
		// NOT be the same.
		if *privKey == *tweakedPriv {
			t.Logf("private key was mutated")
			return false
		}

		// We shuold be able to re-derive the private key from raw
		// bytes and have that match up again.
		privKeyCopy, _ := btcec.PrivKeyFromBytes(privBytes[:])
		if *privKey != *privKeyCopy {
			t.Logf("private doesn't match")
			return false
		}

		return true
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("private key modified: %v", err)
	}
}

// TestTaprootConstructKeyPath tests the key spend only taproot construction.
func TestTaprootConstructKeyPath(t *testing.T) {
	checkPath := func(branch uint32, expectedAddresses []string) {
		path, err := derivePath(rootKey, append(accountPath, branch))
		require.NoError(t, err)

		for index, expectedAddr := range expectedAddresses {
			extendedKey, err := path.Derive(uint32(index))
			require.NoError(t, err)

			pubKey, err := extendedKey.ECPubKey()
			require.NoError(t, err)

			tapKey := ComputeTaprootKeyNoScript(pubKey)

			addr, err := btcutil.NewAddressTaproot(
				schnorr.SerializePubKey(tapKey),
				&chaincfg.MainNetParams,
			)
			require.NoError(t, err)

			require.Equal(t, expectedAddr, addr.String())
		}
	}
	checkPath(0, expectedExternalAddresses)
	checkPath(1, expectedInternalAddresses)
}

func derivePath(key *hdkeychain.ExtendedKey, path []uint32) (
	*hdkeychain.ExtendedKey, error) {

	var (
		currentKey = key
		err        error
	)
	for _, pathPart := range path {
		currentKey, err = currentKey.Derive(pathPart)
		if err != nil {
			return nil, err
		}
	}
	return currentKey, nil
}

// TestTapscriptCommitmentVerification that given a valid control block, proof
// we're able to both generate and validate validate script tree leaf inclusion
// proofs.
func TestTapscriptCommitmentVerification(t *testing.T) {
	t.Parallel()

	// make from 0 to 1 leaf
	// ensure verifies properly
	testCases := []struct {
		numLeaves int

		valid bool

		treeMutateFunc func(*IndexedTapScriptTree)

		ctrlBlockMutateFunc func(*ControlBlock)
	}{
		// A valid merkle proof of a single leaf.
		{
			numLeaves: 1,
			valid:     true,
		},

		// A valid series of merkle proofs with an odd number of leaves.
		{
			numLeaves: 3,
			valid:     true,
		},

		// A valid series of merkle proofs with an even number of leaves.
		{
			numLeaves: 4,
			valid:     true,
		},

		// An invalid merkle proof, we modify the last byte of one of
		// the leaves.
		{
			numLeaves: 4,
			valid:     false,
			treeMutateFunc: func(t *IndexedTapScriptTree) {
				for _, leafProof := range t.LeafMerkleProofs {
					leafProof.InclusionProof[0] ^= 1
				}
			},
		},

		{
			// An invalid series of proofs, we modify the control
			// block to not match the parity of the final output
			// key commitment.
			numLeaves: 2,
			valid:     false,
			ctrlBlockMutateFunc: func(c *ControlBlock) {
				c.OutputKeyYIsOdd = !c.OutputKeyYIsOdd
			},
		},
	}
	for _, testCase := range testCases {
		testName := fmt.Sprintf("num_leaves=%v, valid=%v, treeMutate=%v, "+
			"ctrlBlockMutate=%v", testCase.numLeaves, testCase.valid,
			testCase.treeMutateFunc == nil, testCase.ctrlBlockMutateFunc == nil)

		t.Run(testName, func(t *testing.T) {
			tapScriptLeaves := make([]TapLeaf, testCase.numLeaves)
			for i := 0; i < len(tapScriptLeaves); i++ {
				numLeafBytes := prand.Intn(1000)
				scriptBytes := make([]byte, numLeafBytes)
				if _, err := prand.Read(scriptBytes[:]); err != nil {
					t.Fatalf("unable to read rand bytes: %v", err)
				}
				tapScriptLeaves[i] = NewBaseTapLeaf(scriptBytes)
			}

			scriptTree := AssembleTaprootScriptTree(tapScriptLeaves...)

			if testCase.treeMutateFunc != nil {
				testCase.treeMutateFunc(scriptTree)
			}

			internalKey, _ := btcec.NewPrivateKey()

			rootHash := scriptTree.RootNode.TapHash()
			outputKey := ComputeTaprootOutputKey(
				internalKey.PubKey(), rootHash[:],
			)

			for _, leafProof := range scriptTree.LeafMerkleProofs {
				ctrlBlock := leafProof.ToControlBlock(
					internalKey.PubKey(),
				)

				if testCase.ctrlBlockMutateFunc != nil {
					testCase.ctrlBlockMutateFunc(&ctrlBlock)
				}

				err := VerifyTaprootLeafCommitment(
					&ctrlBlock, schnorr.SerializePubKey(outputKey),
					leafProof.TapLeaf.Script,
				)
				valid := err == nil

				if valid != testCase.valid {
					t.Fatalf("test case mismatch: expected "+
						"valid=%v, got valid=%v", testCase.valid,
						valid)
				}
			}

			// TODO(roasbeef): index correctness
		})
	}
}
