// Copyright (c) 2013-2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"testing"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
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
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash: wire.ShaHash([32]byte{
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
				PkScript: []byte{},
			},
		},
		LockTime: 0,
	}
	pkScript := []byte{txscript.OP_NOP}

	for _, test := range pcTests {
		vm, err := txscript.NewEngine(pkScript, tx, 0, 0)
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
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash: wire.ShaHash([32]byte{
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
				PkScript: []byte{},
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

	vm, err := txscript.NewEngine(pkScript, tx, 0, 0)
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
