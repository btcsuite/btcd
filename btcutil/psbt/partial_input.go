package psbt

import (
	"bytes"
	"encoding/binary"
	"io"
	"sort"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// PInput is a struct encapsulating all the data that can be attached to any
// specific input of the PSBT.
type PInput struct {
	NonWitnessUtxo         *wire.MsgTx
	WitnessUtxo            *wire.TxOut
	PartialSigs            []*PartialSig
	SighashType            txscript.SigHashType
	RedeemScript           []byte
	WitnessScript          []byte
	Bip32Derivation        []*Bip32Derivation
	FinalScriptSig         []byte
	FinalScriptWitness     []byte
	TaprootKeySpendSig     []byte
	TaprootScriptSpendSig  []*TaprootScriptSpendSig
	TaprootLeafScript      []*TaprootTapLeafScript
	TaprootBip32Derivation []*TaprootBip32Derivation
	TaprootInternalKey     []byte
	TaprootMerkleRoot      []byte
	Unknowns               []*Unknown
}

// NewPsbtInput creates an instance of PsbtInput given either a nonWitnessUtxo
// or a witnessUtxo.
//
// NOTE: Only one of the two arguments should be specified, with the other
// being `nil`; otherwise the created PsbtInput object will fail IsSane()
// checks and will not be usable.
func NewPsbtInput(nonWitnessUtxo *wire.MsgTx,
	witnessUtxo *wire.TxOut) *PInput {

	return &PInput{
		NonWitnessUtxo:     nonWitnessUtxo,
		WitnessUtxo:        witnessUtxo,
		PartialSigs:        []*PartialSig{},
		SighashType:        0,
		RedeemScript:       nil,
		WitnessScript:      nil,
		Bip32Derivation:    []*Bip32Derivation{},
		FinalScriptSig:     nil,
		FinalScriptWitness: nil,
		Unknowns:           nil,
	}
}

// IsSane returns true only if there are no conflicting values in the Psbt
// PInput. For segwit v0 no checks are currently implemented.
func (pi *PInput) IsSane() bool {

	// TODO(guggero): Implement sanity checks for segwit v1. For segwit v0
	// it is unsafe to only rely on the witness UTXO so we don't check that
	// only one is set anymore.
	// See https://github.com/bitcoin/bitcoin/pull/19215.

	return true
}

// deserialize attempts to deserialize a new PInput from the passed io.Reader.
func (pi *PInput) deserialize(r io.Reader) error {
	for {
		keyint, keydata, err := getKey(r)
		if err != nil {
			return err
		}
		if keyint == -1 {
			// Reached separator byte
			break
		}
		value, err := wire.ReadVarBytes(
			r, 0, MaxPsbtValueLength, "PSBT value",
		)
		if err != nil {
			return err
		}

		switch InputType(keyint) {

		case NonWitnessUtxoType:
			if pi.NonWitnessUtxo != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}
			tx := wire.NewMsgTx(2)

			err := tx.Deserialize(bytes.NewReader(value))
			if err != nil {
				return err
			}
			pi.NonWitnessUtxo = tx

		case WitnessUtxoType:
			if pi.WitnessUtxo != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}
			txout, err := readTxOut(value)
			if err != nil {
				return err
			}
			pi.WitnessUtxo = txout

		case PartialSigType:
			newPartialSig := PartialSig{
				PubKey:    keydata,
				Signature: value,
			}

			if !newPartialSig.checkValid() {
				return ErrInvalidPsbtFormat
			}

			// Duplicate keys are not allowed
			for _, x := range pi.PartialSigs {
				if bytes.Equal(x.PubKey, newPartialSig.PubKey) {
					return ErrDuplicateKey
				}
			}

			pi.PartialSigs = append(pi.PartialSigs, &newPartialSig)

		case SighashType:
			if pi.SighashType != 0 {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}

			// Bounds check on value here since the sighash type must be a
			// 32-bit unsigned integer.
			if len(value) != 4 {
				return ErrInvalidKeydata
			}

			shtype := txscript.SigHashType(
				binary.LittleEndian.Uint32(value),
			)
			pi.SighashType = shtype

		case RedeemScriptInputType:
			if pi.RedeemScript != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}
			pi.RedeemScript = value

		case WitnessScriptInputType:
			if pi.WitnessScript != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}
			pi.WitnessScript = value

		case Bip32DerivationInputType:
			if !validatePubkey(keydata) {
				return ErrInvalidPsbtFormat
			}
			master, derivationPath, err := readBip32Derivation(value)
			if err != nil {
				return err
			}

			// Duplicate keys are not allowed
			for _, x := range pi.Bip32Derivation {
				if bytes.Equal(x.PubKey, keydata) {
					return ErrDuplicateKey
				}
			}

			pi.Bip32Derivation = append(
				pi.Bip32Derivation,
				&Bip32Derivation{
					PubKey:               keydata,
					MasterKeyFingerprint: master,
					Bip32Path:            derivationPath,
				},
			)

		case FinalScriptSigType:
			if pi.FinalScriptSig != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}

			pi.FinalScriptSig = value

		case FinalScriptWitnessType:
			if pi.FinalScriptWitness != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}

			pi.FinalScriptWitness = value

		case TaprootKeySpendSignatureType:
			if pi.TaprootKeySpendSig != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}

			// The signature can either be 64 or 65 bytes.
			switch {
			case len(value) == schnorrSigMinLength:
				if !validateSchnorrSignature(value) {
					return ErrInvalidKeydata
				}

			case len(value) == schnorrSigMaxLength:
				if !validateSchnorrSignature(
					value[0:schnorrSigMinLength],
				) {
					return ErrInvalidKeydata
				}

			default:
				return ErrInvalidKeydata
			}

			pi.TaprootKeySpendSig = value

		case TaprootScriptSpendSignatureType:
			// The key data for the script spend signature is:
			//   <xonlypubkey> <leafhash>
			if len(keydata) != 32*2 {
				return ErrInvalidKeydata
			}

			newPartialSig := TaprootScriptSpendSig{
				XOnlyPubKey: keydata[:32],
				LeafHash:    keydata[32:],
			}

			// The signature can either be 64 or 65 bytes.
			switch {
			case len(value) == schnorrSigMinLength:
				newPartialSig.Signature = value
				newPartialSig.SigHash = txscript.SigHashDefault

			case len(value) == schnorrSigMaxLength:
				newPartialSig.Signature = value[0:schnorrSigMinLength]
				newPartialSig.SigHash = txscript.SigHashType(
					value[schnorrSigMinLength],
				)

			default:
				return ErrInvalidKeydata
			}

			if !newPartialSig.checkValid() {
				return ErrInvalidKeydata
			}

			// Duplicate keys are not allowed.
			for _, x := range pi.TaprootScriptSpendSig {
				if x.EqualKey(&newPartialSig) {
					return ErrDuplicateKey
				}
			}

			pi.TaprootScriptSpendSig = append(
				pi.TaprootScriptSpendSig, &newPartialSig,
			)

		case TaprootLeafScriptType:
			if len(value) < 1 {
				return ErrInvalidKeydata
			}

			newLeafScript := TaprootTapLeafScript{
				ControlBlock: keydata,
				Script:       value[:len(value)-1],
				LeafVersion: txscript.TapscriptLeafVersion(
					value[len(value)-1],
				),
			}

			if !newLeafScript.checkValid() {
				return ErrInvalidKeydata
			}

			// Duplicate keys are not allowed.
			for _, x := range pi.TaprootLeafScript {
				if bytes.Equal(
					x.ControlBlock,
					newLeafScript.ControlBlock,
				) {
					return ErrDuplicateKey
				}
			}

			pi.TaprootLeafScript = append(
				pi.TaprootLeafScript, &newLeafScript,
			)

		case TaprootBip32DerivationInputType:
			if !validateXOnlyPubkey(keydata) {
				return ErrInvalidKeydata
			}

			taprootDerivation, err := readTaprootBip32Derivation(
				keydata, value,
			)
			if err != nil {
				return err
			}

			// Duplicate keys are not allowed.
			for _, x := range pi.TaprootBip32Derivation {
				if bytes.Equal(x.XOnlyPubKey, keydata) {
					return ErrDuplicateKey
				}
			}

			pi.TaprootBip32Derivation = append(
				pi.TaprootBip32Derivation, taprootDerivation,
			)

		case TaprootInternalKeyInputType:
			if pi.TaprootInternalKey != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}

			if !validateXOnlyPubkey(value) {
				return ErrInvalidKeydata
			}

			pi.TaprootInternalKey = value

		case TaprootMerkleRootType:
			if pi.TaprootMerkleRoot != nil {
				return ErrDuplicateKey
			}
			if keydata != nil {
				return ErrInvalidKeydata
			}

			pi.TaprootMerkleRoot = value

		default:
			// A fall through case for any proprietary types.
			keyintanddata := []byte{byte(keyint)}
			keyintanddata = append(keyintanddata, keydata...)
			newUnknown := &Unknown{
				Key:   keyintanddata,
				Value: value,
			}

			// Duplicate key+keydata are not allowed
			for _, x := range pi.Unknowns {
				if bytes.Equal(x.Key, newUnknown.Key) &&
					bytes.Equal(x.Value, newUnknown.Value) {
					return ErrDuplicateKey
				}
			}

			pi.Unknowns = append(pi.Unknowns, newUnknown)
		}
	}

	return nil
}

// serialize attempts to serialize the target PInput into the passed io.Writer.
func (pi *PInput) serialize(w io.Writer) error {

	if !pi.IsSane() {
		return ErrInvalidPsbtFormat
	}

	if pi.NonWitnessUtxo != nil {
		var buf bytes.Buffer
		err := pi.NonWitnessUtxo.Serialize(&buf)
		if err != nil {
			return err
		}

		err = serializeKVPairWithType(
			w, uint8(NonWitnessUtxoType), nil, buf.Bytes(),
		)
		if err != nil {
			return err
		}
	}
	if pi.WitnessUtxo != nil {
		var buf bytes.Buffer
		err := wire.WriteTxOut(&buf, 0, 0, pi.WitnessUtxo)
		if err != nil {
			return err
		}

		err = serializeKVPairWithType(
			w, uint8(WitnessUtxoType), nil, buf.Bytes(),
		)
		if err != nil {
			return err
		}
	}

	if pi.FinalScriptSig == nil && pi.FinalScriptWitness == nil {
		sort.Sort(PartialSigSorter(pi.PartialSigs))
		for _, ps := range pi.PartialSigs {
			err := serializeKVPairWithType(
				w, uint8(PartialSigType), ps.PubKey,
				ps.Signature,
			)
			if err != nil {
				return err
			}
		}

		if pi.SighashType != 0 {
			var shtBytes [4]byte
			binary.LittleEndian.PutUint32(
				shtBytes[:], uint32(pi.SighashType),
			)

			err := serializeKVPairWithType(
				w, uint8(SighashType), nil, shtBytes[:],
			)
			if err != nil {
				return err
			}
		}

		if pi.RedeemScript != nil {
			err := serializeKVPairWithType(
				w, uint8(RedeemScriptInputType), nil,
				pi.RedeemScript,
			)
			if err != nil {
				return err
			}
		}

		if pi.WitnessScript != nil {
			err := serializeKVPairWithType(
				w, uint8(WitnessScriptInputType), nil,
				pi.WitnessScript,
			)
			if err != nil {
				return err
			}
		}

		sort.Sort(Bip32Sorter(pi.Bip32Derivation))
		for _, kd := range pi.Bip32Derivation {
			err := serializeKVPairWithType(
				w,
				uint8(Bip32DerivationInputType), kd.PubKey,
				SerializeBIP32Derivation(
					kd.MasterKeyFingerprint, kd.Bip32Path,
				),
			)
			if err != nil {
				return err
			}
		}

		if pi.TaprootKeySpendSig != nil {
			err := serializeKVPairWithType(
				w, uint8(TaprootKeySpendSignatureType), nil,
				pi.TaprootKeySpendSig,
			)
			if err != nil {
				return err
			}
		}

		sort.Slice(pi.TaprootScriptSpendSig, func(i, j int) bool {
			return pi.TaprootScriptSpendSig[i].SortBefore(
				pi.TaprootScriptSpendSig[j],
			)
		})
		for _, scriptSpend := range pi.TaprootScriptSpendSig {
			keyData := append([]byte{}, scriptSpend.XOnlyPubKey...)
			keyData = append(keyData, scriptSpend.LeafHash...)
			value := append([]byte{}, scriptSpend.Signature...)
			if scriptSpend.SigHash != txscript.SigHashDefault {
				value = append(value, byte(scriptSpend.SigHash))
			}
			err := serializeKVPairWithType(
				w, uint8(TaprootScriptSpendSignatureType),
				keyData, value,
			)
			if err != nil {
				return err
			}
		}

		sort.Slice(pi.TaprootLeafScript, func(i, j int) bool {
			return pi.TaprootLeafScript[i].SortBefore(
				pi.TaprootLeafScript[j],
			)
		})
		for _, leafScript := range pi.TaprootLeafScript {
			value := append([]byte{}, leafScript.Script...)
			value = append(value, byte(leafScript.LeafVersion))
			err := serializeKVPairWithType(
				w, uint8(TaprootLeafScriptType),
				leafScript.ControlBlock, value,
			)
			if err != nil {
				return err
			}
		}

		sort.Slice(pi.TaprootBip32Derivation, func(i, j int) bool {
			return pi.TaprootBip32Derivation[i].SortBefore(
				pi.TaprootBip32Derivation[j],
			)
		})
		for _, derivation := range pi.TaprootBip32Derivation {
			value, err := serializeTaprootBip32Derivation(
				derivation,
			)
			if err != nil {
				return err
			}
			err = serializeKVPairWithType(
				w, uint8(TaprootBip32DerivationInputType),
				derivation.XOnlyPubKey, value,
			)
			if err != nil {
				return err
			}
		}

		if pi.TaprootInternalKey != nil {
			err := serializeKVPairWithType(
				w, uint8(TaprootInternalKeyInputType), nil,
				pi.TaprootInternalKey,
			)
			if err != nil {
				return err
			}
		}

		if pi.TaprootMerkleRoot != nil {
			err := serializeKVPairWithType(
				w, uint8(TaprootMerkleRootType), nil,
				pi.TaprootMerkleRoot,
			)
			if err != nil {
				return err
			}
		}
	}

	if pi.FinalScriptSig != nil {
		err := serializeKVPairWithType(
			w, uint8(FinalScriptSigType), nil, pi.FinalScriptSig,
		)
		if err != nil {
			return err
		}
	}

	if pi.FinalScriptWitness != nil {
		err := serializeKVPairWithType(
			w, uint8(FinalScriptWitnessType), nil, pi.FinalScriptWitness,
		)
		if err != nil {
			return err
		}
	}

	// Unknown is a special case; we don't have a key type, only a key and
	// a value field
	for _, kv := range pi.Unknowns {
		err := serializeKVpair(w, kv.Key, kv.Value)
		if err != nil {
			return err
		}
	}

	return nil
}
