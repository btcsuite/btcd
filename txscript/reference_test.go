// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

// testName returns a descriptive test name for the given reference test data.
func testName(test []string) (string, error) {
	var name string

	if len(test) < 3 || len(test) > 4 {
		return name, fmt.Errorf("invalid test length %d", len(test))
	}

	if len(test) == 4 {
		name = fmt.Sprintf("test (%s)", test[3])
	} else {
		name = fmt.Sprintf("test ([%s, %s, %s])", test[0], test[1],
			test[2])
	}
	return name, nil
}

// parse hex string into a []byte.
func parseHex(tok string) ([]byte, error) {
	if !strings.HasPrefix(tok, "0x") {
		return nil, errors.New("not a hex number")
	}
	return hex.DecodeString(tok[2:])
}

// shortFormOps holds a map of opcode names to values for use in short form
// parsing.  It is declared here so it only needs to be created once.
var shortFormOps map[string]byte

// parseShortForm parses a string as as used in the reference tests into the
// script it came from.
//
// The format used for these tests is pretty simple if ad-hoc:
//   - Opcodes other than the push opcodes and unknown are present as
//     either OP_NAME or just NAME
//   - Plain numbers are made into push operations
//   - Numbers beginning with 0x are inserted into the []byte as-is (so
//     0x14 is OP_DATA_20)
//   - Single quoted strings are pushed as data
//   - Anything else is an error
func parseShortForm(script string) ([]byte, error) {
	// Only create the short form opcode map once.
	if shortFormOps == nil {
		ops := make(map[string]byte)
		for opcodeName, opcodeValue := range OpcodeByName {
			if strings.Contains(opcodeName, "OP_UNKNOWN") {
				continue
			}
			ops[opcodeName] = opcodeValue

			// The opcodes named OP_# can't have the OP_ prefix
			// stripped or they would conflict with the plain
			// numbers.  Also, since OP_FALSE and OP_TRUE are
			// aliases for the OP_0, and OP_1, respectively, they
			// have the same value, so detect those by name and
			// allow them.
			if (opcodeName == "OP_FALSE" || opcodeName == "OP_TRUE") ||
				(opcodeValue != OP_0 && (opcodeValue < OP_1 ||
					opcodeValue > OP_16)) {

				ops[strings.TrimPrefix(opcodeName, "OP_")] = opcodeValue
			}
		}
		shortFormOps = ops
	}

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
			// Concatenate the bytes manually since the test code
			// intentionally creates scripts that are too large and
			// would cause the builder to error otherwise.
			if builder.err == nil {
				builder.script = append(builder.script, bts...)
			}
		} else if len(tok) >= 2 &&
			tok[0] == '\'' && tok[len(tok)-1] == '\'' {
			builder.AddFullData([]byte(tok[1 : len(tok)-1]))
		} else if opcode, ok := shortFormOps[tok]; ok {
			builder.AddOp(opcode)
		} else {
			return nil, fmt.Errorf("bad token \"%s\"", tok)
		}

	}
	return builder.Script()
}

// parseScriptFlags parses the provided flags string from the format used in the
// reference tests into ScriptFlags suitable for use in the script engine.
func parseScriptFlags(flagStr string) (ScriptFlags, error) {
	var flags ScriptFlags

	sFlags := strings.Split(flagStr, ",")
	for _, flag := range sFlags {
		switch flag {
		case "":
			// Nothing.
		case "CHECKLOCKTIMEVERIFY":
			flags |= ScriptVerifyCheckLockTimeVerify
		case "CHECKSEQUENCEVERIFY":
			flags |= ScriptVerifyCheckSequenceVerify
		case "CLEANSTACK":
			flags |= ScriptVerifyCleanStack
		case "DERSIG":
			flags |= ScriptVerifyDERSignatures
		case "DISCOURAGE_UPGRADABLE_NOPS":
			flags |= ScriptDiscourageUpgradableNops
		case "LOW_S":
			flags |= ScriptVerifyLowS
		case "MINIMALDATA":
			flags |= ScriptVerifyMinimalData
		case "NONE":
			// Nothing.
		case "P2SH":
			flags |= ScriptBip16
		case "SIGPUSHONLY":
			flags |= ScriptVerifySigPushOnly
		case "STRICTENC":
			flags |= ScriptVerifyStrictEncoding
		case "SHA256":
			flags |= ScriptVerifySHA256
		default:
			return flags, fmt.Errorf("invalid flag: %s", flag)
		}
	}
	return flags, nil
}

// createSpendTx generates a basic spending transaction given the passed
// signature and public key scripts.
func createSpendingTx(sigScript, pkScript []byte) *wire.MsgTx {
	coinbaseTx := wire.NewMsgTx()

	outPoint := wire.NewOutPoint(&chainhash.Hash{}, ^uint32(0),
		wire.TxTreeRegular)
	txIn := wire.NewTxIn(outPoint, []byte{OP_0, OP_0})
	txOut := wire.NewTxOut(0, pkScript)
	coinbaseTx.AddTxIn(txIn)
	coinbaseTx.AddTxOut(txOut)

	spendingTx := wire.NewMsgTx()
	coinbaseTxHash := coinbaseTx.TxHash()
	outPoint = wire.NewOutPoint(&coinbaseTxHash, 0, wire.TxTreeRegular)
	txIn = wire.NewTxIn(outPoint, sigScript)
	txOut = wire.NewTxOut(0, nil)

	spendingTx.AddTxIn(txIn)
	spendingTx.AddTxOut(txOut)

	return spendingTx
}

// TestScriptInvalidTests ensures all of the tests in script_invalid.json fail
// as expected.
func TestScriptInvalidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/script_invalid.json")
	if err != nil {
		t.Errorf("TestScriptInvalidTests: %v\n", err)
		return
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestScriptInvalidTests couldn't Unmarshal: %v",
			err)
		return
	}
	sigCache := NewSigCache(10)

	sigCacheToggle := []bool{true, false}
	for _, useSigCache := range sigCacheToggle {
		for i, test := range tests {
			// Skip comments
			if len(test) == 1 {
				continue
			}
			name, err := testName(test)
			if err != nil {
				t.Errorf("TestScriptInvalidTests: invalid test #%d",
					i)
				continue
			}
			scriptSig, err := parseShortForm(test[0])
			if err != nil {
				t.Errorf("%s: can't parse scriptSig; %v", name, err)
				continue
			}
			scriptPubKey, err := parseShortForm(test[1])
			if err != nil {
				t.Errorf("%s: can't parse scriptPubkey; %v", name, err)
				continue
			}
			flags, err := parseScriptFlags(test[2])
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			tx := createSpendingTx(scriptSig, scriptPubKey)

			var vm *Engine
			if useSigCache {
				vm, err = NewEngine(scriptPubKey, tx, 0, flags,
					0, sigCache)
			} else {
				vm, err = NewEngine(scriptPubKey, tx, 0, flags,
					0, nil)
			}

			if err == nil {
				if err := vm.Execute(); err == nil {
					t.Errorf("%s test succeeded when it "+
						"should have failed\n", name)
				}
				continue
			}
		}
	}
}

// TestScriptValidTests ensures all of the tests in script_valid.json pass as
// expected.
func TestScriptValidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/script_valid.json")
	if err != nil {
		t.Errorf("TestScriptValidTests: %v\n", err)
		return
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestScriptValidTests: couldn't Unmarshal: %v",
			err)
		return
	}

	sigCache := NewSigCache(10)

	sigCacheToggle := []bool{true, false}
	for _, useSigCache := range sigCacheToggle {
		for i, test := range tests {
			// Skip comments
			if len(test) == 1 {
				continue
			}
			name, err := testName(test)
			if err != nil {
				t.Errorf("TestScriptValidTests: invalid test #%d",
					i)
				continue
			}
			scriptSig, err := parseShortForm(test[0])
			if err != nil {
				t.Errorf("%s: can't parse scriptSig; %v", name, err)
				continue
			}
			scriptPubKey, err := parseShortForm(test[1])
			if err != nil {
				t.Errorf("%s: can't parse scriptPubkey; %v", name, err)
				continue
			}
			flags, err := parseScriptFlags(test[2])
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			tx := createSpendingTx(scriptSig, scriptPubKey)

			var vm *Engine
			if useSigCache {
				vm, err = NewEngine(scriptPubKey, tx, 0, flags,
					0, sigCache)
			} else {
				vm, err = NewEngine(scriptPubKey, tx, 0, flags,
					0, nil)
			}

			if err != nil {
				t.Errorf("%s failed to create script: %v", name, err)
				continue
			}
			err = vm.Execute()
			if err != nil {
				t.Errorf("%s failed to execute: %v", name, err)
				continue
			}
		}
	}
}

// testVecF64ToUint32 properly handles conversion of float64s read from the JSON
// test data to unsigned 32-bit integers.  This is necessary because some of the
// test data uses -1 as a shortcut to mean max uint32 and direct conversion of a
// negative float to an unsigned int is implementation dependent and therefore
// doesn't result in the expected value on all platforms.  This function woks
// around that limitation by converting to a 32-bit signed integer first and
// then to a 32-bit unsigned integer which results in the expected behavior on
// all platforms.
func testVecF64ToUint32(f float64) uint32 {
	return uint32(int32(f))
}

// TestTxInvalidTests ensures all of the tests in tx_invalid.json fail as
// expected.
func TestTxInvalidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/tx_invalid.json")
	if err != nil {
		t.Errorf("TestTxInvalidTests: %v\n", err)
		return
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestTxInvalidTests couldn't Unmarshal: %v\n", err)
		return
	}

	// form is either:
	//   ["this is a comment "]
	// or:
	//   [[[previous hash, previous index, previous scriptPubKey]...,]
	//	serializedTransaction, verifyFlags]
testloop:
	for i, test := range tests {
		inputs, ok := test[0].([]interface{})
		if !ok {
			continue
		}

		if len(test) != 3 {
			t.Errorf("bad test (bad length) %d: %v", i, test)
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

		tx, err := dcrutil.NewTxFromBytes(serializedTx)
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

		flags, err := parseScriptFlags(verifyFlags)
		if err != nil {
			t.Errorf("bad test %d: %v", i, err)
			continue
		}

		prevOuts := make(map[wire.OutPoint][]byte)
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
				t.Errorf("bad test (%dth input hash not string)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			prevhash, err := chainhash.NewHashFromStr(previoustx)
			if err != nil {
				t.Errorf("bad test (%dth input hash not hash %v)"+
					"%d: %v", j, err, i, test)
				continue testloop
			}

			idxf, ok := input[1].(float64)
			if !ok {
				t.Errorf("bad test (%dth input idx not number)"+
					"%d: %v", j, i, test)
				continue testloop
			}
			idx := testVecF64ToUint32(idxf)

			oscript, ok := input[2].(string)
			if !ok {
				t.Errorf("bad test (%dth input script not "+
					"string) %d: %v", j, i, test)
				continue testloop
			}

			script, err := parseShortForm(oscript)
			if err != nil {
				t.Errorf("bad test (%dth input script doesn't "+
					"parse %v) %d: %v", j, err, i, test)
				continue testloop
			}

			prevOuts[*wire.NewOutPoint(prevhash, idx, wire.TxTreeRegular)] = script
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
			vm, err := NewEngine(pkScript, tx.MsgTx(), k, flags, 0,
				nil)
			if err != nil {
				continue testloop
			}

			err = vm.Execute()
			if err != nil {
				continue testloop
			}

		}
		t.Errorf("test (%d:%v) succeeded when should fail",
			i, test)
	}
}

// TestTxValidTests ensures all of the tests in tx_valid.json pass as expected.
func TestTxValidTests(t *testing.T) {
	file, err := ioutil.ReadFile("data/tx_valid.json")
	if err != nil {
		t.Errorf("TestTxValidTests: %v\n", err)
		return
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestTxValidTests couldn't Unmarshal: %v\n", err)
		return
	}

	// form is either:
	//   ["this is a comment "]
	// or:
	//   [[[previous hash, previous index, previous scriptPubKey]...,]
	//	serializedTransaction, verifyFlags]
testloop:
	for i, test := range tests {
		inputs, ok := test[0].([]interface{})
		if !ok {
			continue
		}

		if len(test) != 3 {
			t.Errorf("bad test (bad length) %d: %v", i, test)
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

		tx, err := dcrutil.NewTxFromBytes(serializedTx)
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

		flags, err := parseScriptFlags(verifyFlags)
		if err != nil {
			t.Errorf("bad test %d: %v", i, err)
			continue
		}

		prevOuts := make(map[wire.OutPoint][]byte)
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
				t.Errorf("bad test (%dth input hash not string)"+
					"%d: %v", j, i, test)
				continue
			}

			prevhash, err := chainhash.NewHashFromStr(previoustx)
			if err != nil {
				t.Errorf("bad test (%dth input hash not hash %v)"+
					"%d: %v", j, err, i, test)
				continue
			}

			idxf, ok := input[1].(float64)
			if !ok {
				t.Errorf("bad test (%dth input idx not number)"+
					"%d: %v", j, i, test)
				continue
			}
			idx := testVecF64ToUint32(idxf)

			oscript, ok := input[2].(string)
			if !ok {
				t.Errorf("bad test (%dth input script not "+
					"string) %d: %v", j, i, test)
				continue
			}

			script, err := parseShortForm(oscript)
			if err != nil {
				t.Errorf("bad test (%dth input script doesn't "+
					"parse %v) %d: %v", j, err, i, test)
				continue
			}

			prevOuts[*wire.NewOutPoint(prevhash, idx, wire.TxTreeRegular)] = script
		}

		for k, txin := range tx.MsgTx().TxIn {
			pkScript, ok := prevOuts[txin.PreviousOutPoint]
			if !ok {
				t.Errorf("bad test (missing %dth input) %d:%v",
					k, i, test)
				continue testloop
			}
			vm, err := NewEngine(pkScript, tx.MsgTx(), k, flags, 0,
				nil)
			if err != nil {
				t.Errorf("test (%d:%v:%d) failed to create "+
					"script: %v", i, test, k, err)
				continue
			}

			err = vm.Execute()
			if err != nil {
				t.Errorf("test (%d:%v:%d) failed to execute: "+
					"%v", i, test, k, err)
				continue
			}
		}
	}
}

// parseSigHashExpectedResult parses the provided expected result string into
// allowed error codes.  An error is returned if the expected result string is
// not supported.
func parseSigHashExpectedResult(expected string) (error, error) {
	switch expected {
	case "OK":
		return nil, nil
	case "SIGHASH_SINGLE_IDX":
		return ErrSighashSingleIdx, nil
	}

	return nil, fmt.Errorf("unrecognized expected result in test data: %v",
		expected)
}

// TestCalcSignatureHashReference runs the reference signature hash calculation
// tests in sighash.json.
func TestCalcSignatureHashReference(t *testing.T) {
	file, err := ioutil.ReadFile("data/sighash.json")
	if err != nil {
		t.Fatalf("TestCalcSignatureHash: %v\n", err)
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Fatalf("TestCalcSignatureHash couldn't Unmarshal: %v\n", err)
	}

	for i, test := range tests {
		// Skip comment lines.
		if len(test) == 1 {
			continue
		}

		// Ensure test is well formed.
		if len(test) < 6 || len(test) > 7 {
			t.Fatalf("Test #%d: wrong length %d", i, len(test))
		}

		// Extract and parse the transaction from the test fields.
		txHex, ok := test[0].(string)
		if !ok {
			t.Errorf("Test #%d: transaction is not a string", i)
			continue
		}
		rawTx, err := hex.DecodeString(txHex)
		if err != nil {
			t.Errorf("Test #%d: unable to parse transaction: %v", i, err)
			continue
		}
		var tx wire.MsgTx
		err = tx.Deserialize(bytes.NewReader(rawTx))
		if err != nil {
			t.Errorf("Test #%d: unable to deserialize transaction: %v", i, err)
			continue
		}

		// Extract and parse the script from the test fields.
		subScriptStr, ok := test[1].(string)
		if !ok {
			t.Errorf("Test #%d: script is not a string", i)
			continue
		}
		subScript, err := hex.DecodeString(subScriptStr)
		if err != nil {
			t.Errorf("Test #%d: unable to decode script: %v", i,
				err)
			continue
		}
		parsedScript, err := parseScript(subScript)
		if err != nil {
			t.Errorf("Test #%d: unable to parse script: %v", i, err)
			continue
		}

		// Extract the input index from the test fields.
		inputIdxF64, ok := test[2].(float64)
		if !ok {
			t.Errorf("Test #%d: input idx is not numeric", i)
			continue
		}

		// Extract and parse the hash type from the test fields.
		hashTypeF64, ok := test[3].(float64)
		if !ok {
			t.Errorf("Test #%d: hash type is not numeric", i)
			continue
		}
		hashType := SigHashType(testVecF64ToUint32(hashTypeF64))

		// Extract and parse the signature hash from the test fields.
		expectedHashStr, ok := test[4].(string)
		if !ok {
			t.Errorf("Test #%d: signature hash is not a string", i)
			continue
		}
		expectedHash, err := hex.DecodeString(expectedHashStr)
		if err != nil {
			t.Errorf("Test #%d: unable to sig hash: %v", i, err)
			continue
		}

		// Extract and parse the expected result from the test fields.
		expectedErrStr, ok := test[5].(string)
		if !ok {
			t.Errorf("Test #%d: result field is not a string", i)
			continue
		}
		expectedErr, err := parseSigHashExpectedResult(expectedErrStr)
		if err != nil {
			t.Errorf("Test #%d: %v", i, err)
			continue
		}

		// Calculate the signature hash and verify expected result.
		hash, err := calcSignatureHash(parsedScript, hashType, &tx,
			int(inputIdxF64), nil)
		if err != expectedErr {
			t.Errorf("Test #%d: unexpected error: want %v, got %v", i,
				expectedErr, err)
			continue
		}
		if !bytes.Equal(hash, expectedHash) {
			t.Errorf("Test #%d: signature hash mismatch - got %x, "+
				"want %x", i, hash, expectedHash)
			continue
		}
	}
}
