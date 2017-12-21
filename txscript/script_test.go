// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

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
		data, err := txscript.PushedData(script)
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
		builder := txscript.NewScriptBuilder()
		builder.AddInt64(int64(i))
		script, err := builder.Script()
		if err != nil {
			t.Errorf("Script: test #%d unexpected error: %v\n", i,
				err)
			continue
		}
		if result := txscript.IsPushOnlyScript(script); !result {
			t.Errorf("IsPushOnlyScript: test #%d failed: %x\n", i,
				script)
			continue
		}
		pops, err := txscript.TstParseScript(script)
		if err != nil {
			t.Errorf("TstParseScript: #%d failed: %v", i, err)
			continue
		}
		for _, pop := range pops {
			if result := txscript.TstHasCanonicalPushes(pop); !result {
				t.Errorf("TstHasCanonicalPushes: test #%d "+
					"failed: %x\n", i, script)
				break
			}
		}
	}
	for i := 0; i <= txscript.MaxScriptElementSize; i++ {
		builder := txscript.NewScriptBuilder()
		builder.AddData(bytes.Repeat([]byte{0x49}, i))
		script, err := builder.Script()
		if err != nil {
			t.Errorf("StandardPushesTests test #%d unexpected error: %v\n", i, err)
			continue
		}
		if result := txscript.IsPushOnlyScript(script); !result {
			t.Errorf("StandardPushesTests IsPushOnlyScript test #%d failed: %x\n", i, script)
			continue
		}
		pops, err := txscript.TstParseScript(script)
		if err != nil {
			t.Errorf("StandardPushesTests #%d failed to TstParseScript: %v", i, err)
			continue
		}
		for _, pop := range pops {
			if result := txscript.TstHasCanonicalPushes(pop); !result {
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
		err       error
	}{
		{
			name:      "scriptSig doesn't parse",
			scriptSig: []byte{txscript.OP_PUSHDATA1, 2},
			err:       txscript.ErrStackShortScript,
		},
		{
			name:      "scriptSig isn't push only",
			scriptSig: []byte{txscript.OP_1, txscript.OP_DUP},
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
			scriptSig: []byte{txscript.OP_1, txscript.OP_1},
			nSigOps:   0,
		},
		{
			name: "pushed script doesn't parse",
			scriptSig: []byte{txscript.OP_DATA_2,
				txscript.OP_PUSHDATA1, 2},
			err: txscript.ErrStackShortScript,
		},
	}

	// The signature in the p2sh script is nonsensical for the tests since
	// this script will never be executed.  What matters is that it matches
	// the right pattern.
	pkScript := mustParseShortForm("HASH160 DATA_20 0x433ec2ac1ffa1b7b7d0" +
		"27f564529c57197f9ae88 EQUAL")
	for _, test := range tests {
		count := txscript.GetPreciseSigOpCount(test.scriptSig, pkScript,
			true)
		if count != test.nSigOps {
			t.Errorf("%s: expected count of %d, got %d", test.name,
				test.nSigOps, count)

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
			remove: txscript.OP_CODESEPARATOR,
			after:  "NOP",
		},
		{
			// Test basic opcode removal.
			name:   "codeseparator 1",
			before: "NOP CODESEPARATOR TRUE",
			remove: txscript.OP_CODESEPARATOR,
			after:  "NOP TRUE",
		},
		{
			// The opcode in question is actually part of the data
			// in a previous opcode.
			name:   "codeseparator by coincidence",
			before: "NOP DATA_1 CODESEPARATOR TRUE",
			remove: txscript.OP_CODESEPARATOR,
			after:  "NOP DATA_1 CODESEPARATOR TRUE",
		},
		{
			name:   "invalid opcode",
			before: "CAT",
			remove: txscript.OP_CODESEPARATOR,
			after:  "CAT",
		},
		{
			name:   "invalid length (insruction)",
			before: "PUSHDATA1",
			remove: txscript.OP_CODESEPARATOR,
			err:    txscript.ErrStackShortScript,
		},
		{
			name:   "invalid length (data)",
			before: "PUSHDATA1 0xff 0xfe",
			remove: txscript.OP_CODESEPARATOR,
			err:    txscript.ErrStackShortScript,
		},
	}

	for _, test := range tests {
		before := mustParseShortForm(test.before)
		after := mustParseShortForm(test.after)
		result, err := txscript.TstRemoveOpcode(before, test.remove)
		if test.err != nil {
			if err != test.err {
				t.Errorf("%s: got unexpected error. exp: \"%v\" "+
					"got: \"%v\"", test.name, test.err, err)
			}
			return
		}
		if err != nil {
			t.Errorf("%s: unexpected failure: \"%v\"", test.name, err)
			return
		}
		if !bytes.Equal(after, result) {
			t.Errorf("%s: value does not equal expected: exp: \"%v\""+
				" got: \"%v\"", test.name, after, result)
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
			before: []byte{txscript.OP_NOP},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{txscript.OP_NOP},
		},
		{
			name:   "simple case",
			before: []byte{txscript.OP_DATA_4, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name:   "simple case (miss)",
			before: []byte{txscript.OP_DATA_4, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 5},
			after:  []byte{txscript.OP_DATA_4, 1, 2, 3, 4},
		},
		// fix to allow for decred tests too
		/*
			{
				name:        "stakesubmission",
				scriptclass: txscript.StakeSubmissionTy,
				stringed:    "stakesubmission",
			},
			{
				name:        "stakegen",
				scriptclass: txscript.StakeGenTy,
				stringed:    "stakegen",
			},
			{
				name:        "stakerevoke",
				scriptclass: txscript.StakeRevocationTy,
				stringed:    "stakerevoke",
			},
		*/
		{
			// padded to keep it canonical.
			name: "simple case (pushdata1)",
			before: append(append([]byte{txscript.OP_PUSHDATA1, 76},
				bytes.Repeat([]byte{0}, 72)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name: "simple case (pushdata1 miss)",
			before: append(append([]byte{txscript.OP_PUSHDATA1, 76},
				bytes.Repeat([]byte{0}, 72)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 5},
			after: append(append([]byte{txscript.OP_PUSHDATA1, 76},
				bytes.Repeat([]byte{0}, 72)...),
				[]byte{1, 2, 3, 4}...),
		},
		{
			name:   "simple case (pushdata1 miss noncanonical)",
			before: []byte{txscript.OP_PUSHDATA1, 4, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{txscript.OP_PUSHDATA1, 4, 1, 2, 3, 4},
		},
		{
			name: "simple case (pushdata2)",
			before: append(append([]byte{txscript.OP_PUSHDATA2, 0, 1},
				bytes.Repeat([]byte{0}, 252)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name: "simple case (pushdata2 miss)",
			before: append(append([]byte{txscript.OP_PUSHDATA2, 0, 1},
				bytes.Repeat([]byte{0}, 252)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4, 5},
			after: append(append([]byte{txscript.OP_PUSHDATA2, 0, 1},
				bytes.Repeat([]byte{0}, 252)...),
				[]byte{1, 2, 3, 4}...),
		},
		{
			name:   "simple case (pushdata2 miss noncanonical)",
			before: []byte{txscript.OP_PUSHDATA2, 4, 0, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{txscript.OP_PUSHDATA2, 4, 0, 1, 2, 3, 4},
		},
		{
			// This is padded to make the push canonical.
			name: "simple case (pushdata4)",
			before: append(append([]byte{txscript.OP_PUSHDATA4, 0, 0, 1, 0},
				bytes.Repeat([]byte{0}, 65532)...),
				[]byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name:   "simple case (pushdata4 miss noncanonical)",
			before: []byte{txscript.OP_PUSHDATA4, 4, 0, 0, 0, 1, 2, 3, 4},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{txscript.OP_PUSHDATA4, 4, 0, 0, 0, 1, 2, 3, 4},
		},
		{
			// This is padded to make the push canonical.
			name: "simple case (pushdata4 miss)",
			before: append(append([]byte{txscript.OP_PUSHDATA4, 0, 0, 1, 0},
				bytes.Repeat([]byte{0}, 65532)...), []byte{1, 2, 3, 4}...),
			remove: []byte{1, 2, 3, 4, 5},
			after: append(append([]byte{txscript.OP_PUSHDATA4, 0, 0, 1, 0},
				bytes.Repeat([]byte{0}, 65532)...), []byte{1, 2, 3, 4}...),
		},
		{
			name:   "invalid opcode ",
			before: []byte{txscript.OP_UNKNOWN193},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{txscript.OP_UNKNOWN193},
		},
		{
			name:   "invalid length (instruction)",
			before: []byte{txscript.OP_PUSHDATA1},
			remove: []byte{1, 2, 3, 4},
			err:    txscript.ErrStackShortScript,
		},
		{
			name:   "invalid length (data)",
			before: []byte{txscript.OP_PUSHDATA1, 255, 254},
			remove: []byte{1, 2, 3, 4},
			err:    txscript.ErrStackShortScript,
		},
	}

	for _, test := range tests {
		result, err := txscript.TstRemoveOpcodeByData(test.before,
			test.remove)
		if test.err != nil {
			if err != test.err {
				t.Errorf("%s: got unexpected error. exp: \"%v\" "+
					"got: \"%v\"", test.name, test.err, err)
			}
			return
		}
		if err != nil {
			t.Errorf("%s: unexpected failure: \"%v\"", test.name, err)
			return
		}
		if !bytes.Equal(test.after, result) {
			t.Errorf("%s: value does not equal expected: exp: \"%v\""+
				" got: \"%v\"", test.name, test.after, result)
		}
	}
}

// TestIsPayToScriptHash ensures the IsPayToScriptHash function returns the
// expected results for all the scripts in scriptClassTests.
func TestIsPayToScriptHash(t *testing.T) {
	t.Parallel()

	for _, test := range scriptClassTests {
		script := mustParseShortForm(test.script)
		shouldBe := (test.class == txscript.ScriptHashTy)
		p2sh := txscript.IsPayToScriptHash(script)
		if p2sh != shouldBe {
			t.Errorf("%s: epxected p2sh %v, got %v", test.name,
				shouldBe, p2sh)
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
		pops, err := txscript.TstParseScript(script)
		if err != nil {
			if test.expected {
				t.Errorf("TstParseScript #%d failed: %v", i, err)
			}
			continue
		}
		for _, pop := range pops {
			if txscript.TstHasCanonicalPushes(pop) != test.expected {
				t.Errorf("TstHasCanonicalPushes: #%d (%s) "+
					"wrong result\ngot: %v\nwant: %v", i,
					test.name, true, test.expected)
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

	if txscript.IsPushOnlyScript(test.script) != test.expected {
		t.Errorf("IsPushOnlyScript (%s) wrong result\ngot: %v\nwant: "+
			"%v", test.name, true, test.expected)
	}
}

// TestCalcSignatureHash does some rudimentary testing of msg hash calculation.
func TestCalcSignatureHash(t *testing.T) {
	tx := new(wire.MsgTx)
	tx.SerType = wire.TxSerializeFull
	tx.Version = 1
	for i := 0; i < 3; i++ {
		txIn := new(wire.TxIn)
		txIn.Sequence = 0xFFFFFFFF
		txIn.PreviousOutPoint.Hash = chainhash.HashH([]byte{byte(i)})
		txIn.PreviousOutPoint.Index = uint32(i)
		txIn.PreviousOutPoint.Tree = int8(0)
		tx.AddTxIn(txIn)
	}
	for i := 0; i < 2; i++ {
		txOut := new(wire.TxOut)
		txOut.PkScript = decodeHex("51")
		txOut.Value = 0x0000FF00FF00FF00
		tx.AddTxOut(txOut)
	}

	want := decodeHex("4ce2cd042d64e35b36fdbd16aff0d38a5abebff0e5e8f6b6b31" +
		"fcd4ac6957905")
	script := decodeHex("51")

	// Test prefix caching.
	msg1, err := txscript.CalcSignatureHash(script, txscript.SigHashAll, tx, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error %v", err.Error())
	}

	prefixHash := tx.TxHash()
	msg2, err := txscript.CalcSignatureHash(script, txscript.SigHashAll, tx, 0,
		&prefixHash)
	if err != nil {
		t.Fatalf("unexpected error %v", err.Error())
	}

	if !bytes.Equal(msg1, want) {
		t.Errorf("for sighash all sig noncached wrong msg -- got %x, want %x",
			msg1,
			want)
	}
	if !bytes.Equal(msg2, want) {
		t.Errorf("for sighash all sig cached wrong msg -- got %x, want %x",
			msg1,
			want)
	}
	if !bytes.Equal(msg1, msg2) {
		t.Errorf("for sighash all sig non-equivalent msgs %x and %x were "+
			"returned when using a cached prefix",
			msg1,
			msg2)
	}

	// Move the index and make sure that we get a whole new hash, despite
	// using the same TxOuts.
	msg3, err := txscript.CalcSignatureHash(script, txscript.SigHashAll, tx, 1,
		&prefixHash)
	if err != nil {
		t.Fatalf("unexpected error %v", err.Error())
	}

	if bytes.Equal(msg1, msg3) {
		t.Errorf("for sighash all sig equivalent msgs %x and %x were "+
			"returned when using a cached prefix but different indices",
			msg1,
			msg3)
	}
}

// TestIsUnspendable ensures the IsUnspendable function returns the expected
// results.
func TestIsUnspendable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		amount   int64
		pkScript []byte
		expected bool
	}{
		{
			// Unspendable
			amount:   100,
			pkScript: []byte{0x6a, 0x04, 0x74, 0x65, 0x73, 0x74},
			expected: true,
		},
		{
			// Unspendable
			amount: 0,
			pkScript: []byte{0x76, 0xa9, 0x14, 0x29, 0x95, 0xa0,
				0xfe, 0x68, 0x43, 0xfa, 0x9b, 0x95, 0x45,
				0x97, 0xf0, 0xdc, 0xa7, 0xa4, 0x4d, 0xf6,
				0xfa, 0x0b, 0x5c, 0x88, 0xac},
			expected: true,
		},
		{
			// Spendable
			amount: 100,
			pkScript: []byte{0x76, 0xa9, 0x14, 0x29, 0x95, 0xa0,
				0xfe, 0x68, 0x43, 0xfa, 0x9b, 0x95, 0x45,
				0x97, 0xf0, 0xdc, 0xa7, 0xa4, 0x4d, 0xf6,
				0xfa, 0x0b, 0x5c, 0x88, 0xac},
			expected: false,
		},
	}

	for i, test := range tests {
		res := txscript.IsUnspendable(test.amount, test.pkScript)
		if res != test.expected {
			t.Errorf("TestIsUnspendable #%d failed: got %v want %v",
				i, res, test.expected)
			continue
		}
	}
}
