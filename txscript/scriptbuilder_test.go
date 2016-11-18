// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"bytes"
	"testing"

	"github.com/decred/dcrd/txscript"
)

// TestScriptBuilderAddOp tests that pushing opcodes to a script via the
// ScriptBuilder API works as expected.
func TestScriptBuilderAddOp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opcodes  []byte
		expected []byte
	}{
		{
			name:     "push OP_0",
			opcodes:  []byte{txscript.OP_0},
			expected: []byte{txscript.OP_0},
		},
		{
			name:     "push OP_1 OP_2",
			opcodes:  []byte{txscript.OP_1, txscript.OP_2},
			expected: []byte{txscript.OP_1, txscript.OP_2},
		},
		{
			name:     "push OP_HASH160 OP_EQUAL",
			opcodes:  []byte{txscript.OP_HASH160, txscript.OP_EQUAL},
			expected: []byte{txscript.OP_HASH160, txscript.OP_EQUAL},
		},
	}

	// Run tests and individually add each op via AddOp.
	builder := txscript.NewScriptBuilder()
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		builder.Reset()
		for _, opcode := range test.opcodes {
			builder.AddOp(opcode)
		}
		result, err := builder.Script()
		if err != nil {
			t.Errorf("ScriptBuilder.AddOp #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}
		if !bytes.Equal(result, test.expected) {
			t.Errorf("ScriptBuilder.AddOp #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, result,
				test.expected)
			continue
		}
	}

	// Run tests and bulk add ops via AddOps.
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		builder.Reset()
		result, err := builder.AddOps(test.opcodes).Script()
		if err != nil {
			t.Errorf("ScriptBuilder.AddOps #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}
		if !bytes.Equal(result, test.expected) {
			t.Errorf("ScriptBuilder.AddOps #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, result,
				test.expected)
			continue
		}
	}

}

// TestScriptBuilderAddInt64 tests that pushing signed integers to a script via
// the ScriptBuilder API works as expected.
func TestScriptBuilderAddInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		val      int64
		expected []byte
	}{
		{name: "push -1", val: -1, expected: []byte{txscript.OP_1NEGATE}},
		{name: "push small int 0", val: 0, expected: []byte{txscript.OP_0}},
		{name: "push small int 1", val: 1, expected: []byte{txscript.OP_1}},
		{name: "push small int 2", val: 2, expected: []byte{txscript.OP_2}},
		{name: "push small int 3", val: 3, expected: []byte{txscript.OP_3}},
		{name: "push small int 4", val: 4, expected: []byte{txscript.OP_4}},
		{name: "push small int 5", val: 5, expected: []byte{txscript.OP_5}},
		{name: "push small int 6", val: 6, expected: []byte{txscript.OP_6}},
		{name: "push small int 7", val: 7, expected: []byte{txscript.OP_7}},
		{name: "push small int 8", val: 8, expected: []byte{txscript.OP_8}},
		{name: "push small int 9", val: 9, expected: []byte{txscript.OP_9}},
		{name: "push small int 10", val: 10, expected: []byte{txscript.OP_10}},
		{name: "push small int 11", val: 11, expected: []byte{txscript.OP_11}},
		{name: "push small int 12", val: 12, expected: []byte{txscript.OP_12}},
		{name: "push small int 13", val: 13, expected: []byte{txscript.OP_13}},
		{name: "push small int 14", val: 14, expected: []byte{txscript.OP_14}},
		{name: "push small int 15", val: 15, expected: []byte{txscript.OP_15}},
		{name: "push small int 16", val: 16, expected: []byte{txscript.OP_16}},
		{name: "push 17", val: 17, expected: []byte{txscript.OP_DATA_1, 0x11}},
		{name: "push 65", val: 65, expected: []byte{txscript.OP_DATA_1, 0x41}},
		{name: "push 127", val: 127, expected: []byte{txscript.OP_DATA_1, 0x7f}},
		{name: "push 128", val: 128, expected: []byte{txscript.OP_DATA_2, 0x80, 0}},
		{name: "push 255", val: 255, expected: []byte{txscript.OP_DATA_2, 0xff, 0}},
		{name: "push 256", val: 256, expected: []byte{txscript.OP_DATA_2, 0, 0x01}},
		{name: "push 32767", val: 32767, expected: []byte{txscript.OP_DATA_2, 0xff, 0x7f}},
		{name: "push 32768", val: 32768, expected: []byte{txscript.OP_DATA_3, 0, 0x80, 0}},
		{name: "push -2", val: -2, expected: []byte{txscript.OP_DATA_1, 0x82}},
		{name: "push -3", val: -3, expected: []byte{txscript.OP_DATA_1, 0x83}},
		{name: "push -4", val: -4, expected: []byte{txscript.OP_DATA_1, 0x84}},
		{name: "push -5", val: -5, expected: []byte{txscript.OP_DATA_1, 0x85}},
		{name: "push -17", val: -17, expected: []byte{txscript.OP_DATA_1, 0x91}},
		{name: "push -65", val: -65, expected: []byte{txscript.OP_DATA_1, 0xc1}},
		{name: "push -127", val: -127, expected: []byte{txscript.OP_DATA_1, 0xff}},
		{name: "push -128", val: -128, expected: []byte{txscript.OP_DATA_2, 0x80, 0x80}},
		{name: "push -255", val: -255, expected: []byte{txscript.OP_DATA_2, 0xff, 0x80}},
		{name: "push -256", val: -256, expected: []byte{txscript.OP_DATA_2, 0x00, 0x81}},
		{name: "push -32767", val: -32767, expected: []byte{txscript.OP_DATA_2, 0xff, 0xff}},
		{name: "push -32768", val: -32768, expected: []byte{txscript.OP_DATA_3, 0x00, 0x80, 0x80}},
	}

	builder := txscript.NewScriptBuilder()
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		builder.Reset().AddInt64(test.val)
		result, err := builder.Script()
		if err != nil {
			t.Errorf("ScriptBuilder.AddInt64 #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}
		if !bytes.Equal(result, test.expected) {
			t.Errorf("ScriptBuilder.AddInt64 #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, result,
				test.expected)
			continue
		}
	}
}

// TestScriptBuilderAddData tests that pushing data to a script via the
// ScriptBuilder API works as expected and conforms to BIP0062.
func TestScriptBuilderAddData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected []byte
		useFull  bool // use AddFullData instead of AddData.
	}{
		// BIP0062: Pushing an empty byte sequence must use OP_0.
		{name: "push empty byte sequence", data: nil, expected: []byte{txscript.OP_0}},
		{name: "push 1 byte 0x00", data: []byte{0x00}, expected: []byte{txscript.OP_0}},

		// BIP0062: Pushing a 1-byte sequence of byte 0x01 through 0x10 must use OP_n.
		{name: "push 1 byte 0x01", data: []byte{0x01}, expected: []byte{txscript.OP_1}},
		{name: "push 1 byte 0x02", data: []byte{0x02}, expected: []byte{txscript.OP_2}},
		{name: "push 1 byte 0x03", data: []byte{0x03}, expected: []byte{txscript.OP_3}},
		{name: "push 1 byte 0x04", data: []byte{0x04}, expected: []byte{txscript.OP_4}},
		{name: "push 1 byte 0x05", data: []byte{0x05}, expected: []byte{txscript.OP_5}},
		{name: "push 1 byte 0x06", data: []byte{0x06}, expected: []byte{txscript.OP_6}},
		{name: "push 1 byte 0x07", data: []byte{0x07}, expected: []byte{txscript.OP_7}},
		{name: "push 1 byte 0x08", data: []byte{0x08}, expected: []byte{txscript.OP_8}},
		{name: "push 1 byte 0x09", data: []byte{0x09}, expected: []byte{txscript.OP_9}},
		{name: "push 1 byte 0x0a", data: []byte{0x0a}, expected: []byte{txscript.OP_10}},
		{name: "push 1 byte 0x0b", data: []byte{0x0b}, expected: []byte{txscript.OP_11}},
		{name: "push 1 byte 0x0c", data: []byte{0x0c}, expected: []byte{txscript.OP_12}},
		{name: "push 1 byte 0x0d", data: []byte{0x0d}, expected: []byte{txscript.OP_13}},
		{name: "push 1 byte 0x0e", data: []byte{0x0e}, expected: []byte{txscript.OP_14}},
		{name: "push 1 byte 0x0f", data: []byte{0x0f}, expected: []byte{txscript.OP_15}},
		{name: "push 1 byte 0x10", data: []byte{0x10}, expected: []byte{txscript.OP_16}},

		// BIP0062: Pushing the byte 0x81 must use OP_1NEGATE.
		{name: "push 1 byte 0x81", data: []byte{0x81}, expected: []byte{txscript.OP_1NEGATE}},

		// BIP0062: Pushing any other byte sequence up to 75 bytes must
		// use the normal data push (opcode byte n, with n the number of
		// bytes, followed n bytes of data being pushed).
		{name: "push 1 byte 0x11", data: []byte{0x11}, expected: []byte{txscript.OP_DATA_1, 0x11}},
		{name: "push 1 byte 0x80", data: []byte{0x80}, expected: []byte{txscript.OP_DATA_1, 0x80}},
		{name: "push 1 byte 0x82", data: []byte{0x82}, expected: []byte{txscript.OP_DATA_1, 0x82}},
		{name: "push 1 byte 0xff", data: []byte{0xff}, expected: []byte{txscript.OP_DATA_1, 0xff}},
		{
			name:     "push data len 17",
			data:     bytes.Repeat([]byte{0x49}, 17),
			expected: append([]byte{txscript.OP_DATA_17}, bytes.Repeat([]byte{0x49}, 17)...),
		},
		{
			name:     "push data len 75",
			data:     bytes.Repeat([]byte{0x49}, 75),
			expected: append([]byte{txscript.OP_DATA_75}, bytes.Repeat([]byte{0x49}, 75)...),
		},

		// BIP0062: Pushing 76 to 255 bytes must use OP_PUSHDATA1.
		{
			name:     "push data len 76",
			data:     bytes.Repeat([]byte{0x49}, 76),
			expected: append([]byte{txscript.OP_PUSHDATA1, 76}, bytes.Repeat([]byte{0x49}, 76)...),
		},
		{
			name:     "push data len 255",
			data:     bytes.Repeat([]byte{0x49}, 255),
			expected: append([]byte{txscript.OP_PUSHDATA1, 255}, bytes.Repeat([]byte{0x49}, 255)...),
		},

		// BIP0062: Pushing 256 to 520 bytes must use OP_PUSHDATA2.
		{
			name:     "push data len 256",
			data:     bytes.Repeat([]byte{0x49}, 256),
			expected: append([]byte{txscript.OP_PUSHDATA2, 0, 1}, bytes.Repeat([]byte{0x49}, 256)...),
		},
		{
			name:     "push data len 520",
			data:     bytes.Repeat([]byte{0x49}, 520),
			expected: append([]byte{txscript.OP_PUSHDATA2, 0x08, 0x02}, bytes.Repeat([]byte{0x49}, 520)...),
		},

		// BIP0062: OP_PUSHDATA4 can never be used, as pushes over 520
		// bytes are not allowed, and those below can be done using
		// other operators.
		{
			name:     "push data len 521",
			data:     bytes.Repeat([]byte{0x49}, 4097),
			expected: nil,
		},
		{
			name:     "push data len 32767 (canonical)",
			data:     bytes.Repeat([]byte{0x49}, 32767),
			expected: nil,
		},
		{
			name:     "push data len 65536 (canonical)",
			data:     bytes.Repeat([]byte{0x49}, 65536),
			expected: nil,
		},

		// Additional tests for the PushFullData function that
		// intentionally allows data pushes to exceed the limit for
		// regression testing purposes.

		// 3-byte data push via OP_PUSHDATA_2.
		{
			name:     "push data len 32767 (non-canonical)",
			data:     bytes.Repeat([]byte{0x49}, 32767),
			expected: append([]byte{txscript.OP_PUSHDATA2, 255, 127}, bytes.Repeat([]byte{0x49}, 32767)...),
			useFull:  true,
		},

		// 5-byte data push via OP_PUSHDATA_4.
		{
			name:     "push data len 65536 (non-canonical)",
			data:     bytes.Repeat([]byte{0x49}, 65536),
			expected: append([]byte{txscript.OP_PUSHDATA4, 0, 0, 1, 0}, bytes.Repeat([]byte{0x49}, 65536)...),
			useFull:  true,
		},
	}

	builder := txscript.NewScriptBuilder()
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		if !test.useFull {
			builder.Reset().AddData(test.data)
		} else {
			builder.Reset().AddFullData(test.data)
		}
		result, _ := builder.Script()
		if !bytes.Equal(result, test.expected) {
			t.Errorf("ScriptBuilder.AddData #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, result,
				test.expected)
			continue
		}
	}
}

// TestExceedMaxScriptSize ensures that all of the functions that can be used
// to add data to a script don't allow the script to exceed the max allowed
// size.
func TestExceedMaxScriptSize(t *testing.T) {
	t.Parallel()

	// Start off by constructing a max size script.
	maxScriptSize := txscript.TstMaxScriptSize
	builder := txscript.NewScriptBuilder()
	builder.Reset().AddFullData(make([]byte, maxScriptSize-3))
	origScript, err := builder.Script()
	if err != nil {
		t.Fatalf("Unexpected error for max size script: %v", err)
	}

	// Ensure adding data that would exceed the maximum size of the script
	// does not add the data.
	script, err := builder.AddData([]byte{0x00}).Script()
	if _, ok := err.(txscript.ErrScriptNotCanonical); !ok || err == nil {
		t.Fatalf("ScriptBuilder.AddData allowed exceeding max script "+
			"size: %v", len(script))
	}
	if !bytes.Equal(script, origScript) {
		t.Fatalf("ScriptBuilder.AddData unexpected modified script - "+
			"got len %d, want len %d", len(script), len(origScript))
	}

	// Ensure adding an opcode that would exceed the maximum size of the
	// script does not add the data.
	builder.Reset().AddFullData(make([]byte, maxScriptSize-3))
	script, err = builder.AddOp(txscript.OP_0).Script()
	if _, ok := err.(txscript.ErrScriptNotCanonical); !ok || err == nil {
		t.Fatalf("ScriptBuilder.AddOp unexpected modified script - "+
			"got len %d, want len %d", len(script), len(origScript))
	}
	if !bytes.Equal(script, origScript) {
		t.Fatalf("ScriptBuilder.AddOp unexpected modified script - "+
			"got len %d, want len %d", len(script), len(origScript))
	}

	// Ensure adding an integer that would exceed the maximum size of the
	// script does not add the data.
	builder.Reset().AddFullData(make([]byte, maxScriptSize-3))
	script, err = builder.AddInt64(0).Script()
	if _, ok := err.(txscript.ErrScriptNotCanonical); !ok || err == nil {
		t.Fatalf("ScriptBuilder.AddInt64 unexpected modified script - "+
			"got len %d, want len %d", len(script), len(origScript))
	}
	if !bytes.Equal(script, origScript) {
		t.Fatalf("ScriptBuilder.AddInt64 unexpected modified script - "+
			"got len %d, want len %d", len(script), len(origScript))
	}
}

// TestErroredScript ensures that all of the functions that can be used to add
// data to a script don't modify the script once an error has happened.
func TestErroredScript(t *testing.T) {
	t.Parallel()

	// Start off by constructing a near max size script that has enough
	// space left to add each data type without an error and force an
	// initial error condition.
	maxScriptSize := txscript.TstMaxScriptSize
	builder := txscript.NewScriptBuilder()
	builder.Reset().AddFullData(make([]byte, maxScriptSize-8))
	origScript, err := builder.Script()
	if err != nil {
		t.Fatalf("ScriptBuilder.AddFullData unexpected error: %v", err)
	}
	script, err := builder.AddData([]byte{0x00, 0x00, 0x00, 0x00, 0x00}).Script()
	if _, ok := err.(txscript.ErrScriptNotCanonical); !ok || err == nil {
		t.Fatalf("ScriptBuilder.AddData allowed exceeding max script "+
			"size: %v", len(script))
	}
	if !bytes.Equal(script, origScript) {
		t.Fatalf("ScriptBuilder.AddData unexpected modified script - "+
			"got len %d, want len %d", len(script), len(origScript))
	}

	// Ensure adding data, even using the non-canonical path, to a script
	// that has errored doesn't succeed.
	script, err = builder.AddFullData([]byte{0x00}).Script()
	if _, ok := err.(txscript.ErrScriptNotCanonical); !ok || err == nil {
		t.Fatal("ScriptBuilder.AddFullData succeeded on errored script")
	}
	if !bytes.Equal(script, origScript) {
		t.Fatalf("ScriptBuilder.AddFullData unexpected modified "+
			"script - got len %d, want len %d", len(script),
			len(origScript))
	}

	// Ensure adding data to a script that has errored doesn't succeed.
	script, err = builder.AddData([]byte{0x00}).Script()
	if _, ok := err.(txscript.ErrScriptNotCanonical); !ok || err == nil {
		t.Fatal("ScriptBuilder.AddData succeeded on errored script")
	}
	if !bytes.Equal(script, origScript) {
		t.Fatalf("ScriptBuilder.AddData unexpected modified "+
			"script - got len %d, want len %d", len(script),
			len(origScript))
	}

	// Ensure adding an opcode to a script that has errored doesn't succeed.
	script, err = builder.AddOp(txscript.OP_0).Script()
	if _, ok := err.(txscript.ErrScriptNotCanonical); !ok || err == nil {
		t.Fatal("ScriptBuilder.AddOp succeeded on errored script")
	}
	if !bytes.Equal(script, origScript) {
		t.Fatalf("ScriptBuilder.AddOp unexpected modified script - "+
			"got len %d, want len %d", len(script), len(origScript))
	}

	// Ensure adding an integer to a script that has errored doesn't
	// succeed.
	script, err = builder.AddInt64(0).Script()
	if _, ok := err.(txscript.ErrScriptNotCanonical); !ok || err == nil {
		t.Fatal("ScriptBuilder.AddInt64 succeeded on errored script")
	}
	if !bytes.Equal(script, origScript) {
		t.Fatalf("ScriptBuilder.AddInt64 unexpected modified script - "+
			"got len %d, want len %d", len(script), len(origScript))
	}

	// Ensure the error has a message set.
	if err.Error() == "" {
		t.Fatal("ErrScriptNotCanonical.Error does not have any text")
	}
}
