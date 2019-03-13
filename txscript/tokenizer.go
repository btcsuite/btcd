// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"encoding/binary"
	"fmt"
)

// opcodeArrayRef is used to break initialization cycles.
var opcodeArrayRef *[256]opcode

func init() {
	opcodeArrayRef = &opcodeArray
}

// ScriptTokenizer provides a facility for easily and efficiently tokenizing
// transaction scripts without creating allocations.  Each successive opcode is
// parsed with the Next function, which returns false when iteration is
// complete, either due to successfully tokenizing the entire script or
// encountering a parse error.  In the case of failure, the Err function may be
// used to obtain the specific parse error.
//
// Upon successfully parsing an opcode, the opcode and data associated with it
// may be obtained via the Opcode and Data functions, respectively.
//
// The ByteIndex function may be used to obtain the tokenizer's current offset
// into the raw script.
type ScriptTokenizer struct {
	script  []byte
	version uint16
	offset  int32
	op      *opcode
	data    []byte
	err     error
}

// Done returns true when either all opcodes have been exhausted or a parse
// failure was encountered and therefore the state has an associated error.
func (t *ScriptTokenizer) Done() bool {
	return t.err != nil || t.offset >= int32(len(t.script))
}

// Next attempts to parse the next opcode and returns whether or not it was
// successful.  It will not be successful if invoked when already at the end of
// the script, a parse failure is encountered, or an associated error already
// exists due to a previous parse failure.
//
// In the case of a true return, the parsed opcode and data can be obtained with
// the associated functions and the offset into the script will either point to
// the next opcode or the end of the script if the final opcode was parsed.
//
// In the case of a false return, the parsed opcode and data will be the last
// successfully parsed values (if any) and the offset into the script will
// either point to the failing opcode or the end of the script if the function
// was invoked when already at the end of the script.
//
// Invoking this function when already at the end of the script is not
// considered an error and will simply return false.
func (t *ScriptTokenizer) Next() bool {
	if t.Done() {
		return false
	}

	op := &opcodeArrayRef[t.script[t.offset]]
	switch {
	// No additional data.  Note that some of the opcodes, notably OP_1NEGATE,
	// OP_0, and OP_[1-16] represent the data themselves.
	case op.length == 1:
		t.offset++
		t.op = op
		t.data = nil
		return true

	// Data pushes of specific lengths -- OP_DATA_[1-75].
	case op.length > 1:
		script := t.script[t.offset:]
		if len(script) < op.length {
			str := fmt.Sprintf("opcode %s requires %d bytes, but script only "+
				"has %d remaining", op.name, op.length, len(script))
			t.err = scriptError(ErrMalformedPush, str)
			return false
		}

		// Move the offset forward and set the opcode and data accordingly.
		t.offset += int32(op.length)
		t.op = op
		t.data = script[1:op.length]
		return true

	// Data pushes with parsed lengths -- OP_PUSHDATA{1,2,4}.
	case op.length < 0:
		script := t.script[t.offset+1:]
		if len(script) < -op.length {
			str := fmt.Sprintf("opcode %s requires %d bytes, but script only "+
				"has %d remaining", op.name, -op.length, len(script))
			t.err = scriptError(ErrMalformedPush, str)
			return false
		}

		// Next -length bytes are little endian length of data.
		var dataLen int32
		switch op.length {
		case -1:
			dataLen = int32(script[0])
		case -2:
			dataLen = int32(binary.LittleEndian.Uint16(script[:2]))
		case -4:
			dataLen = int32(binary.LittleEndian.Uint32(script[:4]))
		default:
			str := fmt.Sprintf("invalid opcode length %d", op.length)
			t.err = scriptError(ErrMalformedPush, str)
			return false
		}

		// Move to the beginning of the data.
		script = script[-op.length:]

		// Disallow entries that do not fit script or were sign extended.
		if dataLen > int32(len(script)) || dataLen < 0 {
			str := fmt.Sprintf("opcode %s pushes %d bytes, but script only "+
				"has %d remaining", op.name, dataLen, len(script))
			t.err = scriptError(ErrMalformedPush, str)
			return false
		}

		// Move the offset forward and set the opcode and data accordingly.
		t.offset += 1 + int32(-op.length) + dataLen
		t.op = op
		t.data = script[:dataLen]
		return true
	}

	// The only remaining case is an opcode with length zero which is
	// impossible.
	panic("unreachable")
}

// Script returns the full script associated with the tokenizer.
func (t *ScriptTokenizer) Script() []byte {
	return t.script
}

// ByteIndex returns the current offset into the full script that will be parsed
// next and therefore also implies everything before it has already been parsed.
func (t *ScriptTokenizer) ByteIndex() int32 {
	return t.offset
}

// Opcode returns the current opcode associated with the tokenizer.
func (t *ScriptTokenizer) Opcode() byte {
	return t.op.value
}

// Data returns the data associated with the most recently successfully parsed
// opcode.
func (t *ScriptTokenizer) Data() []byte {
	return t.data
}

// Err returns any errors currently associated with the tokenizer.  This will
// only be non-nil in the case a parsing error was encountered.
func (t *ScriptTokenizer) Err() error {
	return t.err
}

// MakeScriptTokenizer returns a new instance of a script tokenizer.  Passing
// an unsupported script version will result in the returned tokenizer
// immediately having an err set accordingly.
//
// See the docs for ScriptTokenizer for more details.
func MakeScriptTokenizer(scriptVersion uint16, script []byte) ScriptTokenizer {
	// Only version 0 scripts are currently supported.
	var err error
	if scriptVersion != 0 {
		str := fmt.Sprintf("script version %d is not supported", scriptVersion)
		err = scriptError(ErrUnsupportedScriptVersion, str)

	}
	return ScriptTokenizer{version: scriptVersion, script: script, err: err}
}
