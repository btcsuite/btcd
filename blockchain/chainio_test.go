// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

// TestErrNotInMainChain ensures the functions related to errNotInMainChain work
// as expected.
func TestErrNotInMainChain(t *testing.T) {
	errStr := "no block at height 1 exists"
	err := error(errNotInMainChain(errStr))

	// Ensure the stringized output for the error is as expected.
	if err.Error() != errStr {
		t.Fatalf("errNotInMainChain retuned unexpected error string - "+
			"got %q, want %q", err.Error(), errStr)
	}

	// Ensure error is detected as the correct type.
	if !isNotInMainChainErr(err) {
		t.Fatalf("isNotInMainChainErr did not detect as expected type")
	}
	err = errors.New("something else")
	if isNotInMainChainErr(err) {
		t.Fatalf("isNotInMainChainErr detected incorrect type")
	}
}

// maybeDecompress decompresses the amount and public key script fields of the
// stxo and marks it decompressed if needed.
func (o *spentTxOut) maybeDecompress(version int32) {
	// Nothing to do if it's not compressed.
	if !o.compressed {
		return
	}

	o.amount = int64(decompressTxOutAmount(uint64(o.amount)))
	o.pkScript = decompressScript(o.pkScript, version)
	o.compressed = false
}

// TestStxoSerialization ensures serializing and deserializing spent transaction
// output entries works as expected.
func TestStxoSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stxo       spentTxOut
		txVersion  int32 // When the txout is not fully spent.
		serialized []byte
	}{
		// From block 170 in main blockchain.
		{
			name: "Spends last output of coinbase",
			stxo: spentTxOut{
				amount:     5000000000,
				pkScript:   hexToBytes("410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac"),
				isCoinBase: true,
				height:     9,
				version:    1,
			},
			serialized: hexToBytes("1301320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c"),
		},
		// Adapted from block 100025 in main blockchain.
		{
			name: "Spends last output of non coinbase",
			stxo: spentTxOut{
				amount:     13761000000,
				pkScript:   hexToBytes("76a914b2fb57eadf61e106a100a7445a8c3f67898841ec88ac"),
				isCoinBase: false,
				height:     100024,
				version:    1,
			},
			serialized: hexToBytes("8b99700186c64700b2fb57eadf61e106a100a7445a8c3f67898841ec"),
		},
		// Adapted from block 100025 in main blockchain.
		{
			name: "Does not spend last output",
			stxo: spentTxOut{
				amount:   34405000000,
				pkScript: hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688ac"),
				version:  1,
			},
			txVersion:  1,
			serialized: hexToBytes("0091f20f006edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
		},
	}

	for _, test := range tests {
		// Ensure the function to calculate the serialized size without
		// actually serializing it is calculated properly.
		gotSize := spentTxOutSerializeSize(&test.stxo)
		if gotSize != len(test.serialized) {
			t.Errorf("spentTxOutSerializeSize (%s): did not get "+
				"expected size - got %d, want %d", test.name,
				gotSize, len(test.serialized))
			continue
		}

		// Ensure the stxo serializes to the expected value.
		gotSerialized := make([]byte, gotSize)
		gotBytesWritten := putSpentTxOut(gotSerialized, &test.stxo)
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("putSpentTxOut (%s): did not get expected "+
				"bytes - got %x, want %x", test.name,
				gotSerialized, test.serialized)
			continue
		}
		if gotBytesWritten != len(test.serialized) {
			t.Errorf("putSpentTxOut (%s): did not get expected "+
				"number of bytes written - got %d, want %d",
				test.name, gotBytesWritten,
				len(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// stxo.
		var gotStxo spentTxOut
		gotBytesRead, err := decodeSpentTxOut(test.serialized, &gotStxo,
			test.txVersion)
		if err != nil {
			t.Errorf("decodeSpentTxOut (%s): unexpected error: %v",
				test.name, err)
			continue
		}
		gotStxo.maybeDecompress(test.stxo.version)
		if !reflect.DeepEqual(gotStxo, test.stxo) {
			t.Errorf("decodeSpentTxOut (%s) mismatched entries - "+
				"got %v, want %v", test.name, gotStxo, test.stxo)
			continue
		}
		if gotBytesRead != len(test.serialized) {
			t.Errorf("decodeSpentTxOut (%s): did not get expected "+
				"number of bytes read - got %d, want %d",
				test.name, gotBytesRead, len(test.serialized))
			continue
		}
	}
}

// TestStxoDecodeErrors performs negative tests against decoding spent
// transaction outputs to ensure error paths work as expected.
func TestStxoDecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stxo       spentTxOut
		txVersion  int32 // When the txout is not fully spent.
		serialized []byte
		bytesRead  int // Expected number of bytes read.
		errType    error
	}{
		{
			name:       "nothing serialized",
			stxo:       spentTxOut{},
			serialized: hexToBytes(""),
			errType:    errDeserialize(""),
			bytesRead:  0,
		},
		{
			name:       "no data after header code w/o version",
			stxo:       spentTxOut{},
			serialized: hexToBytes("00"),
			errType:    errDeserialize(""),
			bytesRead:  1,
		},
		{
			name:       "no data after header code with version",
			stxo:       spentTxOut{},
			serialized: hexToBytes("13"),
			errType:    errDeserialize(""),
			bytesRead:  1,
		},
		{
			name:       "no data after version",
			stxo:       spentTxOut{},
			serialized: hexToBytes("1301"),
			errType:    errDeserialize(""),
			bytesRead:  2,
		},
		{
			name:       "no serialized tx version and passed 0",
			stxo:       spentTxOut{},
			serialized: hexToBytes("003205"),
			errType:    AssertError(""),
			bytesRead:  1,
		},
		{
			name:       "incomplete compressed txout",
			stxo:       spentTxOut{},
			txVersion:  1,
			serialized: hexToBytes("0032"),
			errType:    errDeserialize(""),
			bytesRead:  2,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		gotBytesRead, err := decodeSpentTxOut(test.serialized,
			&test.stxo, test.txVersion)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("decodeSpentTxOut (%s): expected error type "+
				"does not match - got %T, want %T", test.name,
				err, test.errType)
			continue
		}

		// Ensure the expected number of bytes read is returned.
		if gotBytesRead != test.bytesRead {
			t.Errorf("decodeSpentTxOut (%s): unexpected number of "+
				"bytes read - got %d, want %d", test.name,
				gotBytesRead, test.bytesRead)
			continue
		}
	}
}

// TestSpendJournalSerialization ensures serializing and deserializing spend
// journal entries works as expected.
func TestSpendJournalSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entry      []spentTxOut
		blockTxns  []*wire.MsgTx
		utxoView   *UtxoViewpoint
		serialized []byte
	}{
		// From block 2 in main blockchain.
		{
			name:       "No spends",
			entry:      nil,
			blockTxns:  nil,
			utxoView:   NewUtxoViewpoint(),
			serialized: nil,
		},
		// From block 170 in main blockchain.
		{
			name: "One tx with one input spends last output of coinbase",
			entry: []spentTxOut{{
				amount:     5000000000,
				pkScript:   hexToBytes("410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac"),
				isCoinBase: true,
				height:     9,
				version:    1,
			}},
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        0xffffffff,
				}},
				TxOut: []*wire.TxOut{{
					Value:    1000000000,
					PkScript: hexToBytes("4104ae1a62fe09c5f51b13905f07f06b99a2f7159b2225f374cd378d71302fa28414e7aab37397f554a7df5f142c21c1b7303b8a0626f1baded5c72a704f7e6cd84cac"),
				}, {
					Value:    4000000000,
					PkScript: hexToBytes("410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac"),
				}},
				LockTime: 0,
			}},
			utxoView:   NewUtxoViewpoint(),
			serialized: hexToBytes("1301320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c"),
		},
		// Adapted from block 100025 in main blockchain.
		{
			name: "Two txns when one spends last output, one doesn't",
			entry: []spentTxOut{{
				amount:   34405000000,
				pkScript: hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688ac"),
				version:  1,
			}, {
				amount:     13761000000,
				pkScript:   hexToBytes("76a914b2fb57eadf61e106a100a7445a8c3f67898841ec88ac"),
				isCoinBase: false,
				height:     100024,
				version:    1,
			}},
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"),
						Index: 1,
					},
					SignatureScript: hexToBytes("493046022100c167eead9840da4a033c9a56470d7794a9bb1605b377ebe5688499b39f94be59022100fb6345cab4324f9ea0b9ee9169337534834638d818129778370f7d378ee4a325014104d962cac5390f12ddb7539507065d0def320d68c040f2e73337c3a1aaaab7195cb5c4d02e0959624d534f3c10c3cf3d73ca5065ebd62ae986b04c6d090d32627c"),
					Sequence:        0xffffffff,
				}},
				TxOut: []*wire.TxOut{{
					Value:    5000000,
					PkScript: hexToBytes("76a914f419b8db4ba65f3b6fcc233acb762ca6f51c23d488ac"),
				}, {
					Value:    34400000000,
					PkScript: hexToBytes("76a914cadf4fc336ab3c6a4610b75f31ba0676b7f663d288ac"),
				}},
				LockTime: 0,
			}, {
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("92fbe1d4be82f765dfabc9559d4620864b05cc897c4db0e29adac92d294e52b7"),
						Index: 0,
					},
					SignatureScript: hexToBytes("483045022100e256743154c097465cf13e89955e1c9ff2e55c46051b627751dee0144183157e02201d8d4f02cde8496aae66768f94d35ce54465bd4ae8836004992d3216a93a13f00141049d23ce8686fe9b802a7a938e8952174d35dd2c2089d4112001ed8089023ab4f93a3c9fcd5bfeaa9727858bf640dc1b1c05ec3b434bb59837f8640e8810e87742"),
					Sequence:        0xffffffff,
				}},
				TxOut: []*wire.TxOut{{
					Value:    5000000,
					PkScript: hexToBytes("76a914a983ad7c92c38fc0e2025212e9f972204c6e687088ac"),
				}, {
					Value:    13756000000,
					PkScript: hexToBytes("76a914a6ebd69952ab486a7a300bfffdcb395dc7d47c2388ac"),
				}},
				LockTime: 0,
			}},
			utxoView: &UtxoViewpoint{entries: map[chainhash.Hash]*UtxoEntry{
				*newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"): {
					version:     1,
					isCoinBase:  false,
					blockHeight: 100024,
					sparseOutputs: map[uint32]*utxoOutput{
						1: {
							amount:   34405000000,
							pkScript: hexToBytes("76a9142084541c3931677527a7eafe56fd90207c344eb088ac"),
						},
					},
				},
			}},
			serialized: hexToBytes("8b99700186c64700b2fb57eadf61e106a100a7445a8c3f67898841ec0091f20f006edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
		},
		// Hand crafted.
		{
			name: "One tx, two inputs from same tx, neither spend last output",
			entry: []spentTxOut{{
				amount:   165125632,
				pkScript: hexToBytes("51"),
				version:  1,
			}, {
				amount:   154370000,
				pkScript: hexToBytes("51"),
				version:  1,
			}},
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"),
						Index: 1,
					},
					SignatureScript: hexToBytes(""),
					Sequence:        0xffffffff,
				}, {
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"),
						Index: 2,
					},
					SignatureScript: hexToBytes(""),
					Sequence:        0xffffffff,
				}},
				TxOut: []*wire.TxOut{{
					Value:    165125632,
					PkScript: hexToBytes("51"),
				}, {
					Value:    154370000,
					PkScript: hexToBytes("51"),
				}},
				LockTime: 0,
			}},
			utxoView: &UtxoViewpoint{entries: map[chainhash.Hash]*UtxoEntry{
				*newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"): {
					version:     1,
					isCoinBase:  false,
					blockHeight: 100000,
					sparseOutputs: map[uint32]*utxoOutput{
						0: {
							amount:   165712179,
							pkScript: hexToBytes("51"),
						},
					},
				},
			}},
			serialized: hexToBytes("0087bc3707510084c3d19a790751"),
		},
	}

	for i, test := range tests {
		// Ensure the journal entry serializes to the expected value.
		gotBytes := serializeSpendJournalEntry(test.entry)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeSpendJournalEntry #%d (%s): "+
				"mismatched bytes - got %x, want %x", i,
				test.name, gotBytes, test.serialized)
			continue
		}

		// Deserialize to a spend journal entry.
		gotEntry, err := deserializeSpendJournalEntry(test.serialized,
			test.blockTxns, test.utxoView)
		if err != nil {
			t.Errorf("deserializeSpendJournalEntry #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		for stxoIdx := range gotEntry {
			stxo := &gotEntry[stxoIdx]
			stxo.maybeDecompress(test.entry[stxoIdx].version)
		}

		// Ensure that the deserialized spend journal entry has the
		// correct properties.
		if !reflect.DeepEqual(gotEntry, test.entry) {
			t.Errorf("deserializeSpendJournalEntry #%d (%s) "+
				"mismatched entries - got %v, want %v",
				i, test.name, gotEntry, test.entry)
			continue
		}
	}
}

// TestSpendJournalErrors performs negative tests against deserializing spend
// journal entries to ensure error paths work as expected.
func TestSpendJournalErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		blockTxns  []*wire.MsgTx
		utxoView   *UtxoViewpoint
		serialized []byte
		errType    error
	}{
		// Adapted from block 170 in main blockchain.
		{
			name: "Force assertion due to missing stxos",
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        0xffffffff,
				}},
				LockTime: 0,
			}},
			utxoView:   NewUtxoViewpoint(),
			serialized: hexToBytes(""),
			errType:    AssertError(""),
		},
		{
			name: "Force deserialization error in stxos",
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        0xffffffff,
				}},
				LockTime: 0,
			}},
			utxoView:   NewUtxoViewpoint(),
			serialized: hexToBytes("1301320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a"),
			errType:    errDeserialize(""),
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned and the returned
		// slice is nil.
		stxos, err := deserializeSpendJournalEntry(test.serialized,
			test.blockTxns, test.utxoView)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeSpendJournalEntry (%s): expected "+
				"error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if stxos != nil {
			t.Errorf("deserializeSpendJournalEntry (%s): returned "+
				"slice of spent transaction outputs is not nil",
				test.name)
			continue
		}
	}
}

// TestUtxoSerialization ensures serializing and deserializing unspent
// trasaction output entries works as expected.
func TestUtxoSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entry      *UtxoEntry
		serialized []byte
	}{
		// From tx in main blockchain:
		// 0e3e2357e806b6cdb1f70b54c3a3a17b6714ee1f0e68bebb44a74b1efd512098
		{
			name: "Only output 0, coinbase",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  true,
				blockHeight: 1,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:   5000000000,
						pkScript: hexToBytes("410496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52da7589379515d4e0a604f8141781e62294721166bf621e73a82cbf2342c858eeac"),
					},
				},
			},
			serialized: hexToBytes("010103320496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52"),
		},
		// From tx in main blockchain:
		// 8131ffb0a2c945ecaf9b9063e59558784f9c3a74741ce6ae2a18d0571dac15bb
		{
			name: "Only output 1, not coinbase",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  false,
				blockHeight: 100001,
				sparseOutputs: map[uint32]*utxoOutput{
					1: {
						amount:   1000000,
						pkScript: hexToBytes("76a914ee8bd501094a7d5ca318da2506de35e1cb025ddc88ac"),
					},
				},
			},
			serialized: hexToBytes("01858c21040700ee8bd501094a7d5ca318da2506de35e1cb025ddc"),
		},
		// Adapted from tx in main blockchain:
		// df3f3f442d9699857f7f49de4ff0b5d0f3448bec31cdc7b5bf6d25f2abd637d5
		{
			name: "Only output 2, coinbase",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  true,
				blockHeight: 99004,
				sparseOutputs: map[uint32]*utxoOutput{
					2: {
						amount:   100937281,
						pkScript: hexToBytes("76a914da33f77cee27c2a975ed5124d7e4f7f97513510188ac"),
					},
				},
			},
			serialized: hexToBytes("0185843c010182b095bf4100da33f77cee27c2a975ed5124d7e4f7f975135101"),
		},
		// Adapted from tx in main blockchain:
		// 4a16969aa4764dd7507fc1de7f0baa4850a246de90c45e59a3207f9a26b5036f
		{
			name: "outputs 0 and 2 not coinbase",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  false,
				blockHeight: 113931,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:   20000000,
						pkScript: hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
					},
					2: {
						amount:   15000000,
						pkScript: hexToBytes("76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac"),
					},
				},
			},
			serialized: hexToBytes("0185f90b0a011200e2ccd6ec7c6e2e581349c77e067385fa8236bf8a800900b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		// Adapted from tx in main blockchain:
		// 4a16969aa4764dd7507fc1de7f0baa4850a246de90c45e59a3207f9a26b5036f
		{
			name: "outputs 0 and 2, not coinbase, 1 marked spent",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  false,
				blockHeight: 113931,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:   20000000,
						pkScript: hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
					},
					1: { // This won't be serialized.
						spent:    true,
						amount:   1000000,
						pkScript: hexToBytes("76a914e43031c3e46f20bf1ccee9553ce815de5a48467588ac"),
					},
					2: {
						amount:   15000000,
						pkScript: hexToBytes("76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac"),
					},
				},
			},
			serialized: hexToBytes("0185f90b0a011200e2ccd6ec7c6e2e581349c77e067385fa8236bf8a800900b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		// Adapted from tx in main blockchain:
		// 4a16969aa4764dd7507fc1de7f0baa4850a246de90c45e59a3207f9a26b5036f
		{
			name: "outputs 0 and 2, not coinbase, output 2 compressed",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  false,
				blockHeight: 113931,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:   20000000,
						pkScript: hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
					},
					2: {
						// Uncompressed Amount: 15000000
						// Uncompressed PkScript: 76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac
						compressed: true,
						amount:     137,
						pkScript:   hexToBytes("00b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
					},
				},
			},
			serialized: hexToBytes("0185f90b0a011200e2ccd6ec7c6e2e581349c77e067385fa8236bf8a800900b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		// Adapted from tx in main blockchain:
		// 4a16969aa4764dd7507fc1de7f0baa4850a246de90c45e59a3207f9a26b5036f
		{
			name: "outputs 0 and 2, not coinbase, output 2 compressed, packed indexes reversed",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  false,
				blockHeight: 113931,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:   20000000,
						pkScript: hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
					},
					2: {
						// Uncompressed Amount: 15000000
						// Uncompressed PkScript: 76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac
						compressed: true,
						amount:     137,
						pkScript:   hexToBytes("00b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
					},
				},
			},
			serialized: hexToBytes("0185f90b0a011200e2ccd6ec7c6e2e581349c77e067385fa8236bf8a800900b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		// From tx in main blockchain:
		// 0e3e2357e806b6cdb1f70b54c3a3a17b6714ee1f0e68bebb44a74b1efd512098
		{
			name: "Only output 0, coinbase, fully spent",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  true,
				blockHeight: 1,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						spent:    true,
						amount:   5000000000,
						pkScript: hexToBytes("410496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52da7589379515d4e0a604f8141781e62294721166bf621e73a82cbf2342c858eeac"),
					},
				},
			},
			serialized: nil,
		},
		// Adapted from tx in main blockchain:
		// 1b02d1c8cfef60a189017b9a420c682cf4a0028175f2f563209e4ff61c8c3620
		{
			name: "Only output 22, not coinbase",
			entry: &UtxoEntry{
				version:     1,
				isCoinBase:  false,
				blockHeight: 338156,
				sparseOutputs: map[uint32]*utxoOutput{
					22: {
						spent:    false,
						amount:   366875659,
						pkScript: hexToBytes("a9141dd46a006572d820e448e12d2bbb38640bc718e687"),
					},
				},
			},
			serialized: hexToBytes("0193d06c100000108ba5b9e763011dd46a006572d820e448e12d2bbb38640bc718e6"),
		},
	}

	for i, test := range tests {
		// Ensure the utxo entry serializes to the expected value.
		gotBytes, err := serializeUtxoEntry(test.entry)
		if err != nil {
			t.Errorf("serializeUtxoEntry #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeUtxoEntry #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Don't try to deserialize if the test entry was fully spent
		// since it will have a nil serialization.
		if test.entry.IsFullySpent() {
			continue
		}

		// Deserialize to a utxo entry.
		utxoEntry, err := deserializeUtxoEntry(test.serialized)
		if err != nil {
			t.Errorf("deserializeUtxoEntry #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}

		// Ensure that the deserialized utxo entry has the same
		// properties for the containing transaction and block height.
		if utxoEntry.Version() != test.entry.Version() {
			t.Errorf("deserializeUtxoEntry #%d (%s) mismatched "+
				"version: got %d, want %d", i, test.name,
				utxoEntry.Version(), test.entry.Version())
			continue
		}
		if utxoEntry.IsCoinBase() != test.entry.IsCoinBase() {
			t.Errorf("deserializeUtxoEntry #%d (%s) mismatched "+
				"coinbase flag: got %v, want %v", i, test.name,
				utxoEntry.IsCoinBase(), test.entry.IsCoinBase())
			continue
		}
		if utxoEntry.BlockHeight() != test.entry.BlockHeight() {
			t.Errorf("deserializeUtxoEntry #%d (%s) mismatched "+
				"block height: got %d, want %d", i, test.name,
				utxoEntry.BlockHeight(),
				test.entry.BlockHeight())
			continue
		}
		if utxoEntry.IsFullySpent() != test.entry.IsFullySpent() {
			t.Errorf("deserializeUtxoEntry #%d (%s) mismatched "+
				"fully spent: got %v, want %v", i, test.name,
				utxoEntry.IsFullySpent(),
				test.entry.IsFullySpent())
			continue
		}

		// Ensure all of the outputs in the test entry match the
		// spentness of the output in the deserialized entry and the
		// deserialized entry does not contain any additional utxos.
		var numUnspent int
		for outputIndex := range test.entry.sparseOutputs {
			gotSpent := utxoEntry.IsOutputSpent(outputIndex)
			wantSpent := test.entry.IsOutputSpent(outputIndex)
			if !wantSpent {
				numUnspent++
			}
			if gotSpent != wantSpent {
				t.Errorf("deserializeUtxoEntry #%d (%s) output "+
					"#%d: mismatched spent: got %v, want "+
					"%v", i, test.name, outputIndex,
					gotSpent, wantSpent)
				continue

			}
		}
		if len(utxoEntry.sparseOutputs) != numUnspent {
			t.Errorf("deserializeUtxoEntry #%d (%s): mismatched "+
				"number of unspent outputs: got %d, want %d", i,
				test.name, len(utxoEntry.sparseOutputs),
				numUnspent)
			continue
		}

		// Ensure all of the amounts and scripts of the utxos in the
		// deserialized entry match the ones in the test entry.
		for outputIndex := range utxoEntry.sparseOutputs {
			gotAmount := utxoEntry.AmountByIndex(outputIndex)
			wantAmount := test.entry.AmountByIndex(outputIndex)
			if gotAmount != wantAmount {
				t.Errorf("deserializeUtxoEntry #%d (%s) "+
					"output #%d: mismatched amounts: got "+
					"%d, want %d", i, test.name,
					outputIndex, gotAmount, wantAmount)
				continue
			}

			gotPkScript := utxoEntry.PkScriptByIndex(outputIndex)
			wantPkScript := test.entry.PkScriptByIndex(outputIndex)
			if !bytes.Equal(gotPkScript, wantPkScript) {
				t.Errorf("deserializeUtxoEntry #%d (%s) "+
					"output #%d mismatched scripts: got "+
					"%x, want %x", i, test.name,
					outputIndex, gotPkScript, wantPkScript)
				continue
			}
		}
	}
}

// TestUtxoEntryHeaderCodeErrors performs negative tests against unspent
// transaction output header codes to ensure error paths work as expected.
func TestUtxoEntryHeaderCodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entry     *UtxoEntry
		code      uint64
		bytesRead int // Expected number of bytes read.
		errType   error
	}{
		{
			name:      "Force assertion due to fully spent tx",
			entry:     &UtxoEntry{},
			errType:   AssertError(""),
			bytesRead: 0,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned and the code is 0.
		code, gotBytesRead, err := utxoEntryHeaderCode(test.entry, 0)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("utxoEntryHeaderCode (%s): expected error "+
				"type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if code != 0 {
			t.Errorf("utxoEntryHeaderCode (%s): unexpected code "+
				"on error - got %d, want 0", test.name, code)
			continue
		}

		// Ensure the expected number of bytes read is returned.
		if gotBytesRead != test.bytesRead {
			t.Errorf("utxoEntryHeaderCode (%s): unexpected number "+
				"of bytes read - got %d, want %d", test.name,
				gotBytesRead, test.bytesRead)
			continue
		}
	}
}

// TestUtxoEntryDeserializeErrors performs negative tests against deserializing
// unspent transaction outputs to ensure error paths work as expected.
func TestUtxoEntryDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errType    error
	}{
		{
			name:       "no data after version",
			serialized: hexToBytes("01"),
			errType:    errDeserialize(""),
		},
		{
			name:       "no data after block height",
			serialized: hexToBytes("0101"),
			errType:    errDeserialize(""),
		},
		{
			name:       "no data after header code",
			serialized: hexToBytes("010102"),
			errType:    errDeserialize(""),
		},
		{
			name:       "not enough bytes for unspentness bitmap",
			serialized: hexToBytes("01017800"),
			errType:    errDeserialize(""),
		},
		{
			name:       "incomplete compressed txout",
			serialized: hexToBytes("01010232"),
			errType:    errDeserialize(""),
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned and the returned
		// entry is nil.
		entry, err := deserializeUtxoEntry(test.serialized)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeUtxoEntry (%s): expected error "+
				"type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if entry != nil {
			t.Errorf("deserializeUtxoEntry (%s): returned entry "+
				"is not nil", test.name)
			continue
		}
	}
}

// TestBestChainStateSerialization ensures serializing and deserializing the
// best chain state works as expected.
func TestBestChainStateSerialization(t *testing.T) {
	t.Parallel()

	workSum := new(big.Int)
	tests := []struct {
		name       string
		state      bestChainState
		serialized []byte
	}{
		{
			name: "genesis",
			state: bestChainState{
				hash:      *newHashFromStr("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"),
				height:    0,
				totalTxns: 1,
				workSum: func() *big.Int {
					workSum.Add(workSum, CalcWork(486604799))
					return new(big.Int).Set(workSum)
				}(), // 0x0100010001
			},
			serialized: hexToBytes("6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d6190000000000000000000100000000000000050000000100010001"),
		},
		{
			name: "block 1",
			state: bestChainState{
				hash:      *newHashFromStr("00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048"),
				height:    1,
				totalTxns: 2,
				workSum: func() *big.Int {
					workSum.Add(workSum, CalcWork(486604799))
					return new(big.Int).Set(workSum)
				}(), // 0x0200020002
			},
			serialized: hexToBytes("4860eb18bf1b1620e37e9490fc8a427514416fd75159ab86688e9a8300000000010000000200000000000000050000000200020002"),
		},
	}

	for i, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeBestChainState(test.state)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeBestChainState #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		state, err := deserializeBestChainState(test.serialized)
		if err != nil {
			t.Errorf("deserializeBestChainState #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(state, test.state) {
			t.Errorf("deserializeBestChainState #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, state, test.state)
			continue

		}
	}
}

// TestBestChainStateDeserializeErrors performs negative tests against
// deserializing the chain state to ensure error paths work as expected.
func TestBestChainStateDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errType    error
	}{
		{
			name:       "nothing serialized",
			serialized: hexToBytes(""),
			errType:    database.Error{ErrorCode: database.ErrCorruption},
		},
		{
			name:       "short data in hash",
			serialized: hexToBytes("0000"),
			errType:    database.Error{ErrorCode: database.ErrCorruption},
		},
		{
			name:       "short data in work sum",
			serialized: hexToBytes("6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d61900000000000000000001000000000000000500000001000100"),
			errType:    database.Error{ErrorCode: database.ErrCorruption},
		},
	}

	for _, test := range tests {
		// Ensure the expected error type and code is returned.
		_, err := deserializeBestChainState(test.serialized)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeBestChainState (%s): expected "+
				"error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if derr, ok := err.(database.Error); ok {
			tderr := test.errType.(database.Error)
			if derr.ErrorCode != tderr.ErrorCode {
				t.Errorf("deserializeBestChainState (%s): "+
					"wrong  error code got: %v, want: %v",
					test.name, derr.ErrorCode,
					tderr.ErrorCode)
				continue
			}
		}
	}
}
