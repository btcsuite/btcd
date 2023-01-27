package psbt

import (
	"bytes"
	"io"
	"sort"

	"github.com/btcsuite/btcd/wire"
)

// POutput is a struct encapsulating all the data that can be attached
// to any specific output of the PSBT.
type POutput struct {
	RedeemScript           []byte
	WitnessScript          []byte
	Bip32Derivation        []*Bip32Derivation
	TaprootInternalKey     []byte
	TaprootTapTree         []byte
	TaprootBip32Derivation []*TaprootBip32Derivation
	Unknowns               []*Unknown
}

// NewPsbtOutput creates an instance of PsbtOutput; the three parameters
// redeemScript, witnessScript and Bip32Derivation are all allowed to be
// `nil`.
func NewPsbtOutput(redeemScript []byte, witnessScript []byte,
	bip32Derivation []*Bip32Derivation) *POutput {
	return &POutput{
		RedeemScript:    redeemScript,
		WitnessScript:   witnessScript,
		Bip32Derivation: bip32Derivation,
	}
}

// deserialize attempts to recode a new POutput from the passed io.Reader.
func (po *POutput) deserialize(r io.Reader) error {
	for {
		keyCode, keyData, err := getKey(r)
		if err != nil {
			return err
		}
		if keyCode == -1 {
			// Reached separator byte, this section is done.
			break
		}

		value, err := wire.ReadVarBytes(
			r, 0, MaxPsbtValueLength, "PSBT value",
		)
		if err != nil {
			return err
		}

		switch OutputType(keyCode) {

		case RedeemScriptOutputType:
			if po.RedeemScript != nil {
				return ErrDuplicateKey
			}
			if keyData != nil {
				return ErrInvalidKeyData
			}
			po.RedeemScript = value

		case WitnessScriptOutputType:
			if po.WitnessScript != nil {
				return ErrDuplicateKey
			}
			if keyData != nil {
				return ErrInvalidKeyData
			}
			po.WitnessScript = value

		case Bip32DerivationOutputType:
			if !validatePubkey(keyData) {
				return ErrInvalidKeyData
			}
			master, derivationPath, err := ReadBip32Derivation(
				value,
			)
			if err != nil {
				return err
			}

			// Duplicate keys are not allowed.
			for _, x := range po.Bip32Derivation {
				if bytes.Equal(x.PubKey, keyData) {
					return ErrDuplicateKey
				}
			}

			po.Bip32Derivation = append(po.Bip32Derivation,
				&Bip32Derivation{
					PubKey:               keyData,
					MasterKeyFingerprint: master,
					Bip32Path:            derivationPath,
				},
			)

		case TaprootInternalKeyOutputType:
			if po.TaprootInternalKey != nil {
				return ErrDuplicateKey
			}
			if keyData != nil {
				return ErrInvalidKeyData
			}

			if !validateXOnlyPubkey(value) {
				return ErrInvalidKeyData
			}

			po.TaprootInternalKey = value

		case TaprootTapTreeType:
			if po.TaprootTapTree != nil {
				return ErrDuplicateKey
			}
			if keyData != nil {
				return ErrInvalidKeyData
			}

			po.TaprootTapTree = value

		case TaprootBip32DerivationOutputType:
			if !validateXOnlyPubkey(keyData) {
				return ErrInvalidKeyData
			}

			taprootDerivation, err := ReadTaprootBip32Derivation(
				keyData, value,
			)
			if err != nil {
				return err
			}

			// Duplicate keys are not allowed.
			for _, x := range po.TaprootBip32Derivation {
				if bytes.Equal(x.XOnlyPubKey, keyData) {
					return ErrDuplicateKey
				}
			}

			po.TaprootBip32Derivation = append(
				po.TaprootBip32Derivation, taprootDerivation,
			)

		default:
			// A fall through case for any proprietary types.
			keyCodeAndData := append(
				[]byte{byte(keyCode)}, keyData...,
			)
			newUnknown := &Unknown{
				Key:   keyCodeAndData,
				Value: value,
			}

			// Duplicate key+keyData are not allowed.
			for _, x := range po.Unknowns {
				if bytes.Equal(x.Key, newUnknown.Key) &&
					bytes.Equal(x.Value, newUnknown.Value) {

					return ErrDuplicateKey
				}
			}

			po.Unknowns = append(po.Unknowns, newUnknown)
		}
	}

	return nil
}

// serialize attempts to write out the target POutput into the passed
// io.Writer.
func (po *POutput) serialize(w io.Writer) error {
	if po.RedeemScript != nil {
		err := serializeKVPairWithType(
			w, uint8(RedeemScriptOutputType), nil, po.RedeemScript,
		)
		if err != nil {
			return err
		}
	}
	if po.WitnessScript != nil {
		err := serializeKVPairWithType(
			w, uint8(WitnessScriptOutputType), nil, po.WitnessScript,
		)
		if err != nil {
			return err
		}
	}

	sort.Sort(Bip32Sorter(po.Bip32Derivation))
	for _, kd := range po.Bip32Derivation {
		err := serializeKVPairWithType(w,
			uint8(Bip32DerivationOutputType),
			kd.PubKey,
			SerializeBIP32Derivation(
				kd.MasterKeyFingerprint,
				kd.Bip32Path,
			),
		)
		if err != nil {
			return err
		}
	}

	if po.TaprootInternalKey != nil {
		err := serializeKVPairWithType(
			w, uint8(TaprootInternalKeyOutputType), nil,
			po.TaprootInternalKey,
		)
		if err != nil {
			return err
		}
	}

	if po.TaprootTapTree != nil {
		err := serializeKVPairWithType(
			w, uint8(TaprootTapTreeType), nil,
			po.TaprootTapTree,
		)
		if err != nil {
			return err
		}
	}

	sort.Slice(po.TaprootBip32Derivation, func(i, j int) bool {
		return po.TaprootBip32Derivation[i].SortBefore(
			po.TaprootBip32Derivation[j],
		)
	})
	for _, derivation := range po.TaprootBip32Derivation {
		value, err := SerializeTaprootBip32Derivation(
			derivation,
		)
		if err != nil {
			return err
		}
		err = serializeKVPairWithType(
			w, uint8(TaprootBip32DerivationOutputType),
			derivation.XOnlyPubKey, value,
		)
		if err != nil {
			return err
		}
	}

	// Unknown is a special case; we don't have a key type, only a key and
	// a value field
	for _, kv := range po.Unknowns {
		err := serializeKVpair(w, kv.Key, kv.Value)
		if err != nil {
			return err
		}
	}

	return nil
}
