package bip322

import (
	"fmt"

	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
)

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
