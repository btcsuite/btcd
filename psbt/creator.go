// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
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
// Constructor, it enforces the PSBT_GLOBAL_TX_MODIFIABLE Inputs-Modifiable flag
// (Bit 0) per BIP 370.
func (p *Packet) AddInputV2(input PInput) error {
	switch p.Version {
	case PsbtVersion2:
		// Valid, continue to add the input.

	default:
		return ErrUnsupportedDynamicAdd
	}

	// Check the TxModifiable rule before adding
	if p.TxModifiable != nil && *p.TxModifiable&FlagInputsModifiable == 0 {
		return ErrInputsNotModifiable
	}
	p.Inputs = append(p.Inputs, input)
	p.InputCount = uint32(len(p.Inputs))
	return nil
}

// AddOutputV2 appends a new POutput to a Version 2 PSBT, incrementing the
// internal count. It returns an error if the PSBT is not Version 2.
// As the Constructor, it enforces the PSBT_GLOBAL_TX_MODIFIABLE
// Outputs-Modifiable flag (Bit 1) per BIP 370.
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
	p.Outputs = append(p.Outputs, output)
	p.OutputCount = uint32(len(p.Outputs))
	return nil
}
