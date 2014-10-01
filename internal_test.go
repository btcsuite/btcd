// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"

	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// this file is present to export some internal interfaces so that we can
// test them reliably.

func TstRemoveOpcode(pkscript []byte, opcode byte) ([]byte, error) {
	pops, err := parseScript(pkscript)
	if err != nil {
		return nil, err
	}
	pops = removeOpcode(pops, opcode)
	return unparseScript(pops)
}

func TstRemoveOpcodeByData(pkscript []byte, data []byte) ([]byte, error) {
	pops, err := parseScript(pkscript)
	if err != nil {
		return nil, err
	}
	pops = removeOpcodeByData(pops, data)
	return unparseScript(pops)
}

// TestSetPC allows the test modules to set the program counter to whatever they
// want.
func (s *Script) TstSetPC(script, off int) {
	s.scriptidx = script
	s.scriptoff = off
}

// Internal tests for opcodde parsing with bad data templates.
func TestParseOpcode(t *testing.T) {
	fakemap := make(map[byte]*opcode)
	// deep copy
	for k, v := range opcodemap {
		fakemap[k] = v
	}
	// wrong length -8.
	fakemap[OP_PUSHDATA4] = &opcode{value: OP_PUSHDATA4,
		name: "OP_PUSHDATA4", length: -8, opfunc: opcodePushData}

	// this script would be fine if -8 was a valid length.
	_, err := parseScriptTemplate([]byte{OP_PUSHDATA4, 0x1, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00}, fakemap)
	if err == nil {
		t.Errorf("no error with dodgy opcode map!")
	}

	// Missing entry.
	fakemap = make(map[byte]*opcode)
	for k, v := range opcodemap {
		fakemap[k] = v
	}
	delete(fakemap, OP_PUSHDATA4)
	// this script would be fine if -8 was a valid length.
	_, err = parseScriptTemplate([]byte{OP_PUSHDATA4, 0x1, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00}, fakemap)
	if err == nil {
		t.Errorf("no error with dodgy opcode map (missing entry)!")
	}
}

type popTest struct {
	name        string
	pop         *parsedOpcode
	expectedErr error
}

var popTests = []popTest{
	{
		name: "OP_FALSE",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_FALSE],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_FALSE long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_FALSE],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_1 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_1],
			data:   nil,
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_1",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_1],
			data:   make([]byte, 1),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_1 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_1],
			data:   make([]byte, 2),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_2 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_2],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_2",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_2],
			data:   make([]byte, 2),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_2 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_2],
			data:   make([]byte, 3),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_3 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_3],
			data:   make([]byte, 2),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_3",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_3],
			data:   make([]byte, 3),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_3 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_3],
			data:   make([]byte, 4),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_4 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_4],
			data:   make([]byte, 3),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_4",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_4],
			data:   make([]byte, 4),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_4 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_4],
			data:   make([]byte, 5),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_5 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_5],
			data:   make([]byte, 4),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_5",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_5],
			data:   make([]byte, 5),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_5 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_5],
			data:   make([]byte, 6),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_6 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_6],
			data:   make([]byte, 5),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_6",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_6],
			data:   make([]byte, 6),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_6 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_6],
			data:   make([]byte, 7),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_7 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_7],
			data:   make([]byte, 6),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_7",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_7],
			data:   make([]byte, 7),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_7 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_7],
			data:   make([]byte, 8),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_8 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_8],
			data:   make([]byte, 7),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_8",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_8],
			data:   make([]byte, 8),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_8 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_8],
			data:   make([]byte, 9),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_9 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_9],
			data:   make([]byte, 8),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_9",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_9],
			data:   make([]byte, 9),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_9 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_9],
			data:   make([]byte, 10),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_10 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_10],
			data:   make([]byte, 9),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_10",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_10],
			data:   make([]byte, 10),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_10 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_10],
			data:   make([]byte, 11),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_11 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_11],
			data:   make([]byte, 10),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_11",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_11],
			data:   make([]byte, 11),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_11 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_11],
			data:   make([]byte, 12),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_12 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_12],
			data:   make([]byte, 11),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_12",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_12],
			data:   make([]byte, 12),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_12 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_12],
			data:   make([]byte, 13),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_13 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_13],
			data:   make([]byte, 12),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_13",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_13],
			data:   make([]byte, 13),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_13 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_13],
			data:   make([]byte, 14),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_14 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_14],
			data:   make([]byte, 13),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_14",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_14],
			data:   make([]byte, 14),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_14 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_14],
			data:   make([]byte, 15),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_15 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_15],
			data:   make([]byte, 14),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_15",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_15],
			data:   make([]byte, 15),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_15 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_15],
			data:   make([]byte, 16),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_16 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_16],
			data:   make([]byte, 15),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_16",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_16],
			data:   make([]byte, 16),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_16 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_16],
			data:   make([]byte, 17),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_17 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_17],
			data:   make([]byte, 16),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_17",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_17],
			data:   make([]byte, 17),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_17 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_17],
			data:   make([]byte, 18),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_18 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_18],
			data:   make([]byte, 17),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_18",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_18],
			data:   make([]byte, 18),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_18 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_18],
			data:   make([]byte, 19),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_19 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_19],
			data:   make([]byte, 18),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_19",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_19],
			data:   make([]byte, 19),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_19 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_19],
			data:   make([]byte, 20),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_20 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_20],
			data:   make([]byte, 19),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_20",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_20],
			data:   make([]byte, 20),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_20 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_20],
			data:   make([]byte, 21),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_21 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_21],
			data:   make([]byte, 20),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_21",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_21],
			data:   make([]byte, 21),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_21 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_21],
			data:   make([]byte, 22),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_22 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_22],
			data:   make([]byte, 21),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_22",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_22],
			data:   make([]byte, 22),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_22 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_22],
			data:   make([]byte, 23),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_23 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_23],
			data:   make([]byte, 22),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_23",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_23],
			data:   make([]byte, 23),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_23 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_23],
			data:   make([]byte, 24),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_24 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_24],
			data:   make([]byte, 23),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_24",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_24],
			data:   make([]byte, 24),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_24 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_24],
			data:   make([]byte, 25),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_25 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_25],
			data:   make([]byte, 24),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_25",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_25],
			data:   make([]byte, 25),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_25 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_25],
			data:   make([]byte, 26),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_26 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_26],
			data:   make([]byte, 25),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_26",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_26],
			data:   make([]byte, 26),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_26 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_26],
			data:   make([]byte, 27),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_27 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_27],
			data:   make([]byte, 26),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_27",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_27],
			data:   make([]byte, 27),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_27 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_27],
			data:   make([]byte, 28),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_28 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_28],
			data:   make([]byte, 27),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_28",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_28],
			data:   make([]byte, 28),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_28 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_28],
			data:   make([]byte, 29),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_29 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_29],
			data:   make([]byte, 28),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_29",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_29],
			data:   make([]byte, 29),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_29 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_29],
			data:   make([]byte, 30),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_30 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_30],
			data:   make([]byte, 29),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_30",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_30],
			data:   make([]byte, 30),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_30 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_30],
			data:   make([]byte, 31),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_31 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_31],
			data:   make([]byte, 30),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_31",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_31],
			data:   make([]byte, 31),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_31 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_31],
			data:   make([]byte, 32),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_32 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_32],
			data:   make([]byte, 31),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_32",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_32],
			data:   make([]byte, 32),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_32 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_32],
			data:   make([]byte, 33),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_33 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_33],
			data:   make([]byte, 32),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_33",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_33],
			data:   make([]byte, 33),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_33 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_33],
			data:   make([]byte, 34),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_34 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_34],
			data:   make([]byte, 33),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_34",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_34],
			data:   make([]byte, 34),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_34 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_34],
			data:   make([]byte, 35),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_35 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_35],
			data:   make([]byte, 34),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_35",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_35],
			data:   make([]byte, 35),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_35 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_35],
			data:   make([]byte, 36),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_36 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_36],
			data:   make([]byte, 35),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_36",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_36],
			data:   make([]byte, 36),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_36 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_36],
			data:   make([]byte, 37),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_37 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_37],
			data:   make([]byte, 36),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_37",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_37],
			data:   make([]byte, 37),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_37 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_37],
			data:   make([]byte, 38),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_38 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_38],
			data:   make([]byte, 37),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_38",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_38],
			data:   make([]byte, 38),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_38 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_38],
			data:   make([]byte, 39),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_39 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_39],
			data:   make([]byte, 38),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_39",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_39],
			data:   make([]byte, 39),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_39 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_39],
			data:   make([]byte, 40),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_40 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_40],
			data:   make([]byte, 39),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_40",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_40],
			data:   make([]byte, 40),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_40 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_40],
			data:   make([]byte, 41),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_41 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_41],
			data:   make([]byte, 40),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_41",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_41],
			data:   make([]byte, 41),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_41 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_41],
			data:   make([]byte, 42),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_42 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_42],
			data:   make([]byte, 41),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_42",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_42],
			data:   make([]byte, 42),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_42 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_42],
			data:   make([]byte, 43),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_43 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_43],
			data:   make([]byte, 42),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_43",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_43],
			data:   make([]byte, 43),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_43 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_43],
			data:   make([]byte, 44),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_44 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_44],
			data:   make([]byte, 43),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_44",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_44],
			data:   make([]byte, 44),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_44 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_44],
			data:   make([]byte, 45),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_45 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_45],
			data:   make([]byte, 44),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_45",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_45],
			data:   make([]byte, 45),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_45 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_45],
			data:   make([]byte, 46),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_46 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_46],
			data:   make([]byte, 45),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_46",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_46],
			data:   make([]byte, 46),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_46 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_46],
			data:   make([]byte, 47),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_47 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_47],
			data:   make([]byte, 46),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_47",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_47],
			data:   make([]byte, 47),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_47 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_47],
			data:   make([]byte, 48),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_48 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_48],
			data:   make([]byte, 47),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_48",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_48],
			data:   make([]byte, 48),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_48 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_48],
			data:   make([]byte, 49),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_49 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_49],
			data:   make([]byte, 48),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_49",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_49],
			data:   make([]byte, 49),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_49 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_49],
			data:   make([]byte, 50),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_50 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_50],
			data:   make([]byte, 49),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_50",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_50],
			data:   make([]byte, 50),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_50 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_50],
			data:   make([]byte, 51),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_51 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_51],
			data:   make([]byte, 50),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_51",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_51],
			data:   make([]byte, 51),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_51 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_51],
			data:   make([]byte, 52),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_52 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_52],
			data:   make([]byte, 51),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_52",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_52],
			data:   make([]byte, 52),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_52 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_52],
			data:   make([]byte, 53),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_53 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_53],
			data:   make([]byte, 52),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_53",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_53],
			data:   make([]byte, 53),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_53 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_53],
			data:   make([]byte, 54),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_54 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_54],
			data:   make([]byte, 53),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_54",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_54],
			data:   make([]byte, 54),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_54 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_54],
			data:   make([]byte, 55),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_55 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_55],
			data:   make([]byte, 54),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_55",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_55],
			data:   make([]byte, 55),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_55 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_55],
			data:   make([]byte, 56),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_56 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_56],
			data:   make([]byte, 55),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_56",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_56],
			data:   make([]byte, 56),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_56 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_56],
			data:   make([]byte, 57),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_57 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_57],
			data:   make([]byte, 56),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_57",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_57],
			data:   make([]byte, 57),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_57 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_57],
			data:   make([]byte, 58),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_58 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_58],
			data:   make([]byte, 57),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_58",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_58],
			data:   make([]byte, 58),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_58 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_58],
			data:   make([]byte, 59),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_59 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_59],
			data:   make([]byte, 58),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_59",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_59],
			data:   make([]byte, 59),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_59 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_59],
			data:   make([]byte, 60),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_60 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_60],
			data:   make([]byte, 59),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_60",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_60],
			data:   make([]byte, 60),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_60 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_60],
			data:   make([]byte, 61),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_61 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_61],
			data:   make([]byte, 60),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_61",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_61],
			data:   make([]byte, 61),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_61 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_61],
			data:   make([]byte, 62),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_62 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_62],
			data:   make([]byte, 61),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_62",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_62],
			data:   make([]byte, 62),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_62 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_62],
			data:   make([]byte, 63),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_63 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_63],
			data:   make([]byte, 62),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_63",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_63],
			data:   make([]byte, 63),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_63 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_63],
			data:   make([]byte, 64),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_64 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_64],
			data:   make([]byte, 63),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_64",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_64],
			data:   make([]byte, 64),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_64 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_64],
			data:   make([]byte, 65),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_65 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_65],
			data:   make([]byte, 64),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_65",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_65],
			data:   make([]byte, 65),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_65 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_65],
			data:   make([]byte, 66),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_66 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_66],
			data:   make([]byte, 65),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_66",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_66],
			data:   make([]byte, 66),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_66 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_66],
			data:   make([]byte, 67),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_67 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_67],
			data:   make([]byte, 66),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_67",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_67],
			data:   make([]byte, 67),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_67 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_67],
			data:   make([]byte, 68),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_68 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_68],
			data:   make([]byte, 67),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_68",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_68],
			data:   make([]byte, 68),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_68 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_68],
			data:   make([]byte, 69),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_69 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_69],
			data:   make([]byte, 68),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_69",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_69],
			data:   make([]byte, 69),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_69 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_69],
			data:   make([]byte, 70),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_70 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_70],
			data:   make([]byte, 69),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_70",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_70],
			data:   make([]byte, 70),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_70 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_70],
			data:   make([]byte, 71),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_71 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_71],
			data:   make([]byte, 70),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_71",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_71],
			data:   make([]byte, 71),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_71 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_71],
			data:   make([]byte, 72),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_72 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_72],
			data:   make([]byte, 71),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_72",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_72],
			data:   make([]byte, 72),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_72 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_72],
			data:   make([]byte, 73),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_73 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_73],
			data:   make([]byte, 72),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_73",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_73],
			data:   make([]byte, 73),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_73 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_73],
			data:   make([]byte, 74),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_74 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_74],
			data:   make([]byte, 73),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_74",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_74],
			data:   make([]byte, 74),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_74 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_74],
			data:   make([]byte, 75),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_75 short",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_75],
			data:   make([]byte, 74),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DATA_75",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_75],
			data:   make([]byte, 75),
		},
		expectedErr: nil,
	},
	{
		name: "OP_DATA_75 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DATA_75],
			data:   make([]byte, 76),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_PUSHDATA1",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PUSHDATA1],
			data:   []byte{0, 1, 2, 3, 4},
		},
		expectedErr: nil,
	},
	{
		name: "OP_PUSHDATA2",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PUSHDATA2],
			data:   []byte{0, 1, 2, 3, 4},
		},
		expectedErr: nil,
	},
	{
		name: "OP_PUSHDATA4",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PUSHDATA1],
			data:   []byte{0, 1, 2, 3, 4},
		},
		expectedErr: nil,
	},
	{
		name: "OP_1NEGATE",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_1NEGATE],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_1NEGATE long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_1NEGATE],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_RESERVED",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RESERVED],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_RESERVED long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RESERVED],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_TRUE",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_TRUE],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_TRUE long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_TRUE],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_3",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_3],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_3 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_3],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_4",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_4],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_4 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_4],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_5",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_5],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_5 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_5],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_6",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_6],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_6 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_6],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_7",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_7],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_7 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_7],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_8",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_8],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_8 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_8],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_9",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_9],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_9 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_9],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_10",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_10],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_10 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_10],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_11",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_11],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_11 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_11],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_12",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_12],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_12 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_12],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_13",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_13],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_13 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_13],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_14",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_14],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_14 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_14],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_15",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_15],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_15 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_15],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_16",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_16],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_16 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_16],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_VER",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_VER],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_VER long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_VER],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_IF",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_IF],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_IF long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_IF],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOTIF",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOTIF],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOTIF long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOTIF],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_VERIF",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_VERIF],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_VERIF long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_VERIF],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_VERNOTIF",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_VERNOTIF],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_VERNOTIF long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_VERNOTIF],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_ELSE",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ELSE],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_ELSE long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ELSE],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_ENDIF",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ENDIF],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_ENDIF long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ENDIF],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_VERIFY",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_VERIFY],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_VERIFY long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_VERIFY],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_RETURN",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RETURN],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_RETURN long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RETURN],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_TOALTSTACK",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_TOALTSTACK],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_TOALTSTACK long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_TOALTSTACK],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_FROMALTSTACK",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_FROMALTSTACK],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_FROMALTSTACK long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_FROMALTSTACK],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2DROP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2DROP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2DROP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2DROP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2DUP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2DUP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2DUP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2DUP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_3DUP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_3DUP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_3DUP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_3DUP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2OVER",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2OVER],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2OVER long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2OVER],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2ROT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2ROT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2ROT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2ROT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2SWAP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2SWAP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2SWAP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2SWAP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_IFDUP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_IFDUP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_IFDUP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_IFDUP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DEPTH",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DEPTH],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_DEPTH long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DEPTH],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DROP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DROP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_DROP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DROP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DUP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DUP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_DUP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DUP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NIP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NIP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NIP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NIP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_OVER",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_OVER],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_OVER long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_OVER],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_PICK",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PICK],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_PICK long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PICK],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_ROLL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ROLL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_ROLL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ROLL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_ROT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ROT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_ROT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ROT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_SWAP",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SWAP],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_SWAP long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SWAP],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_TUCK",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_TUCK],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_TUCK long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_TUCK],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_CAT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CAT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_CAT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CAT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_SUBSTR",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SUBSTR],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_SUBSTR long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SUBSTR],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_LEFT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LEFT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_LEFT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LEFT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_LEFT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LEFT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_LEFT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LEFT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_RIGHT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RIGHT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_RIGHT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RIGHT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_SIZE",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SIZE],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_SIZE long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SIZE],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_INVERT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_INVERT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_INVERT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_INVERT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_AND",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_AND],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_AND long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_AND],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_OR",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_OR],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_OR long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_OR],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_XOR",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_XOR],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_XOR long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_XOR],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_EQUAL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_EQUAL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_EQUAL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_EQUAL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_EQUALVERIFY",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_EQUALVERIFY],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_EQUALVERIFY long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_EQUALVERIFY],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_RESERVED1",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RESERVED1],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_RESERVED1 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RESERVED1],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_RESERVED2",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RESERVED2],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_RESERVED2 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RESERVED2],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_1ADD",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_1ADD],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_1ADD long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_1ADD],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_1SUB",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_1SUB],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_1SUB long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_1SUB],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2MUL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2MUL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2MUL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2MUL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_2DIV",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2DIV],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_2DIV long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_2DIV],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NEGATE",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NEGATE],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NEGATE long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NEGATE],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_ABS",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ABS],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_ABS long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ABS],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_0NOTEQUAL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_0NOTEQUAL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_0NOTEQUAL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_0NOTEQUAL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_ADD",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ADD],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_ADD long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_ADD],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_SUB",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SUB],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_SUB long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SUB],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_MUL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_MUL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_MUL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_MUL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_DIV",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DIV],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_DIV long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_DIV],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_MOD",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_MOD],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_MOD long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_MOD],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_LSHIFT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LSHIFT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_LSHIFT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LSHIFT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_RSHIFT",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RSHIFT],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_RSHIFT long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RSHIFT],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_BOOLAND",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_BOOLAND],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_BOOLAND long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_BOOLAND],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_BOOLOR",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_BOOLOR],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_BOOLOR long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_BOOLOR],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NUMEQUAL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NUMEQUAL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NUMEQUAL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NUMEQUAL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NUMEQUALVERIFY",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NUMEQUALVERIFY],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NUMEQUALVERIFY long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NUMEQUALVERIFY],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NUMNOTEQUAL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NUMNOTEQUAL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NUMNOTEQUAL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NUMNOTEQUAL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_LESSTHAN",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LESSTHAN],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_LESSTHAN long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LESSTHAN],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_GREATERTHAN",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_GREATERTHAN],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_GREATERTHAN long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_GREATERTHAN],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_LESSTHANOREQUAL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LESSTHANOREQUAL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_LESSTHANOREQUAL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_LESSTHANOREQUAL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_GREATERTHANOREQUAL",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_GREATERTHANOREQUAL],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_GREATERTHANOREQUAL long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_GREATERTHANOREQUAL],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_MIN",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_MIN],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_MIN long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_MIN],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_MAX",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_MAX],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_MAX long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_MAX],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_WITHIN",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_WITHIN],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_WITHIN long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_WITHIN],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_RIPEMD160",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RIPEMD160],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_RIPEMD160 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_RIPEMD160],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_SHA1",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SHA1],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_SHA1 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SHA1],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_SHA256",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SHA256],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_SHA256 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_SHA256],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_HASH160",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_HASH160],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_HASH160 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_HASH160],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_HASH256",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_HASH256],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_HASH256 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_HASH256],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_CODESAPERATOR",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CODESEPARATOR],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_CODESEPARATOR long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CODESEPARATOR],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_CHECKSIG",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CHECKSIG],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_CHECKSIG long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CHECKSIG],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_CHECKSIGVERIFY",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CHECKSIGVERIFY],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_CHECKSIGVERIFY long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CHECKSIGVERIFY],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_CHECKMULTISIG",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CHECKMULTISIG],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_CHECKMULTISIG long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CHECKMULTISIG],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_CHECKMULTISIGVERIFY",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CHECKMULTISIGVERIFY],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_CHECKMULTISIGVERIFY long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_CHECKMULTISIGVERIFY],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP1",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP1],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP1 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP1],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP2",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP2],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP2 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP2],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP3",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP3],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP3 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP3],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP4",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP4],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP4 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP4],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP5",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP5],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP5 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP5],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP6",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP6],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP6 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP6],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP7",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP7],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP7 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP7],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP8",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP8],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP8 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP8],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP9",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP9],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP9 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP9],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_NOP10",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP10],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_NOP10 long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_NOP10],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_PUBKEYHASH",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PUBKEYHASH],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_PUBKEYHASH long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PUBKEYHASH],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_PUBKEY",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PUBKEY],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_PUBKEY long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_PUBKEY],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
	{
		name: "OP_INVALIDOPCODE",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_INVALIDOPCODE],
			data:   nil,
		},
		expectedErr: nil,
	},
	{
		name: "OP_INVALIDOPCODE long",
		pop: &parsedOpcode{
			opcode: opcodemapPreinit[OP_INVALIDOPCODE],
			data:   make([]byte, 1),
		},
		expectedErr: StackErrInvalidOpcode,
	},
}

func TestUnparsingInvalidOpcodes(t *testing.T) {
	for _, test := range popTests {
		_, err := test.pop.bytes()
		if err != test.expectedErr {
			t.Errorf("Parsed Opcode test '%s' failed", test.name)
			t.Error(err, test.expectedErr)
		}
	}
}

// parse hex string into a []byte.
func parseHex(tok string) ([]byte, error) {
	if !strings.HasPrefix(tok, "0x") {
		return nil, errors.New("not a hex number")
	}
	return hex.DecodeString(tok[2:])
}

// ParseShortForm parses a string as as used in the bitcoind reference tests
// into the script it came from.
func ParseShortForm(script string) ([]byte, error) {
	ops := make(map[string]*opcode)

	// the format used for these tests is pretty simple if ad-hoc:
	// -  opcodes other than the push opcodes and unknown are present as
	// either OP_NAME or just NAME
	// - plain numbers are made into push operations
	// -  numbers beginning with 0x are inserted into the []byte as-is (so
	// 0x14 is OP_DATA_20)
	// - single quoted strings are pushed as data.
	// - anything else is an error.
	for _, op := range opcodemap {
		if op.value < OP_NOP && op.value != OP_RESERVED {
			continue
		}
		if strings.Contains(op.name, "OP_UNKNOWN") {
			continue
		}
		ops[op.name] = op
		ops[strings.TrimPrefix(op.name, "OP_")] = op
	}
	// do once, build map.

	// Split only does one separator so convert all \n and tab into  space.
	script = strings.Replace(script, "\n", " ", -1)
	script = strings.Replace(script, "\t", " ", -1)
	tokens := strings.Split(script, " ")
	builder := NewScriptBuilder()

	for _, tok := range tokens {
		if len(tok) == 0 {
			continue
		}
		// if parses as a plain number
		if num, err := strconv.ParseInt(tok, 10, 64); err == nil {
			builder.AddInt64(num)
			continue
		} else if bts, err := parseHex(tok); err == nil {
			// naughty...
			builder.script = append(builder.script, bts...)
		} else if len(tok) >= 2 &&
			tok[0] == '\'' && tok[len(tok)-1] == '\'' {
			builder.AddData([]byte(tok[1 : len(tok)-1]))
		} else if opcode, ok := ops[tok]; ok {
			builder.AddOp(opcode.value)
		} else {
			return nil, fmt.Errorf("bad token \"%s\"", tok)
		}

	}
	return builder.Script(), nil
}

func TestBitcoindInvalidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/script_invalid.json")
	if err != nil {
		t.Errorf("TestBitcoindInvalidTests: %v\n", err)
		return
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestBitcoindInvalidTests couldn't Unmarshal: %v\n",
			err)
		return
	}
	tx := btcwire.NewMsgTx()
	for x, test := range tests {
		if len(test) < 2 && len(test) > 3 {
			t.Errorf("TestBitcoindInvalidTests: invalid test #%d\n",
				x)
			continue
		}
		name := ""
		if len(test) == 3 {
			name = fmt.Sprintf("test (%s)", test[2])
		} else {
			name = fmt.Sprintf("test ([%s, %s])", test[0], test[1])
		}

		scriptSig, err := ParseShortForm(test[0])
		if err != nil {
			t.Errorf("%s: can't parse scriptSig; %v", name, err)
			continue
		}

		scriptPubKey, err := ParseShortForm(test[1])
		if err != nil {
			t.Errorf("%s: can't parse scriptPubkey; %v", name, err)
			continue
		}

		s, err := NewScript(scriptSig, scriptPubKey, 0, tx, ScriptBip16)
		if err == nil {
			if err := s.Execute(); err == nil {
				t.Errorf("%s test succeeded when it "+
					"should have failed\n", name)
			}
			continue
		}
	}
}

func TestBitcoindValidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/script_valid.json")
	if err != nil {
		t.Errorf("TestBitcoinValidTests: %v\n", err)
		return
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestBitcoindInvalidTests couldn't Unmarshal: %v\n",
			err)
		return
	}
	tx := btcwire.NewMsgTx()
	for x, test := range tests {
		if len(test) < 2 && len(test) > 3 {
			t.Errorf("TestBitcoindInvalidTests: invalid test #%d\n",
				x)
			continue
		}
		name := ""
		if len(test) == 3 {
			name = fmt.Sprintf("test (%s)", test[2])
		} else {
			name = fmt.Sprintf("test ([%s, %s])", test[0], test[1])
		}

		scriptSig, err := ParseShortForm(test[0])
		if err != nil {
			t.Errorf("%s: can't parse scriptSig; %v", name, err)
			continue
		}

		scriptPubKey, err := ParseShortForm(test[1])
		if err != nil {
			t.Errorf("%s: can't parse scriptPubkey; %v", name, err)
			continue
		}

		s, err := NewScript(scriptSig, scriptPubKey, 0, tx, ScriptBip16)
		if err != nil {
			t.Errorf("%s failed to create script: %v", name, err)
			continue
		}

		err = s.Execute()
		if err != nil {
			t.Errorf("%s failed to execute: %v", name, err)
			continue
		}
	}
}

func TestBitcoindTxValidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/tx_valid.json")
	if err != nil {
		t.Errorf("TestBitcoindInvalidTests: %v\n", err)
		return
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestBitcoindInvalidTests couldn't Unmarshal: %v\n",
			err)
		return
	}

	// for ma is eaitehr a ["this is a comment "]
	// or [[[previous hash, previous index, previous scripbPubKey]...,]
	//	serializedTransaction, verifyFlags]
testloop:
	for i, test := range tests {
		inputs, ok := test[0].([]interface{})
		if !ok {
			continue
		}

		if len(test) != 3 {
			t.Errorf("bad test (bad lenggh) %d: %v", i, test)
			continue

		}
		serializedhex, ok := test[1].(string)
		if !ok {
			t.Errorf("bad test (arg 2 not string) %d: %v", i, test)
			continue
		}
		serializedTx, err := hex.DecodeString(serializedhex)
		if err != nil {
			t.Errorf("bad test (arg 2 not hex %v) %d: %v", err, i,
				test)
			continue
		}

		tx, err := btcutil.NewTxFromBytes(serializedTx)
		if err != nil {
			t.Errorf("bad test (arg 2 not msgtx %v) %d: %v", err,
				i, test)
			continue
		}

		verifyFlags, ok := test[2].(string)
		if !ok {
			t.Errorf("bad test (arg 3 not string) %d: %v", i, test)
			continue
		}

		var flags ScriptFlags
		vFlags := strings.Split(verifyFlags, ",")
		for _, flag := range vFlags {
			switch flag {
			case "P2SH":
				flags |= ScriptBip16
			case "NULLDUMMY":
				flags |= ScriptStrictMultiSig
			}
		}

		prevOuts := make(map[btcwire.OutPoint][]byte)
		for j, iinput := range inputs {
			input, ok := iinput.([]interface{})
			if !ok {
				t.Errorf("bad test (%dth input not array)"+
					"%d: %v", j, i, test)
				continue
			}

			if len(input) != 3 {
				t.Errorf("bad test (%dth input wrong length)"+
					"%d: %v", j, i, test)
				continue
			}

			previoustx, ok := input[0].(string)
			if !ok {
				t.Errorf("bad test (%dth input sha not string)"+
					"%d: %v", j, i, test)
				continue
			}

			prevhash, err := btcwire.NewShaHashFromStr(previoustx)
			if err != nil {
				t.Errorf("bad test (%dth input sha not sha %v)"+
					"%d: %v", j, err, i, test)
				continue
			}

			idxf, ok := input[1].(float64)
			if !ok {
				t.Errorf("bad test (%dth input idx not number)"+
					"%d: %v", j, i, test)
				continue
			}

			idx := uint32(idxf) // (floor(idxf) == idxf?)

			oscript, ok := input[2].(string)
			if !ok {
				t.Errorf("bad test (%dth input script not "+
					"string) %d: %v", j, i, test)
				continue
			}

			script, err := ParseShortForm(oscript)
			if err != nil {
				t.Errorf("bad test (%dth input script doesn't "+
					"parse %v) %d: %v", j, err, i, test)
				continue
			}

			prevOuts[*btcwire.NewOutPoint(prevhash, idx)] = script
		}

		for k, txin := range tx.MsgTx().TxIn {
			pkScript, ok := prevOuts[txin.PreviousOutPoint]
			if !ok {
				t.Errorf("bad test (missing %dth input) %d:%v",
					k, i, test)
				continue testloop
			}
			s, err := NewScript(txin.SignatureScript, pkScript, k,
				tx.MsgTx(), flags)
			if err != nil {
				t.Errorf("test (%d:%v:%d) failed to create "+
					"script: %v", i, test, k, err)
				continue
			}

			err = s.Execute()
			if err != nil {
				t.Errorf("test (%d:%v:%d) failed to execute: "+
					"%v", i, test, k, err)
				continue
			}
		}
	}
}

func TestBitcoindTxInvalidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/tx_invalid.json")
	if err != nil {
		t.Errorf("TestBitcoindInvalidTests: %v\n", err)
		return
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestBitcoindInvalidTests couldn't Unmarshal: %v\n",
			err)
		return
	}

	// for ma is eaitehr a ["this is a comment "]
	// or [[[previous hash, previous index, previous scripbPubKey]...,]
	//	serializedTransaction, verifyFlags]
testloop:
	for i, test := range tests {
		inputs, ok := test[0].([]interface{})
		if !ok {
			continue
		}

		if len(test) != 3 {
			t.Errorf("bad test (bad lenggh) %d: %v", i, test)
			continue

		}
		serializedhex, ok := test[1].(string)
		if !ok {
			t.Errorf("bad test (arg 2 not string) %d: %v", i, test)
			continue
		}
		serializedTx, err := hex.DecodeString(serializedhex)
		if err != nil {
			t.Errorf("bad test (arg 2 not hex %v) %d: %v", err, i,
				test)
			continue
		}

		tx, err := btcutil.NewTxFromBytes(serializedTx)
		if err != nil {
			t.Errorf("bad test (arg 2 not msgtx %v) %d: %v", err,
				i, test)
			continue
		}

		verifyFlags, ok := test[2].(string)
		if !ok {
			t.Errorf("bad test (arg 3 not string) %d: %v", i, test)
			continue
		}

		var flags ScriptFlags
		vFlags := strings.Split(verifyFlags, ",")
		for _, flag := range vFlags {
			switch flag {
			case "P2SH":
				flags |= ScriptBip16
			case "NULLDUMMY":
				flags |= ScriptStrictMultiSig
			}
		}

		prevOuts := make(map[btcwire.OutPoint][]byte)
		for j, iinput := range inputs {
			input, ok := iinput.([]interface{})
			if !ok {
				t.Errorf("bad test (%dth input not array)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			if len(input) != 3 {
				t.Errorf("bad test (%dth input wrong length)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			previoustx, ok := input[0].(string)
			if !ok {
				t.Errorf("bad test (%dth input sha not string)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			prevhash, err := btcwire.NewShaHashFromStr(previoustx)
			if err != nil {
				t.Errorf("bad test (%dth input sha not sha %v)"+
					"%d: %v", j, err, i, test)
				continue testloop
			}

			idxf, ok := input[1].(float64)
			if !ok {
				t.Errorf("bad test (%dth input idx not number)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			idx := uint32(idxf) // (floor(idxf) == idxf?)

			oscript, ok := input[2].(string)
			if !ok {
				t.Errorf("bad test (%dth input script not "+
					"string) %d: %v", j, i, test)
				continue testloop
			}

			script, err := ParseShortForm(oscript)
			if err != nil {
				t.Errorf("bad test (%dth input script doesn't "+
					"parse %v) %d: %v", j, err, i, test)
				continue testloop
			}

			prevOuts[*btcwire.NewOutPoint(prevhash, idx)] = script
		}

		for k, txin := range tx.MsgTx().TxIn {
			pkScript, ok := prevOuts[txin.PreviousOutPoint]
			if !ok {
				t.Errorf("bad test (missing %dth input) %d:%v",
					k, i, test)
				continue testloop
			}
			// These are meant to fail, so as soon as the first
			// input fails the transaction has failed. (some of the
			// test txns have good inputs, too..
			s, err := NewScript(txin.SignatureScript, pkScript, k,
				tx.MsgTx(), flags)
			if err != nil {
				continue testloop
			}

			err = s.Execute()
			if err != nil {
				continue testloop
			}

		}
		t.Errorf("test (%d:%v) succeeded when should fail",
			i, test)
	}
}
