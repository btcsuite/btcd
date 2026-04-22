// Regression tests for previously-reported btcd vs. Bitcoin Core consensus
// divergences in nested P2SH witness handling.
package txscript

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
)

// consensusBlockFlags matches the flag set used by the consensus-path
// regressions in this file.
func consensusBlockFlags() ScriptFlags {
	return ScriptBip16 | ScriptVerifyWitness | ScriptStrictMultiSig
}

// buildSpendingTx creates a minimal transaction whose first input spends the
// supplied scriptSig and witness pair.
func buildSpendingTx(sigScript []byte, witness wire.TxWitness) *wire.MsgTx {
	tx := &wire.MsgTx{Version: 1}
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0},
		SignatureScript:  sigScript,
		Witness:          witness,
		Sequence:         0xffffffff,
	})
	tx.AddTxOut(&wire.TxOut{Value: 0, PkScript: []byte{0x51}})
	return tx
}

// p2shPkScript wraps the provided redeem script in a standard P2SH output.
func p2shPkScript(redeem []byte) []byte {
	h := btcutil.Hash160(redeem)
	pk := make([]byte, 0, 23)
	pk = append(pk, 0xa9, 0x14)
	pk = append(pk, h...)
	pk = append(pk, 0x87)
	return pk
}

// mustExecuteErrorIs runs the VM and asserts the returned script error code.
func mustExecuteErrorIs(t *testing.T, vm *Engine, want ErrorCode,
	label string) {

	t.Helper()
	err := vm.Execute()
	if err == nil {
		t.Fatalf("%s: expected rejection (code=%v), got success", label, want)
	}
	var se Error
	if !errors.As(err, &se) {
		t.Fatalf("%s: expected scriptError (code=%v), got %T: %v",
			label, want, err, err)
	}
	if se.ErrorCode != want {
		t.Fatalf("%s: expected error code %v, got %v (%v)",
			label, want, se.ErrorCode, err)
	}
}

func TestRegression_EmptyNestedP2SHWitness_P2WPKH(t *testing.T) {
	keyHash := make([]byte, 20)
	for i := range keyHash {
		keyHash[i] = 0x42
	}

	redeem := make([]byte, 0, 22)
	redeem = append(redeem, 0x00, 0x14)
	redeem = append(redeem, keyHash...)

	sigScript := make([]byte, 0, 23)
	sigScript = append(sigScript, 0x16)
	sigScript = append(sigScript, redeem...)

	pkScript := p2shPkScript(redeem)
	tx := buildSpendingTx(sigScript, nil)

	vm, err := NewEngine(
		pkScript, tx, 0, consensusBlockFlags(), nil, nil, 0, nil,
	)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	mustExecuteErrorIs(t, vm, ErrWitnessProgramMismatch,
		"empty-witness P2SH-P2WPKH")
}

func TestRegression_EmptyNestedP2SHWitness_P2WSH(t *testing.T) {
	scriptHash := make([]byte, 32)
	for i := range scriptHash {
		scriptHash[i] = 0x42
	}

	redeem := make([]byte, 0, 34)
	redeem = append(redeem, 0x00, 0x20)
	redeem = append(redeem, scriptHash...)

	sigScript := make([]byte, 0, 35)
	sigScript = append(sigScript, 0x22)
	sigScript = append(sigScript, redeem...)

	pkScript := p2shPkScript(redeem)
	tx := buildSpendingTx(sigScript, nil)

	vm, err := NewEngine(
		pkScript, tx, 0, consensusBlockFlags(), nil, nil, 0, nil,
	)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	mustExecuteErrorIs(t, vm, ErrWitnessProgramEmpty,
		"empty-witness P2SH-P2WSH")
}

func TestRegression_P2SHWitnessPrefix(t *testing.T) {
	redeem := make([]byte, 32)
	for i := range redeem {
		redeem[i] = 0x61
	}

	// This stays push-only, but the final pushed element is not a witness
	// program even though the bytes after the first push opcode resemble one.
	sigScript := []byte{0x01, 0x51, 0x20}
	sigScript = append(sigScript, redeem...)

	pkScript := p2shPkScript(redeem)
	witness := wire.TxWitness{{}}
	tx := buildSpendingTx(sigScript, witness)

	_, err := NewEngine(
		pkScript, tx, 0, consensusBlockFlags(), nil, nil, 0, nil,
	)
	if err == nil {
		t.Fatalf("expected NewEngine to reject non-empty witness on non-witness P2SH, got success")
	}
	var se Error
	if !errors.As(err, &se) {
		t.Fatalf("expected scriptError, got %T: %v", err, err)
	}
	if se.ErrorCode != ErrWitnessUnexpected {
		t.Fatalf("expected ErrWitnessUnexpected, got %v (%v)",
			se.ErrorCode, err)
	}
}

func TestRegression_FakeNestedP2WPKHSigopCount(t *testing.T) {
	redeem, err := hex.DecodeString("7551616161616161616161616161616161616161")
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}

	// The bytes after the first push look like a v0 key-hash witness program,
	// but the actual redeem script is the final pushed element above.
	sigScript := []byte{0x01, 0x00, 0x14}
	sigScript = append(sigScript, redeem...)
	pkScript := p2shPkScript(redeem)

	gotSigOps := GetWitnessSigOpCount(sigScript, pkScript, nil)
	if gotSigOps != 0 {
		t.Fatalf("expected 0 witness sigops, got %d", gotSigOps)
	}

	tx := buildSpendingTx(sigScript, nil)
	vm, err := NewEngine(
		pkScript, tx, 0, consensusBlockFlags(), nil, nil, 0, nil,
	)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	if err := vm.Execute(); err != nil {
		t.Fatalf("expected legacy P2SH spend to succeed, got: %v", err)
	}
}

func TestRegression_HonestNestedP2WPKHHappyPath(t *testing.T) {
	keyHash := make([]byte, 20)
	for i := range keyHash {
		keyHash[i] = 0x99
	}

	redeem := make([]byte, 0, 22)
	redeem = append(redeem, 0x00, 0x14)
	redeem = append(redeem, keyHash...)

	sigScript := make([]byte, 0, 23)
	sigScript = append(sigScript, 0x16)
	sigScript = append(sigScript, redeem...)

	pkScript := p2shPkScript(redeem)
	witness := wire.TxWitness{{0x00}, {0x00}}
	tx := buildSpendingTx(sigScript, witness)

	vm, err := NewEngine(
		pkScript, tx, 0, consensusBlockFlags(), nil, nil, 0, nil,
	)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	err = vm.Execute()
	if err == nil {
		return
	}
	var se Error
	if errors.As(err, &se) {
		if se.ErrorCode == ErrWitnessProgramMismatch ||
			se.ErrorCode == ErrWitnessProgramEmpty ||
			se.ErrorCode == ErrWitnessMalleatedP2SH {

			t.Fatalf("fix broke the happy path: got %v", err)
		}
	}
}

func TestRegression_NonCanonicalNestedP2WPKHPush(t *testing.T) {
	keyHash := make([]byte, 20)
	for i := range keyHash {
		keyHash[i] = 0x11
	}

	redeem := make([]byte, 0, 22)
	redeem = append(redeem, 0x00, 0x14)
	redeem = append(redeem, keyHash...)

	// PUSHDATA1 makes this a non-canonical single push for a 22-byte redeem
	// script, so nested witness recognition must reject it as malleated.
	sigScript := make([]byte, 0, 24)
	sigScript = append(sigScript, 0x4c, 0x16)
	sigScript = append(sigScript, redeem...)
	pkScript := p2shPkScript(redeem)
	tx := buildSpendingTx(sigScript, nil)

	_, err := NewEngine(
		pkScript, tx, 0, consensusBlockFlags(), nil, nil, 0, nil,
	)
	if err == nil {
		t.Fatalf("expected NewEngine to reject non-canonical nested P2SH witness push, got success")
	}
	var se Error
	if !errors.As(err, &se) {
		t.Fatalf("expected scriptError, got %T: %v", err, err)
	}
	if se.ErrorCode != ErrWitnessMalleatedP2SH {
		t.Fatalf("expected ErrWitnessMalleatedP2SH, got %v (%v)",
			se.ErrorCode, err)
	}
}
