// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

// TestBadPC sets the pc to a deliberately bad result then confirms that Step()
// and Disasm fail correctly.
func TestBadPC(t *testing.T) {
	t.Parallel()

	type pcTest struct {
		script, off int
	}
	pcTests := []pcTest{
		{
			script: 2,
			off:    0,
		},
		{
			script: 0,
			off:    2,
		},
	}
	// tx with almost empty scripts.
	tx := &wire.MsgTx{
		SerType: wire.TxSerializeFull,
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
				SignatureScript: []uint8{txscript.OP_NOP},
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
	pkScript := []byte{txscript.OP_NOP}

	for _, test := range pcTests {
		vm, err := txscript.NewEngine(pkScript, tx, 0, 0, 0, nil)
		if err != nil {
			t.Errorf("Failed to create script: %v", err)
		}

		// set to after all scripts
		vm.TstSetPC(test.script, test.off)

		_, err = vm.Step()
		if err == nil {
			t.Errorf("Step with invalid pc (%v) succeeds!", test)
			continue
		}

		_, err = vm.DisasmPC()
		if err == nil {
			t.Errorf("DisasmPC with invalid pc (%v) succeeds!",
				test)
		}
	}
}

// TestCheckErrorCondition tests the execute early test in CheckErrorCondition()
// since most code paths are tested elsewhere.
func TestCheckErrorCondition(t *testing.T) {
	t.Parallel()

	// tx with almost empty scripts.
	tx := &wire.MsgTx{
		SerType: wire.TxSerializeFull,
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
				SignatureScript: []uint8{},
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
	pkScript := []byte{
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_NOP,
		txscript.OP_TRUE,
	}

	vm, err := txscript.NewEngine(pkScript, tx, 0, 0, 0, nil)
	if err != nil {
		t.Errorf("failed to create script: %v", err)
	}

	for i := 0; i < len(pkScript)-1; i++ {
		done, err := vm.Step()
		if err != nil {
			t.Errorf("failed to step %dth time: %v", i, err)
			return
		}
		if done {
			t.Errorf("finshed early on %dth time", i)
			return
		}

		err = vm.CheckErrorCondition(false)
		if err != txscript.ErrStackScriptUnfinished {
			t.Errorf("got unexepected error %v on %dth iteration",
				err, i)
			return
		}
	}
	done, err := vm.Step()
	if err != nil {
		t.Errorf("final step failed %v", err)
		return
	}
	if !done {
		t.Errorf("final step isn't done!")
		return
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

	tests := []txscript.ScriptFlags{
		txscript.ScriptVerifyCleanStack,
	}

	// tx with almost empty scripts.
	tx := &wire.MsgTx{
		SerType: wire.TxSerializeFull,
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
				SignatureScript: []uint8{txscript.OP_NOP},
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
	pkScript := []byte{txscript.OP_NOP}

	for i, test := range tests {
		_, err := txscript.NewEngine(pkScript, tx, 0, test, 0, nil)
		if err != txscript.ErrInvalidFlags {
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
			key: decodeHex("0411db93e1dcdb8a016b49840f8c53bc1eb68" +
				"a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf" +
				"9744464f82e160bfa9b8b64f9d4c03f999b8643f656b" +
				"412a3"),
			isValid: true,
		},
		{
			name: "compressed ok",
			key: decodeHex("02ce0b14fb842b1ba549fdd675c98075f12e9" +
				"c510f8ef52bd021a9a1f4809d3b4d"),
			isValid: true,
		},
		{
			name: "compressed ok",
			key: decodeHex("032689c7c2dab13309fb143e0e8fe39634252" +
				"1887e976690b6b47f5b2a4b7d448e"),
			isValid: true,
		},
		{
			name: "hybrid",
			key: decodeHex("0679be667ef9dcbbac55a06295ce870b07029" +
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

	flags := txscript.ScriptVerifyStrictEncoding
	for _, test := range tests {
		err := txscript.TstCheckPubKeyEncoding(test.key, flags)
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
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
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
			sig: decodeHex("314402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "bad 1st int marker magic",
			sig: decodeHex("304403204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "bad 2nd int marker",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41032018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "short len",
			sig: decodeHex("304302204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long len",
			sig: decodeHex("304502204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long X",
			sig: decodeHex("304402424e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long Y",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022118152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "short Y",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41021918152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "trailing crap",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d0901"),
			isValid: false,
		},
		{
			name: "X == N ",
			sig: decodeHex("30440220fffffffffffffffffffffffffffff" +
				"ffebaaedce6af48a03bbfd25e8cd0364141022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "X == N ",
			sig: decodeHex("30440220fffffffffffffffffffffffffffff" +
				"ffebaaedce6af48a03bbfd25e8cd0364142022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "Y == N",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410220fffff" +
				"ffffffffffffffffffffffffffebaaedce6af48a03bb" +
				"fd25e8cd0364141"),
			isValid: false,
		},
		{
			name: "Y > N",
			sig: decodeHex("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410220fffff" +
				"ffffffffffffffffffffffffffebaaedce6af48a03bb" +
				"fd25e8cd0364142"),
			isValid: false,
		},
		{
			name: "0 len X",
			sig: decodeHex("302402000220181522ec8eca07de4860a4acd" +
				"d12909d831cc56cbbac4622082221a8768d1d09"),
			isValid: false,
		},
		{
			name: "0 len Y",
			sig: decodeHex("302402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410200"),
			isValid: false,
		},
		{
			name: "extra R padding",
			sig: decodeHex("30450221004e45e16932b8af514961a1d3a1a" +
				"25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181" +
				"522ec8eca07de4860a4acdd12909d831cc56cbbac462" +
				"2082221a8768d1d09"),
			isValid: false,
		},
		{
			name: "extra S padding",
			sig: decodeHex("304502204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022100181" +
				"522ec8eca07de4860a4acdd12909d831cc56cbbac462" +
				"2082221a8768d1d09"),
			isValid: false,
		},
	}

	flags := txscript.ScriptVerifyStrictEncoding
	for _, test := range tests {
		err := txscript.TstCheckSignatureEncoding(test.sig, flags)
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
