// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"

	"github.com/btcsuite/btcd/wire/v2"
)

// MinTxVersion is the lowest transaction version that we'll permit.
const MinTxVersion = 1

// New on provision of an input and output 'skeleton' for the transaction, a
// new partially populated PBST packet. The populated packet will include the
// unsigned transaction, and the set of known inputs and outputs contained
// within the unsigned transaction.  The values of nLockTime, nSequence (per
// input) and transaction version (must be 1 of 2) must be specified here. Note
// that the default nSequence value is wire.MaxTxInSequenceNum.  Referencing
// the PSBT BIP, this function serves the roles of the Creator.
func New(inputs []*wire.OutPoint, outputs []*wire.TxOut, version int32,
	nLockTime uint32, nSequences []uint32) (*Packet, error) {

	// Create the new struct; the input and output lists will be empty, the
	// unsignedTx object must be constructed and serialized, and that
	// serialization should be entered as the only entry for the
	// globalKVPairs list.
	//
	// Ensure that the version of the transaction is greater then our
	// minimum allowed transaction version. There must be one sequence
	// number per input.
	if version < MinTxVersion || len(nSequences) != len(inputs) {
		return nil, ErrInvalidPsbtFormat
	}

	unsignedTx := wire.NewMsgTx(version)
	unsignedTx.LockTime = nLockTime
	for i, in := range inputs {
		unsignedTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: *in,
			Sequence:         nSequences[i],
		})
	}
	for _, out := range outputs {
		unsignedTx.AddTxOut(out)
	}

	// The input and output lists are empty, but there is a list of those
	// two lists, and each one must be of length matching the unsigned
	// transaction; the unknown list can be nil.
	pInputs := make([]PInput, len(unsignedTx.TxIn))
	for i := range pInputs {
		pInputs[i].Sequence = nSequences[i]
	}
	pOutputs := make([]POutput, len(unsignedTx.TxOut))

	// This new Psbt is "raw" and contains no key-value fields, so sanity
	// checking with c.Cpsbt.SanityCheck() is not required.
	return &Packet{
		UnsignedTx: unsignedTx,
		Inputs:     pInputs,
		Outputs:    pOutputs,
		Unknowns:   nil,
	}, nil
}

// NewV2 creates a new PSBTv2 Packet.
func NewV2(txVersion int32, fallbackLocktime *uint32,
	txModifiable *uint8) (*Packet, error) {

	if txVersion < 2 {
		return nil, ErrV2TxVersionBelowTwo
	}

	// For V2, UnsignedTx must be nil and TxVersion is explicitly required.
	return &Packet{
		Version:          PsbtVersion2,
		TxVersion:        txVersion,
		FallbackLocktime: fallbackLocktime,
		TxModifiable:     txModifiable,
		Inputs:           []PInput{},
		Outputs:          []POutput{},
		XPubs:            nil,
		Unknowns:         nil,
	}, nil
}

// AddInputV2 appends a new PInput to a Version 2 PSBT, incrementing the
// internal count. It returns an error if the PSBT is not Version 2. As the
// Constructor, it enforces the PSBT_GLOBAL_TX_MODIFIABLE Inputs-Modifiable
// flag (Bit 0), the required-field and locktime-range rules, and the
// locktime-compatibility rules of BIP-370's Constructor section.
// Note: the SIGHASH_SINGLE input/output pairing rule is not enforced
// here; tracked as a follow-up.
func (p *Packet) AddInputV2(input PInput) error {
	switch p.Version {
	case PsbtVersion2:
	// Valid, continue to add the input.

	default:
		return ErrUnsupportedDynamicAdd
	}

	// Check the TxModifiable rule before adding. BIP-370 allows
	// PSBT_GLOBAL_TX_MODIFIABLE to be omitted when the Creator is
	// also the Constructor, so nil is treated as modifiable.
	if p.TxModifiable != nil && *p.TxModifiable&FlagInputsModifiable == 0 {
		return ErrInputsNotModifiable
	}

	// BIP-370: required fields — a previous txid of
	// exactly 32 bytes that is not all zeros. Mirrors the
	// parse-side rules so add and parse accept the same set.
	if len(input.PreviousTxid) != 32 {
		return ErrInvalidKeyData
	}
	if bytes.Equal(input.PreviousTxid, make([]byte, 32)) {
		return ErrInvalidKeyData
	}

	// BIP-370 locktime ranges (mirrors partial_input.go):
	// time-based locktimes are 0 (unset) or >= 500000000;
	// height-based locktimes are < 500000000.
	if input.TimeLocktime != 0 && input.TimeLocktime < 500000000 {
		return ErrInvalidKeyData
	}
	if input.HeightLocktime >= 500000000 {
		return ErrInvalidKeyData
	}

	// BIP-370: the new input's locktime requirement
	// must be compatible with the existing inputs, and the
	// determined locktime must not change once signatures exist.
	// DetermineLockTime reads p.Inputs, so append tentatively,
	// evaluate, then roll back unconditionally before deciding.
	var oldLocktime uint32
	if len(p.Inputs) > 0 {
		var oldErr error
		oldLocktime, oldErr = p.DetermineLockTime()
		if oldErr != nil {
			return oldErr
		}
	}

	p.Inputs = append(p.Inputs, input)
	newLocktime, newErr := p.DetermineLockTime()
	p.Inputs = p.Inputs[:len(p.Inputs)-1]

	if newErr != nil {
		return newErr
	}
	if len(p.Inputs) > 0 && newLocktime != oldLocktime {
		for i := range p.Inputs {
			existing := &p.Inputs[i]
			if len(existing.PartialSigs) > 0 ||
				existing.TaprootKeySpendSig != nil ||
				len(existing.TaprootScriptSpendSig) > 0 {

				return ErrLockTimeImmutable
			}
		}
	}
	p.Inputs = append(p.Inputs, input)
	p.InputCount = uint32(len(p.Inputs))
	return nil
}

// AddOutputV2 appends a new POutput to a Version 2 PSBT, incrementing the
// internal count. It returns an error if the PSBT is not Version 2.
// As the Constructor, it enforces the PSBT_GLOBAL_TX_MODIFIABLE
// Outputs-Modifiable flag (Bit 1) and requires the output script to be
// set per BIP-370. The SIGHASH_SINGLE pairing rule is not enforced here;
// tracked as a follow-up.
func (p *Packet) AddOutputV2(output POutput) error {
	switch p.Version {
	case PsbtVersion2:
		// Valid, continue to add the output.

	default:
		return ErrUnsupportedDynamicAdd
	}

	// Check the TxModifiable rule before adding
	if p.TxModifiable != nil && *p.TxModifiable&FlagOutputsModifiable == 0 {
		return ErrOutputsNotModifiable
	}

	// BIP-370: an output added by the Constructor must
	// have its script set. Amount 0 is legal (e.g. OP_RETURN), so
	// only the script is checked here.
	if output.Script == nil {
		return ErrInvalidPsbtFormat
	}
	p.Outputs = append(p.Outputs, output)
	p.OutputCount = uint32(len(p.Outputs))
	return nil
}
