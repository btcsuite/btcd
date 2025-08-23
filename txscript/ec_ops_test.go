// Copyright (c) 2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"
)

// ecOpsVerifyFlags defines the standard flags used for testing EC operations.
// This includes the standard verification flags, taproot, and EC operations.
const ecOpsVerifyFlags = StandardVerifyFlags |
	ScriptVerifyTaproot |
	ScriptVerifyECOps

// taprootInitialSigOpsBudget is the initial sigops budget for taproot scripts.
const taprootInitialSigOpsBudget = 50000

// ecOpTestVector represents a test case that can be serialized to JSON.
type ecOpTestVector struct {
	Tx       string        `json:"tx"`
	Prevouts []string      `json:"prevouts"`
	Index    int           `json:"index"`
	Flags    string        `json:"flags"`
	Comment  string        `json:"comment"`
	Success  *inputWitness `json:"success,omitempty"`
	Failure  *inputWitness `json:"failure,omitempty"`
}

// taprootSpendInfo contains all necessary components for a taproot spend.
type taprootSpendInfo struct {
	internalKey    *btcec.PrivateKey
	outputKey      *btcec.PublicKey
	controlBlock   *ControlBlock
	p2trScript     []byte
	ctrlBlockBytes []byte
}

// createTaprootSpendInfo creates all necessary components for a taproot spend.
func createTaprootSpendInfo(testScript []byte, testName string) (*taprootSpendInfo, error) {
	// Use deterministic key for taproot internal key when generating vectors
	internalKey, err := generateTestPrivateKey(testName, 0)
	if err != nil {
		return nil, err
	}

	tapLeaf := NewBaseTapLeaf(testScript)
	tapScriptTree := AssembleTaprootScriptTree(tapLeaf)

	rootHash := tapScriptTree.RootNode.TapHash()
	outputKey := ComputeTaprootOutputKey(internalKey.PubKey(), rootHash[:])

	ctrlBlock := tapScriptTree.LeafMerkleProofs[0].ToControlBlock(
		internalKey.PubKey(),
	)

	p2trScript, err := PayToTaprootScript(outputKey)
	if err != nil {
		return nil, err
	}

	ctrlBlockBytes, err := ctrlBlock.ToBytes()
	if err != nil {
		return nil, err
	}

	return &taprootSpendInfo{
		internalKey:    internalKey,
		outputKey:      outputKey,
		controlBlock:   &ctrlBlock,
		p2trScript:     p2trScript,
		ctrlBlockBytes: ctrlBlockBytes,
	}, nil
}

// createTestScriptFromTemplate is a helper that creates a script from a
// template with error handling.
func createTestScriptFromTemplate(t *testing.T,
	template string, params map[string]interface{}) []byte {

	t.Helper()
	script, err := ScriptTemplate(
		template, WithScriptTemplateParams(params),
	)
	require.NoError(t, err, "failed to create script from template")

	return script
}

// createTaprootSpendingTx creates a transaction that spends from a taproot
// output.
func createTaprootSpendingTx(testScript []byte,
	ctrlBlockBytes []byte) *wire.MsgTx {

	spendTx := wire.NewMsgTx(2)
	spendTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0,
		},
		Witness: wire.TxWitness{
			testScript,
			ctrlBlockBytes,
		},
	})
	spendTx.AddTxOut(&wire.TxOut{
		Value:    100000,
		PkScript: []byte{},
	})

	return spendTx
}

// generateECOpTestVector generates a test vector for EC opcode tests.
func generateECOpTestVector(t *testing.T, spendTx *wire.MsgTx,
	prevOut *wire.TxOut, testScript []byte, ctrlBlockBytes []byte,
	testName string, shouldFail bool) {

	t.Helper()

	// If the build tag isn't active, then this function does nothing and
	// will just return back to the caller.
	if !shouldGenerateVectors() {
		return
	}

	var txBuf bytes.Buffer
	if err := spendTx.Serialize(&txBuf); err != nil {
		return
	}

	prevOutHex, err := serializePrevOut(prevOut)
	require.NoError(t, err, "failed to serialize previous output")

	vector := ecOpTestVector{
		Tx:       hex.EncodeToString(txBuf.Bytes()),
		Prevouts: []string{prevOutHex},
		Index:    0,
		Flags:    "P2SH,WITNESS,TAPROOT,EC_OPS",
		Comment:  testName,
	}

	if shouldFail {
		vector.Failure = &inputWitness{
			ScriptSig: "",
			Witness: []string{
				hex.EncodeToString(testScript),
				hex.EncodeToString(ctrlBlockBytes),
			},
		}
	} else {
		vector.Success = &inputWitness{
			ScriptSig: "",
			Witness: []string{
				hex.EncodeToString(testScript),
				hex.EncodeToString(ctrlBlockBytes),
			},
		}
	}

	writeTestVector(vector, shouldFail)
}

// checkTestResult verifies the execution result matches expectations.
func checkTestResult(t *testing.T, err error, shouldFail bool,
	errorCode ErrorCode) {

	t.Helper()

	if shouldFail {
		require.Error(
			t, err, "expected script to fail but it succeeded",
		)

		if errorCode != 0 {
			require.True(
				t, IsErrorCode(err, errorCode),
				"expected error code %v, got %v", errorCode,
				err,
			)
		}
		return
	}

	require.NoError(t, err, "unexpected error")
}

// executeECOpTestWithVM is a variant that returns the VM for additional
// verification. This allows tests like TestECOpsSigOpsBudget to access
// internal state.
func executeECOpTestWithVM(t *testing.T, testScript []byte, testName string,
	shouldFail bool, errorCode ErrorCode, generateVector bool) (*Engine, error) {
	t.Helper()

	// Given the test script, first create a spendInfo bundle. This
	// contains all the information needed to generate and verify a spend.
	spendInfo, err := createTaprootSpendInfo(testScript, testName)
	require.NoError(t, err, "failed to create taproot spend info")

	// With the spend info, we can now create a spending transaction.
	spendTx := createTaprootSpendingTx(testScript, spendInfo.ctrlBlockBytes)

	// As final prep, we'll make a prev output fetcher that returns the
	// input we want to spend, then use that to init our VM instance.
	prevOut := wire.NewTxOut(100000, spendInfo.p2trScript)
	prevOutFetcher := NewCannedPrevOutputFetcher(
		spendInfo.p2trScript, 100000,
	)
	vm, err := NewEngine(
		spendInfo.p2trScript, spendTx, 0, ecOpsVerifyFlags, nil, nil,
		100000, prevOutFetcher,
	)
	require.NoError(t, err, "failed to create engine")

	execErr := vm.Execute()

	// If this test should be saved to disk as a test vector, then we'll
	// generate that now.
	if generateVector {
		generateECOpTestVector(
			t, spendTx, prevOut, testScript,
			spendInfo.ctrlBlockBytes, testName, shouldFail,
		)
	}

	// Either way, we'll check the error to make sure the test functions as
	// expected.
	checkTestResult(t, execErr, shouldFail, errorCode)

	return vm, execErr
}

// executeECOpTest is a helper function that handles the common logic for EC
// opcode tests. It creates proper taproot spending transactions and optionally
// generates test vectors.
func executeECOpTest(t *testing.T, testScript []byte, testName string,
	shouldFail bool, errorCode ErrorCode, generateVector bool) {

	t.Helper()

	executeECOpTestWithVM(
		t, testScript, testName, shouldFail, errorCode, generateVector,
	)
}

// TestECPointAdd tests the OP_EC_POINT_ADD opcode. This ensures that the op
// codes functions just as if the computations were performed with native
// code.
func TestECPointAdd(t *testing.T) {
	t.Parallel()

	// Create test private keys and compute public keys.
	// Use deterministic keys when generating test vectors.
	privKey1, _ := generateTestPrivateKey(t.Name(), 1)
	pubKey1 := privKey1.PubKey()
	privKey2, _ := generateTestPrivateKey(t.Name(), 2)
	pubKey2 := privKey2.PubKey()

	var point1, point2, expectedSum secp256k1.JacobianPoint
	pubKey1.AsJacobian(&point1)
	pubKey2.AsJacobian(&point2)
	secp256k1.AddNonConst(&point1, &point2, &expectedSum)

	expectedSum.ToAffine()

	expectedSumBytes := expectedSum.X.Bytes()[:]

	tests := []struct {
		name       string
		template   string
		params     map[string]interface{}
		shouldFail bool
		errorCode  ErrorCode
		testVector bool
	}{
		{
			name: "add two 33-byte points",
			template: `
			{{ hex .Point1 }} {{ hex .Point2 }} OP_EC_POINT_ADD
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Point1": pubKey1.SerializeCompressed(),
				"Point2": pubKey2.SerializeCompressed(),
				"Expected": func() []byte {
					if expectedSum.Y.IsOdd() {
						return append(
							[]byte{0x03},
							expectedSumBytes...,
						)
					}
					return append(
						[]byte{0x02},
						expectedSumBytes...,
					)
				}(),
			},
			testVector: true,
		},
		{
			name: "add point to itself (point doubling)",
			template: `
			{{ hex .Point }} {{ hex .Point }} OP_EC_POINT_ADD 
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Point": pubKey1.SerializeCompressed(),
				"Expected": func() []byte {
					var doubled secp256k1.JacobianPoint
					pubKey1.AsJacobian(&point1)
					secp256k1.DoubleNonConst(
						&point1, &doubled,
					)

					doubled.ToAffine()

					doubledBytes := doubled.X.Bytes()[:]

					if doubled.Y.IsOdd() {
						return append(
							[]byte{0x03},
							doubledBytes...,
						)
					}

					return append(
						[]byte{0x02}, doubledBytes...,
					)
				}(),
			},
			testVector: true,
		},
		{
			name: "add point and its negation (infinity)",
			template: `
			{{ hex .Point }} {{ hex .NegPoint }} OP_EC_POINT_ADD 
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Point": pubKey1.SerializeCompressed(),
				"NegPoint": func() []byte {
					var neg secp256k1.JacobianPoint
					pubKey1.AsJacobian(&neg)
					neg.Y.Negate(1).Normalize()
					neg.ToAffine()

					negBytes := neg.X.Bytes()[:]

					if neg.Y.IsOdd() {
						return append(
							[]byte{0x03},
							negBytes...,
						)
					}

					return append(
						[]byte{0x02},
						neg.X.Bytes()[:]...,
					)
				}(),
				"Expected": []byte{},
			},
			testVector: true,
		},
		{
			name:       "insufficient stack items",
			template:   `OP_EC_POINT_ADD`,
			params:     nil,
			shouldFail: true,
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},
		{
			name: "reject 32-byte x-only input",
			template: `
			{{ hex .Point1 }} {{ hex .Point2 }} OP_EC_POINT_ADD`,
			params: map[string]interface{}{
				"Point1": pubKey1.SerializeCompressed()[1:],
				"Point2": pubKey2.SerializeCompressed(),
			},
			shouldFail: true,
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},
		{
			name: "invalid point - x coordinate too large",
			template: `
			{{ hex .InvalidPoint }} {{ hex .Point2 }} 
			OP_EC_POINT_ADD`,
			params: map[string]interface{}{
				"InvalidPoint": append(
					[]byte{0x02}, bytes.Repeat(
						[]byte{0xff}, 32)...,
				),
				"Point2": pubKey2.SerializeCompressed(),
			},
			shouldFail: true,
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testScript := createTestScriptFromTemplate(
				t, test.template, test.params,
			)

			executeECOpTest(
				t, testScript, test.name, test.shouldFail,
				test.errorCode, test.testVector,
			)
		})
	}
}

// TestECPointMul tests the OP_EC_POINT_MUL opcode.
func TestECPointMul(t *testing.T) {
	t.Parallel()

	// Create test keys and scalars for the test.
	// Use deterministic keys when generating test vectors.
	privKey, _ := generateTestPrivateKey(t.Name(), 1)
	pubKey := privKey.PubKey()

	// We'll make a test scalar, which is just 2 in this case.
	scalar := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
	}

	tests := []struct {
		name       string
		template   string
		params     map[string]interface{}
		shouldFail bool
		errorCode  ErrorCode
		testVector bool
	}{
		{
			name: "multiply point by 2",
			template: `
			{{ hex .Scalar }} {{ hex .Point }} OP_EC_POINT_MUL
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Point":  pubKey.SerializeCompressed(),
				"Scalar": scalar,
				"Expected": func() []byte {
					var (
						point   secp256k1.JacobianPoint
						doubled secp256k1.JacobianPoint
					)

					pubKey.AsJacobian(&point)
					secp256k1.DoubleNonConst(
						&point, &doubled,
					)
					doubled.ToAffine()

					doubledBytes := doubled.X.Bytes()[:]

					if doubled.Y.IsOdd() {
						return append(
							[]byte{0x03},
							doubledBytes...,
						)
					}

					return append(
						[]byte{0x02},
						doubledBytes[:]...,
					)
				}(),
			},
			testVector: true,
		},
		{
			name: "multiply generator by scalar (empty point)",
			template: `
			{{ hex .Scalar }} {{ hex .Point }} OP_EC_POINT_MUL
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Scalar": scalar,
				"Point":  []byte{},
				"Expected": func() []byte {
					var scalar secp256k1.ModNScalar
					scalar.SetInt(2)
					result := new(secp256k1.JacobianPoint)
					secp256k1.ScalarBaseMultNonConst(
						&scalar, result,
					)

					result.ToAffine()

					resBytes := result.X.Bytes()[:]

					if result.Y.IsOdd() {
						return append(
							[]byte{0x03}, resBytes...,
						)
					}

					return append([]byte{0x02}, resBytes...)
				}(),
			},
			testVector: true,
		},
		{
			name: "multiply by zero (infinity)",
			template: `
			{{ hex .Scalar }} {{ hex .Point }} OP_EC_POINT_MUL 
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Point":    pubKey.SerializeCompressed(),
				"Scalar":   bytes.Repeat([]byte{0x00}, 32),
				"Expected": []byte{},
			},
			testVector: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testScript := createTestScriptFromTemplate(
				t, test.template, test.params,
			)

			executeECOpTest(
				t, testScript, test.name, test.shouldFail,
				test.errorCode, test.testVector,
			)
		})
	}
}

// TestECPointNegate tests the OP_EC_POINT_NEGATE opcode.
func TestECPointNegate(t *testing.T) {
	t.Parallel()

	// Use deterministic keys when generating test vectors.
	privKey, _ := generateTestPrivateKey(t.Name(), 1)
	pubKey := privKey.PubKey()

	tests := []struct {
		name       string
		template   string
		params     map[string]interface{}
		shouldFail bool
		errorCode  ErrorCode
		testVector bool
	}{
		{
			name: "negate point",
			template: `
			{{ hex .Point }} OP_EC_POINT_NEGATE 
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Point": pubKey.SerializeCompressed(),
				"Expected": func() []byte {
					var point secp256k1.JacobianPoint
					pubKey.AsJacobian(&point)
					point.Y.Negate(1).Normalize()
					point.ToAffine()

					pointBytes := point.X.Bytes()[:]

					if point.Y.IsOdd() {
						return append(
							[]byte{0x03},
							pointBytes...,
						)
					}

					return append(
						[]byte{0x02}, pointBytes...,
					)
				}(),
			},
			testVector: true,
		},
		{
			name:       "negate infinity",
			template:   "OP_EC_POINT_NEGATE",
			params:     nil,
			shouldFail: true,
			testVector: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testScript := createTestScriptFromTemplate(
				t, test.template, test.params,
			)

			executeECOpTest(
				t, testScript, test.name, test.shouldFail,
				test.errorCode, test.testVector,
			)
		})
	}
}

// TestECPointXCoord tests the OP_EC_POINT_X_COORD opcode.
func TestECPointXCoord(t *testing.T) {
	t.Parallel()

	// Use deterministic keys when generating test vectors.
	privKey, _ := generateTestPrivateKey(t.Name(), 1)
	pubKey := privKey.PubKey()

	tests := []struct {
		name       string
		template   string
		params     map[string]interface{}
		shouldFail bool
		errorCode  ErrorCode
		testVector bool
	}{
		{
			name: "extract x from 32-byte point",
			template: `
			{{ hex .Point }} OP_EC_POINT_X_COORD 
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Point":    pubKey.SerializeCompressed()[1:],
				"Expected": pubKey.SerializeCompressed()[1:],
			},
			// 32-byte points are not accepted.
			shouldFail: true,
			testVector: true,
		},
		{
			name: "extract x from 33-byte point",
			template: `
			{{ hex .Point }} OP_EC_POINT_X_COORD 
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Expected": pubKey.SerializeCompressed()[1:],
				"Point":    pubKey.SerializeCompressed(),
			},
			testVector: true,
		},
		{
			name:       "extract x from infinity fails",
			template:   "OP_EC_POINT_X_COORD",
			params:     nil,
			shouldFail: true,
			testVector: true,
		},
		{
			name:     "invalid point",
			template: "{{ hex .Point }} OP_EC_POINT_X_COORD",
			params: map[string]interface{}{
				"Point": append(
					[]byte{0x02}, bytes.Repeat(
						[]byte{0xff}, 32,
					)...,
				),
			},
			shouldFail: true,
			testVector: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testScript := createTestScriptFromTemplate(
				t, test.template, test.params,
			)

			executeECOpTest(
				t, testScript, test.name, test.shouldFail,
				test.errorCode, test.testVector,
			)
		})
	}
}

// TestECOpsSigOpsBudget tests that the opcodes properly consume sigops budget.
func TestECOpsSigOpsBudget(t *testing.T) {
	t.Parallel()

	// Use deterministic keys when generating test vectors.
	privKey, _ := generateTestPrivateKey(t.Name(), 1)
	pubKey := privKey.PubKey()
	point33 := pubKey.SerializeCompressed()
	scalar := bytes.Repeat([]byte{0x00}, 31)
	scalar = append(scalar, 0x02)

	tests := []struct {
		name          string
		template      string
		params        map[string]interface{}
		expectedDelta int
		shouldFail    bool
		errorCode     ErrorCode
		testVector    bool
	}{
		{
			name: "OP_EC_POINT_ADD consumes 10 units",
			template: `
			{{ hex .Point1 }} {{ hex .Point2 }} OP_EC_POINT_ADD OP_DROP OP_1`,
			params: map[string]interface{}{
				"Point1": point33,
				"Point2": point33,
			},
			expectedDelta: EcPointAddCost,
			testVector:    true,
		},
		{
			name: "OP_EC_POINT_MUL consumes 30 units",
			template: `
			{{ hex .Scalar }} {{ hex .Point }} OP_EC_POINT_MUL OP_DROP OP_1`,
			params: map[string]interface{}{
				"Scalar": scalar,
				"Point":  point33,
			},
			expectedDelta: EcPointMulCost,
		},
		{
			name:     "OP_EC_POINT_NEGATE consumes 5 units",
			template: "{{ hex .Point }} OP_EC_POINT_NEGATE OP_DROP OP_1",
			params: map[string]interface{}{
				"Point": point33,
			},
			expectedDelta: EcPointNegateCost,
		},
		{
			name:     "OP_EC_POINT_X_COORD consumes 1 unit",
			template: "{{ hex .Point }} OP_EC_POINT_X_COORD OP_DROP OP_1",
			params: map[string]interface{}{
				"Point": point33,
			},
			expectedDelta: EcPointXCoordCost,
		},
		{
			// This test verifies that multiple EC operations
			// correctly consume the sigops budget cumulatively.
			// The operations are:
			// 1. OP_EC_POINT_MUL: 30 units
			// 2. OP_EC_POINT_NEGATE: 5 units
			// 3. OP_EC_POINT_ADD: 10 units
			// 4. OP_EC_POINT_X_COORD: 1 unit
			// Total: 46 units
			name: "multiple operations consume budget correctly",
			template: `
			{{ hex .Scalar }} {{ hex .EmptyPoint }} OP_EC_POINT_MUL 
			OP_EC_POINT_NEGATE 
			{{ hex .Point }} OP_EC_POINT_ADD 
			OP_EC_POINT_X_COORD OP_DROP OP_1`,
			params: map[string]interface{}{
				"Scalar":     scalar,
				"EmptyPoint": []byte{},
				"Point":      point33,
			},
			expectedDelta: EcPointMulCost + EcPointNegateCost + EcPointAddCost + EcPointXCoordCost,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create teh script template, then execute the VM test.
			testScript := createTestScriptFromTemplate(
				t, test.template, test.params,
			)
			vm, _ := executeECOpTestWithVM(
				t, testScript, test.name, test.shouldFail,
				test.errorCode, test.testVector,
			)

			if test.shouldFail {
				return
			}

			// If test should pass, verify budget consumption.
			//
			// Calculate the expected budgets based on
			// witness size.
			witnessSize := vm.tx.TxIn[0].Witness.SerializeSize()
			expectedInitialBudget := int32(
				sigOpsDelta + witnessSize,
			)
			expectedFinalBudget := expectedInitialBudget - int32(
				test.expectedDelta,
			)

			// Verify budget consumption - we have access
			// to internal fields in tests.
			require.NotNil(
				t, vm.taprootCtx, "taproot context "+
					"should be initialized",
			)

			// Verify the actual remaining budget matches
			// our expectation.
			actualFinalBudget := vm.taprootCtx.sigOpsBudget
			require.Equal(
				t, expectedFinalBudget,
				actualFinalBudget,
				"budget mismatch: expected %d "+
					"remaining, got %d (consumed "+
					"%d instead of %d)",
				expectedFinalBudget, actualFinalBudget,
				expectedInitialBudget-actualFinalBudget,
				test.expectedDelta,
			)

			// Verify the consumed amount exactly matches
			// the expected delta.
			actualConsumed := (expectedInitialBudget -
				actualFinalBudget)
			require.Equal(
				t, int32(test.expectedDelta), actualConsumed,
				"consumed budget mismatch: expected to "+
					"consume %d, actually consumed %d",
				test.expectedDelta, actualConsumed,
			)

			// Log the budget details for debugging.
			t.Logf("Witness size: %d bytes", witnessSize)
			t.Logf("Initial budget: %d (50 + %d)",
				expectedInitialBudget, witnessSize)
			t.Logf("Expected delta: %d", test.expectedDelta)
			t.Logf("Final budget: %d", actualFinalBudget)
			t.Logf("Actual consumed: %d", actualConsumed)
		})
	}
}

// TestECOpsExamples tests examples of some common use cases.
func TestECOpsExamples(t *testing.T) {
	t.Parallel()

	// Use deterministic keys when generating test vectors.
	privKey, _ := generateTestPrivateKey(t.Name(), 1)
	internalKey := privKey.PubKey()

	tweak := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
	}

	tests := []struct {
		name       string
		template   string
		params     map[string]interface{}
		testVector bool
	}{
		{
			name: "Computing a Taproot Tweak (P + tweak*G)",
			template: `
			{{ hex .Tweak }} {{ hex .EmptyVector }} OP_EC_POINT_MUL 
			{{ hex .P }} OP_EC_POINT_ADD {{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"Tweak":       tweak,
				"EmptyVector": []byte{},
				"P":           internalKey.SerializeCompressed(),
				"Expected": func() []byte {
					// First, we compute t*G.
					var tweakScalar secp256k1.ModNScalar
					tweakScalar.SetBytes((*[32]byte)(tweak))
					var tweakedPoint, tempPoint secp256k1.JacobianPoint
					secp256k1.ScalarBaseMultNonConst(
						&tweakScalar, &tempPoint,
					)

					// Finally we 'll add t*G to P.
					var internalPoint secp256k1.JacobianPoint
					internalKey.AsJacobian(&internalPoint)
					secp256k1.AddNonConst(
						&internalPoint, &tempPoint,
						&tweakedPoint,
					)

					tweakedPoint.ToAffine()

					tBytes := tweakedPoint.X.Bytes()[:]

					if tweakedPoint.Y.IsOdd() {
						return append(
							[]byte{0x03}, tBytes...,
						)
					}

					return append([]byte{0x02}, tBytes...)
				}(),
			},
			testVector: true,
		},
		{
			name: "Point Doubling (2*P)",
			template: `
			{{ hex .P }} OP_DUP OP_EC_POINT_ADD 
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"P": internalKey.SerializeCompressed(),
				"Expected": func() []byte {
					var (
						point   secp256k1.JacobianPoint
						doubled secp256k1.JacobianPoint
					)
					internalKey.AsJacobian(&point)
					secp256k1.DoubleNonConst(
						&point, &doubled,
					)

					doubled.ToAffine()

					dBytes := doubled.X.Bytes()[:]

					if doubled.Y.IsOdd() {
						return append(
							[]byte{0x03}, dBytes...,
						)
					}

					return append([]byte{0x02}, dBytes...)
				}(),
			},
		},
		{
			name: "X-Coordinate Extraction",
			template: `
			{{ hex .P }} OP_EC_POINT_X_COORD 
			{{ hex .Expected }} OP_EQUAL`,
			params: map[string]interface{}{
				"P":        internalKey.SerializeCompressed(),
				"Expected": internalKey.SerializeCompressed()[1:],
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testScript := createTestScriptFromTemplate(
				t, test.template, test.params,
			)

			executeECOpTest(
				t, testScript, test.name, false, 0,
				test.testVector,
			)
		})
	}
}

// TestECOpsNonTaproot tests that EC opcodes fail in non-taproot contexts.
func TestECOpsNonTaproot(t *testing.T) {
	t.Parallel()

	// Create test data - use deterministic keys when generating test vectors.
	privKey, _ := generateTestPrivateKey(t.Name(), 1)
	pubKey := privKey.PubKey()
	point33 := pubKey.SerializeCompressed()

	opcodes := []struct {
		name     string
		template string
		params   map[string]interface{}
	}{
		{
			name: "OP_EC_POINT_ADD in non-taproot",
			template: `
			{{ hex .Point1 }} {{ hex .Point2 }} OP_EC_POINT_ADD`,
			params: map[string]interface{}{
				"Point1": point33,
				"Point2": point33,
			},
		},
		{
			name: "OP_EC_POINT_MUL in non-taproot",
			template: `
			{{ hex .Scalar }} {{ hex .Point }} OP_EC_POINT_MUL`,
			params: map[string]interface{}{
				"Scalar": bytes.Repeat([]byte{0x00}, 32),
				"Point":  point33,
			},
		},
		{
			name:     "OP_EC_POINT_NEGATE in non-taproot",
			template: "{{ hex .Point }} OP_EC_POINT_NEGATE",
			params: map[string]interface{}{
				"Point": point33,
			},
		},
		{
			name:     "OP_EC_POINT_X_COORD in non-taproot",
			template: "{{ hex .Point }} OP_EC_POINT_X_COORD",
			params: map[string]interface{}{
				"Point": point33,
			},
		},
	}

	for _, test := range opcodes {
		t.Run(test.name, func(t *testing.T) {
			script := createTestScriptFromTemplate(
				t, test.template, test.params,
			)

			tx := &wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Index: 0,
					},
				}},
			}

			// Create engine WITHOUT taproot flag.
			vm, err := NewEngine(
				script, tx, 0,
				StandardVerifyFlags,
				nil, nil, 0, nil,
			)
			require.NoError(t, err, "failed to create engine")

			err = vm.Execute()

			require.Error(
				t, err, "expected script to fail "+
					"in non-taproot context but it succeeded",
			)

			// Should fail with disabled opcode error
			require.True(
				t, IsErrorCode(err, ErrDisabledOpcode),
				"expected ErrDisabledOpcode, got %v", err,
			)
		})
	}
}

// TestECOpsErrors tests error/edge cases for the EC opcodes.
func TestECOpsErrors(t *testing.T) {
	t.Parallel()

	// Create valid test keys for use in error tests.
	// Use deterministic keys when generating test vectors.
	privKey1, _ := generateTestPrivateKey(t.Name(), 1)
	pubKey1 := privKey1.PubKey()
	privKey2, _ := generateTestPrivateKey(t.Name(), 2)
	pubKey2 := privKey2.PubKey()

	tests := []struct {
		name       string
		template   string
		params     map[string]interface{}
		errorCode  ErrorCode
		testVector bool
	}{
		// OP_EC_POINT_ADD errors.
		{
			name: "ADD: budget exceeded",
			// Keep script small to minimize budget, maximize
			// operations. Push two points once, then repeatedly
			// DUP and ADD.
			//
			// Script size: 2*33 (points) + 300*3 (DUP DUP ADD) =
			// 966 bytes
			//
			// Budget: 50 + 966 = 1016.
			// Operations: 300 ADDs = 3000 units.
			template: `
			{{ hex .Point1 }} {{ hex .Point2 }}
			{{- range .Loop }}
			OP_2DUP OP_EC_POINT_ADD OP_DROP
			{{- end }}
			OP_EC_POINT_ADD OP_DROP OP_1`,
			params: map[string]interface{}{
				"Point1": pubKey1.SerializeCompressed(),
				"Point2": pubKey2.SerializeCompressed(),
				// 300 ADDs = 3000 units consumed.
				// Script ~1000 bytes, budget ~1050>
				"Loop": make([]int, 300),
			},
			errorCode:  ErrTaprootMaxSigOps,
			testVector: true,
		},
		{
			name: "ADD: invalid point encoding length",
			template: `
			{{ hex .Point1 }} {{ hex .Point2 }} OP_EC_POINT_ADD`,
			params: map[string]interface{}{
				"Point1": []byte{0x02, 0x03},
				"Point2": pubKey2.SerializeCompressed(),
			},
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},

		// OP_EC_POINT_MUL errors.
		{
			name: "MUL: budget exceeded",
			// Keep script small to minimize budget, maximize
			// operations. Push scalar and point once, then
			// repeatedly DUP and MUL.
			//
			// Script size: 32 + 33 (scalar+point) + 100*4 (2DUP
			// MUL DROP) = 465 bytes.
			//
			// Budget: 50 + 465 = 515.
			// Operations: 100 MULs = 3000 units.
			// This exceeds the budget
			template: `
			{{ hex .Scalar }} {{ hex .Point }}
			{{- range .Loop }}
			OP_2DUP OP_EC_POINT_MUL OP_DROP
			{{- end }}
			OP_EC_POINT_MUL OP_DROP OP_1`,
			params: map[string]interface{}{
				"Scalar": append(
					bytes.Repeat([]byte{0x00}, 31), 0x02,
				),
				"Point": pubKey1.SerializeCompressed(),
				// 100 MULs = 3000 units consumed.
				// Script ~465 bytes, budget ~515>
				"Loop": make([]int, 100),
			},
			errorCode:  ErrTaprootMaxSigOps,
			testVector: true,
		},
		{
			name: "MUL: invalid scalar length",
			template: `
			{{ hex .Scalar }} {{ hex .Point }} OP_EC_POINT_MUL`,
			params: map[string]interface{}{
				"Scalar": []byte{0x01, 0x02, 0x03},
				"Point":  pubKey1.SerializeCompressed(),
			},
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},
		{
			name:     "MUL: insufficient stack",
			template: "{{ hex .Scalar }} OP_EC_POINT_MUL",
			params: map[string]interface{}{
				"Scalar": make([]byte, 32),
			},
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},

		// OP_EC_POINT_NEGATE errors.
		{
			name: "NEGATE: budget exceeded",
			// Do enough negates to exceed the budget naturally,
			// Each NEGATE costs 5 units.
			template: `
			{{ hex .Point }}
			{{- range .Loop }}
			OP_EC_POINT_NEGATE
			{{- end }}`,
			params: map[string]interface{}{
				"Point": pubKey1.SerializeCompressed(),
				"Loop":  make([]int, 500),
			},
			errorCode:  ErrTaprootMaxSigOps,
			testVector: true,
		},
		{
			name:       "NEGATE: insufficient stack",
			template:   "OP_EC_POINT_NEGATE",
			params:     nil,
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},

		// OP_EC_POINT_X_COORD errors.
		{
			name:     "X_COORD: point at infinity",
			template: "{{ hex .Point }} OP_EC_POINT_X_COORD",
			params: map[string]interface{}{
				"Point": []byte{},
			},
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},
		{
			name:       "X_COORD: insufficient stack",
			template:   "OP_EC_POINT_X_COORD",
			params:     nil,
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},
		{
			name:     "X_COORD: invalid point",
			template: "{{ hex .Point }} OP_EC_POINT_X_COORD",
			params: map[string]interface{}{
				"Point": []byte{0x04},
			},
			errorCode:  ErrInvalidStackOperation,
			testVector: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testScript := createTestScriptFromTemplate(
				t, test.template, test.params,
			)

			executeECOpTest(
				t, testScript, test.name, true,
				test.errorCode, test.testVector,
			)
		})
	}
}
