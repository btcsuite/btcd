// Copyright (c) 2018-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

var (
	// manyInputsBenchTx is a transaction that contains a lot of inputs which is
	// useful for benchmarking signature hash calculation.
	manyInputsBenchTx wire.MsgTx

	// A mock previous output script to use in the signing benchmark.
	prevOutScript = hexToBytes("a914f5916158e3e2c4551c1796708db8367207ed13bb87")
)

func init() {
	// tx 620f57c92cf05a7f7e7f7d28255d5f7089437bc48e34dcfebf7751d08b7fb8f5
	txHex, err := ioutil.ReadFile("data/many_inputs_tx.hex")
	if err != nil {
		panic(fmt.Sprintf("unable to read benchmark tx file: %v", err))
	}

	txBytes := hexToBytes(string(txHex))
	err = manyInputsBenchTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		panic(err)
	}
}

// BenchmarkCalcSigHash benchmarks how long it takes to calculate the signature
// hashes for all inputs of a transaction with many inputs.
func BenchmarkCalcSigHash(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(manyInputsBenchTx.TxIn); j++ {
			_, err := CalcSignatureHash(prevOutScript, SigHashAll,
				&manyInputsBenchTx, j)
			if err != nil {
				b.Fatalf("failed to calc signature hash: %v", err)
			}
		}
	}
}

// BenchmarkCalcWitnessSigHash benchmarks how long it takes to calculate the
// witness signature hashes for all inputs of a transaction with many inputs.
func BenchmarkCalcWitnessSigHash(b *testing.B) {
	sigHashes := NewTxSigHashes(&manyInputsBenchTx)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(manyInputsBenchTx.TxIn); j++ {
			_, err := CalcWitnessSigHash(
				prevOutScript, sigHashes, SigHashAll,
				&manyInputsBenchTx, j, 5,
			)
			if err != nil {
				b.Fatalf("failed to calc signature hash: %v", err)
			}
		}
	}
}

// genComplexScript returns a script comprised of half as many opcodes as the
// maximum allowed followed by as many max size data pushes fit without
// exceeding the max allowed script size.
func genComplexScript() ([]byte, error) {
	var scriptLen int
	builder := NewScriptBuilder()
	for i := 0; i < MaxOpsPerScript/2; i++ {
		builder.AddOp(OP_TRUE)
		scriptLen++
	}
	maxData := bytes.Repeat([]byte{0x02}, MaxScriptElementSize)
	for i := 0; i < (MaxScriptSize-scriptLen)/(MaxScriptElementSize+3); i++ {
		builder.AddData(maxData)
	}
	return builder.Script()
}

// BenchmarkScriptParsing benchmarks how long it takes to parse a very large
// script.
func BenchmarkScriptParsing(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	const scriptVersion = 0
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tokenizer := MakeScriptTokenizer(scriptVersion, script)
		for tokenizer.Next() {
			_ = tokenizer.Opcode()
			_ = tokenizer.Data()
			_ = tokenizer.ByteIndex()
		}
		if err := tokenizer.Err(); err != nil {
			b.Fatalf("failed to parse script: %v", err)
		}
	}
}

// BenchmarkDisasmString benchmarks how long it takes to disassemble a very
// large script.
func BenchmarkDisasmString(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := DisasmString(script)
		if err != nil {
			b.Fatalf("failed to disasm script: %v", err)
		}
	}
}

// BenchmarkIsPubKeyScript benchmarks how long it takes to analyze a very large
// script to determine if it is a standard pay-to-pubkey script.
func BenchmarkIsPubKeyScript(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = IsPayToPubKey(script)
	}
}

// BenchmarkIsPubKeyHashScript benchmarks how long it takes to analyze a very
// large script to determine if it is a standard pay-to-pubkey-hash script.
func BenchmarkIsPubKeyHashScript(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = IsPayToPubKeyHash(script)
	}
}

// BenchmarkIsPayToScriptHash benchmarks how long it takes IsPayToScriptHash to
// analyze a very large script.
func BenchmarkIsPayToScriptHash(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = IsPayToScriptHash(script)
	}
}

// BenchmarkIsMultisigScriptLarge benchmarks how long it takes IsMultisigScript
// to analyze a very large script.
func BenchmarkIsMultisigScriptLarge(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		isMultisig, err := IsMultisigScript(script)
		if err != nil {
			b.Fatalf("unexpected err: %v", err)
		}
		if isMultisig {
			b.Fatalf("script should NOT be reported as mutisig script")
		}
	}
}

// BenchmarkIsMultisigScript benchmarks how long it takes IsMultisigScript to
// analyze a 1-of-2 multisig public key script.
func BenchmarkIsMultisigScript(b *testing.B) {
	multisigShortForm := "1 " +
		"DATA_33 " +
		"0x030478aaaa2be30772f1e69e581610f1840b3cf2fe7228ee0281cd599e5746f81e " +
		"DATA_33 " +
		"0x0284f4d078b236a9ff91661f8ffbe012737cd3507566f30fd97d25f2b23539f3cd " +
		"2 CHECKMULTISIG"
	pkScript := mustParseShortForm(multisigShortForm)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		isMultisig, err := IsMultisigScript(pkScript)
		if err != nil {
			b.Fatalf("unexpected err: %v", err)
		}
		if !isMultisig {
			b.Fatalf("script should be reported as a mutisig script")
		}
	}
}
