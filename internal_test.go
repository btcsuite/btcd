// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

import (
	"testing"
)

// this file is present to export some internal interfaces so that we can
// test them reliably.

func TstRemoveOpcode(pkscript []byte, opcode byte) ([]byte, error) {
	pops, err := parseScript(pkscript)
	if err != nil {
		return nil, err
	}
	pops = removeOpcode(pops, opcode)
	return unparseScript(pops), nil
}

func TstRemoveOpcodeByData(pkscript []byte, data []byte) ([]byte, error) {
	pops, err := parseScript(pkscript)
	if err != nil {
		return nil, err
	}
	pops = removeOpcodeByData(pops, data)
	return unparseScript(pops), nil
}

type TstScriptType scriptType

const (
	TstPubKeyTy      TstScriptType = TstScriptType(pubKeyTy)
	TstPubKeyHashTy                = TstScriptType(pubKeyHashTy)
	TstScriptHashTy                = TstScriptType(scriptHashTy)
	TstMultiSigTy                  = TstScriptType(multiSigTy)
	TstNonStandardTy               = TstScriptType(nonStandardTy)
)

func TstTypeOfScript(script []byte) TstScriptType {
	pops, err := parseScript(script)
	if err != nil {
		return TstNonStandardTy
	}
	return TstScriptType(typeOfScript(pops))
}

// TestSetPC allows the test modules to set the program counter to whatever they
// want.
func (s *Script) TstSetPC(script, off int) {
	s.scriptidx = script
	s.scriptoff = off
}

// Tests for internal error cases in ScriptToAddress.
// We pass bad format definitions to  ScriptToAddrss to make sure the internal
// checks work correctly. This is located in internal_test.go and not address.go
// because of the ridiculous amount of internal types/constants that would
// otherwise need to be exported here.

type pkformatTest struct {
	name   string
	format pkformat
	script []byte
	ty     ScriptType
	err    error
}

var TstPkFormats = []pkformatTest{
	pkformatTest{
		name: "bad offset",
		format: pkformat{
			addrtype:  ScriptAddr,
			parsetype: scrNoAddr,
			length:    4,
			databytes: []pkbytes{{0, OP_1}, {1, OP_2}, {2,
				OP_3}, /* wrong - too long */ {9, OP_4}},
			allowmore: true,
		},
		script: []byte{OP_1, OP_2, OP_3, OP_4},
		err:    StackErrInvalidAddrOffset,
	},
	pkformatTest{
		name: "Bad parsetype",
		format: pkformat{
			addrtype:  ScriptAddr,
			parsetype: 8, // invalid type
			length:    4,
			databytes: []pkbytes{{0, OP_1}, {1, OP_2}, {2,
				OP_3}, /* wrong - too long */ {3, OP_4}},
			allowmore: true,
		},
		script: []byte{OP_1, OP_2, OP_3, OP_4},
		err:    StackErrInvalidParseType,
	},
}

func TestBadPkFormat(t *testing.T) {
	for _, test := range TstPkFormats {
		ty, addr, err := scriptToAddressTemplate(test.script,
			[]pkformat{test.format})
		if err != nil {
			if err != test.err {
				t.Errorf("%s got error \"%v\". Was expecrting "+
					"\"%v\"", test.name, err, test.err)
			}
			continue
		}
		if ty != test.ty {
			t.Errorf("%s: unexpected type \"%s\". Wanted \"%s\" (addr %v)",
				test.name, ty, test.ty, addr)
			continue
		}
	}

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
}
