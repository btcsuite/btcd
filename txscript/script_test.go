// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

// TestParseOpcode tests for opcode parsing with bad data templates.
func TestParseOpcode(t *testing.T) {
	// Deep copy the array and make one of the opcodes invalid by setting it
	// to the wrong length.
	fakeArray := opcodeArray
	fakeArray[OP_PUSHDATA4] = opcode{value: OP_PUSHDATA4,
		name: "OP_PUSHDATA4", length: -8, opfunc: opcodePushData}

	// This script would be fine if -8 was a valid length.
	_, err := parseScriptTemplate([]byte{OP_PUSHDATA4, 0x1, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00}, &fakeArray)
	if err == nil {
		t.Errorf("no error with dodgy opcode array!")
	}
}

// TestUnparsingInvalidOpcodes tests for errors when unparsing invalid parsed
// opcodes.
func TestUnparsingInvalidOpcodes(t *testing.T) {
	tests := []struct {
		name        string
		pop         *parsedOpcode
		expectedErr error
	}{
		{
			name: "OP_FALSE",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_FALSE],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_FALSE long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_FALSE],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_1 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_1],
				data:   nil,
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_1",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_1],
				data:   make([]byte, 1),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_1 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_1],
				data:   make([]byte, 2),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_2 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_2],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_2",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_2],
				data:   make([]byte, 2),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_2 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_2],
				data:   make([]byte, 3),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_3 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_3],
				data:   make([]byte, 2),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_3",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_3],
				data:   make([]byte, 3),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_3 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_3],
				data:   make([]byte, 4),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_4 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_4],
				data:   make([]byte, 3),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_4",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_4],
				data:   make([]byte, 4),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_4 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_4],
				data:   make([]byte, 5),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_5 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_5],
				data:   make([]byte, 4),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_5",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_5],
				data:   make([]byte, 5),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_5 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_5],
				data:   make([]byte, 6),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_6 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_6],
				data:   make([]byte, 5),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_6",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_6],
				data:   make([]byte, 6),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_6 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_6],
				data:   make([]byte, 7),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_7 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_7],
				data:   make([]byte, 6),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_7",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_7],
				data:   make([]byte, 7),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_7 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_7],
				data:   make([]byte, 8),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_8 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_8],
				data:   make([]byte, 7),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_8",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_8],
				data:   make([]byte, 8),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_8 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_8],
				data:   make([]byte, 9),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_9 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_9],
				data:   make([]byte, 8),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_9",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_9],
				data:   make([]byte, 9),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_9 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_9],
				data:   make([]byte, 10),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_10 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_10],
				data:   make([]byte, 9),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_10",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_10],
				data:   make([]byte, 10),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_10 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_10],
				data:   make([]byte, 11),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_11 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_11],
				data:   make([]byte, 10),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_11",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_11],
				data:   make([]byte, 11),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_11 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_11],
				data:   make([]byte, 12),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_12 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_12],
				data:   make([]byte, 11),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_12",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_12],
				data:   make([]byte, 12),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_12 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_12],
				data:   make([]byte, 13),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_13 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_13],
				data:   make([]byte, 12),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_13",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_13],
				data:   make([]byte, 13),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_13 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_13],
				data:   make([]byte, 14),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_14 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_14],
				data:   make([]byte, 13),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_14",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_14],
				data:   make([]byte, 14),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_14 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_14],
				data:   make([]byte, 15),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_15 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_15],
				data:   make([]byte, 14),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_15",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_15],
				data:   make([]byte, 15),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_15 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_15],
				data:   make([]byte, 16),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_16 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_16],
				data:   make([]byte, 15),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_16",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_16],
				data:   make([]byte, 16),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_16 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_16],
				data:   make([]byte, 17),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_17 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_17],
				data:   make([]byte, 16),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_17",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_17],
				data:   make([]byte, 17),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_17 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_17],
				data:   make([]byte, 18),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_18 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_18],
				data:   make([]byte, 17),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_18",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_18],
				data:   make([]byte, 18),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_18 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_18],
				data:   make([]byte, 19),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_19 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_19],
				data:   make([]byte, 18),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_19",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_19],
				data:   make([]byte, 19),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_19 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_19],
				data:   make([]byte, 20),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_20 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_20],
				data:   make([]byte, 19),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_20",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_20],
				data:   make([]byte, 20),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_20 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_20],
				data:   make([]byte, 21),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_21 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_21],
				data:   make([]byte, 20),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_21",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_21],
				data:   make([]byte, 21),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_21 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_21],
				data:   make([]byte, 22),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_22 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_22],
				data:   make([]byte, 21),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_22",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_22],
				data:   make([]byte, 22),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_22 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_22],
				data:   make([]byte, 23),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_23 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_23],
				data:   make([]byte, 22),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_23",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_23],
				data:   make([]byte, 23),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_23 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_23],
				data:   make([]byte, 24),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_24 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_24],
				data:   make([]byte, 23),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_24",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_24],
				data:   make([]byte, 24),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_24 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_24],
				data:   make([]byte, 25),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_25 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_25],
				data:   make([]byte, 24),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_25",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_25],
				data:   make([]byte, 25),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_25 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_25],
				data:   make([]byte, 26),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_26 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_26],
				data:   make([]byte, 25),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_26",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_26],
				data:   make([]byte, 26),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_26 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_26],
				data:   make([]byte, 27),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_27 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_27],
				data:   make([]byte, 26),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_27",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_27],
				data:   make([]byte, 27),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_27 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_27],
				data:   make([]byte, 28),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_28 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_28],
				data:   make([]byte, 27),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_28",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_28],
				data:   make([]byte, 28),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_28 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_28],
				data:   make([]byte, 29),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_29 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_29],
				data:   make([]byte, 28),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_29",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_29],
				data:   make([]byte, 29),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_29 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_29],
				data:   make([]byte, 30),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_30 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_30],
				data:   make([]byte, 29),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_30",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_30],
				data:   make([]byte, 30),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_30 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_30],
				data:   make([]byte, 31),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_31 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_31],
				data:   make([]byte, 30),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_31",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_31],
				data:   make([]byte, 31),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_31 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_31],
				data:   make([]byte, 32),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_32 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_32],
				data:   make([]byte, 31),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_32",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_32],
				data:   make([]byte, 32),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_32 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_32],
				data:   make([]byte, 33),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_33 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_33],
				data:   make([]byte, 32),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_33",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_33],
				data:   make([]byte, 33),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_33 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_33],
				data:   make([]byte, 34),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_34 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_34],
				data:   make([]byte, 33),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_34",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_34],
				data:   make([]byte, 34),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_34 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_34],
				data:   make([]byte, 35),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_35 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_35],
				data:   make([]byte, 34),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_35",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_35],
				data:   make([]byte, 35),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_35 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_35],
				data:   make([]byte, 36),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_36 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_36],
				data:   make([]byte, 35),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_36",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_36],
				data:   make([]byte, 36),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_36 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_36],
				data:   make([]byte, 37),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_37 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_37],
				data:   make([]byte, 36),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_37",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_37],
				data:   make([]byte, 37),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_37 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_37],
				data:   make([]byte, 38),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_38 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_38],
				data:   make([]byte, 37),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_38",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_38],
				data:   make([]byte, 38),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_38 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_38],
				data:   make([]byte, 39),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_39 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_39],
				data:   make([]byte, 38),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_39",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_39],
				data:   make([]byte, 39),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_39 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_39],
				data:   make([]byte, 40),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_40 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_40],
				data:   make([]byte, 39),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_40",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_40],
				data:   make([]byte, 40),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_40 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_40],
				data:   make([]byte, 41),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_41 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_41],
				data:   make([]byte, 40),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_41",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_41],
				data:   make([]byte, 41),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_41 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_41],
				data:   make([]byte, 42),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_42 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_42],
				data:   make([]byte, 41),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_42",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_42],
				data:   make([]byte, 42),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_42 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_42],
				data:   make([]byte, 43),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_43 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_43],
				data:   make([]byte, 42),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_43",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_43],
				data:   make([]byte, 43),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_43 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_43],
				data:   make([]byte, 44),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_44 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_44],
				data:   make([]byte, 43),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_44",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_44],
				data:   make([]byte, 44),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_44 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_44],
				data:   make([]byte, 45),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_45 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_45],
				data:   make([]byte, 44),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_45",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_45],
				data:   make([]byte, 45),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_45 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_45],
				data:   make([]byte, 46),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_46 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_46],
				data:   make([]byte, 45),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_46",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_46],
				data:   make([]byte, 46),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_46 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_46],
				data:   make([]byte, 47),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_47 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_47],
				data:   make([]byte, 46),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_47",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_47],
				data:   make([]byte, 47),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_47 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_47],
				data:   make([]byte, 48),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_48 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_48],
				data:   make([]byte, 47),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_48",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_48],
				data:   make([]byte, 48),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_48 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_48],
				data:   make([]byte, 49),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_49 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_49],
				data:   make([]byte, 48),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_49",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_49],
				data:   make([]byte, 49),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_49 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_49],
				data:   make([]byte, 50),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_50 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_50],
				data:   make([]byte, 49),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_50",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_50],
				data:   make([]byte, 50),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_50 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_50],
				data:   make([]byte, 51),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_51 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_51],
				data:   make([]byte, 50),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_51",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_51],
				data:   make([]byte, 51),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_51 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_51],
				data:   make([]byte, 52),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_52 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_52],
				data:   make([]byte, 51),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_52",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_52],
				data:   make([]byte, 52),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_52 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_52],
				data:   make([]byte, 53),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_53 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_53],
				data:   make([]byte, 52),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_53",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_53],
				data:   make([]byte, 53),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_53 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_53],
				data:   make([]byte, 54),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_54 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_54],
				data:   make([]byte, 53),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_54",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_54],
				data:   make([]byte, 54),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_54 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_54],
				data:   make([]byte, 55),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_55 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_55],
				data:   make([]byte, 54),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_55",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_55],
				data:   make([]byte, 55),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_55 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_55],
				data:   make([]byte, 56),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_56 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_56],
				data:   make([]byte, 55),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_56",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_56],
				data:   make([]byte, 56),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_56 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_56],
				data:   make([]byte, 57),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_57 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_57],
				data:   make([]byte, 56),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_57",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_57],
				data:   make([]byte, 57),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_57 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_57],
				data:   make([]byte, 58),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_58 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_58],
				data:   make([]byte, 57),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_58",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_58],
				data:   make([]byte, 58),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_58 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_58],
				data:   make([]byte, 59),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_59 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_59],
				data:   make([]byte, 58),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_59",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_59],
				data:   make([]byte, 59),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_59 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_59],
				data:   make([]byte, 60),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_60 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_60],
				data:   make([]byte, 59),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_60",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_60],
				data:   make([]byte, 60),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_60 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_60],
				data:   make([]byte, 61),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_61 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_61],
				data:   make([]byte, 60),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_61",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_61],
				data:   make([]byte, 61),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_61 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_61],
				data:   make([]byte, 62),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_62 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_62],
				data:   make([]byte, 61),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_62",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_62],
				data:   make([]byte, 62),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_62 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_62],
				data:   make([]byte, 63),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_63 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_63],
				data:   make([]byte, 62),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_63",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_63],
				data:   make([]byte, 63),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_63 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_63],
				data:   make([]byte, 64),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_64 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_64],
				data:   make([]byte, 63),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_64",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_64],
				data:   make([]byte, 64),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_64 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_64],
				data:   make([]byte, 65),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_65 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_65],
				data:   make([]byte, 64),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_65",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_65],
				data:   make([]byte, 65),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_65 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_65],
				data:   make([]byte, 66),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_66 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_66],
				data:   make([]byte, 65),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_66",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_66],
				data:   make([]byte, 66),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_66 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_66],
				data:   make([]byte, 67),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_67 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_67],
				data:   make([]byte, 66),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_67",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_67],
				data:   make([]byte, 67),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_67 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_67],
				data:   make([]byte, 68),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_68 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_68],
				data:   make([]byte, 67),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_68",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_68],
				data:   make([]byte, 68),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_68 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_68],
				data:   make([]byte, 69),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_69 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_69],
				data:   make([]byte, 68),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_69",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_69],
				data:   make([]byte, 69),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_69 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_69],
				data:   make([]byte, 70),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_70 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_70],
				data:   make([]byte, 69),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_70",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_70],
				data:   make([]byte, 70),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_70 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_70],
				data:   make([]byte, 71),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_71 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_71],
				data:   make([]byte, 70),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_71",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_71],
				data:   make([]byte, 71),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_71 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_71],
				data:   make([]byte, 72),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_72 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_72],
				data:   make([]byte, 71),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_72",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_72],
				data:   make([]byte, 72),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_72 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_72],
				data:   make([]byte, 73),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_73 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_73],
				data:   make([]byte, 72),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_73",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_73],
				data:   make([]byte, 73),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_73 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_73],
				data:   make([]byte, 74),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_74 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_74],
				data:   make([]byte, 73),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_74",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_74],
				data:   make([]byte, 74),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_74 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_74],
				data:   make([]byte, 75),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_75 short",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_75],
				data:   make([]byte, 74),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DATA_75",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_75],
				data:   make([]byte, 75),
			},
			expectedErr: nil,
		},
		{
			name: "OP_DATA_75 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DATA_75],
				data:   make([]byte, 76),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_PUSHDATA1",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PUSHDATA1],
				data:   []byte{0, 1, 2, 3, 4},
			},
			expectedErr: nil,
		},
		{
			name: "OP_PUSHDATA2",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PUSHDATA2],
				data:   []byte{0, 1, 2, 3, 4},
			},
			expectedErr: nil,
		},
		{
			name: "OP_PUSHDATA4",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PUSHDATA1],
				data:   []byte{0, 1, 2, 3, 4},
			},
			expectedErr: nil,
		},
		{
			name: "OP_1NEGATE",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_1NEGATE],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_1NEGATE long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_1NEGATE],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_RESERVED",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RESERVED],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_RESERVED long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RESERVED],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_TRUE",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_TRUE],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_TRUE long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_TRUE],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_3",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_3],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_3 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_3],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_4",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_4],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_4 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_4],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_5",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_5],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_5 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_5],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_6",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_6],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_6 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_6],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_7",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_7],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_7 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_7],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_8",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_8],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_8 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_8],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_9",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_9],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_9 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_9],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_10",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_10],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_10 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_10],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_11",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_11],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_11 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_11],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_12",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_12],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_12 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_12],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_13",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_13],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_13 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_13],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_14",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_14],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_14 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_14],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_15",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_15],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_15 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_15],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_16",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_16],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_16 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_16],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_VER",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_VER],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_VER long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_VER],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_IF",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_IF],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_IF long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_IF],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOTIF",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOTIF],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOTIF long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOTIF],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_VERIF",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_VERIF],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_VERIF long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_VERIF],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_VERNOTIF",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_VERNOTIF],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_VERNOTIF long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_VERNOTIF],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_ELSE",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ELSE],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_ELSE long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ELSE],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_ENDIF",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ENDIF],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_ENDIF long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ENDIF],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_VERIFY",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_VERIFY],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_VERIFY long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_VERIFY],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_RETURN",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RETURN],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_RETURN long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RETURN],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_TOALTSTACK",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_TOALTSTACK],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_TOALTSTACK long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_TOALTSTACK],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_FROMALTSTACK",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_FROMALTSTACK],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_FROMALTSTACK long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_FROMALTSTACK],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2DROP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2DROP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2DROP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2DROP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2DUP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2DUP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2DUP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2DUP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_3DUP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_3DUP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_3DUP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_3DUP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2OVER",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2OVER],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2OVER long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2OVER],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2ROT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2ROT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2ROT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2ROT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2SWAP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2SWAP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2SWAP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2SWAP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_IFDUP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_IFDUP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_IFDUP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_IFDUP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DEPTH",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DEPTH],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_DEPTH long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DEPTH],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DROP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DROP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_DROP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DROP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DUP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DUP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_DUP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DUP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NIP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NIP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NIP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NIP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_OVER",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_OVER],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_OVER long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_OVER],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_PICK",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PICK],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_PICK long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PICK],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_ROLL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ROLL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_ROLL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ROLL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_ROT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ROT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_ROT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ROT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_SWAP",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SWAP],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_SWAP long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SWAP],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_TUCK",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_TUCK],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_TUCK long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_TUCK],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_CAT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CAT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_CAT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CAT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_SUBSTR",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SUBSTR],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_SUBSTR long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SUBSTR],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_LEFT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LEFT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_LEFT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LEFT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_LEFT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LEFT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_LEFT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LEFT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_RIGHT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RIGHT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_RIGHT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RIGHT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_SIZE",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SIZE],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_SIZE long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SIZE],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_INVERT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_INVERT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_INVERT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_INVERT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_AND",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_AND],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_AND long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_AND],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_OR",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_OR],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_OR long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_OR],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_XOR",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_XOR],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_XOR long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_XOR],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_EQUAL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_EQUAL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_EQUAL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_EQUAL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_EQUALVERIFY",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_EQUALVERIFY],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_EQUALVERIFY long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_EQUALVERIFY],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_RESERVED1",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RESERVED1],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_RESERVED1 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RESERVED1],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_RESERVED2",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RESERVED2],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_RESERVED2 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RESERVED2],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_1ADD",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_1ADD],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_1ADD long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_1ADD],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_1SUB",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_1SUB],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_1SUB long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_1SUB],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2MUL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2MUL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2MUL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2MUL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_2DIV",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2DIV],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_2DIV long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_2DIV],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NEGATE",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NEGATE],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NEGATE long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NEGATE],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_ABS",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ABS],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_ABS long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ABS],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_0NOTEQUAL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_0NOTEQUAL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_0NOTEQUAL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_0NOTEQUAL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_ADD",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ADD],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_ADD long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_ADD],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_SUB",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SUB],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_SUB long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SUB],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_MUL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_MUL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_MUL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_MUL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_DIV",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DIV],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_DIV long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_DIV],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_MOD",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_MOD],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_MOD long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_MOD],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_LSHIFT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LSHIFT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_LSHIFT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LSHIFT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_RSHIFT",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RSHIFT],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_RSHIFT long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RSHIFT],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_BOOLAND",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_BOOLAND],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_BOOLAND long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_BOOLAND],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_BOOLOR",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_BOOLOR],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_BOOLOR long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_BOOLOR],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NUMEQUAL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NUMEQUAL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NUMEQUAL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NUMEQUAL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NUMEQUALVERIFY",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NUMEQUALVERIFY],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NUMEQUALVERIFY long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NUMEQUALVERIFY],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NUMNOTEQUAL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NUMNOTEQUAL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NUMNOTEQUAL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NUMNOTEQUAL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_LESSTHAN",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LESSTHAN],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_LESSTHAN long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LESSTHAN],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_GREATERTHAN",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_GREATERTHAN],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_GREATERTHAN long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_GREATERTHAN],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_LESSTHANOREQUAL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LESSTHANOREQUAL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_LESSTHANOREQUAL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_LESSTHANOREQUAL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_GREATERTHANOREQUAL",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_GREATERTHANOREQUAL],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_GREATERTHANOREQUAL long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_GREATERTHANOREQUAL],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_MIN",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_MIN],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_MIN long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_MIN],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_MAX",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_MAX],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_MAX long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_MAX],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_WITHIN",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_WITHIN],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_WITHIN long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_WITHIN],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_RIPEMD160",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RIPEMD160],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_RIPEMD160 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_RIPEMD160],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_SHA1",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SHA1],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_SHA1 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SHA1],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_SHA256",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SHA256],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_SHA256 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_SHA256],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_HASH160",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_HASH160],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_HASH160 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_HASH160],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_HASH256",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_HASH256],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_HASH256 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_HASH256],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_CODESAPERATOR",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CODESEPARATOR],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_CODESEPARATOR long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CODESEPARATOR],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_CHECKSIG",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CHECKSIG],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_CHECKSIG long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CHECKSIG],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_CHECKSIGVERIFY",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CHECKSIGVERIFY],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_CHECKSIGVERIFY long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CHECKSIGVERIFY],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_CHECKMULTISIG",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CHECKMULTISIG],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_CHECKMULTISIG long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CHECKMULTISIG],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_CHECKMULTISIGVERIFY",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CHECKMULTISIGVERIFY],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_CHECKMULTISIGVERIFY long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_CHECKMULTISIGVERIFY],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP1",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP1],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP1 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP1],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP2",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP2],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP2 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP2],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP3",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP3],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP3 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP3],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP4",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP4],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP4 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP4],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP5",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP5],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP5 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP5],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP6",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP6],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP6 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP6],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP7",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP7],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP7 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP7],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP8",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP8],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP8 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP8],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP9",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP9],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP9 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP9],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_NOP10",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP10],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_NOP10 long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_NOP10],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_PUBKEYHASH",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PUBKEYHASH],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_PUBKEYHASH long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PUBKEYHASH],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_PUBKEY",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PUBKEY],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_PUBKEY long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_PUBKEY],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
		{
			name: "OP_INVALIDOPCODE",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_INVALIDOPCODE],
				data:   nil,
			},
			expectedErr: nil,
		},
		{
			name: "OP_INVALIDOPCODE long",
			pop: &parsedOpcode{
				opcode: &opcodeArray[OP_INVALIDOPCODE],
				data:   make([]byte, 1),
			},
			expectedErr: scriptError(ErrInternal, ""),
		},
	}

	for _, test := range tests {
		_, err := test.pop.bytes()
		if e := tstCheckScriptError(err, test.expectedErr); e != nil {
			t.Errorf("Parsed opcode test '%s': %v", test.name, e)
			continue
		}
	}
}

// TestPushedData ensured the PushedData function extracts the expected data out
// of various scripts.
func TestPushedData(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		script string
		out    [][]byte
		valid  bool
	}{
		{
			"0 IF 0 ELSE 2 ENDIF",
			[][]byte{nil, nil},
			true,
		},
		{
			"16777216 10000000",
			[][]byte{
				{0x00, 0x00, 0x00, 0x01}, // 16777216
				{0x80, 0x96, 0x98, 0x00}, // 10000000
			},
			true,
		},
		{
			"DUP HASH160 '17VZNX1SN5NtKa8UQFxwQbFeFc3iqRYhem' EQUALVERIFY CHECKSIG",
			[][]byte{
				// 17VZNX1SN5NtKa8UQFxwQbFeFc3iqRYhem
				{
					0x31, 0x37, 0x56, 0x5a, 0x4e, 0x58, 0x31, 0x53, 0x4e, 0x35,
					0x4e, 0x74, 0x4b, 0x61, 0x38, 0x55, 0x51, 0x46, 0x78, 0x77,
					0x51, 0x62, 0x46, 0x65, 0x46, 0x63, 0x33, 0x69, 0x71, 0x52,
					0x59, 0x68, 0x65, 0x6d,
				},
			},
			true,
		},
		{
			"PUSHDATA4 1000 EQUAL",
			nil,
			false,
		},
	}

	for i, test := range tests {
		script := mustParseShortForm(test.script)
		data, err := PushedData(script)
		if test.valid && err != nil {
			t.Errorf("TestPushedData failed test #%d: %v\n", i, err)
			continue
		} else if !test.valid && err == nil {
			t.Errorf("TestPushedData failed test #%d: test should "+
				"be invalid\n", i)
			continue
		}
		if !reflect.DeepEqual(data, test.out) {
			t.Errorf("TestPushedData failed test #%d: want: %x "+
				"got: %x\n", i, test.out, data)
		}
	}
}

// TestHasCanonicalPush ensures the canonicalPush function works as expected.
func TestHasCanonicalPush(t *testing.T) {
	t.Parallel()

	for i := 0; i < 65535; i++ {
		script, err := NewScriptBuilder().AddInt64(int64(i)).Script()
		if err != nil {
			t.Errorf("Script: test #%d unexpected error: %v\n", i,
				err)
			continue
		}
		if result := IsPushOnlyScript(script); !result {
			t.Errorf("IsPushOnlyScript: test #%d failed: %x\n", i,
				script)
			continue
		}
		pops, err := parseScript(script)
		if err != nil {
			t.Errorf("parseScript: #%d failed: %v", i, err)
			continue
		}
		for _, pop := range pops {
			if result := canonicalPush(pop); !result {
				t.Errorf("canonicalPush: test #%d failed: %x\n",
					i, script)
				break
			}
		}
	}
	for i := 0; i <= MaxScriptElementSize; i++ {
		builder := NewScriptBuilder()
		builder.AddData(bytes.Repeat([]byte{0x49}, i))
		script, err := builder.Script()
		if err != nil {
			t.Errorf("StandardPushesTests test #%d unexpected error: %v\n", i, err)
			continue
		}
		if result := IsPushOnlyScript(script); !result {
			t.Errorf("StandardPushesTests IsPushOnlyScript test #%d failed: %x\n", i, script)
			continue
		}
		pops, err := parseScript(script)
		if err != nil {
			t.Errorf("StandardPushesTests #%d failed to TstParseScript: %v", i, err)
			continue
		}
		for _, pop := range pops {
			if result := canonicalPush(pop); !result {
				t.Errorf("StandardPushesTests TstHasCanonicalPushes test #%d failed: %x\n", i, script)
				break
			}
		}
	}
}

// TestGetPreciseSigOps ensures the more precise signature operation counting
// mechanism which includes signatures in P2SH scripts works as expected.
func TestGetPreciseSigOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		scriptSig []byte
		nSigOps   int
	}{
		{
			name:      "scriptSig doesn't parse",
			scriptSig: mustParseShortForm("PUSHDATA1 0x02"),
		},
		{
			name:      "scriptSig isn't push only",
			scriptSig: mustParseShortForm("1 DUP"),
			nSigOps:   0,
		},
		{
			name:      "scriptSig length 0",
			scriptSig: nil,
			nSigOps:   0,
		},
		{
			name: "No script at the end",
			// No script at end but still push only.
			scriptSig: mustParseShortForm("1 1"),
			nSigOps:   0,
		},
		{
			name:      "pushed script doesn't parse",
			scriptSig: mustParseShortForm("DATA_2 PUSHDATA1 0x02"),
		},
	}

	// The signature in the p2sh script is nonsensical for the tests since
	// this script will never be executed.  What matters is that it matches
	// the right pattern.
	pkScript := mustParseShortForm("HASH160 DATA_20 0x433ec2ac1ffa1b7b7d0" +
		"27f564529c57197f9ae88 EQUAL")
	for _, test := range tests {
		count := GetPreciseSigOpCount(test.scriptSig, pkScript, true)
		if count != test.nSigOps {
			t.Errorf("%s: expected count of %d, got %d", test.name,
				test.nSigOps, count)

		}
	}
}

// TestGetWitnessSigOpCount tests that the sig op counting for p2wkh, p2wsh,
// nested p2sh, and invalid variants are counted properly.
func TestGetWitnessSigOpCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string

		sigScript []byte
		pkScript  []byte
		witness   wire.TxWitness

		numSigOps int
	}{
		// A regualr p2wkh witness program. The output being spent
		// should only have a single sig-op counted.
		{
			name: "p2wkh",
			pkScript: mustParseShortForm("OP_0 DATA_20 " +
				"0x365ab47888e150ff46f8d51bce36dcd680f1283f"),
			witness: wire.TxWitness{
				hexToBytes("3045022100ee9fe8f9487afa977" +
					"6647ebcf0883ce0cd37454d7ce19889d34ba2c9" +
					"9ce5a9f402200341cb469d0efd3955acb9e46" +
					"f568d7e2cc10f9084aaff94ced6dc50a59134ad01"),
				hexToBytes("03f0000d0639a22bfaf217e4c9428" +
					"9c2b0cc7fa1036f7fd5d9f61a9d6ec153100e"),
			},
			numSigOps: 1,
		},
		// A p2wkh witness program nested within a p2sh output script.
		// The pattern should be recognized properly and attribute only
		// a single sig op.
		{
			name: "nested p2sh",
			sigScript: hexToBytes("160014ad0ffa2e387f07" +
				"e7ead14dc56d5a97dbd6ff5a23"),
			pkScript: mustParseShortForm("HASH160 DATA_20 " +
				"0xb3a84b564602a9d68b4c9f19c2ea61458ff7826c EQUAL"),
			witness: wire.TxWitness{
				hexToBytes("3045022100cb1c2ac1ff1d57d" +
					"db98f7bdead905f8bf5bcc8641b029ce8eef25" +
					"c75a9e22a4702203be621b5c86b771288706be5" +
					"a7eee1db4fceabf9afb7583c1cc6ee3f8297b21201"),
				hexToBytes("03f0000d0639a22bfaf217e4c9" +
					"4289c2b0cc7fa1036f7fd5d9f61a9d6ec153100e"),
			},
			numSigOps: 1,
		},
		// A p2sh script that spends a 2-of-2 multi-sig output.
		{
			name:      "p2wsh multi-sig spend",
			numSigOps: 2,
			pkScript: hexToBytes("0020e112b88a0cd87ba387f" +
				"449d443ee2596eb353beb1f0351ab2cba8909d875db23"),
			witness: wire.TxWitness{
				hexToBytes("522103b05faca7ceda92b493" +
					"3f7acdf874a93de0dc7edc461832031cd69cbb1d1e" +
					"6fae2102e39092e031c1621c902e3704424e8d8" +
					"3ca481d4d4eeae1b7970f51c78231207e52ae"),
			},
		},
		// A p2wsh witness program. However, the witness script fails
		// to parse after the valid portion of the script. As a result,
		// the valid portion of the script should still be counted.
		{
			name:      "witness script doesn't parse",
			numSigOps: 1,
			pkScript: hexToBytes("0020e112b88a0cd87ba387f44" +
				"9d443ee2596eb353beb1f0351ab2cba8909d875db23"),
			witness: wire.TxWitness{
				mustParseShortForm("DUP HASH160 " +
					"'17VZNX1SN5NtKa8UQFxwQbFeFc3iqRYhem'" +
					" EQUALVERIFY CHECKSIG DATA_20 0x91"),
			},
		},
	}

	for _, test := range tests {
		count := GetWitnessSigOpCount(test.sigScript, test.pkScript,
			test.witness)
		if count != test.numSigOps {
			t.Errorf("%s: expected count of %d, got %d", test.name,
				test.numSigOps, count)

		}
	}
}

// TestRemoveOpcodes ensures that removing opcodes from scripts behaves as
// expected.
func TestRemoveOpcodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		before string
		remove byte
		err    error
		after  string
	}{
		{
			// Nothing to remove.
			name:   "nothing to remove",
			before: "NOP",
			remove: OP_CODESEPARATOR,
			after:  "NOP",
		},
		{
			// Test basic opcode removal.
			name:   "codeseparator 1",
			before: "NOP CODESEPARATOR TRUE",
			remove: OP_CODESEPARATOR,
			after:  "NOP TRUE",
		},
		{
			// The opcode in question is actually part of the data
			// in a previous opcode.
			name:   "codeseparator by coincidence",
			before: "NOP DATA_1 CODESEPARATOR TRUE",
			remove: OP_CODESEPARATOR,
			after:  "NOP DATA_1 CODESEPARATOR TRUE",
		},
		{
			name:   "invalid opcode",
			before: "CAT",
			remove: OP_CODESEPARATOR,
			after:  "CAT",
		},
		{
			name:   "invalid length (instruction)",
			before: "PUSHDATA1",
			remove: OP_CODESEPARATOR,
			err:    scriptError(ErrMalformedPush, ""),
		},
		{
			name:   "invalid length (data)",
			before: "PUSHDATA1 0xff 0xfe",
			remove: OP_CODESEPARATOR,
			err:    scriptError(ErrMalformedPush, ""),
		},
	}

	// tstRemoveOpcode is a convenience function to parse the provided
	// raw script, remove the passed opcode, then unparse the result back
	// into a raw script.
	tstRemoveOpcode := func(script []byte, opcode byte) ([]byte, error) {
		pops, err := parseScript(script)
		if err != nil {
			return nil, err
		}
		pops = removeOpcode(pops, opcode)
		return unparseScript(pops)
	}

	for _, test := range tests {
		before := mustParseShortForm(test.before)
		after := mustParseShortForm(test.after)
		result, err := tstRemoveOpcode(before, test.remove)
		if e := tstCheckScriptError(err, test.err); e != nil {
			t.Errorf("%s: %v", test.name, e)
			continue
		}

		if !bytes.Equal(after, result) {
			t.Errorf("%s: value does not equal expected: exp: %q"+
				" got: %q", test.name, after, result)
		}
	}
}

// TestRemoveOpcodeByData ensures that removing data carrying opcodes based on
// the data they contain works as expected.
func TestRemoveOpcodeByData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		before []byte
		remove []byte
		err    error
		after  []byte
	}{
		{
			name:   "nothing to do",
			before: []byte{OP_NOP},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{OP_NOP},
		},
		{
			name:   "simple case",
			before: []byte{OP_DATA_4, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name:   "simple case (miss)",
			before: []byte{OP_DATA_4, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 5},
			after:  []byte{OP_DATA_4, 1, 2, 3, 4},
		},
		{
			// padded to keep it canonical.
			name: "simple case (pushdata1)",
			before: append(append([]byte{OP_PUSHDATA1, 76},
				bytes.Repeat([]byte{0}, 72)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name: "simple case (pushdata1 miss)",
			before: append(append([]byte{OP_PUSHDATA1, 76},
				bytes.Repeat([]byte{0}, 72)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 5},
			after: append(append([]byte{OP_PUSHDATA1, 76},
				bytes.Repeat([]byte{0}, 72)...),
				[]byte{1, 2, 3, 4}...),
		},
		{
			name:   "simple case (pushdata1 miss noncanonical)",
			before: []byte{OP_PUSHDATA1, 4, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{OP_PUSHDATA1, 4, 1, 2, 3, 4},
		},
		{
			name: "simple case (pushdata2)",
			before: append(append([]byte{OP_PUSHDATA2, 0, 1},
				bytes.Repeat([]byte{0}, 252)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name: "simple case (pushdata2 miss)",
			before: append(append([]byte{OP_PUSHDATA2, 0, 1},
				bytes.Repeat([]byte{0}, 252)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4, 5},
			after: append(append([]byte{OP_PUSHDATA2, 0, 1},
				bytes.Repeat([]byte{0}, 252)...),
				[]byte{1, 2, 3, 4}...),
		},
		{
			name:   "simple case (pushdata2 miss noncanonical)",
			before: []byte{OP_PUSHDATA2, 4, 0, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{OP_PUSHDATA2, 4, 0, 1, 2, 3, 4},
		},
		{
			// This is padded to make the push canonical.
			name: "simple case (pushdata4)",
			before: append(append([]byte{OP_PUSHDATA4, 0, 0, 1, 0},
				bytes.Repeat([]byte{0}, 65532)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name:   "simple case (pushdata4 miss noncanonical)",
			before: []byte{OP_PUSHDATA4, 4, 0, 0, 0, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{OP_PUSHDATA4, 4, 0, 0, 0, 1, 2, 3, 4},
		},
		{
			// This is padded to make the push canonical.
			name: "simple case (pushdata4 miss)",
			before: append(append([]byte{OP_PUSHDATA4, 0, 0, 1, 0},
				bytes.Repeat([]byte{0}, 65532)...), []byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4, 5},
			after: append(append([]byte{OP_PUSHDATA4, 0, 0, 1, 0},
				bytes.Repeat([]byte{0}, 65532)...), []byte{1, 2, 3, 4}...),
		},
		{
			name:   "invalid opcode ",
			before: []byte{OP_UNKNOWN187},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{OP_UNKNOWN187},
		},
		{
			name:   "invalid length (instruction)",
			before: []byte{OP_PUSHDATA1},
			remove: []byte{1, 2, 3, 4},
			err:    scriptError(ErrMalformedPush, ""),
		},
		{
			name:   "invalid length (data)",
			before: []byte{OP_PUSHDATA1, 255, 254},
			remove: []byte{1, 2, 3, 4},
			err:    scriptError(ErrMalformedPush, ""),
		},
	}

	// tstRemoveOpcodeByData is a convenience function to parse the provided
	// raw script, remove the passed data, then unparse the result back
	// into a raw script.
	tstRemoveOpcodeByData := func(script []byte, data []byte) ([]byte, error) {
		pops, err := parseScript(script)
		if err != nil {
			return nil, err
		}
		pops = removeOpcodeByData(pops, data)
		return unparseScript(pops)
	}

	for _, test := range tests {
		result, err := tstRemoveOpcodeByData(test.before, test.remove)
		if e := tstCheckScriptError(err, test.err); e != nil {
			t.Errorf("%s: %v", test.name, e)
			continue
		}

		if !bytes.Equal(test.after, result) {
			t.Errorf("%s: value does not equal expected: exp: %q"+
				" got: %q", test.name, test.after, result)
		}
	}
}

// TestIsPayToScriptHash ensures the IsPayToScriptHash function returns the
// expected results for all the scripts in scriptClassTests.
func TestIsPayToScriptHash(t *testing.T) {
	t.Parallel()

	for _, test := range scriptClassTests {
		script := mustParseShortForm(test.script)
		shouldBe := (test.class == ScriptHashTy)
		p2sh := IsPayToScriptHash(script)
		if p2sh != shouldBe {
			t.Errorf("%s: expected p2sh %v, got %v", test.name,
				shouldBe, p2sh)
		}
	}
}

// TestIsPayToWitnessScriptHash ensures the IsPayToWitnessScriptHash function
// returns the expected results for all the scripts in scriptClassTests.
func TestIsPayToWitnessScriptHash(t *testing.T) {
	t.Parallel()

	for _, test := range scriptClassTests {
		script := mustParseShortForm(test.script)
		shouldBe := (test.class == WitnessV0ScriptHashTy)
		p2wsh := IsPayToWitnessScriptHash(script)
		if p2wsh != shouldBe {
			t.Errorf("%s: expected p2wsh %v, got %v", test.name,
				shouldBe, p2wsh)
		}
	}
}

// TestIsPayToWitnessPubKeyHash ensures the IsPayToWitnessPubKeyHash function
// returns the expected results for all the scripts in scriptClassTests.
func TestIsPayToWitnessPubKeyHash(t *testing.T) {
	t.Parallel()

	for _, test := range scriptClassTests {
		script := mustParseShortForm(test.script)
		shouldBe := (test.class == WitnessV0PubKeyHashTy)
		p2wkh := IsPayToWitnessPubKeyHash(script)
		if p2wkh != shouldBe {
			t.Errorf("%s: expected p2wkh %v, got %v", test.name,
				shouldBe, p2wkh)
		}
	}
}

// TestHasCanonicalPushes ensures the canonicalPush function properly determines
// what is considered a canonical push for the purposes of removeOpcodeByData.
func TestHasCanonicalPushes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{
			name: "does not parse",
			script: "0x046708afdb0fe5548271967f1a67130b7105cd6a82" +
				"8e03909a67962e0ea1f61d",
			expected: false,
		},
		{
			name:     "non-canonical push",
			script:   "PUSHDATA1 0x04 0x01020304",
			expected: false,
		},
	}

	for i, test := range tests {
		script := mustParseShortForm(test.script)
		pops, err := parseScript(script)
		if err != nil {
			if test.expected {
				t.Errorf("TstParseScript #%d failed: %v", i, err)
			}
			continue
		}
		for _, pop := range pops {
			if canonicalPush(pop) != test.expected {
				t.Errorf("canonicalPush: #%d (%s) wrong result"+
					"\ngot: %v\nwant: %v", i, test.name,
					true, test.expected)
				break
			}
		}
	}
}

// TestIsPushOnlyScript ensures the IsPushOnlyScript function returns the
// expected results.
func TestIsPushOnlyScript(t *testing.T) {
	t.Parallel()

	test := struct {
		name     string
		script   []byte
		expected bool
	}{
		name: "does not parse",
		script: mustParseShortForm("0x046708afdb0fe5548271967f1a67130" +
			"b7105cd6a828e03909a67962e0ea1f61d"),
		expected: false,
	}

	if IsPushOnlyScript(test.script) != test.expected {
		t.Errorf("IsPushOnlyScript (%s) wrong result\ngot: %v\nwant: "+
			"%v", test.name, true, test.expected)
	}
}

// TestIsUnspendable ensures the IsUnspendable function returns the expected
// results.
func TestIsUnspendable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pkScript []byte
		expected bool
	}{
		{
			// Unspendable
			pkScript: []byte{0x6a, 0x04, 0x74, 0x65, 0x73, 0x74},
			expected: true,
		},
		{
			// Spendable
			pkScript: []byte{0x76, 0xa9, 0x14, 0x29, 0x95, 0xa0,
				0xfe, 0x68, 0x43, 0xfa, 0x9b, 0x95, 0x45,
				0x97, 0xf0, 0xdc, 0xa7, 0xa4, 0x4d, 0xf6,
				0xfa, 0x0b, 0x5c, 0x88, 0xac},
			expected: false,
		},
		{
			// Spendable
			pkScript: []byte{0xa9, 0x14, 0x82, 0x1d, 0xba, 0x94, 0xbc, 0xfb,
				0xa2, 0x57, 0x36, 0xa3, 0x9e, 0x5d, 0x14, 0x5d, 0x69, 0x75,
				0xba, 0x8c, 0x0b, 0x42, 0x87},
			expected: false,
		},
		{
			// Not Necessarily Unspendable
			pkScript: []byte{},
			expected: false,
		},
		{
			// Spendable
			pkScript: []byte{OP_TRUE},
			expected: false,
		},
		{
			// Unspendable
			pkScript: []byte{OP_RETURN},
			expected: true,
		},
	}

	for i, test := range tests {
		res := IsUnspendable(test.pkScript)
		if res != test.expected {
			t.Errorf("TestIsUnspendable #%d failed: got %v want %v",
				i, res, test.expected)
			continue
		}
	}
}

// BenchmarkIsUnspendable adds a benchmark to compare the time and allocations
// necessary for the IsUnspendable function.
func BenchmarkIsUnspendable(b *testing.B) {
	pkScriptToUse := []byte{0xa9, 0x14, 0x82, 0x1d, 0xba, 0x94, 0xbc, 0xfb, 0xa2, 0x57, 0x36, 0xa3, 0x9e, 0x5d, 0x14, 0x5d, 0x69, 0x75, 0xba, 0x8c, 0x0b, 0x42, 0x87}
	var res bool = false
	for i := 0; i < b.N; i++ {
		res = IsUnspendable(pkScriptToUse)
	}
	if res {
		b.Fatalf("Benchmark should never have res be %t\n", res)
	}
}
