// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

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

	. "github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
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

// parseShortForm parses a string as as used in the Bitcoin Core reference tests
// into the script it came from.
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
			builder.TstConcatRawScript(bts)
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
		case "NULLDUMMY":
			flags |= ScriptStrictMultiSig
		case "P2SH":
			flags |= ScriptBip16
		case "SIGPUSHONLY":
			flags |= ScriptVerifySigPushOnly
		case "STRICTENC":
			flags |= ScriptVerifyStrictEncoding
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

	outPoint := wire.NewOutPoint(&wire.ShaHash{}, ^uint32(0))
	txIn := wire.NewTxIn(outPoint, []byte{OP_0, OP_0})
	txOut := wire.NewTxOut(0, pkScript)
	coinbaseTx.AddTxIn(txIn)
	coinbaseTx.AddTxOut(txOut)

	spendingTx := wire.NewMsgTx()
	coinbaseTxSha := coinbaseTx.TxSha()
	outPoint = wire.NewOutPoint(&coinbaseTxSha, 0)
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
		t.Errorf("TestBitcoindInvalidTests: %v\n", err)
		return
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestBitcoindInvalidTests couldn't Unmarshal: %v",
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
				t.Errorf("TestBitcoindInvalidTests: invalid test #%d",
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
				vm, err = NewEngine(scriptPubKey, tx, 0, flags, sigCache)
			} else {
				vm, err = NewEngine(scriptPubKey, tx, 0, flags, nil)
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
		t.Errorf("TestBitcoinValidTests: %v\n", err)
		return
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestBitcoindValidTests couldn't Unmarshal: %v",
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
				t.Errorf("TestBitcoindValidTests: invalid test #%d",
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
				vm, err = NewEngine(scriptPubKey, tx, 0, flags, sigCache)
			} else {
				vm, err = NewEngine(scriptPubKey, tx, 0, flags, nil)
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

// TestTxInvalidTests ensures all of the tests in tx_invalid.json fail as
// expected.
func TestTxInvalidTests(t *testing.T) {
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
				t.Errorf("bad test (%dth input sha not string)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			prevhash, err := wire.NewShaHashFromStr(previoustx)
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

			script, err := parseShortForm(oscript)
			if err != nil {
				t.Errorf("bad test (%dth input script doesn't "+
					"parse %v) %d: %v", j, err, i, test)
				continue testloop
			}

			prevOuts[*wire.NewOutPoint(prevhash, idx)] = script
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
			vm, err := NewEngine(pkScript, tx.MsgTx(), k, flags, nil)
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
				t.Errorf("bad test (%dth input sha not string)"+
					"%d: %v", j, i, test)
				continue
			}

			prevhash, err := wire.NewShaHashFromStr(previoustx)
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

			script, err := parseShortForm(oscript)
			if err != nil {
				t.Errorf("bad test (%dth input script doesn't "+
					"parse %v) %d: %v", j, err, i, test)
				continue
			}

			prevOuts[*wire.NewOutPoint(prevhash, idx)] = script
		}

		for k, txin := range tx.MsgTx().TxIn {
			pkScript, ok := prevOuts[txin.PreviousOutPoint]
			if !ok {
				t.Errorf("bad test (missing %dth input) %d:%v",
					k, i, test)
				continue testloop
			}
			vm, err := NewEngine(pkScript, tx.MsgTx(), k, flags, nil)
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

// TestCalcSignatureHash runs the Bitcoin Core signature hash calculation tests
// in sighash.json.
// https://github.com/bitcoin/bitcoin/blob/master/src/test/data/sighash.json
func TestCalcSignatureHash(t *testing.T) {
	file, err := ioutil.ReadFile("data/sighash.json")
	if err != nil {
		t.Errorf("TestCalcSignatureHash: %v\n", err)
		return
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestCalcSignatureHash couldn't Unmarshal: %v\n",
			err)
		return
	}

	for i, test := range tests {
		if i == 0 {
			// Skip first line -- contains comments only.
			continue
		}
		if len(test) != 5 {
			t.Fatalf("TestCalcSignatureHash: Test #%d has "+
				"wrong length.", i)
		}
		tx := wire.NewMsgTx()
		rawTx, _ := hex.DecodeString(test[0].(string))
		err := tx.Deserialize(bytes.NewReader(rawTx))
		if err != nil {
			t.Errorf("TestCalcSignatureHash failed test #%d: "+
				"Failed to parse transaction: %v", i, err)
			continue
		}

		subScript, _ := hex.DecodeString(test[1].(string))
		parsedScript, err := TstParseScript(subScript)
		if err != nil {
			t.Errorf("TestCalcSignatureHash failed test #%d: "+
				"Failed to parse sub-script: %v", i, err)
			continue
		}
		hash := TstCalcSignatureHash(parsedScript,
			SigHashType(test[3].(float64)),
			tx, int(test[2].(float64)))

		expectedHash, _ := wire.NewShaHashFromStr(test[4].(string))
		if !bytes.Equal(hash, expectedHash.Bytes()) {
			t.Errorf("TestCalcSignatureHash failed test #%d: "+
				"Signature hash mismatch.", i)
		}
	}
}
