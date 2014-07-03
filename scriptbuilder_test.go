// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript_test

import (
	"bytes"
	"testing"

	"github.com/conformal/btcscript"
)

// TestScriptBuilderAddOp tests that pushing opcodes to a script via the
// ScriptBuilder API works as expected.
func TestScriptBuilderAddOp(t *testing.T) {
	tests := []struct {
		name     string
		opcodes  []byte
		expected []byte
	}{
		{
			name:     "push OP_0",
			opcodes:  []byte{btcscript.OP_0},
			expected: []byte{btcscript.OP_0},
		},
		{
			name:     "push OP_1 OP_2",
			opcodes:  []byte{btcscript.OP_1, btcscript.OP_2},
			expected: []byte{btcscript.OP_1, btcscript.OP_2},
		},
		{
			name:     "push OP_HASH160 OP_EQUAL",
			opcodes:  []byte{btcscript.OP_HASH160, btcscript.OP_EQUAL},
			expected: []byte{btcscript.OP_HASH160, btcscript.OP_EQUAL},
		},
	}

	builder := btcscript.NewScriptBuilder()
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		builder.Reset()
		for _, opcode := range test.opcodes {
			builder.AddOp(opcode)
		}
		result := builder.Script()
		if !bytes.Equal(result, test.expected) {
			t.Errorf("ScriptBuilder.AddOp #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, result,
				test.expected)
			continue
		}
	}
}

// TestScriptBuilderAddInt64 tests that pushing signed integers to a script via
// the ScriptBuilder API works as expected.
func TestScriptBuilderAddInt64(t *testing.T) {
	tests := []struct {
		name     string
		val      int64
		expected []byte
	}{
		{name: "push -1", val: -1, expected: []byte{btcscript.OP_1NEGATE}},
		{name: "push small int 0", val: 0, expected: []byte{btcscript.OP_0}},
		{name: "push small int 1", val: 1, expected: []byte{btcscript.OP_1}},
		{name: "push small int 2", val: 2, expected: []byte{btcscript.OP_2}},
		{name: "push small int 3", val: 3, expected: []byte{btcscript.OP_3}},
		{name: "push small int 4", val: 4, expected: []byte{btcscript.OP_4}},
		{name: "push small int 5", val: 5, expected: []byte{btcscript.OP_5}},
		{name: "push small int 6", val: 6, expected: []byte{btcscript.OP_6}},
		{name: "push small int 7", val: 7, expected: []byte{btcscript.OP_7}},
		{name: "push small int 8", val: 8, expected: []byte{btcscript.OP_8}},
		{name: "push small int 9", val: 9, expected: []byte{btcscript.OP_9}},
		{name: "push small int 10", val: 10, expected: []byte{btcscript.OP_10}},
		{name: "push small int 11", val: 11, expected: []byte{btcscript.OP_11}},
		{name: "push small int 12", val: 12, expected: []byte{btcscript.OP_12}},
		{name: "push small int 13", val: 13, expected: []byte{btcscript.OP_13}},
		{name: "push small int 14", val: 14, expected: []byte{btcscript.OP_14}},
		{name: "push small int 15", val: 15, expected: []byte{btcscript.OP_15}},
		{name: "push small int 16", val: 16, expected: []byte{btcscript.OP_16}},
		{name: "push 17", val: 17, expected: []byte{btcscript.OP_DATA_1, 0x11}},
		{name: "push 65", val: 65, expected: []byte{btcscript.OP_DATA_1, 0x41}},
		{name: "push 127", val: 127, expected: []byte{btcscript.OP_DATA_1, 0x7f}},
		{name: "push 128", val: 128, expected: []byte{btcscript.OP_DATA_2, 0x80, 0}},
		{name: "push 255", val: 255, expected: []byte{btcscript.OP_DATA_2, 0xff, 0}},
		{name: "push 256", val: 256, expected: []byte{btcscript.OP_DATA_2, 0, 0x01}},
		{name: "push 32767", val: 32767, expected: []byte{btcscript.OP_DATA_2, 0xff, 0x7f}},
		{name: "push 32768", val: 32768, expected: []byte{btcscript.OP_DATA_3, 0, 0x80, 0}},
		{name: "push -2", val: -2, expected: []byte{btcscript.OP_DATA_1, 0x82}},
		{name: "push -3", val: -3, expected: []byte{btcscript.OP_DATA_1, 0x83}},
		{name: "push -4", val: -4, expected: []byte{btcscript.OP_DATA_1, 0x84}},
		{name: "push -5", val: -5, expected: []byte{btcscript.OP_DATA_1, 0x85}},
		{name: "push -17", val: -17, expected: []byte{btcscript.OP_DATA_1, 0x91}},
		{name: "push -65", val: -65, expected: []byte{btcscript.OP_DATA_1, 0xc1}},
		{name: "push -127", val: -127, expected: []byte{btcscript.OP_DATA_1, 0xff}},
		{name: "push -128", val: -128, expected: []byte{btcscript.OP_DATA_2, 0x80, 0x80}},
		{name: "push -255", val: -255, expected: []byte{btcscript.OP_DATA_2, 0xff, 0x80}},
		{name: "push -256", val: -256, expected: []byte{btcscript.OP_DATA_2, 0x00, 0x81}},
		{name: "push -32767", val: -32767, expected: []byte{btcscript.OP_DATA_2, 0xff, 0xff}},
		{name: "push -32768", val: -32768, expected: []byte{btcscript.OP_DATA_3, 0x00, 0x80, 0x80}},
	}

	builder := btcscript.NewScriptBuilder()
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		builder.Reset().AddInt64(test.val)
		result := builder.Script()
		if !bytes.Equal(result, test.expected) {
			t.Errorf("ScriptBuilder.AddInt64 #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, result,
				test.expected)
			continue
		}
	}
}

// TestScriptBuilderAddUint64 tests that pushing unsigned integers to a script
// via the ScriptBuilder API works as expected.
func TestScriptBuilderAddUint64(t *testing.T) {
	tests := []struct {
		name     string
		val      uint64
		expected []byte
	}{
		{name: "push small int 0", val: 0, expected: []byte{btcscript.OP_0}},
		{name: "push small int 1", val: 1, expected: []byte{btcscript.OP_1}},
		{name: "push small int 2", val: 2, expected: []byte{btcscript.OP_2}},
		{name: "push small int 3", val: 3, expected: []byte{btcscript.OP_3}},
		{name: "push small int 4", val: 4, expected: []byte{btcscript.OP_4}},
		{name: "push small int 5", val: 5, expected: []byte{btcscript.OP_5}},
		{name: "push small int 6", val: 6, expected: []byte{btcscript.OP_6}},
		{name: "push small int 7", val: 7, expected: []byte{btcscript.OP_7}},
		{name: "push small int 8", val: 8, expected: []byte{btcscript.OP_8}},
		{name: "push small int 9", val: 9, expected: []byte{btcscript.OP_9}},
		{name: "push small int 10", val: 10, expected: []byte{btcscript.OP_10}},
		{name: "push small int 11", val: 11, expected: []byte{btcscript.OP_11}},
		{name: "push small int 12", val: 12, expected: []byte{btcscript.OP_12}},
		{name: "push small int 13", val: 13, expected: []byte{btcscript.OP_13}},
		{name: "push small int 14", val: 14, expected: []byte{btcscript.OP_14}},
		{name: "push small int 15", val: 15, expected: []byte{btcscript.OP_15}},
		{name: "push small int 16", val: 16, expected: []byte{btcscript.OP_16}},
		{name: "push 17", val: 17, expected: []byte{btcscript.OP_DATA_1, 0x11}},
		{name: "push 65", val: 65, expected: []byte{btcscript.OP_DATA_1, 0x41}},
		{name: "push 127", val: 127, expected: []byte{btcscript.OP_DATA_1, 0x7f}},
		{name: "push 128", val: 128, expected: []byte{btcscript.OP_DATA_2, 0x80, 0}},
		{name: "push 255", val: 255, expected: []byte{btcscript.OP_DATA_2, 0xff, 0}},
		{name: "push 256", val: 256, expected: []byte{btcscript.OP_DATA_2, 0, 0x01}},
		{name: "push 32767", val: 32767, expected: []byte{btcscript.OP_DATA_2, 0xff, 0x7f}},
		{name: "push 32768", val: 32768, expected: []byte{btcscript.OP_DATA_3, 0, 0x80, 0}},
	}

	builder := btcscript.NewScriptBuilder()
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		builder.Reset().AddUint64(test.val)
		result := builder.Script()
		if !bytes.Equal(result, test.expected) {
			t.Errorf("ScriptBuilder.AddUint64 #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, result,
				test.expected)
			continue
		}
	}
}

// TestScriptBuilderAddData tests that pushing data to a script via the
// ScriptBuilder API works as expected.
func TestScriptBuilderAddData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []byte
	}{
		// Start off with the small ints to ensure canonical encoding.
		{name: "push small int 0", data: []byte{0}, expected: []byte{btcscript.OP_0}},
		{name: "push small int 1", data: []byte{1}, expected: []byte{btcscript.OP_1}},
		{name: "push small int 2", data: []byte{2}, expected: []byte{btcscript.OP_2}},
		{name: "push small int 3", data: []byte{3}, expected: []byte{btcscript.OP_3}},
		{name: "push small int 4", data: []byte{4}, expected: []byte{btcscript.OP_4}},
		{name: "push small int 5", data: []byte{5}, expected: []byte{btcscript.OP_5}},
		{name: "push small int 6", data: []byte{6}, expected: []byte{btcscript.OP_6}},
		{name: "push small int 7", data: []byte{7}, expected: []byte{btcscript.OP_7}},
		{name: "push small int 8", data: []byte{8}, expected: []byte{btcscript.OP_8}},
		{name: "push small int 9", data: []byte{9}, expected: []byte{btcscript.OP_9}},
		{name: "push small int 10", data: []byte{10}, expected: []byte{btcscript.OP_10}},
		{name: "push small int 11", data: []byte{11}, expected: []byte{btcscript.OP_11}},
		{name: "push small int 12", data: []byte{12}, expected: []byte{btcscript.OP_12}},
		{name: "push small int 13", data: []byte{13}, expected: []byte{btcscript.OP_13}},
		{name: "push small int 14", data: []byte{14}, expected: []byte{btcscript.OP_14}},
		{name: "push small int 15", data: []byte{15}, expected: []byte{btcscript.OP_15}},
		{name: "push small int 16", data: []byte{16}, expected: []byte{btcscript.OP_16}},

		// 1-byte data push opcodes.
		{
			name:     "push data len 17",
			data:     bytes.Repeat([]byte{0x49}, 17),
			expected: append([]byte{btcscript.OP_DATA_17}, bytes.Repeat([]byte{0x49}, 17)...),
		},
		{
			name:     "push data len 75",
			data:     bytes.Repeat([]byte{0x49}, 75),
			expected: append([]byte{btcscript.OP_DATA_75}, bytes.Repeat([]byte{0x49}, 75)...),
		},

		// 2-byte data push via OP_PUSHDATA_1.
		{
			name:     "push data len 76",
			data:     bytes.Repeat([]byte{0x49}, 76),
			expected: append([]byte{btcscript.OP_PUSHDATA1, 76}, bytes.Repeat([]byte{0x49}, 76)...),
		},
		{
			name:     "push data len 255",
			data:     bytes.Repeat([]byte{0x49}, 255),
			expected: append([]byte{btcscript.OP_PUSHDATA1, 255}, bytes.Repeat([]byte{0x49}, 255)...),
		},

		// 3-byte data push via OP_PUSHDATA_2.
		{
			name:     "push data len 256",
			data:     bytes.Repeat([]byte{0x49}, 256),
			expected: append([]byte{btcscript.OP_PUSHDATA2, 0, 1}, bytes.Repeat([]byte{0x49}, 256)...),
		},
		{
			name:     "push data len 32767",
			data:     bytes.Repeat([]byte{0x49}, 32767),
			expected: append([]byte{btcscript.OP_PUSHDATA2, 255, 127}, bytes.Repeat([]byte{0x49}, 32767)...),
		},

		// 5-byte data push via OP_PUSHDATA_4.
		{
			name:     "push data len 65536",
			data:     bytes.Repeat([]byte{0x49}, 65536),
			expected: append([]byte{btcscript.OP_PUSHDATA4, 0, 0, 1, 0}, bytes.Repeat([]byte{0x49}, 65536)...),
		},
	}

	builder := btcscript.NewScriptBuilder()
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		builder.Reset().AddData(test.data)
		result := builder.Script()
		if !bytes.Equal(result, test.expected) {
			t.Errorf("ScriptBuilder.AddData #%d (%s) wrong result\n"+
				"got: %x\nwant: %x", i, test.name, result,
				test.expected)
			continue
		}
	}
}
