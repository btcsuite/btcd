package bip322

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// validateToSignTx checks that the to_sign transaction structure conforms to
// the BIP-322 verification process basic validation requirements:
//   - to_sign has at least one input
//   - to_sign's first input spends the output of to_spend (at index 0)
//   - to_sign has exactly one output with value 0 and an OP_RETURN scriptPubKey
//
// The toSpendTxHash argument is the txid of the to_spend transaction
// reconstructed from the message and challenge script.
func validateToSignTx(tx *wire.MsgTx, toSpendTxHash chainhash.Hash) error {
	if len(tx.TxIn) == 0 {
		return fmt.Errorf("%w: to_sign must have at least one input",
			ErrInvalidToSign)
	}

	// The first input must spend the to_spend transaction's only output
	// (at index 0).
	firstPrev := tx.TxIn[0].PreviousOutPoint
	if firstPrev.Hash != toSpendTxHash {
		return fmt.Errorf("%w: to_sign first input prev hash %s does "+
			"not match to_spend txid %s", ErrInvalidToSign,
			firstPrev.Hash, toSpendTxHash)
	}
	if firstPrev.Index != 0 {
		return fmt.Errorf("%w: to_sign first input prev index must be "+
			"0, got %d", ErrInvalidToSign, firstPrev.Index)
	}

	// to_sign must have exactly one output: value 0, scriptPubKey
	// OP_RETURN.
	if len(tx.TxOut) != 1 {
		return fmt.Errorf("%w: to_sign must have exactly one output, "+
			"got %d", ErrInvalidToSign, len(tx.TxOut))
	}
	out := tx.TxOut[0]
	if out.Value != 0 {
		return fmt.Errorf("%w: to_sign output value must be 0, got %d",
			ErrInvalidToSign, out.Value)
	}
	if len(out.PkScript) != 1 || out.PkScript[0] != txscript.OP_RETURN {
		return fmt.Errorf("%w: to_sign output scriptPubKey must be a "+
			"single OP_RETURN byte, got %x", ErrInvalidToSign,
			out.PkScript)
	}

	return nil
}

// validateUpgradeableRules implements the BIP-322 "upgradeable rules" that
// aren't enforced by the script engine via upgradeableRuleVerifyFlags. A
// violation causes an inconclusive result:
//   - The to_sign tx version must be 0 or 2.
//   - No taproot input must use an annex, which BIP-341 reserves for future
//     soft forks.
//
// The utxos map must contain the previous output of every input of the
// transaction.
func validateUpgradeableRules(tx *wire.MsgTx,
	utxos map[wire.OutPoint]*wire.TxOut) error {

	if tx.Version != 0 && tx.Version != 2 {
		return fmt.Errorf("%w: to_sign tx version must be 0 or 2, "+
			"got %d", ErrInconclusive, tx.Version)
	}

	for idx, txIn := range tx.TxIn {
		pkScript := utxos[txIn.PreviousOutPoint].PkScript
		if !txscript.IsPayToTaproot(pkScript) {
			continue
		}

		if len(stripAnnex(txIn.Witness)) != len(txIn.Witness) {
			return fmt.Errorf("%w: taproot annex is reserved for "+
				"upgrades, used by input %d", ErrInconclusive,
				idx)
		}
	}

	return nil
}

// scriptExecutionError maps an error returned by the script engine to the
// BIP-322 verification result it stands for, or nil if the scripts were
// executed successfully. Violations of the upgradeable rules (see
// upgradeableRuleVerifyFlags) result in ErrInconclusive, all other errors in
// ErrInvalidSignature or a more specific invalid result.
func scriptExecutionError(err error) error {
	if err == nil {
		return nil
	}

	// The Error() function on the VM's Error struct doesn't return a nice
	// error message. So we parse the error to get something more
	// descriptive. If parsing fails, we at least make the error detectable
	// as signature verification having failed.
	var vmErr txscript.Error
	if !errors.As(err, &vmErr) {
		return fmt.Errorf("%w: %w", ErrInvalidSignature, err)
	}

	switch vmErr.ErrorCode {
	// Surface a disallowed sighash type as a dedicated BIP-322 error so
	// callers can distinguish it from a generic signature failure.
	case txscript.ErrDisallowedSigHashType:
		return ErrInvalidSigHashFlag

	case txscript.ErrDiscourageUpgradableNOPs,
		txscript.ErrDiscourageUpgradableWitnessProgram,
		txscript.ErrDiscourageUpgradeableTaprootVersion,
		txscript.ErrDiscourageOpSuccess,
		txscript.ErrDiscourageUpgradeablePubKeyType:

		return fmt.Errorf("%w: err_code=%s, err_desc=%s",
			ErrInconclusive, vmErr.ErrorCode.String(),
			vmErr.Description)

	default:
		return fmt.Errorf("%w: err_code=%s, err_desc=%s",
			ErrInvalidSignature, vmErr.ErrorCode.String(),
			vmErr.Description)
	}
}

// validateUtxoCorrectness enforces the BIP-174 rule that an input spending a
// non-segwit (legacy) output must not have a witness UTXO field
// (PSBT_IN_WITNESS_UTXO) present.
func validateUtxoCorrectness(packet *psbt.Packet) error {
	for idx := range packet.Inputs {
		in := &packet.Inputs[idx]

		// We just check for legacy inputs that might incorrectly have a
		// WitnessUtxo field set. If there is no such field, that's
		// okay. A witness input is allowed to only have the
		// NonWitnessUtxo field set. But we can't check for that here
		// as things might need to be de-duplicated first.
		if in.WitnessUtxo == nil {
			continue
		}

		// Only a witness UTXO is present, sufficient only for a
		// (possibly P2SH-nested) segwit input. Its PkScript is the
		// scriptPubKey of the output the input spends.
		_, _, isSegWit := inputWitnessProgram(
			in.WitnessUtxo.PkScript, in.FinalScriptSig,
		)
		if !isSegWit {
			return fmt.Errorf("input %d presents a witness UTXO "+
				"for spending a non-segwit script, but a "+
				"non-segwit input requires a non-witness UTXO "+
				"(BIP-174)", idx)
		}
	}

	return nil
}

// validateNoCodeSeparator enforces the BIP-322 required rule that forbids the
// use of OP_CODESEPARATOR.
//
// The script engine, even with scriptVerifyFlags, only rejects
// OP_CODESEPARATOR in non-segwit scripts (via ScriptVerifyConstScriptCode). It
// does NOT reject it inside a segwit witness script (P2WSH) or a taproot leaf
// script (P2TR script-path spend), so those scripts are inspected here. The
// non-segwit cases (legacy scriptSig, P2SH redeem script) are already handled
// by the script engine and don't need to be re-checked.
func validateNoCodeSeparator(pkScript []byte, witness wire.TxWitness,
	sigScript []byte) error {

	scripts := revealedWitnessScripts(pkScript, sigScript, witness)
	for _, script := range scripts {
		hasSep, err := scriptHasCodeSeparator(script)
		if err != nil {
			// A malformed script will be rejected by the script
			// engine during execution, so we don't fail here.
			continue
		}
		if hasSep {
			return ErrCodeSeparator
		}
	}

	return nil
}

// revealedWitnessScripts returns the scripts that a segwit witness reveals and
// that the script engine does not scan for OP_CODESEPARATOR under
// scriptVerifyFlags: the witness script of a P2WSH spend and the leaf script
// of a P2TR script-path spend. Nested (P2SH-wrapped) segwit spends are resolved
// to their underlying witness program first.
func revealedWitnessScripts(pkScript, sigScript []byte,
	witness wire.TxWitness) [][]byte {

	version, program, ok := inputWitnessProgram(pkScript, sigScript)
	if !ok {
		return nil
	}

	items := stripAnnex(witness)
	switch {
	// P2WSH: a v0 program of 32 bytes. The witness script is the last
	// witness item.
	case version == 0 && len(program) == 32 && len(items) >= 1:
		return [][]byte{items[len(items)-1]}

	// P2TR script-path spend: a v1 program of 32 bytes with at least two
	// items (after annex removal). The leaf script is the second-to-last
	// item (the last item being the control block).
	case version == 1 && len(program) == 32 && len(items) >= 2:
		return [][]byte{items[len(items)-2]}
	}

	return nil
}

// scriptHasCodeSeparator reports whether the passed script contains an
// OP_CODESEPARATOR opcode. It walks the whole script (including unexecuted
// branches), matching the script engine's behavior for non-segwit scripts.
func scriptHasCodeSeparator(script []byte) (bool, error) {
	const scriptVersion = 0
	tokenizer := txscript.MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		if tokenizer.Opcode() == txscript.OP_CODESEPARATOR {
			return true, nil
		}
	}

	return false, tokenizer.Err()
}

// inputWitnessProgram returns the witness version and program that govern an
// input's execution, resolving a nested P2SH redeem script if present. ok is
// false when the input is not a (possibly nested) segwit spend.
func inputWitnessProgram(pkScript, sigScript []byte) (version int,
	program []byte, ok bool) {

	script := pkScript
	if txscript.IsPayToScriptHash(pkScript) {
		redeem, err := redeemScript(sigScript)
		if err != nil || len(redeem) == 0 {
			return 0, nil, false
		}
		script = redeem
	}

	if !txscript.IsWitnessProgram(script) {
		return 0, nil, false
	}

	v, prog, err := txscript.ExtractWitnessProgramInfo(script)
	if err != nil {
		return 0, nil, false
	}

	return v, prog, true
}

// redeemScript returns the redeem script of a P2SH input, which is the last
// data push of the scriptSig.
func redeemScript(sigScript []byte) ([]byte, error) {
	pushes, err := txscript.PushedData(sigScript)
	if err != nil || len(pushes) == 0 {
		return nil, err
	}

	return pushes[len(pushes)-1], nil
}

// stripAnnex removes the taproot annex from a witness stack if present. Per
// BIP-341, if there are at least two items and the last one starts with the
// annex tag (0x50), it is the annex.
func stripAnnex(witness wire.TxWitness) wire.TxWitness {
	if len(witness) >= 2 {
		last := witness[len(witness)-1]
		if len(last) > 0 && last[0] == txscript.TaprootAnnexTag {
			return witness[:len(witness)-1]
		}
	}

	return witness
}

// validateAmounts validates the previous output amount ranges against consensus
// rules. Each transaction output must not be negative or more than the max
// allowed per transaction. Also, the total of all outputs must abide by the
// same restrictions. This is a partial copy of
// blockchain.CheckTransactionSanity, which can't be used directly without
// depending on the whole btcd project.
func validateAmounts(outputs []*wire.TxOut) error {
	var totalSatoshi int64
	for _, txOut := range outputs {
		satoshi := txOut.Value
		if satoshi < 0 {
			return fmt.Errorf("transaction output has negative "+
				"value of %v", satoshi)
		}
		if satoshi > btcutil.MaxSatoshi {
			return fmt.Errorf("transaction output value is "+
				"higher than max allowed value: %v > %v ",
				satoshi, btcutil.MaxSatoshi)
		}

		// Two's complement int64 overflow guarantees that any overflow
		// is detected and reported.  This is impossible for Bitcoin,
		// but perhaps possible if an alt increases the total money
		// supply.
		totalSatoshi += satoshi
		if totalSatoshi < 0 {
			return fmt.Errorf("total value of all transaction "+
				"outputs exceeds max allowed value of %v",
				btcutil.MaxSatoshi)
		}
		if totalSatoshi > btcutil.MaxSatoshi {
			return fmt.Errorf("total value of all transaction "+
				"outputs is %v which is higher than max "+
				"allowed value of %v", totalSatoshi,
				btcutil.MaxSatoshi)
		}
	}

	return nil
}

// validateSizes validates the size related consensus limits of the final
// to_sign transaction: its weight and its signature operation cost. The utxos
// map must contain the previous output of every input of the transaction.
func validateSizes(tx *wire.MsgTx,
	utxos map[wire.OutPoint]*wire.TxOut) error {

	// Now that we've added all the witness data, we need to check the
	// weight again, to check it's still consensus valid. The calculation
	// is a copy of blockchain.GetTransactionWeight, which is just
	// (baseSize * 3) + totalSize.
	baseSize := tx.SerializeSizeStripped()
	totalSize := tx.SerializeSize()
	if ((baseSize * 3) + totalSize) > maxWitnessItems {
		return fmt.Errorf("serialized transaction weight exceeds "+
			"maximum of %d", maxWitnessItems)
	}

	sigOpCost, err := txSigOpCost(tx, utxos)
	if err != nil {
		return err
	}

	if sigOpCost > maxTxSigOpCost {
		return fmt.Errorf("transaction signature operation cost %d "+
			"exceeds maximum of %d", sigOpCost, maxTxSigOpCost)
	}

	return nil
}

// txSigOpCost returns the signature operation cost of the transaction as
// defined in BIP-141. It is the sum of the legacy signature operations in all
// signature scripts and output scripts, plus the precise signature operations
// of all P2SH redeem scripts, both scaled by the witness scale factor, plus the
// unscaled signature operations of all (possibly P2SH-nested) witness v0
// programs. Taproot inputs don't count towards the cost, as their signature
// operations are limited by a separate, witness size based budget (BIP-342).
// This is a copy of blockchain.GetSigOpCost, which can't be used directly
// without depending on the whole btcd project.
func txSigOpCost(tx *wire.MsgTx,
	utxos map[wire.OutPoint]*wire.TxOut) (int, error) {

	// Accumulate the number of legacy signature operations in all
	// transaction outputs. The signature operations of the outputs being
	// spent are not counted, as they were already accounted for in the
	// transactions that created them.
	sigOpCost := 0
	for _, txOut := range tx.TxOut {
		sigOpCost += txscript.GetSigOpCount(txOut.PkScript) *
			witnessScaleFactor
	}

	for idx, txIn := range tx.TxIn {
		utxo, ok := utxos[txIn.PreviousOutPoint]
		if !ok || utxo == nil {
			return 0, fmt.Errorf("missing UTXO for input index %d",
				idx)
		}

		// Legacy signature operations in the signature script. These
		// are only possible with non-push opcodes, which the script
		// engine would reject anyway, but they still count.
		sigScript := txIn.SignatureScript
		sigOpCost += txscript.GetSigOpCount(sigScript) *
			witnessScaleFactor

		// For P2SH inputs, the redeem script is counted precisely,
		// meaning a multisig with n public keys counts as n signature
		// operations instead of the maximum of 20.
		pkScript := utxo.PkScript
		if txscript.IsPayToScriptHash(pkScript) {
			sigOpCost += txscript.GetPreciseSigOpCount(
				sigScript, pkScript, true,
			) * witnessScaleFactor
		}

		// Witness signature operations are not scaled, which is what
		// gives them their discount compared to legacy ones.
		sigOpCost += txscript.GetWitnessSigOpCount(
			sigScript, pkScript, txIn.Witness,
		)
	}

	return sigOpCost, nil
}
