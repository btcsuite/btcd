// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"fmt"
	"testing"
)

// TestScriptTokenizer ensures a wide variety of behavior provided by the script
// tokenizer performs as expected.
func TestScriptTokenizer(t *testing.T) {
	type expectedResult struct {
		op    byte   // expected parsed opcode
		data  []byte // expected parsed data
		index int32  // expected index into raw script after parsing token
	}

	type tokenizerTest struct {
		name     string           // test description
		script   []byte           // the script to tokenize
		expected []expectedResult // the expected info after parsing each token
		finalIdx int32            // the expected final byte index
		err      error            // expected error
	}

	// Add both positive and negative tests for OP_DATA_1 through OP_DATA_75.
	const numTestsHint = 100 // Make prealloc linter happy.
	tests := make([]tokenizerTest, 0, numTestsHint)
	for op := byte(OP_DATA_1); op < OP_DATA_75; op++ {
		data := bytes.Repeat([]byte{0x01}, int(op))
		tests = append(tests, tokenizerTest{
			name:     fmt.Sprintf("OP_DATA_%d", op),
			script:   append([]byte{op}, data...),
			expected: []expectedResult{{op, data, 1 + int32(op)}},
			finalIdx: 1 + int32(op),
			err:      nil,
		})

		// Create test that provides one less byte than the data push requires.
		tests = append(tests, tokenizerTest{
			name:     fmt.Sprintf("short OP_DATA_%d", op),
			script:   append([]byte{op}, data[1:]...),
			expected: nil,
			finalIdx: 0,
			err:      scriptError(ErrMalformedPush, ""),
		})
	}

	// Add both positive and negative tests for OP_PUSHDATA{1,2,4}.
	data := mustParseShortForm("0x01{76}")
	tests = append(tests, []tokenizerTest{{
		name:     "OP_PUSHDATA1",
		script:   mustParseShortForm("OP_PUSHDATA1 0x4c 0x01{76}"),
		expected: []expectedResult{{OP_PUSHDATA1, data, 2 + int32(len(data))}},
		finalIdx: 2 + int32(len(data)),
		err:      nil,
	}, {
		name:     "OP_PUSHDATA1 no data length",
		script:   mustParseShortForm("OP_PUSHDATA1"),
		expected: nil,
		finalIdx: 0,
		err:      scriptError(ErrMalformedPush, ""),
	}, {
		name:     "OP_PUSHDATA1 short data by 1 byte",
		script:   mustParseShortForm("OP_PUSHDATA1 0x4c 0x01{75}"),
		expected: nil,
		finalIdx: 0,
		err:      scriptError(ErrMalformedPush, ""),
	}, {
		name:     "OP_PUSHDATA2",
		script:   mustParseShortForm("OP_PUSHDATA2 0x4c00 0x01{76}"),
		expected: []expectedResult{{OP_PUSHDATA2, data, 3 + int32(len(data))}},
		finalIdx: 3 + int32(len(data)),
		err:      nil,
	}, {
		name:     "OP_PUSHDATA2 no data length",
		script:   mustParseShortForm("OP_PUSHDATA2"),
		expected: nil,
		finalIdx: 0,
		err:      scriptError(ErrMalformedPush, ""),
	}, {
		name:     "OP_PUSHDATA2 short data by 1 byte",
		script:   mustParseShortForm("OP_PUSHDATA2 0x4c00 0x01{75}"),
		expected: nil,
		finalIdx: 0,
		err:      scriptError(ErrMalformedPush, ""),
	}, {
		name:     "OP_PUSHDATA4",
		script:   mustParseShortForm("OP_PUSHDATA4 0x4c000000 0x01{76}"),
		expected: []expectedResult{{OP_PUSHDATA4, data, 5 + int32(len(data))}},
		finalIdx: 5 + int32(len(data)),
		err:      nil,
	}, {
		name:     "OP_PUSHDATA4 no data length",
		script:   mustParseShortForm("OP_PUSHDATA4"),
		expected: nil,
		finalIdx: 0,
		err:      scriptError(ErrMalformedPush, ""),
	}, {
		name:     "OP_PUSHDATA4 short data by 1 byte",
		script:   mustParseShortForm("OP_PUSHDATA4 0x4c000000 0x01{75}"),
		expected: nil,
		finalIdx: 0,
		err:      scriptError(ErrMalformedPush, ""),
	}}...)

	// Add tests for OP_0, and OP_1 through OP_16 (small integers/true/false).
	opcodes := []byte{OP_0}
	for op := byte(OP_1); op < OP_16; op++ {
		opcodes = append(opcodes, op)
	}
	for _, op := range opcodes {
		tests = append(tests, tokenizerTest{
			name:     fmt.Sprintf("OP_%d", op),
			script:   []byte{op},
			expected: []expectedResult{{op, nil, 1}},
			finalIdx: 1,
			err:      nil,
		})
	}

	// Add various positive and negative tests for multi-opcode scripts.
	tests = append(tests, []tokenizerTest{{
		name:   "pay-to-pubkey-hash",
		script: mustParseShortForm("DUP HASH160 DATA_20 0x01{20} EQUAL CHECKSIG"),
		expected: []expectedResult{
			{OP_DUP, nil, 1}, {OP_HASH160, nil, 2},
			{OP_DATA_20, mustParseShortForm("0x01{20}"), 23},
			{OP_EQUAL, nil, 24}, {OP_CHECKSIG, nil, 25},
		},
		finalIdx: 25,
		err:      nil,
	}, {
		name:   "almost pay-to-pubkey-hash (short data)",
		script: mustParseShortForm("DUP HASH160 DATA_20 0x01{17} EQUAL CHECKSIG"),
		expected: []expectedResult{
			{OP_DUP, nil, 1}, {OP_HASH160, nil, 2},
		},
		finalIdx: 2,
		err:      scriptError(ErrMalformedPush, ""),
	}, {
		name:   "almost pay-to-pubkey-hash (overlapped data)",
		script: mustParseShortForm("DUP HASH160 DATA_20 0x01{19} EQUAL CHECKSIG"),
		expected: []expectedResult{
			{OP_DUP, nil, 1}, {OP_HASH160, nil, 2},
			{OP_DATA_20, mustParseShortForm("0x01{19} EQUAL"), 23},
			{OP_CHECKSIG, nil, 24},
		},
		finalIdx: 24,
		err:      nil,
	}, {
		name:   "pay-to-script-hash",
		script: mustParseShortForm("HASH160 DATA_20 0x01{20} EQUAL"),
		expected: []expectedResult{
			{OP_HASH160, nil, 1},
			{OP_DATA_20, mustParseShortForm("0x01{20}"), 22},
			{OP_EQUAL, nil, 23},
		},
		finalIdx: 23,
		err:      nil,
	}, {
		name:   "almost pay-to-script-hash (short data)",
		script: mustParseShortForm("HASH160 DATA_20 0x01{18} EQUAL"),
		expected: []expectedResult{
			{OP_HASH160, nil, 1},
		},
		finalIdx: 1,
		err:      scriptError(ErrMalformedPush, ""),
	}, {
		name:   "almost pay-to-script-hash (overlapped data)",
		script: mustParseShortForm("HASH160 DATA_20 0x01{19} EQUAL"),
		expected: []expectedResult{
			{OP_HASH160, nil, 1},
			{OP_DATA_20, mustParseShortForm("0x01{19} EQUAL"), 22},
		},
		finalIdx: 22,
		err:      nil,
	}}...)

	const scriptVersion = 0
	for _, test := range tests {
		tokenizer := MakeScriptTokenizer(scriptVersion, test.script)
		var opcodeNum int
		for tokenizer.Next() {
			// Ensure Next never returns true when there is an error set.
			if err := tokenizer.Err(); err != nil {
				t.Fatalf("%q: Next returned true when tokenizer has err: %v",
					test.name, err)
			}

			// Ensure the test data expects a token to be parsed.
			op := tokenizer.Opcode()
			data := tokenizer.Data()
			if opcodeNum >= len(test.expected) {
				t.Fatalf("%q: unexpected token '%d' (data: '%x')", test.name,
					op, data)
			}
			expected := &test.expected[opcodeNum]

			// Ensure the opcode and data are the expected values.
			if op != expected.op {
				t.Fatalf("%q: unexpected opcode -- got %v, want %v", test.name,
					op, expected.op)
			}
			if !bytes.Equal(data, expected.data) {
				t.Fatalf("%q: unexpected data -- got %x, want %x", test.name,
					data, expected.data)
			}

			tokenizerIdx := tokenizer.ByteIndex()
			if tokenizerIdx != expected.index {
				t.Fatalf("%q: unexpected byte index -- got %d, want %d",
					test.name, tokenizerIdx, expected.index)
			}

			opcodeNum++
		}

		// Ensure the tokenizer claims it is done.  This should be the case
		// regardless of whether or not there was a parse error.
		if !tokenizer.Done() {
			t.Fatalf("%q: tokenizer claims it is not done", test.name)
		}

		// Ensure the error is as expected.
		if test.err == nil && tokenizer.Err() != nil {
			t.Fatalf("%q: unexpected tokenizer err -- got %v, want nil",
				test.name, tokenizer.Err())
		} else if test.err != nil {
			if !IsErrorCode(tokenizer.Err(), test.err.(Error).ErrorCode) {
				t.Fatalf("%q: unexpected tokenizer err -- got %v, want %v",
					test.name, tokenizer.Err(), test.err.(Error).ErrorCode)
			}
		}

		// Ensure the final index is the expected value.
		tokenizerIdx := tokenizer.ByteIndex()
		if tokenizerIdx != test.finalIdx {
			t.Fatalf("%q: unexpected final byte index -- got %d, want %d",
				test.name, tokenizerIdx, test.finalIdx)
		}
	}
}

// TestScriptTokenizerUnsupportedVersion ensures the tokenizer fails immediately
// with an unsupported script version.
func TestScriptTokenizerUnsupportedVersion(t *testing.T) {
	const scriptVersion = 65535
	tokenizer := MakeScriptTokenizer(scriptVersion, nil)
	if !IsErrorCode(tokenizer.Err(), ErrUnsupportedScriptVersion) {
		t.Fatalf("script tokenizer did not error with unsupported version")
	}
}
