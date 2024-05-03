// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestBadPC sets the pc to a deliberately bad result then confirms that Step
// and Disasm fail correctly.
func TestBadPC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scriptIdx int
	}{
		{scriptIdx: 2},
		{scriptIdx: 3},
	}

	// tx with almost empty scripts.
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash: chainhash.Hash([32]byte{
						0xc9, 0x97, 0xa5, 0xe5,
						0x6e, 0x10, 0x41, 0x02,
						0xfa, 0x20, 0x9c, 0x6a,
						0x85, 0x2d, 0xd9, 0x06,
						0x60, 0xa2, 0x0b, 0x2d,
						0x9c, 0x35, 0x24, 0x23,
						0xed, 0xce, 0x25, 0x85,
						0x7f, 0xcd, 0x37, 0x04,
					}),
					Index: 0,
				},
				SignatureScript: mustParseShortForm("NOP"),
				Sequence:        4294967295,
			},
		},
		TxOut: []*wire.TxOut{{
			Value:    1000000000,
			PkScript: nil,
		}},
		LockTime: 0,
	}
	pkScript := mustParseShortForm("NOP")

	for _, test := range tests {
		vm, err := NewEngine(pkScript, tx, 0, 0, nil, nil, -1, nil)
		if err != nil {
			t.Errorf("Failed to create script: %v", err)
		}

		// Set to after all scripts.
		vm.scriptIdx = test.scriptIdx

		// Ensure attempting to step fails.
		_, err = vm.Step()
		if err == nil {
			t.Errorf("Step with invalid pc (%v) succeeds!", test)
			continue
		}

		// Ensure attempting to disassemble the current program counter fails.
		_, err = vm.DisasmPC()
		if err == nil {
			t.Errorf("DisasmPC with invalid pc (%v) succeeds!", test)
		}
	}
}

// TestCheckErrorCondition tests the execute early test in CheckErrorCondition()
// since most code paths are tested elsewhere.
func TestCheckErrorCondition(t *testing.T) {
	t.Parallel()

	// tx with almost empty scripts.
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash: chainhash.Hash([32]byte{
					0xc9, 0x97, 0xa5, 0xe5,
					0x6e, 0x10, 0x41, 0x02,
					0xfa, 0x20, 0x9c, 0x6a,
					0x85, 0x2d, 0xd9, 0x06,
					0x60, 0xa2, 0x0b, 0x2d,
					0x9c, 0x35, 0x24, 0x23,
					0xed, 0xce, 0x25, 0x85,
					0x7f, 0xcd, 0x37, 0x04,
				}),
				Index: 0,
			},
			SignatureScript: nil,
			Sequence:        4294967295,
		}},
		TxOut: []*wire.TxOut{{
			Value:    1000000000,
			PkScript: nil,
		}},
		LockTime: 0,
	}
	pkScript := mustParseShortForm("NOP NOP NOP NOP NOP NOP NOP NOP NOP" +
		" NOP TRUE")

	vm, err := NewEngine(pkScript, tx, 0, 0, nil, nil, 0, nil)
	if err != nil {
		t.Errorf("failed to create script: %v", err)
	}

	for i := 0; i < len(pkScript)-1; i++ {
		done, err := vm.Step()
		if err != nil {
			t.Fatalf("failed to step %dth time: %v", i, err)
		}
		if done {
			t.Fatalf("finished early on %dth time", i)
		}

		err = vm.CheckErrorCondition(false)
		if !IsErrorCode(err, ErrScriptUnfinished) {
			t.Fatalf("got unexpected error %v on %dth iteration",
				err, i)
		}
	}
	done, err := vm.Step()
	if err != nil {
		t.Fatalf("final step failed %v", err)
	}
	if !done {
		t.Fatalf("final step isn't done!")
	}

	err = vm.CheckErrorCondition(false)
	if err != nil {
		t.Errorf("unexpected error %v on final check", err)
	}
}

// TestInvalidFlagCombinations ensures the script engine returns the expected
// error when disallowed flag combinations are specified.
func TestInvalidFlagCombinations(t *testing.T) {
	t.Parallel()

	tests := []ScriptFlags{
		ScriptVerifyCleanStack,
	}

	// tx with almost empty scripts.
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash: chainhash.Hash([32]byte{
						0xc9, 0x97, 0xa5, 0xe5,
						0x6e, 0x10, 0x41, 0x02,
						0xfa, 0x20, 0x9c, 0x6a,
						0x85, 0x2d, 0xd9, 0x06,
						0x60, 0xa2, 0x0b, 0x2d,
						0x9c, 0x35, 0x24, 0x23,
						0xed, 0xce, 0x25, 0x85,
						0x7f, 0xcd, 0x37, 0x04,
					}),
					Index: 0,
				},
				SignatureScript: []uint8{OP_NOP},
				Sequence:        4294967295,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value:    1000000000,
				PkScript: nil,
			},
		},
		LockTime: 0,
	}
	pkScript := []byte{OP_NOP}

	for i, test := range tests {
		_, err := NewEngine(pkScript, tx, 0, test, nil, nil, -1, nil)
		if !IsErrorCode(err, ErrInvalidFlags) {
			t.Fatalf("TestInvalidFlagCombinations #%d unexpected "+
				"error: %v", i, err)
		}
	}
}

// TestCheckPubKeyEncoding ensures the internal checkPubKeyEncoding function
// works as expected.
func TestCheckPubKeyEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     []byte
		isValid bool
	}{
		{
			name: "uncompressed ok",
			key: hexToBytes("0411db93e1dcdb8a016b49840f8c53bc1eb68" +
				"a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf" +
				"9744464f82e160bfa9b8b64f9d4c03f999b8643f656b" +
				"412a3"),
			isValid: true,
		},
		{
			name: "compressed ok",
			key: hexToBytes("02ce0b14fb842b1ba549fdd675c98075f12e9" +
				"c510f8ef52bd021a9a1f4809d3b4d"),
			isValid: true,
		},
		{
			name: "compressed ok",
			key: hexToBytes("032689c7c2dab13309fb143e0e8fe39634252" +
				"1887e976690b6b47f5b2a4b7d448e"),
			isValid: true,
		},
		{
			name: "hybrid",
			key: hexToBytes("0679be667ef9dcbbac55a06295ce870b07029" +
				"bfcdb2dce28d959f2815b16f81798483ada7726a3c46" +
				"55da4fbfc0e1108a8fd17b448a68554199c47d08ffb1" +
				"0d4b8"),
			isValid: false,
		},
		{
			name:    "empty",
			key:     nil,
			isValid: false,
		},
	}

	vm := Engine{flags: ScriptVerifyStrictEncoding}
	for _, test := range tests {
		err := vm.checkPubKeyEncoding(test.key)
		if err != nil && test.isValid {
			t.Errorf("checkSignatureEncoding test '%s' failed "+
				"when it should have succeeded: %v", test.name,
				err)
		} else if err == nil && !test.isValid {
			t.Errorf("checkSignatureEncooding test '%s' succeeded "+
				"when it should have failed", test.name)
		}
	}

}

// TestCheckSignatureEncoding ensures the internal checkSignatureEncoding
// function works as expected.
func TestCheckSignatureEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sig     []byte
		isValid bool
	}{
		{
			name: "valid signature",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: true,
		},
		{
			name:    "empty.",
			sig:     nil,
			isValid: false,
		},
		{
			name: "bad magic",
			sig: hexToBytes("314402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "bad 1st int marker magic",
			sig: hexToBytes("304403204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "bad 2nd int marker",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41032018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "short len",
			sig: hexToBytes("304302204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long len",
			sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long X",
			sig: hexToBytes("304402424e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long Y",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022118152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "short Y",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41021918152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "trailing crap",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d0901"),
			isValid: false,
		},
		{
			name: "X == N ",
			sig: hexToBytes("30440220fffffffffffffffffffffffffffff" +
				"ffebaaedce6af48a03bbfd25e8cd0364141022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "X == N ",
			sig: hexToBytes("30440220fffffffffffffffffffffffffffff" +
				"ffebaaedce6af48a03bbfd25e8cd0364142022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "Y == N",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410220fffff" +
				"ffffffffffffffffffffffffffebaaedce6af48a03bb" +
				"fd25e8cd0364141"),
			isValid: false,
		},
		{
			name: "Y > N",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410220fffff" +
				"ffffffffffffffffffffffffffebaaedce6af48a03bb" +
				"fd25e8cd0364142"),
			isValid: false,
		},
		{
			name: "0 len X",
			sig: hexToBytes("302402000220181522ec8eca07de4860a4acd" +
				"d12909d831cc56cbbac4622082221a8768d1d09"),
			isValid: false,
		},
		{
			name: "0 len Y",
			sig: hexToBytes("302402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410200"),
			isValid: false,
		},
		{
			name: "extra R padding",
			sig: hexToBytes("30450221004e45e16932b8af514961a1d3a1a" +
				"25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181" +
				"522ec8eca07de4860a4acdd12909d831cc56cbbac462" +
				"2082221a8768d1d09"),
			isValid: false,
		},
		{
			name: "extra S padding",
			sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022100181" +
				"522ec8eca07de4860a4acdd12909d831cc56cbbac462" +
				"2082221a8768d1d09"),
			isValid: false,
		},
	}

	vm := Engine{flags: ScriptVerifyStrictEncoding}
	for _, test := range tests {
		err := vm.checkSignatureEncoding(test.sig)
		if err != nil && test.isValid {
			t.Errorf("checkSignatureEncoding test '%s' failed "+
				"when it should have succeeded: %v", test.name,
				err)
		} else if err == nil && !test.isValid {
			t.Errorf("checkSignatureEncooding test '%s' succeeded "+
				"when it should have failed", test.name)
		}
	}
}

// TestOpcodeCat tests that scripts with OP_CAT are executed correctly.
func TestOpcodeCat(t *testing.T) {
	t.Parallel()

	// Define helper constants for non-error cases.
	const (
		NO_ERROR = -1
		SUCCESS  = -2
	)

	tests := []struct {
		flags      ScriptFlags
		expErr     ErrorCode
		startStack [][]byte
		expStack   [][]byte
		nonTaproot bool
	}{

		// No elements to cat.
		{
			startStack: [][]byte{},
			flags:      ScriptVerifyOpCat,
			expErr:     ErrInvalidStackOperation,
		},

		// Only a single element to cat.
		{
			startStack: [][]byte{
				{0xaa},
			},
			flags:  ScriptVerifyOpCat,
			expErr: ErrInvalidStackOperation,
		},

		// Normal cat.
		{
			startStack: [][]byte{
				{0xaa},
				{0xbb},
			},
			flags:  ScriptVerifyOpCat,
			expErr: NO_ERROR,
			expStack: [][]byte{
				{0xaa, 0xbb},
			},
		},

		// Disabled in non-taproot context.
		{
			startStack: [][]byte{
				{0xaa},
				{0xbb},
			},
			flags:      ScriptVerifyOpCat,
			expErr:     ErrDisabledOpcode,
			nonTaproot: true,
		},

		// Cat with empty element.
		{
			startStack: [][]byte{
				{0xaa},
				{},
			},
			flags:  ScriptVerifyOpCat,
			expErr: NO_ERROR,
			expStack: [][]byte{
				{0xaa},
			},
		},

		// Cat with empty element.
		{
			startStack: [][]byte{
				{},
				{0xbb},
			},
			flags:  ScriptVerifyOpCat,
			expErr: NO_ERROR,
			expStack: [][]byte{
				{0xbb},
			},
		},

		// Cat elements of different lengths.
		{
			startStack: [][]byte{
				{0xaa, 0xbb, 0xcc},
				{0xdd},
			},

			flags:  ScriptVerifyOpCat,
			expErr: NO_ERROR,
			expStack: [][]byte{
				{0xaa, 0xbb, 0xcc, 0xdd},
			},
		},

		// Cat up to max element size.
		{
			startStack: [][]byte{
				bytes.Repeat(
					[]byte{0xaa}, MaxScriptElementSize-1,
				),
				{0xdd},
			},
			flags:  ScriptVerifyOpCat,
			expErr: NO_ERROR,
			expStack: [][]byte{
				append(
					bytes.Repeat([]byte{0xaa}, MaxScriptElementSize-1),
					0xdd,
				),
			},
		},

		// Failing to when result exceeds max element size.
		{
			startStack: [][]byte{
				bytes.Repeat(
					[]byte{0xaa}, MaxScriptElementSize-1,
				),
				{0xdd, 0xee},
			},
			flags:  ScriptVerifyOpCat,
			expErr: ErrElementTooBig,
		},

		// ======== Flag tests =========

		// Discourage CAT.
		{
			startStack: [][]byte{
				{0xaa}, {0xbb},
			},
			flags:  ScriptVerifyDiscourageOpCat,
			expErr: ErrDiscourageOpSuccess,
		},

		// Discourage CAT when CAT is active.
		{
			startStack: [][]byte{
				{0xaa}, {0xbb},
			},
			flags:  ScriptVerifyDiscourageOpCat | ScriptVerifyOpCat,
			expErr: ErrDiscourageOpSuccess,
		},

		// Valid CAT but CAT is not active. It should behave as
		// OP_SUCCESS.
		{
			startStack: [][]byte{
				{0xaa}, {0xbb},
			},
			expErr: SUCCESS,
		},

		// Invalid CAT when CAT is not active. It should behave as
		// OP_SUCCESS.
		{
			startStack: [][]byte{
				{0xaa},
			},
			expErr: SUCCESS,
		},

		// Invalid CAT when CAT is active.
		{
			startStack: [][]byte{
				{0xaa},
			},
			flags:  ScriptVerifyOpCat,
			expErr: ErrInvalidStackOperation,
		},
	}

	for _, test := range tests {
		tx := wire.NewMsgTx(2)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Index: 0,
			},
		})

		// OP_CAT will be our one and only script opcode apart from
		// making sure the script is valid.
		script, err := NewScriptBuilder().
			AddOp(OP_CAT).
			AddOp(OP_DROP).
			AddInt64(1).
			Script()
		if err != nil {
			t.Error(err)
		}

		// Assmble the script into a taproot output key.
		tapScriptTree := AssembleTaprootScriptTree(
			NewBaseTapLeaf(script),
		)
		privKey, err := btcec.NewPrivateKey()
		if err != nil {
			t.Error(err)
			continue
		}

		inputKey := privKey.PubKey()
		ctrlBlock := tapScriptTree.LeafMerkleProofs[0].ToControlBlock(inputKey)
		tapScriptRootHash := tapScriptTree.RootNode.TapHash()
		inputTapKey := ComputeTaprootOutputKey(
			inputKey, tapScriptRootHash[:],
		)

		inputScript, err := PayToTaprootScript(inputTapKey)
		if err != nil {
			t.Error(err)
			continue

		}

		cbBytes, err := ctrlBlock.ToBytes()
		if err != nil {
			t.Error(err)
			continue
		}

		w := wire.TxWitness{}
		for _, e := range test.startStack {
			w = append(w, e)
		}

		w = append(w, script, cbBytes)
		tx.TxIn[0].Witness = w

		// As a special case, if we are testing non-taproot spends, we
		// recreate the pkscript as a P2WSH.
		if test.nonTaproot {
			scriptHash := sha256.Sum256(script)
			inputScript, err = payToWitnessScriptHashScript(scriptHash[:])
			if err != nil {
				t.Error(err)
				continue
			}
			tx.TxIn[0].Witness = wire.TxWitness{script}
		}

		prevOut := &wire.TxOut{
			Value:    1e8,
			PkScript: inputScript,
		}
		prevOutFetcher := NewCannedPrevOutputFetcher(
			prevOut.PkScript, prevOut.Value,
		)

		sigHashes := NewTxSigHashes(tx, prevOutFetcher)

		// We'll record the stack after the CAT operation, since we
		// want to check it against what we expect.
		finalStack := [][]byte{}
		cb := func(step *StepInfo) error {
			if step.ScriptIndex != 2 {
				return nil
			}

			// Our script has OP_CAT as first opcode.
			if step.OpcodeIndex != 1 {
				return nil
			}

			finalStack = step.Stack
			return nil
		}

		flags := StandardVerifyFlags | test.flags
		vm, err := NewDebugEngine(
			prevOut.PkScript, tx, 0, flags, nil,
			sigHashes, prevOut.Value, nil, cb,
		)
		if err != nil {
			t.Errorf("Failed to create script: %v", err)
			continue
		}

		err = vm.Execute()
		if err != nil {
			if !IsErrorCode(err, test.expErr) {
				t.Errorf("Expected error %v, got %v", test.expErr, err)
			}
		} else {
			// Check expected stack if we didn't expect error
			// during execution. We skip this check for the SUCCESS
			// case, as stack is not altered at all.
			if test.expErr == NO_ERROR {
				require.Equal(t, test.expStack, finalStack)
			} else if test.expErr != SUCCESS {
				t.Errorf("Expected error %v, got %v", test.expErr, err)
			}
		}
	}
}
