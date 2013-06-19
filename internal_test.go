// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

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
