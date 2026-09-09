package psbt

import (
	"fmt"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

type taprootScriptToken struct {
	opcode byte
	data   []byte
}

// taprootScriptSpendWitnessStack returns the witness elements required before
// the tapscript and control block for a script-path spend. multi_a scripts are
// matched by pubkey instead of relying on PSBT signature slice ordering.
func taprootScriptSpendWitnessStack(script []byte,
	scriptSpendSigs []*TaprootScriptSpendSig) (wire.TxWitness, error) {

	keys, threshold, isMultiA, err := parseTaprootMultiA(script)
	if err != nil {
		return nil, err
	}

	// Preserve the existing ordering behavior for script types the finalizer
	// already handled. CHECKSIGADD scripts are parsed as multi_a below so an
	// unsupported CHECKSIGADD construction cannot silently produce a witness.
	if !isMultiA {
		witnessStack := make(wire.TxWitness, 0, len(scriptSpendSigs))
		for _, scriptSpendSig := range scriptSpendSigs {
			witnessStack = append(
				witnessStack, taprootScriptSpendSigBytes(scriptSpendSig),
			)
		}

		return witnessStack, nil
	}

	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[string(key)] = struct{}{}
	}

	sigByKey := make(map[string][]byte, len(scriptSpendSigs))
	for idx, scriptSpendSig := range scriptSpendSigs {
		key := string(scriptSpendSig.XOnlyPubKey)
		if _, ok := keySet[key]; !ok {
			return nil, fmt.Errorf("taproot script spend signature %d "+
				"does not match a multi_a key: %w", idx,
				ErrInvalidPsbtFormat)
		}
		if _, ok := sigByKey[key]; ok {
			return nil, fmt.Errorf("duplicate taproot script spend "+
				"signature for multi_a key: %w", ErrInvalidPsbtFormat)
		}

		sigByKey[key] = taprootScriptSpendSigBytes(scriptSpendSig)
	}

	available := 0
	for _, key := range keys {
		if _, ok := sigByKey[string(key)]; ok {
			available++
		}
	}
	if available < threshold {
		return nil, ErrNotFinalizable
	}

	// NUMEQUAL requires exactly threshold successful signature checks. If
	// the PSBT contains more signatures, select a deterministic subset in
	// script order and leave the other witness positions empty.
	selected := make([][]byte, len(keys))
	remaining := threshold
	for idx, key := range keys {
		if remaining == 0 {
			break
		}

		if sig, ok := sigByKey[string(key)]; ok {
			selected[idx] = sig
			remaining--
		}
	}

	// CHECKSIG and CHECKSIGADD consume witness elements from the top of the
	// stack, so multi_a signatures are supplied in reverse script-key order.
	witnessStack := make(wire.TxWitness, 0, len(keys))
	for idx := len(selected) - 1; idx >= 0; idx-- {
		witnessStack = append(witnessStack, selected[idx])
	}

	return witnessStack, nil
}

// taprootScriptSpendSigBytes returns a script-spend signature with its
// non-default sighash byte appended.
func taprootScriptSpendSigBytes(scriptSpendSig *TaprootScriptSpendSig) []byte {
	sig := append([]byte{}, scriptSpendSig.Signature...)
	if scriptSpendSig.SigHash != txscript.SigHashDefault {
		sig = append(sig, byte(scriptSpendSig.SigHash))
	}

	return sig
}

// parseTaprootMultiA recognizes the standard multi_a tapscript template:
//
//	<key> CHECKSIG [<key> CHECKSIGADD ...] <threshold> NUMEQUAL
func parseTaprootMultiA(script []byte) ([][]byte, int, bool, error) {
	tokenizer := txscript.MakeScriptTokenizer(0, script)
	tokens := make([]taprootScriptToken, 0, 8)
	hasCheckSigAdd := false
	for tokenizer.Next() {
		token := taprootScriptToken{
			opcode: tokenizer.Opcode(),
			data:   tokenizer.Data(),
		}
		if token.opcode == txscript.OP_CHECKSIGADD {
			hasCheckSigAdd = true
		}
		tokens = append(tokens, token)
	}
	if tokenizer.Err() != nil {
		return nil, 0, false, ErrUnsupportedScriptType
	}

	if len(tokens) < 4 || len(tokens[0].data) != 32 ||
		tokens[1].opcode != txscript.OP_CHECKSIG {

		if hasCheckSigAdd {
			return nil, 0, false, ErrUnsupportedScriptType
		}
		return nil, 0, false, nil
	}

	keys := make([][]byte, 0, len(tokens)/2)
	keys = append(keys, tokens[0].data)

	idx := 2
	for idx+1 < len(tokens) && len(tokens[idx].data) == 32 &&
		tokens[idx+1].opcode == txscript.OP_CHECKSIGADD {

		keys = append(keys, tokens[idx].data)
		idx += 2
	}

	if idx+2 != len(tokens) || tokens[idx+1].opcode != txscript.OP_NUMEQUAL {
		if hasCheckSigAdd {
			return nil, 0, false, ErrUnsupportedScriptType
		}
		return nil, 0, false, nil
	}

	threshold, ok := taprootMultiAThreshold(tokens[idx])
	if !ok || threshold < 1 || threshold > len(keys) {
		return nil, 0, false, ErrUnsupportedScriptType
	}

	return keys, threshold, true, nil
}

// taprootMultiAThreshold decodes the threshold token used by multi_a.
func taprootMultiAThreshold(token taprootScriptToken) (int, bool) {
	if txscript.IsSmallInt(token.opcode) {
		return txscript.AsSmallInt(token.opcode), true
	}
	if token.data == nil {
		return 0, false
	}

	num, err := txscript.MakeScriptNum(token.data, true, 4)
	if err != nil {
		return 0, false
	}

	return int(num.Int32()), true
}
