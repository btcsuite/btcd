package psbt

import (
	"bytes"
	"sort"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// InPlaceSort modifies the passed packet's wire TX inputs and outputs to be
// sorted based on BIP 69. The sorting happens in a way that the packet's
// partial inputs and outputs are also modified to match the sorted TxIn and
// TxOuts of the wire transaction.
//
// WARNING: This function must NOT be called with packages that already contain
// (partial) witness data since it will mutate the transaction if it's not
// already sorted. This can cause issues if you mutate a tx in a block, for
// example, which would invalidate the block. It could also cause cached hashes,
// such as in a btcutil.Tx to become invalidated.
//
// The function should only be used if the caller is creating the transaction or
// is otherwise 100% positive mutating will not cause adverse affects due to
// other dependencies.
func InPlaceSort(packet *Packet) error {
	// To make sure we don't run into any nil pointers or array index
	// violations during sorting, do a very basic sanity check first.
	err := VerifyInputOutputLen(packet, false, false)
	if err != nil {
		return err
	}

	sort.Sort(&sortableInputs{p: packet})
	sort.Sort(&sortableOutputs{p: packet})

	return nil
}

// sortableInputs is a simple wrapper around a packet that implements the
// sort.Interface for sorting the wire and partial inputs of a packet.
type sortableInputs struct {
	p *Packet
}

// sortableOutputs is a simple wrapper around a packet that implements the
// sort.Interface for sorting the wire and partial outputs of a packet.
type sortableOutputs struct {
	p *Packet
}

// For sortableInputs and sortableOutputs, three functions are needed to make
// them sortable with sort.Sort() -- Len, Less, and Swap.
// Len and Swap are trivial. Less is BIP 69 specific.
func (s *sortableInputs) Len() int { return len(s.p.UnsignedTx.TxIn) }
func (s sortableOutputs) Len() int { return len(s.p.UnsignedTx.TxOut) }

// Swap swaps two inputs.
func (s *sortableInputs) Swap(i, j int) {
	tx := s.p.UnsignedTx
	tx.TxIn[i], tx.TxIn[j] = tx.TxIn[j], tx.TxIn[i]
	s.p.Inputs[i], s.p.Inputs[j] = s.p.Inputs[j], s.p.Inputs[i]
}

// Swap swaps two outputs.
func (s *sortableOutputs) Swap(i, j int) {
	tx := s.p.UnsignedTx
	tx.TxOut[i], tx.TxOut[j] = tx.TxOut[j], tx.TxOut[i]
	s.p.Outputs[i], s.p.Outputs[j] = s.p.Outputs[j], s.p.Outputs[i]
}

// Less is the input comparison function. First sort based on input hash
// (reversed / rpc-style), then index.
func (s *sortableInputs) Less(i, j int) bool {
	ins := s.p.UnsignedTx.TxIn

	// Input hashes are the same, so compare the index.
	ihash := ins[i].PreviousOutPoint.Hash
	jhash := ins[j].PreviousOutPoint.Hash
	if ihash == jhash {
		return ins[i].PreviousOutPoint.Index <
			ins[j].PreviousOutPoint.Index
	}

	// At this point, the hashes are not equal, so reverse them to
	// big-endian and return the result of the comparison.
	const hashSize = chainhash.HashSize
	for b := 0; b < hashSize/2; b++ {
		ihash[b], ihash[hashSize-1-b] = ihash[hashSize-1-b], ihash[b]
		jhash[b], jhash[hashSize-1-b] = jhash[hashSize-1-b], jhash[b]
	}
	return bytes.Compare(ihash[:], jhash[:]) == -1
}

// Less is the output comparison function. First sort based on amount (smallest
// first), then PkScript.
func (s *sortableOutputs) Less(i, j int) bool {
	outs := s.p.UnsignedTx.TxOut

	if outs[i].Value == outs[j].Value {
		return bytes.Compare(outs[i].PkScript, outs[j].PkScript) < 0
	}
	return outs[i].Value < outs[j].Value
}
