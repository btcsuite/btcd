// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

import (
	"encoding/binary"
	"math/big"
)

const (
	// defaultScriptAlloc is the default size used for the backing array
	// for a script being built by the ScriptBuilder.  The array will
	// dynamically grow as needed, but this figure is intended to provide
	// enough space for vast majority of scripts without needing to grow the
	// backing array multiple times.
	defaultScriptAlloc = 500
)

// ScriptBuilder provides a facility for building custom scripts.  It allows
// you to push opcodes, ints, and data while respecting canonical encoding.  It
// does not ensure the script will execute correctly.
//
// For example, the following would build a 2-of-3 multisig script for usage in
// a pay-to-script-hash (although in this situation MultiSigScript() would be a
// better choice to generate the script):
// 	builder := btcscript.NewScriptBuilder()
// 	builder.AddOp(btcscript.OP_2).AddData(pubKey1).AddData(pubKey2)
// 	builder.AddData(pubKey3).AddOp(btcscript.OP_3)
// 	builder.AddOp(btcscript.OP_CHECKMULTISIG)
// 	fmt.Printf("Final multi-sig script: %x\n", builder.Script())
type ScriptBuilder struct {
	script []byte
}

// AddOp pushes the passed opcode to the end of the script.
func (b *ScriptBuilder) AddOp(opcode byte) *ScriptBuilder {
	b.script = append(b.script, opcode)
	return b
}

// AddData pushes the passed data to the end of the script.  It automatically
// chooses canonical opcodes depending on the length of the data. A zero length
// buffer will lead to a push of empty data onto the stack.
func (b *ScriptBuilder) AddData(data []byte) *ScriptBuilder {
	dataLen := len(data)

	// When the data consists of a single number that can be represented
	// by one of the "small integer" opcodes, use that opcode instead of
	// a data push opcode followed by the number.
	if dataLen == 0 || dataLen == 1 && data[0] == 0 {
		b.script = append(b.script, OP_0)
		return b
	} else if dataLen == 1 && data[0] <= 16 {
		b.script = append(b.script, byte((OP_1-1)+data[0]))
		return b
	}

	// Use one of the OP_DATA_# opcodes if the length of the data is small
	// enough so the data push instruction is only a single byte.
	// Otherwise, choose the smallest possible OP_PUSHDATA# opcode that
	// can represent the length of the data.
	if dataLen < OP_PUSHDATA1 {
		b.script = append(b.script, byte((OP_DATA_1-1)+dataLen))
	} else if dataLen <= 0xff {
		b.script = append(b.script, OP_PUSHDATA1, byte(dataLen))
	} else if dataLen <= 0xffff {
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(dataLen))
		b.script = append(b.script, OP_PUSHDATA2)
		b.script = append(b.script, buf...)
	} else {
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(dataLen))
		b.script = append(b.script, OP_PUSHDATA4)
		b.script = append(b.script, buf...)
	}

	// Append the actual data.
	b.script = append(b.script, data...)

	return b
}

// AddInt64 pushes the passed integer to the end of the script.
func (b *ScriptBuilder) AddInt64(val int64) *ScriptBuilder {
	// Fast path for small integers and OP_1NEGATE.
	if val == 0 {
		b.script = append(b.script, OP_0)
		return b
	}
	if val == -1 || (val >= 1 && val <= 16) {
		b.script = append(b.script, byte((OP_1-1)+val))
		return b
	}

	return b.AddData(fromInt(new(big.Int).SetInt64(val)))
}

// AddUint64 pushes the passed integer to the end of the script.
func (b *ScriptBuilder) AddUint64(val uint64) *ScriptBuilder {
	// Fast path for small integers.
	if val == 0 {
		b.script = append(b.script, OP_0)
		return b
	}
	if val >= 1 && val <= 16 {
		b.script = append(b.script, byte((OP_1-1)+val))
		return b
	}

	return b.AddData(fromInt(new(big.Int).SetUint64(val)))
}

// Reset resets the script so it has no content.
func (b *ScriptBuilder) Reset() *ScriptBuilder {
	b.script = b.script[0:0]
	return b
}

// Script returns the currently built script.
func (b *ScriptBuilder) Script() []byte {
	return b.script
}

// NewScriptBuilder returns a new instance of a script builder.  See
// ScriptBuilder for details.
func NewScriptBuilder() *ScriptBuilder {
	return &ScriptBuilder{
		script: make([]byte, 0, defaultScriptAlloc),
	}
}
