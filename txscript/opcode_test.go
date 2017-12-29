// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/decred/dcrd/wire"
)

// testScriptFlags are the script flags which are used in the tests when
// executing transaction scripts to enforce additional checks.  Note these flags
// are different than what is required for the consensus rules in that they are
// more strict.
const testScriptFlags = ScriptBip16 |
	ScriptVerifyDERSignatures |
	ScriptVerifyStrictEncoding |
	ScriptVerifyMinimalData |
	ScriptDiscourageUpgradableNops |
	ScriptVerifyCleanStack |
	ScriptVerifyCheckLockTimeVerify |
	ScriptVerifyCheckSequenceVerify |
	ScriptVerifyLowS |
	ScriptVerifySHA256

// TestOpcodeDisabled tests the opcodeDisabled function manually because all
// disabled opcodes result in a script execution failure when executed normally,
// so the function is not called under normal circumstances.
func TestOpcodeDisabled(t *testing.T) {
	t.Parallel()
	tests := []byte{OP_CAT, OP_SUBSTR, OP_LEFT, OP_RIGHT, OP_INVERT,
		OP_AND, OP_OR, OP_2MUL, OP_2DIV, OP_MUL, OP_DIV, OP_MOD,
		OP_LSHIFT, OP_RSHIFT,
	}
	for _, opcodeVal := range tests {
		pop := parsedOpcode{opcode: &opcodeArray[opcodeVal], data: nil}
		if err := opcodeDisabled(&pop, nil); err != ErrStackOpDisabled {
			t.Errorf("opcodeDisabled: unexpected error - got %v, "+
				"want %v", err, ErrStackOpDisabled)
			return
		}
	}
}

// TestOpcodeDisasm tests the print function for all opcodes in both the oneline
// and full modes to ensure it provides the expected disassembly.
func TestOpcodeDisasm(t *testing.T) {
	t.Parallel()

	// First, test the oneline disassembly.

	// The expected strings for the data push opcodes are replaced in the
	// test loops below since they involve repeating bytes.  Also, the
	// OP_NOP# and OP_UNKNOWN# are replaced below too, since it's easier
	// than manually listing them here.
	oneBytes := []byte{0x01}
	oneStr := "01"
	expectedStrings := [256]string{0x00: "0", 0x4f: "-1",
		0x50: "OP_RESERVED", 0x61: "OP_NOP", 0x62: "OP_VER",
		0x63: "OP_IF", 0x64: "OP_NOTIF", 0x65: "OP_VERIF",
		0x66: "OP_VERNOTIF", 0x67: "OP_ELSE", 0x68: "OP_ENDIF",
		0x69: "OP_VERIFY", 0x6a: "OP_RETURN", 0x6b: "OP_TOALTSTACK",
		0x6c: "OP_FROMALTSTACK", 0x6d: "OP_2DROP", 0x6e: "OP_2DUP",
		0x6f: "OP_3DUP", 0x70: "OP_2OVER", 0x71: "OP_2ROT",
		0x72: "OP_2SWAP", 0x73: "OP_IFDUP", 0x74: "OP_DEPTH",
		0x75: "OP_DROP", 0x76: "OP_DUP", 0x77: "OP_NIP",
		0x78: "OP_OVER", 0x79: "OP_PICK", 0x7a: "OP_ROLL",
		0x7b: "OP_ROT", 0x7c: "OP_SWAP", 0x7d: "OP_TUCK",
		0x7e: "OP_CAT", 0x7f: "OP_SUBSTR", 0x80: "OP_LEFT",
		0x81: "OP_RIGHT", 0x82: "OP_SIZE", 0x83: "OP_INVERT",
		0x84: "OP_AND", 0x85: "OP_OR", 0x86: "OP_XOR",
		0x87: "OP_EQUAL", 0x88: "OP_EQUALVERIFY", 0x89: "OP_ROTR",
		0x8a: "OP_ROTL", 0x8b: "OP_1ADD", 0x8c: "OP_1SUB",
		0x8d: "OP_2MUL", 0x8e: "OP_2DIV", 0x8f: "OP_NEGATE",
		0x90: "OP_ABS", 0x91: "OP_NOT", 0x92: "OP_0NOTEQUAL",
		0x93: "OP_ADD", 0x94: "OP_SUB", 0x95: "OP_MUL", 0x96: "OP_DIV",
		0x97: "OP_MOD", 0x98: "OP_LSHIFT", 0x99: "OP_RSHIFT",
		0x9a: "OP_BOOLAND", 0x9b: "OP_BOOLOR", 0x9c: "OP_NUMEQUAL",
		0x9d: "OP_NUMEQUALVERIFY", 0x9e: "OP_NUMNOTEQUAL",
		0x9f: "OP_LESSTHAN", 0xa0: "OP_GREATERTHAN",
		0xa1: "OP_LESSTHANOREQUAL", 0xa2: "OP_GREATERTHANOREQUAL",
		0xa3: "OP_MIN", 0xa4: "OP_MAX", 0xa5: "OP_WITHIN",
		0xa6: "OP_RIPEMD160", 0xa7: "OP_SHA1", 0xa8: "OP_BLAKE256",
		0xa9: "OP_HASH160", 0xaa: "OP_HASH256", 0xab: "OP_CODESEPARATOR",
		0xac: "OP_CHECKSIG", 0xad: "OP_CHECKSIGVERIFY",
		0xae: "OP_CHECKMULTISIG", 0xaf: "OP_CHECKMULTISIGVERIFY",
		0xf9: "OP_INVALID249", 0xfa: "OP_SMALLINTEGER",
		0xfb: "OP_PUBKEYS", 0xfd: "OP_PUBKEYHASH", 0xfe: "OP_PUBKEY",
		0xff: "OP_INVALIDOPCODE", 0xba: "OP_SSTX", 0xbb: "OP_SSGEN",
		0xbc: "OP_SSRTX", 0xbd: "OP_SSTXCHANGE", 0xbe: "OP_CHECKSIGALT",
		0xbf: "OP_CHECKSIGALTVERIFY", 0xc0: "OP_SHA256",
	}
	for opcodeVal, expectedStr := range expectedStrings {
		var data []byte
		switch {
		// OP_DATA_1 through OP_DATA_65 display the pushed data.
		case opcodeVal >= 0x01 && opcodeVal < 0x4c:
			data = bytes.Repeat(oneBytes, opcodeVal)
			expectedStr = strings.Repeat(oneStr, opcodeVal)

		// OP_PUSHDATA1.
		case opcodeVal == 0x4c:
			data = bytes.Repeat(oneBytes, 1)
			expectedStr = strings.Repeat(oneStr, 1)

		// OP_PUSHDATA2.
		case opcodeVal == 0x4d:
			data = bytes.Repeat(oneBytes, 2)
			expectedStr = strings.Repeat(oneStr, 2)

		// OP_PUSHDATA4.
		case opcodeVal == 0x4e:
			data = bytes.Repeat(oneBytes, 3)
			expectedStr = strings.Repeat(oneStr, 3)

		// OP_1 through OP_16 display the numbers themselves.
		case opcodeVal >= 0x51 && opcodeVal <= 0x60:
			val := byte(opcodeVal - (0x51 - 1))
			data = []byte{val}
			expectedStr = strconv.Itoa(int(val))

		// OP_NOP1 through OP_NOP10.
		case opcodeVal >= 0xb0 && opcodeVal <= 0xb9:
			switch opcodeVal {
			case 0xb1:
				// OP_NOP2 is an alias of OP_CHECKLOCKTIMEVERIFY
				expectedStr = "OP_CHECKLOCKTIMEVERIFY"
			case 0xb2:
				// OP_NOP3 is an alias of OP_CHECKSEQUENCEVERIFY
				expectedStr = "OP_CHECKSEQUENCEVERIFY"
			default:
				val := byte(opcodeVal - (0xb0 - 1))
				expectedStr = "OP_NOP" + strconv.Itoa(int(val))
			}

		// OP_UNKNOWN#.
		case opcodeVal >= 0xc1 && opcodeVal <= 0xf8 || opcodeVal == 0xfc:
			expectedStr = "OP_UNKNOWN" + strconv.Itoa(int(opcodeVal))
		}

		pop := parsedOpcode{opcode: &opcodeArray[opcodeVal], data: data}
		gotStr := pop.print(true)
		if gotStr != expectedStr {
			t.Errorf("pop.print (opcode %x): Unexpected disasm "+
				"string - got %v, want %v", opcodeVal, gotStr,
				expectedStr)
			continue
		}
	}

	// Now, replace the relevant fields and test the full disassembly.
	expectedStrings[0x00] = "OP_0"
	expectedStrings[0x4f] = "OP_1NEGATE"
	for opcodeVal, expectedStr := range expectedStrings {
		var data []byte
		switch {
		// OP_DATA_1 through OP_DATA_65 display the opcode followed by
		// the pushed data.
		case opcodeVal >= 0x01 && opcodeVal < 0x4c:
			data = bytes.Repeat(oneBytes, opcodeVal)
			expectedStr = fmt.Sprintf("OP_DATA_%d 0x%s", opcodeVal,
				strings.Repeat(oneStr, opcodeVal))

		// OP_PUSHDATA1.
		case opcodeVal == 0x4c:
			data = bytes.Repeat(oneBytes, 1)
			expectedStr = fmt.Sprintf("OP_PUSHDATA1 0x%02x 0x%s",
				len(data), strings.Repeat(oneStr, 1))

		// OP_PUSHDATA2.
		case opcodeVal == 0x4d:
			data = bytes.Repeat(oneBytes, 2)
			expectedStr = fmt.Sprintf("OP_PUSHDATA2 0x%04x 0x%s",
				len(data), strings.Repeat(oneStr, 2))

		// OP_PUSHDATA4.
		case opcodeVal == 0x4e:
			data = bytes.Repeat(oneBytes, 3)
			expectedStr = fmt.Sprintf("OP_PUSHDATA4 0x%08x 0x%s",
				len(data), strings.Repeat(oneStr, 3))

		// OP_1 through OP_16.
		case opcodeVal >= 0x51 && opcodeVal <= 0x60:
			val := byte(opcodeVal - (0x51 - 1))
			data = []byte{val}
			expectedStr = "OP_" + strconv.Itoa(int(val))

		// OP_NOP1 through OP_NOP10.
		case opcodeVal >= 0xb0 && opcodeVal <= 0xb9:
			switch opcodeVal {
			case 0xb1:
				// OP_NOP2 is an alias of OP_CHECKLOCKTIMEVERIFY
				expectedStr = "OP_CHECKLOCKTIMEVERIFY"
			case 0xb2:
				// OP_NOP3 is an alias of OP_CHECKSEQUENCEVERIFY
				expectedStr = "OP_CHECKSEQUENCEVERIFY"
			default:
				val := byte(opcodeVal - (0xb0 - 1))
				expectedStr = "OP_NOP" + strconv.Itoa(int(val))
			}

		// OP_UNKNOWN#.
		case opcodeVal >= 0xc1 && opcodeVal <= 0xf8 || opcodeVal == 0xfc:
			expectedStr = "OP_UNKNOWN" + strconv.Itoa(int(opcodeVal))
		}

		pop := parsedOpcode{opcode: &opcodeArray[opcodeVal], data: data}
		gotStr := pop.print(false)
		if gotStr != expectedStr {
			t.Errorf("pop.print (opcode %x): Unexpected disasm "+
				"string - got %v, want %v", opcodeVal, gotStr,
				expectedStr)
			continue
		}
	}
}

func TestNewlyEnabledOpCodes(t *testing.T) {
	sigScriptMath := []byte{
		0x04,
		0xff, 0xff, 0xff, 0x7f,
		0x04,
		0xee, 0xee, 0xee, 0x6e,
	}
	sigScriptShift := []byte{
		0x04,
		0xff, 0xff, 0xff, 0x7f,
		0x53,
	}
	sigScriptRot := []byte{
		0x04,
		0x21, 0x12, 0x34, 0x56,
		0x53,
	}
	sigScriptInv := []byte{
		0x04,
		0xff, 0x00, 0xf0, 0x0f,
	}
	sigScriptLogic := []byte{
		0x04,
		0x21, 0x12, 0x34, 0x56,
		0x04,
		0x0f, 0xf0, 0x00, 0xff,
	}
	sigScriptCat := []byte{
		0x06,
		0x21, 0x12, 0x34, 0x56, 0x44, 0x55,
		0x06,
		0x0f, 0xf0, 0x00, 0xff, 0x88, 0x99,
	}
	lotsOf01s := bytes.Repeat([]byte{0x01}, 2050)
	builder := NewScriptBuilder()
	builder.AddData(lotsOf01s).AddData(lotsOf01s)
	sigScriptCatOverflow, _ := builder.Script()
	sigScriptSubstr := []byte{
		0x08,
		0x21, 0x12, 0x34, 0x56, 0x59, 0x32, 0x40, 0x21,
		0x56,
		0x52,
	}
	sigScriptLR := []byte{
		0x08,
		0x21, 0x12, 0x34, 0x56, 0x59, 0x32, 0x40, 0x21,
		0x54,
	}

	tests := []struct {
		name      string
		pkScript  []byte
		sigScript []byte
		expected  bool
	}{
		{
			name: "add",
			pkScript: []byte{
				0x93, // OP_ADD
				0x05, // Expected result push
				0xed, 0xee, 0xee, 0xee, 0x00,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptMath,
			expected:  true,
		},
		{
			name: "sub",
			pkScript: []byte{
				0x94, // OP_SUB
				0x04, // Expected result push
				0x11, 0x11, 0x11, 0x11,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptMath,
			expected:  true,
		},
		{
			name: "mul",
			pkScript: []byte{
				0x95, // OP_MUL
				0x04, // Expected result push
				0xee, 0xee, 0xee, 0xee,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptMath,
			expected:  true,
		},
		{
			name: "div",
			pkScript: []byte{
				0x96, // OP_DIV
				0x51, // Expected result push
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptMath,
			expected:  true,
		},
		{
			name: "mod",
			pkScript: []byte{
				0x97, // OP_MOD
				0x04, // Expected result push
				0x11, 0x11, 0x11, 0x11,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptMath,
			expected:  true,
		},
		{
			name: "lshift",
			pkScript: []byte{
				0x98, // OP_LSHIFT
				0x01, // Expected result push
				0x88,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptShift,
			expected:  true,
		},
		{
			name: "rshift",
			pkScript: []byte{
				0x99, // OP_RSHIFT
				0x04, // Expected result push
				0xff, 0xff, 0xff, 0x0f,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptShift,
			expected:  true,
		},
		{
			name: "rotr",
			pkScript: []byte{
				0x89, // OP_ROTR
				0x04, // Expected result push
				0x44, 0x82, 0xc6, 0x2a,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptRot,
			expected:  true,
		},
		{
			name: "rotl",
			pkScript: []byte{
				0x8a, // OP_ROTL
				0x04, // Expected result push
				0xf6, 0x6e, 0x5f, 0xce,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptRot,
			expected:  true,
		},
		{
			name: "inv",
			pkScript: []byte{
				0x83, // OP_INV
				0x04, // Expected result push
				0x00, 0x01, 0xf0, 0x8f,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptInv,
			expected:  true,
		},
		{
			name: "and",
			pkScript: []byte{
				0x84, // OP_AND
				0x03, // Expected result push
				0x21, 0x02, 0x34,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptLogic,
			expected:  true,
		},
		{
			name: "or",
			pkScript: []byte{
				0x85, // OP_OR
				0x04, // Expected result push
				0x0f, 0xe0, 0x00, 0xa9,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptLogic,
			expected:  true,
		},
		{
			name: "xor",
			pkScript: []byte{
				0x86, // OP_XOR
				0x04, // Expected result push
				0x30, 0xe2, 0x34, 0xa9,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptLogic,
			expected:  true,
		},
		{
			name: "cat",
			pkScript: []byte{
				0x7e, // OP_CAT
				0x0c, // Expected result push
				0x21, 0x12, 0x34, 0x56, 0x44, 0x55,
				0x0f, 0xf0, 0x00, 0xff, 0x88, 0x99,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptCat,
			expected:  true,
		},
		{
			name: "catoverflow",
			pkScript: []byte{
				0x7e, // OP_CAT
				0x0c, // Expected result push
				0x21, 0x12, 0x34, 0x56, 0x44, 0x55,
				0x0f, 0xf0, 0x00, 0xff, 0x88, 0x99,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptCatOverflow,
			expected:  false,
		},
		{
			name: "substr",
			pkScript: []byte{
				0x7f, // OP_SUBSTR
				0x04, // Expected result push
				0x34, 0x56, 0x59, 0x32,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptSubstr,
			expected:  true,
		},
		{
			name: "left",
			pkScript: []byte{
				0x80, // OP_LEFT
				0x04, // Expected result push
				0x21, 0x12, 0x34, 0x56,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptLR,
			expected:  true,
		},
		{
			name: "right",
			pkScript: []byte{
				0x81, // OP_RIGHT
				0x04, // Expected result push
				0x59, 0x32, 0x40, 0x21,
				0x87, // OP_EQUAL
			},
			sigScript: sigScriptLR,
			expected:  true,
		},
	}

	for _, test := range tests {
		msgTx := new(wire.MsgTx)
		msgTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{},
			SignatureScript:  test.sigScript,
			Sequence:         0xFFFFFFFF,
		})
		msgTx.AddTxOut(&wire.TxOut{
			Value:    0x00FFFFFF00000000,
			PkScript: []byte{0x01},
		})
		engine, err := NewEngine(test.pkScript, msgTx, 0,
			testScriptFlags, 0, nil)
		if err != nil {
			t.Errorf("Bad script result for test %v because of error: %v",
				test.name, err.Error())
			continue
		}
		err = engine.Execute()
		if err != nil && test.expected {
			t.Errorf("Bad script exec for test %v because of error: %v",
				test.name, err.Error())
		}
	}
}

func randByteSliceSlice(i int, maxLen int, src int) [][]byte {
	r := rand.New(rand.NewSource(int64(src)))

	slices := make([][]byte, i)
	for j := 0; j < i; j++ {
		for {
			sz := r.Intn(maxLen) + 1

			sl := make([]byte, sz)
			for k := 0; k < sz; k++ {
				randByte := r.Intn(255)
				sl[k] = uint8(randByte)
			}

			// No duplicates allowed.
			if j > 0 &&
				(bytes.Equal(sl, slices[j-1])) {
				r.Seed(int64(j) + r.Int63n(12345))
				continue
			}

			slices[j] = sl
			break
		}
	}

	return slices
}

// TestForVMFailure feeds random scripts to the VMs to check and see if it
// crashes. Try increasing the number of iterations or the length of the
// byte string to sample a greater space.
func TestForVMFailure(t *testing.T) {
	numTests := 2
	bsLength := 11

	for i := 0; i < numTests; i++ {
		tests := randByteSliceSlice(65536, bsLength, i)

		for j := range tests {
			if j == 0 {
				continue
			}

			msgTx := new(wire.MsgTx)
			msgTx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{},
				SignatureScript:  tests[j-1],
				Sequence:         0xFFFFFFFF,
			})
			msgTx.AddTxOut(&wire.TxOut{
				Value:    0x00FFFFFF00000000,
				PkScript: []byte{0x01},
			})
			engine, err := NewEngine(tests[j], msgTx, 0,
				testScriptFlags, 0, nil)

			if err == nil {
				engine.Execute()
			}
		}
	}
}
