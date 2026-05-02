// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// This example demonstrates how to create a new PSBT (Partially Signed Bitcoin
// Transaction) from a set of inputs and outputs.
func ExampleNew() {
	// Create a previous outpoint (e.g. from a UTXO to spend).
	txHash, _ := chainhash.NewHashFromStr(
		"0000000000000000000000000000000000000000000000000000000000000000")
	prevOut := wire.NewOutPoint(txHash, 0)

	// Create an output paying 100000 satoshis to a P2PKH script.
	scriptPubKey, _ := hex.DecodeString("76a914" +
		"0000000000000000000000000000000000000000" + "88ac")
	out := wire.NewTxOut(100000, scriptPubKey)

	p, err := psbt.New(
		[]*wire.OutPoint{prevOut},
		[]*wire.TxOut{out},
		2, // transaction version
		0,
		[]uint32{wire.MaxTxInSequenceNum},
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	encoded, _ := p.B64Encode()
	fmt.Println("PSBT created, length:", len(encoded))
	fmt.Println("Inputs:", len(p.UnsignedTx.TxIn))
	fmt.Println("Outputs:", len(p.UnsignedTx.TxOut))

	// Output:
	// PSBT created, length: 128
	// Inputs: 1
	// Outputs: 1
}

// This example demonstrates how to decode a PSBT from base64 and inspect it.
func ExampleNewFromRawBytes() {
	// Create a minimal PSBT and encode it so we have valid base64 to decode.
	txHash, _ := chainhash.NewHashFromStr(
		"0000000000000000000000000000000000000000000000000000000000000000")
	prevOut := wire.NewOutPoint(txHash, 0)
	scriptPubKey, _ := hex.DecodeString("76a914" +
		"0000000000000000000000000000000000000000" + "88ac")
	out := wire.NewTxOut(100000, scriptPubKey)
	create, _ := psbt.New(
		[]*wire.OutPoint{prevOut},
		[]*wire.TxOut{out},
		2, 0,
		[]uint32{wire.MaxTxInSequenceNum},
	)
	base64PSBT, _ := create.B64Encode()

	p, err := psbt.NewFromRawBytes(strings.NewReader(base64PSBT), true)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Version:", p.UnsignedTx.Version)
	fmt.Println("Inputs:", len(p.UnsignedTx.TxIn))
	fmt.Println("Outputs:", len(p.UnsignedTx.TxOut))
	fmt.Println("Complete:", p.IsComplete())

	// Output:
	// Version: 2
	// Inputs: 1
	// Outputs: 1
	// Complete: false
}

// This example demonstrates how to serialize a PSBT to base64.
func ExamplePacket_B64Encode() {
	txHash, _ := chainhash.NewHashFromStr(
		"0000000000000000000000000000000000000000000000000000000000000000")
	prevOut := wire.NewOutPoint(txHash, 0)
	scriptPubKey, _ := hex.DecodeString("76a914" +
		"0000000000000000000000000000000000000000" + "88ac")
	out := wire.NewTxOut(btcutil.SatoshiPerBitcoin/1000, scriptPubKey)

	p, err := psbt.New(
		[]*wire.OutPoint{prevOut},
		[]*wire.TxOut{out},
		2, // transaction version
		0,
		[]uint32{wire.MaxTxInSequenceNum},
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	b64, err := p.B64Encode()
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Base64 length:", len(b64))
	fmt.Println("Starts with magic:", strings.HasPrefix(b64, "cHNid"))

	// Output:
	// Base64 length: 128
	// Starts with magic: true
}

// This example demonstrates that a finalized PSBT has FinalScriptSig or
// FinalScriptWitness set on each finalized input. When you sign a PSBT (e.g.
// with Updater.Sign or taproot-specific methods), partial signatures are
// stored in the input; the Finalizer (MaybeFinalize / MaybeFinalizeAll) then
// builds the full scriptSig and/or witness and writes them to
// FinalScriptSig and FinalScriptWitness. For Taproot and other witness
// inputs, the witness stack is written to FinalScriptWitness. This shows the
// difference between an unsigned and a finalized PSBT.
func Example_finalizedInput() {
	// BIP 174 test vector: one P2PKH and one P2SH-P2WPKH input; first input
	// is signed and finalized (has FinalScriptSig set).
	psbtHex := "70736274ff0100a00200000002ab0949a08c5af7c49b8212f417e2f15ab3f5c33dcf153821a8139f877a5b7be40000000000feffffffab0949a08c5af7c49b8212f417e2f15ab3f5c33dcf153821a8139f877a5b7be40100000000feffffff02603bea0b000000001976a914768a40bbd740cbe81d988e71de2a4d5c71396b1d88ac8e240000000000001976a9146f4620b553fa095e721b9ee0efe9fa039cca459788ac000000000001076a47304402204759661797c01b036b25928948686218347d89864b719e1f7fcf57d1e511658702205309eabf56aa4d8891ffd111fdf1336f3a29da866d7f8486d75546ceedaf93190121035cdc61fc7ba971c0b501a646a2a83b102cb43881217ca682dc86e2d73fa882920001012000e1f5050000000017a9143545e6e33b832c47050f24d3eeb93c9c03948bc787010416001485d13537f2e265405a34dbafa9e3dda01fb82308000000"
	raw, err := hex.DecodeString(psbtHex)
	if err != nil {
		fmt.Println(err)
		return
	}

	p, err := psbt.NewFromRawBytes(bytes.NewReader(raw), false)
	if err != nil {
		fmt.Println(err)
		return
	}

	for i := range p.Inputs {
		in := &p.Inputs[i]
		hasSig := in.FinalScriptSig != nil
		hasWitness := in.FinalScriptWitness != nil
		fmt.Printf("Input %d: FinalScriptSig=%v FinalScriptWitness=%v\n",
			i, hasSig, hasWitness)
	}
	fmt.Println("Complete:", p.IsComplete())
	fmt.Println("Finalized inputs contain the final script data.")

	// Output:
	// Input 0: FinalScriptSig=true FinalScriptWitness=false
	// Input 1: FinalScriptSig=false FinalScriptWitness=false
	// Complete: false
	// Finalized inputs contain the final script data.
}
