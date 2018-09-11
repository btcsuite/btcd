// Copyright (c) 2013-2017 The btcsuite developers
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
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

var (
	// tokenRE is a regular expression used to parse tokens from short form
	// scripts.  It splits on repeated tokens and spaces.  Repeated tokens are
	// denoted by being wrapped in angular brackets followed by a suffix which
	// consists of a number inside braces.
	tokenRE = regexp.MustCompile(`\<.+?\>\{[0-9]+\}|[^\s]+`)

	// repTokenRE is a regular expression used to parse short form scripts
	// for a series of tokens repeated a specified number of times.
	repTokenRE = regexp.MustCompile(`^\<(.+)\>\{([0-9]+)\}$`)

	// repRawRE is a regular expression used to parse short form scripts
	// for raw data that is to be repeated a specified number of times.
	repRawRE = regexp.MustCompile(`^(0[xX][0-9a-fA-F]+)\{([0-9]+)\}$`)

	// repQuoteRE is a regular expression used to parse short form scripts for
	// quoted data that is to be repeated a specified number of times.
	repQuoteRE = regexp.MustCompile(`^'(.*)'\{([0-9]+)\}$`)
)

// scriptTestName returns a descriptive test name for the given reference script
// test data.
func scriptTestName(test []string) (string, error) {
	// The test must consist of at least a signature script, public key script,
	// verification flags, and expected error.  Finally, it may optionally
	// contain a comment.
	if len(test) < 4 || len(test) > 5 {
		return "", fmt.Errorf("invalid test length %d", len(test))
	}

	// Use the comment for the test name if one is specified, otherwise,
	// construct the name based on the signature script, public key script,
	// and flags.
	var name string
	if len(test) == 5 {
		name = fmt.Sprintf("test (%s)", test[4])
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
//   - Opcodes other than the push opcodes and unknown are present as either
//     OP_NAME or just NAME
//   - Plain numbers are made into push operations
//   - Numbers beginning with 0x are inserted into the []byte as-is (so 0x14 is
//     OP_DATA_20)
//   - Numbers beginning with 0x which have a suffix which consists of a number
//     in braces (e.g. 0x6161{10}) repeat the raw bytes the specified number of
//     times and are inserted as-is
//   - Single quoted strings are pushed as data
//   - Single quoted strings that have a suffix which consists of a number in
//     braces (e.g. 'b'{10}) repeat the data the specified number of times and
//     are pushed as a single data push
//   - Tokens inside of angular brackets with a suffix which consists of a
//     number in braces (e.g. <0 0 CHECKMULTSIG>{5}) is parsed as if the tokens
//     inside the angular brackets were manually repeated the specified number
//     of times
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

	builder := NewScriptBuilder()

	var handleToken func(tok string) error
	handleToken = func(tok string) error {
		// Multiple repeated tokens.
		if m := repTokenRE.FindStringSubmatch(tok); m != nil {
			count, err := strconv.ParseInt(m[2], 10, 32)
			if err != nil {
				return fmt.Errorf("bad token %q", tok)
			}
			tokens := tokenRE.FindAllStringSubmatch(m[1], -1)
			for i := 0; i < int(count); i++ {
				for _, t := range tokens {
					if err := handleToken(t[0]); err != nil {
						return err
					}
				}
			}
			return nil
		}

		// Plain number.
		if num, err := strconv.ParseInt(tok, 10, 64); err == nil {
			builder.AddInt64(num)
			return nil
		}

		// Raw data.
		if bts, err := parseHex(tok); err == nil {
			// Concatenate the bytes manually since the test code
			// intentionally creates scripts that are too large and
			// would cause the builder to error otherwise.
			if builder.err == nil {
				builder.script = append(builder.script, bts...)
			}
			return nil
		}

		// Repeated raw bytes.
		if m := repRawRE.FindStringSubmatch(tok); m != nil {
			bts, err := parseHex(m[1])
			if err != nil {
				return fmt.Errorf("bad token %q", tok)
			}
			count, err := strconv.ParseInt(m[2], 10, 32)
			if err != nil {
				return fmt.Errorf("bad token %q", tok)
			}

			// Concatenate the bytes manually since the test code
			// intentionally creates scripts that are too large and
			// would cause the builder to error otherwise.
			bts = bytes.Repeat(bts, int(count))
			if builder.err == nil {
				builder.script = append(builder.script, bts...)
			}
			return nil
		}

		// Quoted data.
		if len(tok) >= 2 && tok[0] == '\'' && tok[len(tok)-1] == '\'' {
			builder.AddFullData([]byte(tok[1 : len(tok)-1]))
			return nil
		}

		// Repeated quoted data.
		if m := repQuoteRE.FindStringSubmatch(tok); m != nil {
			count, err := strconv.ParseInt(m[2], 10, 32)
			if err != nil {
				return fmt.Errorf("bad token %q", tok)
			}
			data := strings.Repeat(m[1], int(count))
			builder.AddFullData([]byte(data))
			return nil
		}

		// Named opcode.
		if opcode, ok := shortFormOps[tok]; ok {
			builder.AddOp(opcode)
			return nil
		}

		return fmt.Errorf("bad token %q", tok)
	}

	for _, tokens := range tokenRE.FindAllStringSubmatch(script, -1) {
		if err := handleToken(tokens[0]); err != nil {
			return nil, err
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
		case "DISCOURAGE_UPGRADABLE_NOPS":
			flags |= ScriptDiscourageUpgradableNops
		case "NONE":
			// Nothing.
		case "SIGPUSHONLY":
			flags |= ScriptVerifySigPushOnly
		case "SHA256":
			flags |= ScriptVerifySHA256
		default:
			return flags, fmt.Errorf("invalid flag: %s", flag)
		}
	}
	return flags, nil
}

// parseExpectedResult parses the provided expected result string into allowed
// script error codes.  An error is returned if the expected result string is
// not supported.
func parseExpectedResult(expected string) ([]ErrorCode, error) {
	switch expected {
	case "OK":
		return nil, nil
	case "ERR_EARLY_RETURN":
		return []ErrorCode{ErrEarlyReturn}, nil
	case "ERR_EMPTY_STACK":
		return []ErrorCode{ErrEmptyStack}, nil
	case "ERR_EVAL_FALSE":
		return []ErrorCode{ErrEvalFalse}, nil
	case "ERR_SCRIPT_SIZE":
		return []ErrorCode{ErrScriptTooBig}, nil
	case "ERR_PUSH_SIZE":
		return []ErrorCode{ErrElementTooBig}, nil
	case "ERR_OP_COUNT":
		return []ErrorCode{ErrTooManyOperations}, nil
	case "ERR_STACK_SIZE":
		return []ErrorCode{ErrStackOverflow}, nil
	case "ERR_PUBKEY_COUNT":
		return []ErrorCode{ErrInvalidPubKeyCount}, nil
	case "ERR_SIG_COUNT":
		return []ErrorCode{ErrInvalidSignatureCount}, nil
	case "ERR_OUT_OF_RANGE":
		return []ErrorCode{ErrNumOutOfRange}, nil
	case "ERR_VERIFY":
		return []ErrorCode{ErrVerify}, nil
	case "ERR_EQUAL_VERIFY":
		return []ErrorCode{ErrEqualVerify}, nil
	case "ERR_DISABLED_OPCODE":
		return []ErrorCode{ErrDisabledOpcode}, nil
	case "ERR_RESERVED_OPCODE":
		return []ErrorCode{ErrReservedOpcode}, nil
	case "ERR_P2SH_STAKE_OPCODES":
		return []ErrorCode{ErrP2SHStakeOpCodes}, nil
	case "ERR_MALFORMED_PUSH":
		return []ErrorCode{ErrMalformedPush}, nil
	case "ERR_INVALID_STACK_OPERATION", "ERR_INVALID_ALTSTACK_OPERATION":
		return []ErrorCode{ErrInvalidStackOperation}, nil
	case "ERR_UNBALANCED_CONDITIONAL":
		return []ErrorCode{ErrUnbalancedConditional}, nil
	case "ERR_NEGATIVE_SUBSTR_INDEX":
		return []ErrorCode{ErrNegativeSubstrIdx}, nil
	case "ERR_OVERFLOW_SUBSTR_INDEX":
		return []ErrorCode{ErrOverflowSubstrIdx}, nil
	case "ERR_NEGATIVE_ROTATION":
		return []ErrorCode{ErrNegativeRotation}, nil
	case "ERR_OVERFLOW_ROTATION":
		return []ErrorCode{ErrOverflowRotation}, nil
	case "ERR_DIVIDE_BY_ZERO":
		return []ErrorCode{ErrDivideByZero}, nil
	case "ERR_NEGATIVE_SHIFT":
		return []ErrorCode{ErrNegativeShift}, nil
	case "ERR_OVERFLOW_SHIFT":
		return []ErrorCode{ErrOverflowShift}, nil
	case "ERR_MINIMAL_DATA":
		return []ErrorCode{ErrMinimalData}, nil
	case "ERR_SIG_HASH_TYPE":
		return []ErrorCode{ErrInvalidSigHashType}, nil
	case "ERR_SIG_TOO_SHORT":
		return []ErrorCode{ErrSigTooShort}, nil
	case "ERR_SIG_TOO_LONG":
		return []ErrorCode{ErrSigTooLong}, nil
	case "ERR_SIG_INVALID_SEQ_ID":
		return []ErrorCode{ErrSigInvalidSeqID}, nil
	case "ERR_SIG_INVALID_DATA_LEN":
		return []ErrorCode{ErrSigInvalidDataLen}, nil
	case "ERR_SIG_MISSING_S_TYPE_ID":
		return []ErrorCode{ErrSigMissingSTypeID}, nil
	case "ERR_SIG_MISSING_S_LEN":
		return []ErrorCode{ErrSigMissingSLen}, nil
	case "ERR_SIG_INVALID_S_LEN":
		return []ErrorCode{ErrSigInvalidSLen}, nil
	case "ERR_SIG_INVALID_R_INT_ID":
		return []ErrorCode{ErrSigInvalidRIntID}, nil
	case "ERR_SIG_ZERO_R_LEN":
		return []ErrorCode{ErrSigZeroRLen}, nil
	case "ERR_SIG_NEGATIVE_R":
		return []ErrorCode{ErrSigNegativeR}, nil
	case "ERR_SIG_TOO_MUCH_R_PADDING":
		return []ErrorCode{ErrSigTooMuchRPadding}, nil
	case "ERR_SIG_INVALID_S_INT_ID":
		return []ErrorCode{ErrSigInvalidSIntID}, nil
	case "ERR_SIG_ZERO_S_LEN":
		return []ErrorCode{ErrSigZeroSLen}, nil
	case "ERR_SIG_NEGATIVE_S":
		return []ErrorCode{ErrSigNegativeS}, nil
	case "ERR_SIG_TOO_MUCH_S_PADDING":
		return []ErrorCode{ErrSigTooMuchSPadding}, nil
	case "ERR_SIG_HIGH_S":
		return []ErrorCode{ErrSigHighS}, nil
	case "ERR_SIG_PUSHONLY":
		return []ErrorCode{ErrNotPushOnly}, nil
	case "ERR_PUBKEY_TYPE":
		return []ErrorCode{ErrPubKeyType}, nil
	case "ERR_CLEAN_STACK":
		return []ErrorCode{ErrCleanStack}, nil
	case "ERR_DISCOURAGE_UPGRADABLE_NOPS":
		return []ErrorCode{ErrDiscourageUpgradableNOPs}, nil
	case "ERR_NEGATIVE_LOCKTIME":
		return []ErrorCode{ErrNegativeLockTime}, nil
	case "ERR_UNSATISFIED_LOCKTIME":
		return []ErrorCode{ErrUnsatisfiedLockTime}, nil
	}

	return nil, fmt.Errorf("unrecognized expected result in test data: %v",
		expected)
}

// createSpendTx generates a basic spending transaction given the passed
// signature and public key scripts.
func createSpendingTx(sigScript, pkScript []byte) *wire.MsgTx {
	coinbaseTx := wire.NewMsgTx()

	outPoint := wire.NewOutPoint(&chainhash.Hash{}, ^uint32(0),
		wire.TxTreeRegular)
	txIn := wire.NewTxIn(outPoint, 0, []byte{OP_0, OP_0})
	txOut := wire.NewTxOut(0, pkScript)
	coinbaseTx.AddTxIn(txIn)
	coinbaseTx.AddTxOut(txOut)

	spendingTx := wire.NewMsgTx()
	coinbaseTxHash := coinbaseTx.TxHash()
	outPoint = wire.NewOutPoint(&coinbaseTxHash, 0, wire.TxTreeRegular)
	txIn = wire.NewTxIn(outPoint, 0, sigScript)
	txOut = wire.NewTxOut(0, nil)

	spendingTx.AddTxIn(txIn)
	spendingTx.AddTxOut(txOut)

	return spendingTx
}

// testScripts ensures all of the passed script tests execute with the expected
// results with or without using a signature cache, as specified by the
// parameter.
func testScripts(t *testing.T, tests [][]string, useSigCache bool) {
	// Create a signature cache to use only if requested.
	var sigCache *SigCache
	if useSigCache {
		sigCache = NewSigCache(10)
	}

	// "Format is: [scriptSig, scriptPubKey, flags, expectedScriptError, ...
	//   comments]"
	for i, test := range tests {
		// Skip single line comments.
		if len(test) == 1 {
			continue
		}

		// Construct a name for the test based on the comment and test data.
		name, err := scriptTestName(test)
		if err != nil {
			t.Errorf("TestScripts: invalid test #%d: %v", i, err)
			continue
		}

		// Extract and parse the signature script from the test fields.
		scriptSig, err := parseShortForm(test[0])
		if err != nil {
			t.Errorf("%s: can't parse scriptSig; %v", name, err)
			continue
		}

		// Extract and parse the public key script from the test fields.
		scriptPubKey, err := parseShortForm(test[1])
		if err != nil {
			t.Errorf("%s: can't parse scriptPubkey; %v", name, err)
			continue
		}

		// Extract and parse the script flags from the test fields.
		flags, err := parseScriptFlags(test[2])
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}

		// Extract and parse the expected result from the test fields.
		//
		// Convert the expected result string into the allowed script error
		// codes.  This allows txscript to be more fine grained with its errors
		// than the reference test data by allowing some of the test data errors
		// to map to more than one possibility.
		resultStr := test[3]
		allowedErrorCodes, err := parseExpectedResult(resultStr)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}

		// Generate a transaction pair such that one spends from the other and
		// the provided signature and public key scripts are used, then create a
		// new engine to execute the scripts.
		tx := createSpendingTx(scriptSig, scriptPubKey)
		vm, err := NewEngine(scriptPubKey, tx, 0, flags, 0, sigCache)
		if err == nil {
			err = vm.Execute()
		}

		// Ensure there were no errors when the expected result is OK.
		if resultStr == "OK" {
			if err != nil {
				t.Errorf("%s failed to execute: %v", name, err)
			}
			continue
		}

		// At this point an error was expected so ensure the result of the
		// execution matches it.
		success := false
		for _, code := range allowedErrorCodes {
			if IsErrorCode(err, code) {
				success = true
				break
			}
		}
		if !success {
			if serr, ok := err.(Error); ok {
				t.Errorf("%s: want error codes %v, got %v", name,
					allowedErrorCodes, serr.ErrorCode)
				continue
			}
			t.Errorf("%s: want error codes %v, got err: %v (%T)", name,
				allowedErrorCodes, err, err)
			continue
		}
	}
}

// TestScripts ensures all of the tests in script_tests.json execute with the
// expected results as defined in the test data.
func TestScripts(t *testing.T) {
	file, err := ioutil.ReadFile("data/script_tests.json")
	if err != nil {
		t.Fatalf("TestScripts: %v\n", err)
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Fatalf("TestScripts failed to unmarshal: %v", err)
	}

	// Run all script tests with and without the signature cache.
	testScripts(t, tests, true)
	testScripts(t, tests, false)
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
func parseSigHashExpectedResult(expected string) (*ErrorCode, error) {
	switch expected {
	case "OK":
		return nil, nil
	case "SIGHASH_SINGLE_IDX":
		code := ErrInvalidSigHashSingleIndex
		return &code, nil
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
		if (err == nil) != (expectedErr == nil) ||
			expectedErr != nil && !IsErrorCode(err, *expectedErr) {

			if serr, ok := err.(Error); ok {
				t.Errorf("Test #%d: want error code %v, got %v", i, expectedErr,
					serr.ErrorCode)
				continue
			}
			t.Errorf("Test #%d: want error code %v, got err: %v (%T)", i,
				expectedErr, err, err)
			continue
		}
		if !bytes.Equal(hash, expectedHash) {
			t.Errorf("Test #%d: signature hash mismatch - got %x, want %x", i,
				hash, expectedHash)
			continue
		}
	}
}
