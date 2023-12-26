package txscript

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

const (
	// TXFSVersion is the bit that indicates that the version should be
	// hashed into the TxHash.
	TXFSVersion uint8 = 1 << 0

	// TXFSLockTime is the bit that indicates that the locktime should be
	// hashed into the TxHash.
	TXFSLockTime uint8 = 1 << 1

	// TXFSCurrentInputIdx is the bit that indicates that the current input
	// index should be hashed into the TxHash.
	TXFSCurrentInputIdx uint8 = 1 << 2

	// TXFSCurrentInputControlBlock is the bit that indicates that the
	// control block of the current input should be hashed into the TxHash.
	TXFSCurrentInputControlBlock uint8 = 1 << 3

	// TXFSCurrentInputLastCodeseparatorPos is the bit that indicates that
	// the last codeseparator position of the current input should be hashed
	// into the TxHash.
	TXFSCurrentInputLastCodeseparatorPos uint8 = 1 << 4

	// TXFSInputs is the bit that indicates that the inputs should be hashed
	// into the TxHash.
	TXFSInputs uint8 = 1 << 5

	// TXFSOutputs is the bit that indicates that the outputs should be
	// hashed into the TxHash.
	TXFSOutputs uint8 = 1 << 6

	// TXFSControl is the bit that indicates that the TxFieldSelector
	// should be hashed into the TxHash.
	TXFSControl uint8 = 1 << 7

	// TXFSAll is the bit that indicates that all fields should be hashed
	// into the TxHash.
	TXFSAll uint8 = TXFSVersion |
		TXFSLockTime |
		TXFSCurrentInputIdx |
		TXFSCurrentInputControlBlock |
		TXFSCurrentInputLastCodeseparatorPos |
		TXFSInputs |
		TXFSOutputs |
		TXFSControl
)
const (
	// TXFSInputsPrevouts is the bit that indicates that the previous
	// outpoints should be hashed into the TxHash.
	TXFSInputsPrevouts uint8 = 1 << 0

	// TXFSInputsSequences is the bit that indicates that the sequences
	// should be hashed into the TxHash.
	TXFSInputsSequences uint8 = 1 << 1

	// TXFSInputsScriptsigs is the bit that indicates that the scriptSigs
	// should be hashed into the TxHash.
	TXFSInputsScriptsigs uint8 = 1 << 2

	// TXFSInputsPrevScriptpubkeys is the bit that indicates that the
	// previous scriptPubkeys should be hashed into the TxHash.
	TXFSInputsPrevScriptpubkeys uint8 = 1 << 3

	// TXFSInputsPrevValues is the bit that indicates that the previous
	// values should be hashed into the TxHash.
	TXFSInputsPrevValues uint8 = 1 << 4

	// TXFSInputsTaprootAnnexes is the bit that indicates that the annexes
	// of the taproot inputs should be hashed into the TxHash.
	TXFSInputsTaprootAnnexes uint8 = 1 << 5

	// TXFSOutputsScriptpubkeys is the bit that indicates that the
	// scriptPubkeys should be hashed into the TxHash.
	TXFSOutputsScriptpubkeys uint8 = 1 << 6

	// TXFSOutputsValues is the bit that indicates that the values should
	// be hashed into the TxHash.
	TXFSOutputsValues uint8 = 1 << 7

	// TXFSInputsAll is the bit that indicates that all input fields should
	// be hashed into the TxHash.
	TXFSInputsAll uint8 = TXFSInputsPrevouts |
		TXFSInputsSequences |
		TXFSInputsScriptsigs |
		TXFSInputsPrevScriptpubkeys |
		TXFSInputsPrevValues |
		TXFSInputsTaprootAnnexes

	// TXFSInputsTemplate is the bit that indicates that the input fields
	// that are part of the template should be hashed into the TxHash.
	TXFSInputsTemplate uint8 = TXFSInputsSequences |
		TXFSInputsScriptsigs |
		TXFSInputsPrevValues |
		TXFSInputsTaprootAnnexes

	// TXFSOutputsAll is the bit that indicates that all output fields
	// should be hashed into the TxHash.
	TXFSOutputsAll uint8 = TXFSOutputsScriptpubkeys | TXFSOutputsValues

	// TXFSInoutNumber is the bit that indicates that the number of inputs
	// or outputs should be hashed into the TxHash.
	TXFSInoutNumber uint8 = 1 << 7

	// TXFSInoutSelectionNone is the bit that indicates that no inputs or
	// outputs should be selected.
	TXFSInoutSelectionNone uint8 = 0x00

	// TXFSInoutSelectionCurrent is the bit that indicates that the current
	// input or output should be selected.
	TXFSInoutSelectionCurrent uint8 = 0x40

	// TXFSInoutSelectionAll is the bit that indicates that all inputs or
	// outputs should be selected.
	TXFSInoutSelectionAll uint8 = 0x3f

	// TXFSInoutSelectionMode is the bit that indicates that the selection
	// mode is leading or individual.
	TXFSInoutSelectionMode uint8 = 1 << 6

	// TXFSInoutSelectionSize is the bit that indicates the size of the
	// selection.
	TXFSInoutSelectionSize uint8 = 1 << 5

	// TXFSInoutSelectionMask is the mask that selects the number of inputs
	// or outputs to be selected.
	TXFSInoutSelectionMask uint8 = 0xff ^ TXFSInoutNumber ^
		TXFSInoutSelectionMode ^ TXFSInoutSelectionSize
)

// TXFSSpecialAll is a template that means everything should be hashed into the
// TxHash. This is useful to emulate SIGHASH_ALL when OP_TXHASH is used in
// combination with OP_CHECKSIGFROMSTACK.
var TXFSSpecialAll = [4]uint8{
	TXFSAll,
	TXFSInputsAll | TXFSOutputsAll,
	TXFSInoutNumber | TXFSInoutSelectionAll,
	TXFSInoutNumber | TXFSInoutSelectionAll,
}

// TXFSSpecialAll is a template that means everything except the prevouts and
// the prevout scriptPubkeys should be hashed into the TxHash.
var TXFSSpecialTemplate = [4]uint8{
	TXFSAll,
	TXFSInputsTemplate | TXFSOutputsAll,
	TXFSInoutNumber | TXFSInoutSelectionAll,
	TXFSInoutNumber | TXFSInoutSelectionAll,
}

// InOutSelector is an interface for selecting inputs or outputs from a
// transaction. It is used to select the inputs or outputs that are hashed into
// the TxHash.
type InOutSelector interface {
	SelectInputs(tx *wire.MsgTx, currentInput uint32,
		inputs bool) ([]int, error)
}

// InOutSelectorNone selects no inputs or outputs.
type InOutSelectorNone struct{}

// SelectInputs returns an empty slice.
func (s *InOutSelectorNone) SelectInputs(tx *wire.MsgTx, currentInput uint32,
	inputs bool) ([]int, error) {

	return nil, nil
}

// InOutSelectorAll selects all inputs or outputs.
type InOutSelectorAll struct{}

// SelectInputs returns a slice of all inputs or outputs.
func (s *InOutSelectorAll) SelectInputs(tx *wire.MsgTx, currentInput uint32,
	inputs bool) ([]int, error) {

	var selected []int
	if inputs {
		selected = make([]int, len(tx.TxIn))
		for i := 0; i < len(tx.TxIn); i++ {
			selected[i] = i
		}
	} else {
		selected = make([]int, len(tx.TxOut))
		for i := 0; i < len(tx.TxOut); i++ {
			selected[i] = i
		}
	}
	return selected, nil
}

// InOutSelectorCurrent selects the current input or output.
type InOutSelectorCurrent struct{}

// SelectInputs returns a slice containing the current input or output.
func (s *InOutSelectorCurrent) SelectInputs(tx *wire.MsgTx, currentInput uint32,
	inputs bool) ([]int, error) {

	if int(currentInput) >= len(tx.TxIn) && inputs ||
		(int(currentInput) > len(tx.TxOut)) && !inputs {

		return nil, fmt.Errorf("current input index exceeds number" +
			" of inputs")
	}
	return []int{int(currentInput)}, nil
}

// InOutSelectorLeading selects a leading number of inputs or outputs.
type InOutSelectorLeading struct {
	Count int
}

// SelectInputs returns a slice containing the leading number of inputs or
// outputs.
func (s *InOutSelectorLeading) SelectInputs(tx *wire.MsgTx, currentInput uint32,
	inputs bool) ([]int, error) {

	if (s.Count > len(tx.TxIn)) && inputs ||
		(s.Count > len(tx.TxOut)) && !inputs {

		return nil, fmt.Errorf("selected number of inputs exceeds " +
			"number of inputs")
	}
	selected := make([]int, s.Count)
	for i := 0; i < s.Count; i++ {
		selected[i] = i
	}
	return selected, nil
}

// InOutSelectorIndividual selects individual inputs or outputs.
type InOutSelectorIndividual struct {
	Indices []int
}

// SelectInputs returns a slice containing the selected inputs or outputs.
func (s *InOutSelectorIndividual) SelectInputs(tx *wire.MsgTx,
	currentInput uint32, inputs bool) ([]int, error) {

	for _, idx := range s.Indices {
		if idx >= len(tx.TxIn) && inputs ||
			idx >= len(tx.TxOut) && !inputs {

			return nil, fmt.Errorf(
				"selected input index exceeds number of inputs",
			)
		}
	}
	return s.Indices, nil
}

// newInOutSelector creates a new InOutSelector from a byte slice.
func newInOutSelector(txfs []byte) (InOutSelector, bool, []byte, error) {
	first := (txfs)[0]
	txfs = txfs[1:]

	commitNumber := first&TXFSInoutNumber != 0

	selection := first & (0xff ^ TXFSInoutNumber)

	var selector InOutSelector
	switch {
	case selection == TXFSInoutSelectionNone:
		selector = &InOutSelectorNone{}

	case selection == TXFSInoutSelectionAll:
		selector = &InOutSelectorAll{}

	case selection == TXFSInoutSelectionCurrent:
		selector = &InOutSelectorCurrent{}

	// Leading mode
	case selection&TXFSInoutSelectionMode == 0:
		var count int
		if selection&TXFSInoutSelectionSize == 0 {
			count = int(selection & TXFSInoutSelectionMask)
		} else {
			txfs = txfs[1:]
			if selection&TXFSInoutSelectionMask == 0 {
				return nil, false, nil, fmt.Errorf(
					"leading mode with size selection" +
						"but no size given")
			}
			count = (int(selection&TXFSInoutSelectionMask) << 8) +
				int(txfs[0])
		}

		if count == 0 {
			return nil, false, nil, fmt.Errorf("leading mode with" +
				" size selection but no size given")
		}
		selector = &InOutSelectorLeading{Count: count}

	// Individual mode
	default:
		count := int(selection & TXFSInoutSelectionMask)
		if count == 0 {
			return nil, false, nil, fmt.Errorf("can't select 0 " +
				"in/outputs in individual mode")
		}
		if len(txfs) < count {
			return nil, false, nil, fmt.Errorf("not enough " +
				"single-byte indices")
		}
		indices := make([]int, count)
		for i := 0; i < count; i++ {
			if selection&TXFSInoutSelectionSize == 0 {
				indices[i] = int(txfs[0])
				txfs = txfs[1:]
			} else {
				first := txfs[0]
				second := txfs[1]
				indices[i] = (int(first) << 8) + int(second)
				txfs = txfs[2:]
			}
		}
		selector = &InOutSelectorIndividual{Indices: indices}
	}

	return selector, commitNumber, txfs, nil
}

// inOutSelectorToBytes serializes an InOutSelector to a byte slice.
func inOutSelectorToBytes(selector InOutSelector,
	commitNumber bool) ([]byte, error) {

	var first byte
	var txfs []byte

	switch s := selector.(type) {
	case *InOutSelectorNone:
		first = TXFSInoutSelectionNone

	case *InOutSelectorAll:
		first = TXFSInoutSelectionAll

	case *InOutSelectorCurrent:
		first = TXFSInoutSelectionCurrent

	case *InOutSelectorLeading:
		count := s.Count
		if count <= int(TXFSInoutSelectionMask) {
			first = byte(count) & TXFSInoutSelectionMask
		} else {
			first = byte(uint8(count>>8)&TXFSInoutSelectionMask) | TXFSInoutSelectionSize
			txfs = append(txfs, byte(count&0xFF))
		}

	case *InOutSelectorIndividual:
		count := len(s.Indices)
		first = TXFSInoutSelectionMode | byte(uint8(count)&TXFSInoutSelectionMask)
		if count > int(TXFSInoutSelectionMask) {
			first |= TXFSInoutSelectionSize
		}
		for _, idx := range s.Indices {
			if count > int(TXFSInoutSelectionMask) {
				txfs = append(txfs, byte((idx>>8)&0xFF), byte(idx&0xFF))
			} else {
				txfs = append(txfs, byte(idx&0xFF))
			}
		}

	default:
		return nil, fmt.Errorf("unknown InOutSelector type")
	}

	if commitNumber {
		first |= TXFSInoutNumber
	}

	// Prepend the first byte
	txfs = append([]byte{first}, txfs...)

	return txfs, nil
}

// InputSelector is used to get the hash of the inputs of a transaction, as
// specified by the TxFieldSelector.
type InputSelector struct {
	// InputsCache is a cache that can be used to speed up the hashing of
	// inputs.
	InputsCache *TxfsInputsCache

	// CommitNumber is true if the number of inputs should be committed to
	// the TxHash.
	CommitNumber bool

	// PrevOuts is true if the previous outpoints should be hashed into the
	// TxHash.
	PrevOuts bool

	// Sequences is true if the Sequences should be hashed into the
	// TxHash.
	Sequences bool

	// ScriptSigs is true if the ScriptSigs should be hashed into the
	// TxHash.
	ScriptSigs bool

	// PrevScriptPubkeys is true if the previous scriptPubkeys should be
	// hashed into the TxHash.
	PrevScriptPubkeys bool

	// PrevValues is true if the previous values should be hashed into the
	// TxHash.
	PrevValues bool

	// TaprootAnnexes is true if the annexes of the taproot inputs should
	// be hashed into the TxHash.
	TaprootAnnexes bool

	// InOutSelector is the InOutSelector that selects the inputs that
	// should be used to calculate the TxHash.
	InOutSelector InOutSelector
}

// NewInputSelectorFromBytes creates a new inputSelector from a byte slice.
func NewInputSelectorFromBytes(inoutFields byte, txfs []byte,
	inputsCache *TxfsInputsCache) (*InputSelector, []byte, error) {

	inputSelector := &InputSelector{
		PrevOuts:          inoutFields&TXFSInputsPrevouts != 0,
		Sequences:         inoutFields&TXFSInputsSequences != 0,
		ScriptSigs:        inoutFields&TXFSInputsScriptsigs != 0,
		PrevScriptPubkeys: inoutFields&TXFSInputsPrevScriptpubkeys != 0,
		PrevValues:        inoutFields&TXFSInputsPrevValues != 0,
		TaprootAnnexes:    inoutFields&TXFSInputsTaprootAnnexes != 0,
		InputsCache:       inputsCache,
	}

	var err error

	inputSelector.InOutSelector, inputSelector.CommitNumber,
		txfs, err = newInOutSelector(txfs)

	if err != nil {
		return nil, nil, err
	}

	return inputSelector, txfs, nil
}

// writeInputsHash writes the hash of the inputs of a transaction to the given
// buffer.
func (s *InputSelector) writeInputsHash(txHashBuffer *bytes.Buffer,
	tx *wire.MsgTx, prevoutFetcher PrevOutputFetcher,
	currentInputIdx uint32) error {

	// Select the inputs that we should hash into the TxHash.
	inputIndices, err := s.InOutSelector.SelectInputs(
		tx, currentInputIdx, true,
	)
	if err != nil {
		return err
	}

	// Write the number of inputs to the buffer if the CommitNumber bit is
	// set.
	if s.CommitNumber {
		binary.Write(txHashBuffer, binary.LittleEndian, uint32(len(tx.TxIn)))
	}

	if len(inputIndices) > 0 && s.PrevOuts {
		// If we have a cache, use it to speed up the hashing.
		if s.InputsCache != nil {
			bufferHash := s.InputsCache.GetPrevoutHash(
				tx, inputIndices,
			)
			txHashBuffer.Write(bufferHash)
		} else {
			var buffer bytes.Buffer
			for _, idx := range inputIndices {
				wire.WriteOutPoint(
					&buffer, 0, tx.Version,
					&tx.TxIn[idx].PreviousOutPoint,
				)
			}
			bufferHash := chainhash.HashB(buffer.Bytes())
			txHashBuffer.Write(bufferHash)
		}
	}

	if len(inputIndices) > 0 && s.Sequences {
		// If we have a cache, use it to speed up the hashing.
		if s.InputsCache != nil {
			bufferHash := s.InputsCache.GetSequenceHash(
				tx, inputIndices,
			)
			txHashBuffer.Write(bufferHash)
		} else {
			var buffer bytes.Buffer
			for _, idx := range inputIndices {
				binary.Write(
					&buffer, binary.LittleEndian,
					tx.TxIn[idx].Sequence,
				)
			}
			bufferHash := chainhash.HashB(buffer.Bytes())
			txHashBuffer.Write(bufferHash)
		}
	}

	if len(inputIndices) > 0 && s.ScriptSigs {
		// If we have a cache, use it to speed up the hashing.
		if s.InputsCache != nil {
			bufferHash := s.InputsCache.GetScriptSigHash(
				tx, inputIndices,
			)
			txHashBuffer.Write(bufferHash)
		} else {
			var buffer bytes.Buffer
			for _, idx := range inputIndices {
				buffer.Write(
					chainhash.HashB(tx.TxIn[idx].SignatureScript),
				)
			}
			bufferHash := chainhash.HashB(buffer.Bytes())
			txHashBuffer.Write(bufferHash)
		}
	}

	if len(inputIndices) > 0 && s.PrevScriptPubkeys {
		// If we have a cache, use it to speed up the hashing.
		if s.InputsCache != nil {
			bufferHash := s.InputsCache.GetPrevScriptPubkeyHash(
				tx, inputIndices, prevoutFetcher,
			)
			txHashBuffer.Write(bufferHash)
		} else {
			var buffer bytes.Buffer
			for _, idx := range inputIndices {
				prevout := prevoutFetcher.FetchPrevOutput(
					tx.TxIn[idx].PreviousOutPoint,
				)
				buffer.Write(chainhash.HashB(prevout.PkScript))
			}
			bufferHash := chainhash.HashB(buffer.Bytes())
			txHashBuffer.Write(bufferHash)
		}
	}

	if len(inputIndices) > 0 && s.PrevValues {
		// If we have a cache, use it to speed up the hashing.
		if s.InputsCache != nil {
			bufferHash := s.InputsCache.GetPrevValueHash(
				tx, inputIndices, prevoutFetcher,
			)
			txHashBuffer.Write(bufferHash)
		} else {
			var buffer bytes.Buffer
			for _, idx := range inputIndices {
				prevout := prevoutFetcher.FetchPrevOutput(
					tx.TxIn[idx].PreviousOutPoint,
				)
				binary.Write(
					&buffer, binary.LittleEndian, prevout.Value,
				)
			}
			bufferHash := chainhash.HashB(buffer.Bytes())
			txHashBuffer.Write(bufferHash)
		}
	}

	if len(inputIndices) > 0 && s.TaprootAnnexes {
		// If we have a cache, use it to speed up the hashing.
		if s.InputsCache != nil {
			bufferHash := s.InputsCache.GetTaprootAnnexHash(
				tx, inputIndices, prevoutFetcher,
			)
			txHashBuffer.Write(bufferHash)
		} else {
			var buffer bytes.Buffer
			for _, idx := range inputIndices {
				if IsPayToTaproot(prevoutFetcher.FetchPrevOutput(
					tx.TxIn[idx].PreviousOutPoint).PkScript,
				) {

					if isAnnexedWitness(tx.TxIn[idx].Witness) {
						annex, err := extractAnnex(
							tx.TxIn[idx].Witness,
						)
						if err != nil {
							return err
						}
						buffer.Write(chainhash.HashB(annex))
					} else {
						buffer.Write(chainhash.HashB([]byte{}))
					}
				} else {
					buffer.Write(chainhash.HashB([]byte{}))
				}
			}
			bufferHash := chainhash.HashB(buffer.Bytes())
			txHashBuffer.Write(bufferHash)
		}
	}

	return nil
}

// OutputSelector is used to get the hash of the outputs of a transaction, as
// specified by the TxFieldSelector.
type OutputSelector struct {
	// OutputsCache is a cache that can be used to speed up the hashing of
	// outputs.
	OutputsCache *TxfsOutputsCache

	// CommitNumber is true if the number of outputs should be committed to
	// the TxHash.
	CommitNumber bool

	// ScriptPubkeys is true if the ScriptPubkeys should be hashed into the
	// TxHash.
	ScriptPubkeys bool

	// Values is true if the Values should be hashed into the TxHash.
	Values bool

	// InOutSelector is the InOutSelector that selects the outputs that
	// should be used to calculate the TxHash.
	InOutSelector InOutSelector
}

// newOutputSelectorFromBytes creates a new outputSelector from a byte slice.
func newOutputSelectorFromBytes(inoutFields byte, txfs []byte,
	outputsCache *TxfsOutputsCache) (*OutputSelector, []byte, error) {

	outputSelector := &OutputSelector{
		ScriptPubkeys: inoutFields&TXFSOutputsScriptpubkeys != 0,
		Values:        inoutFields&TXFSOutputsValues != 0,
		OutputsCache:  outputsCache,
	}

	var err error

	outputSelector.InOutSelector, outputSelector.CommitNumber,
		txfs, err = newInOutSelector(txfs)

	if err != nil {
		return nil, nil, err
	}

	return outputSelector, txfs, nil
}

// writeOutputsHash writes the hash of the outputs of a transaction to the given
// buffer.
func (s *OutputSelector) writeOutputsHash(sigMsg *bytes.Buffer, tx *wire.MsgTx,
	currentInputIdx uint32) error {

	// Select the outputs that we should hash into the TxHash.
	outputIndices, err := s.InOutSelector.SelectInputs(
		tx, currentInputIdx, false,
	)
	if err != nil {
		return err
	}

	// Write the number of outputs to the buffer if the CommitNumber bit is
	// set.
	if s.CommitNumber {
		binary.Write(sigMsg, binary.LittleEndian, uint32(len(tx.TxOut)))
	}

	if len(outputIndices) > 0 && s.ScriptPubkeys {
		if s.OutputsCache != nil {
			bufferHash := s.OutputsCache.GetScriptPubkeyHash(
				tx, outputIndices,
			)
			sigMsg.Write(bufferHash)
		} else {
			var buffer bytes.Buffer
			for _, idx := range outputIndices {
				buffer.Write(chainhash.HashB(tx.TxOut[idx].PkScript))
			}
			bufferHash := chainhash.HashB(buffer.Bytes())
			sigMsg.Write(bufferHash)
		}
	}

	if len(outputIndices) > 0 && s.Values {
		if s.OutputsCache != nil {
			bufferHash := s.OutputsCache.GetValueHash(
				tx, outputIndices,
			)
			sigMsg.Write(bufferHash)
		} else {
			var buffer bytes.Buffer
			for _, idx := range outputIndices {
				binary.Write(
					&buffer, binary.LittleEndian,
					tx.TxOut[idx].Value,
				)
			}
			bufferHash := chainhash.HashB(buffer.Bytes())
			sigMsg.Write(bufferHash)
		}
	}

	return nil
}

// TxFieldSelector is used to select the fields of a transaction that should be
// hashed into the TxHash.
type TxFieldSelector struct {
	// Control is true if the TxFieldSelector itself should be hashed into
	// the TxHash.
	Control bool

	// Version is true if the version should be hashed into the TxHash.
	Version bool

	// LockTime is true if the locktime should be hashed into the TxHash.
	LockTime bool

	// CurrentInputIdx is true if the current input index should be hashed
	// into the TxHash.
	CurrentInputIdx bool

	// CurrentInputControlBlock is true if the control block of the current
	// input should be hashed into the TxHash.
	CurrentInputControlBlock bool

	// CurrentInputLastCodeseparatorPos is true if the last codeseparator
	// position of the current input should be hashed into the TxHash.
	CurrentInputLastCodeseparatorPos bool

	// Inputs is the InputSelector that selects the inputs that should be
	// used to calculate the TxHash.
	Inputs *InputSelector

	// Outputs is the OutputSelector that selects the outputs that should
	// be used to calculate the TxHash.
	Outputs *OutputSelector
}

// NewTxFieldSelectorFromBytes creates a new TxFieldSelector from a byte slice.
func NewTxFieldSelectorFromBytes(txfs []byte, inputsCache *TxfsInputsCache,
	outputsCache *TxfsOutputsCache) (*TxFieldSelector, error) {

	if len(txfs) == 0 {
		txfs = TXFSSpecialTemplate[:]
	} else if len(txfs) == 1 && txfs[0] == 0x00 {
		txfs = TXFSSpecialAll[:]
	}

	// The first byte is the global byte.
	global := txfs[0]

	// Create the TxFieldSelector from the global byte.
	fields := &TxFieldSelector{
		Control:                          global&TXFSControl != 0,
		Version:                          global&TXFSVersion != 0,
		LockTime:                         global&TXFSLockTime != 0,
		CurrentInputIdx:                  global&TXFSCurrentInputIdx != 0,
		CurrentInputControlBlock:         global&TXFSCurrentInputControlBlock != 0,
		CurrentInputLastCodeseparatorPos: global&TXFSCurrentInputLastCodeseparatorPos != 0,
	}

	// Stop early if no inputs or outputs are selected.
	if global&TXFSInputs == 0 && global&TXFSOutputs == 0 {
		return fields, nil
	}

	if len(txfs) < 2 {
		return nil, fmt.Errorf("in- or output bit set but only one byte")
	}

	txfs = txfs[1:]

	// The second byte is the in- and output byte.
	inoutFields := txfs[0]

	txfs = txfs[1:]

	var err error
	// If the TXFSInputs bit is set at the global byte, create the
	// InputSelector from the in- and output byte.
	if global&TXFSInputs != 0 {
		fields.Inputs, txfs, err = NewInputSelectorFromBytes(
			inoutFields, txfs, inputsCache,
		)
		if err != nil {
			return nil, err
		}
	}

	// If the TXFSOutputs bit is set at the global byte, create the
	// OutputSelector from the in- and output byte.
	if global&TXFSOutputs != 0 {
		fields.Outputs, _, err = newOutputSelectorFromBytes(
			inoutFields, txfs, outputsCache,
		)
		if err != nil {
			return nil, err
		}
	}

	return fields, nil
}

// GetTxHash returns the TxHash of a transaction, as specified by the
// TxFieldSelector.
func (f *TxFieldSelector) GetTxHash(tx *wire.MsgTx, currentInputIdx uint32,
	prevOutputFetcher PrevOutputFetcher,
	currentInputLastCodeseparatorPos uint32) ([]byte, error) {

	var sigMsg bytes.Buffer

	// If the control bit is set, write the full TxFieldSelector to the
	// buffer.
	if f.Control {
		fullTxfs, err := f.ToBytes()
		if err != nil {
			return nil, err
		}
		sigMsg.Write(fullTxfs)
	}

	// Now we'll write the version, locktime, current input index,
	// current input control block and current input last codeseparator
	// position to the buffer.

	if f.Version {
		binary.Write(&sigMsg, binary.LittleEndian, tx.Version)
	}

	if f.LockTime {
		binary.Write(&sigMsg, binary.LittleEndian, tx.LockTime)
	}

	if f.CurrentInputIdx {
		binary.Write(&sigMsg, binary.LittleEndian, currentInputIdx)
	}

	if f.CurrentInputControlBlock {
		controlBytes := []byte{}

		prevout := prevOutputFetcher.FetchPrevOutput(
			tx.TxIn[currentInputIdx].PreviousOutPoint,
		)

		// If the prevout is p2tr, unwrap the control block.
		if IsPayToTaproot(prevout.PkScript) {
			controlBytes = extractControlBlock(
				tx.TxIn[currentInputIdx].Witness,
			)
		}

		sigMsg.Write(chainhash.HashB(controlBytes))
	}

	if f.CurrentInputLastCodeseparatorPos {
		if currentInputLastCodeseparatorPos == 0 {
			// Set the current input last codeseparator position to
			// the max value if it is not set.
			currentInputLastCodeseparatorPos = 0xffffffff
		}
		binary.Write(
			&sigMsg, binary.LittleEndian,
			currentInputLastCodeseparatorPos,
		)
	}

	// If we have an InputSelector, write the hash of the inputs to the
	// buffer.
	if f.Inputs != nil {
		err := f.Inputs.writeInputsHash(
			&sigMsg, tx, prevOutputFetcher, currentInputIdx,
		)
		if err != nil {
			return nil, err
		}
	}

	// If we have an OutputSelector, write the hash of the outputs to the
	// buffer.
	if f.Outputs != nil {
		err := f.Outputs.writeOutputsHash(&sigMsg, tx, currentInputIdx)
		if err != nil {
			return nil, err
		}
	}

	// Return the hash of the buffer.
	return chainhash.HashB(sigMsg.Bytes()), nil
}

// ToBytes serializes a TxFieldSelector to a byte slice.
func (f *TxFieldSelector) ToBytes() ([]byte, error) {
	var txfs []byte
	var global byte
	if f.Control {
		global |= TXFSControl
	}
	if f.Version {
		global |= TXFSVersion
	}
	if f.LockTime {
		global |= TXFSLockTime
	}
	if f.CurrentInputIdx {
		global |= TXFSCurrentInputIdx
	}
	if f.CurrentInputControlBlock {
		global |= TXFSCurrentInputControlBlock
	}
	if f.CurrentInputLastCodeseparatorPos {
		global |= TXFSCurrentInputLastCodeseparatorPos
	}
	var err error
	var inout byte

	var inputsBytes []byte
	if f.Inputs != nil {
		global |= TXFSInputs

		if f.Inputs.PrevOuts {
			inout |= TXFSInputsPrevouts
		}
		if f.Inputs.Sequences {
			inout |= TXFSInputsSequences
		}
		if f.Inputs.ScriptSigs {
			inout |= TXFSInputsScriptsigs
		}
		if f.Inputs.PrevScriptPubkeys {
			inout |= TXFSInputsPrevScriptpubkeys
		}
		if f.Inputs.PrevValues {
			inout |= TXFSInputsPrevValues
		}
		if f.Inputs.TaprootAnnexes {
			inout |= TXFSInputsTaprootAnnexes
		}

		// Serialize the InOutSelector part
		inputsBytes, err = inOutSelectorToBytes(
			f.Inputs.InOutSelector, f.Inputs.CommitNumber,
		)
		if err != nil {
			return nil, err
		}
	}

	var outputsBytes []byte
	if f.Outputs != nil {
		global |= TXFSOutputs

		if f.Outputs.ScriptPubkeys {
			inout |= TXFSOutputsScriptpubkeys
		}
		if f.Outputs.Values {
			inout |= TXFSOutputsValues
		}

		// Serialize the InOutSelector part
		outputsBytes, err = inOutSelectorToBytes(
			f.Outputs.InOutSelector, f.Outputs.CommitNumber,
		)
		if err != nil {
			return nil, err
		}

	}

	txfs = append(txfs, global)
	if inout != 0 {
		txfs = append(txfs, inout)
	}

	if len(inputsBytes) > 0 {
		txfs = append(txfs, inputsBytes...)
	}
	if len(outputsBytes) > 0 {
		txfs = append(txfs, outputsBytes...)
	}

	return txfs, nil
}

// TxfsInputsCache is a cache that can be used to speed up the hashing of
// inputs.
type TxfsInputsCache struct {
	// PrevoutHashes is a map from the previous outpoint to the hash of the
	// previous outpoint.
	PrevoutHashes map[string][]byte

	// SequenceHashes is a map from the previous outpoint to the hash of
	// the sequence.
	SequenceHashes map[string][]byte

	// ScriptSigHashes is a map from the previous outpoint to the hash of
	// the scriptSig.
	ScriptSigHashes map[string][]byte

	// PrevScriptPubkeyHashes is a map from the previous outpoint to the
	// hash of the previous scriptPubkey.
	PrevScriptPubkeyHashes map[string][]byte

	// PrevValueHashes is a map from the previous outpoint to the hash of
	// the previous value.
	PrevValueHashes map[string][]byte

	// TaprootAnnexHashes is a map from the previous outpoint to the hash
	// of the annex of the taproot input.
	TaprootAnnexHashes map[string][]byte
}

// NewTxfsInputsCache creates a new TxfsInputsCache.
func NewTxfsInputsCache() *TxfsInputsCache {
	return &TxfsInputsCache{
		PrevoutHashes:          make(map[string][]byte),
		SequenceHashes:         make(map[string][]byte),
		ScriptSigHashes:        make(map[string][]byte),
		PrevScriptPubkeyHashes: make(map[string][]byte),
		PrevValueHashes:        make(map[string][]byte),
		TaprootAnnexHashes:     make(map[string][]byte),
	}
}

// GetPrevoutHash returns the hash of the previous outpoint. If the hash is not
// in the cache, it is calculated and added to the cache.
func (c *TxfsInputsCache) GetPrevoutHash(tx *wire.MsgTx,
	inputIndices []int) []byte {

	// Create a string from the tx hash and the input indices. This is
	// used as a key in the cache.
	var buffer bytes.Buffer
	for _, idx := range inputIndices {
		binary.Write(&buffer, binary.LittleEndian, uint32(idx))
	}
	key := hex.EncodeToString(buffer.Bytes())

	hash, ok := c.PrevoutHashes[key]
	if !ok {
		var buffer bytes.Buffer
		for _, idx := range inputIndices {
			wire.WriteOutPoint(
				&buffer, 0, tx.Version,
				&tx.TxIn[idx].PreviousOutPoint,
			)
		}
		hash = chainhash.HashB(buffer.Bytes())
		c.PrevoutHashes[key] = hash
	}

	return hash
}

// GetSequenceHash returns the hash of the sequence. If the hash is not in the
// cache, it is calculated and added to the cache.
func (c *TxfsInputsCache) GetSequenceHash(tx *wire.MsgTx,
	inputIndices []int) []byte {

	// Create a string from the tx hash and the input indices. This is
	// used as a key in the cache.
	var buffer bytes.Buffer
	for _, idx := range inputIndices {
		binary.Write(&buffer, binary.LittleEndian, uint32(idx))
	}
	key := hex.EncodeToString(buffer.Bytes())

	hash, ok := c.SequenceHashes[key]
	if !ok {
		var buffer bytes.Buffer
		for _, idx := range inputIndices {
			binary.Write(
				&buffer, binary.LittleEndian,
				tx.TxIn[idx].Sequence,
			)
		}
		hash = chainhash.HashB(buffer.Bytes())
		c.SequenceHashes[key] = hash
	}

	return hash
}

// GetScriptSigHash returns the hash of the scriptSig. If the hash is not in the
// cache, it is calculated and added to the cache.
func (c *TxfsInputsCache) GetScriptSigHash(tx *wire.MsgTx,
	inputIndices []int) []byte {

	// Create a string from the tx hash and the input indices. This is
	// used as a key in the cache.
	var buffer bytes.Buffer
	for _, idx := range inputIndices {
		binary.Write(&buffer, binary.LittleEndian, uint32(idx))
	}
	key := hex.EncodeToString(buffer.Bytes())

	hash, ok := c.ScriptSigHashes[key]
	if !ok {
		var buffer bytes.Buffer
		for _, idx := range inputIndices {
			buffer.Write(
				chainhash.HashB(tx.TxIn[idx].SignatureScript),
			)
		}
		hash = chainhash.HashB(buffer.Bytes())
		c.ScriptSigHashes[key] = hash
	}

	return hash
}

// GetPrevScriptPubkeyHash returns the hash of the previous scriptPubkey. If the
// hash is not in the cache, it is calculated and added to the cache.
func (c *TxfsInputsCache) GetPrevScriptPubkeyHash(tx *wire.MsgTx,
	inputIndices []int, prevOutputFetcher PrevOutputFetcher) []byte {

	// Create a string from the tx hash and the input indices. This is
	// used as a key in the cache.
	var buffer bytes.Buffer
	for _, idx := range inputIndices {
		binary.Write(&buffer, binary.LittleEndian, uint32(idx))
	}
	key := hex.EncodeToString(buffer.Bytes())

	hash, ok := c.PrevScriptPubkeyHashes[key]
	if !ok {
		var buffer bytes.Buffer
		for _, idx := range inputIndices {
			prevout := prevOutputFetcher.FetchPrevOutput(
				tx.TxIn[idx].PreviousOutPoint,
			)
			buffer.Write(chainhash.HashB(prevout.PkScript))
		}
		hash = chainhash.HashB(buffer.Bytes())
		c.PrevScriptPubkeyHashes[key] = hash
	}

	return hash
}

// GetPrevValueHash returns the hash of the previous value. If the hash is not
// in the cache, it is calculated and added to the cache.
func (c *TxfsInputsCache) GetPrevValueHash(tx *wire.MsgTx,
	inputIndices []int, prevOutputFetcher PrevOutputFetcher) []byte {

	// Create a string from the tx hash and the input indices. This is
	// used as a key in the cache.
	var buffer bytes.Buffer
	for _, idx := range inputIndices {
		binary.Write(&buffer, binary.LittleEndian, uint32(idx))
	}
	key := hex.EncodeToString(buffer.Bytes())

	hash, ok := c.PrevValueHashes[key]
	if !ok {
		var buffer bytes.Buffer
		for _, idx := range inputIndices {
			prevout := prevOutputFetcher.FetchPrevOutput(
				tx.TxIn[idx].PreviousOutPoint,
			)
			binary.Write(
				&buffer, binary.LittleEndian, prevout.Value,
			)
		}
		hash = chainhash.HashB(buffer.Bytes())
		c.PrevValueHashes[key] = hash
	}

	return hash
}

// GetTaprootAnnexHash returns the hash of the annex of the taproot input. If
// the hash is not in the cache, it is calculated and added to the cache.
func (c *TxfsInputsCache) GetTaprootAnnexHash(tx *wire.MsgTx,
	inputIndices []int, prevOutputFetcher PrevOutputFetcher) []byte {

	// Create a string from the tx hash and the input indices. This is
	// used as a key in the cache.
	var buffer bytes.Buffer
	for _, idx := range inputIndices {
		binary.Write(&buffer, binary.LittleEndian, uint32(idx))
	}
	key := hex.EncodeToString(buffer.Bytes())

	hash, ok := c.TaprootAnnexHashes[key]
	if !ok {
		var buffer bytes.Buffer
		for _, idx := range inputIndices {
			if IsPayToTaproot(prevOutputFetcher.FetchPrevOutput(
				tx.TxIn[idx].PreviousOutPoint).PkScript,
			) {

				if isAnnexedWitness(tx.TxIn[idx].Witness) {
					annex, err := extractAnnex(
						tx.TxIn[idx].Witness,
					)
					if err != nil {
						return nil
					}
					buffer.Write(chainhash.HashB(annex))
				} else {
					buffer.Write(chainhash.HashB([]byte{}))
				}
			} else {
				buffer.Write(chainhash.HashB([]byte{}))
			}
		}
		hash = chainhash.HashB(buffer.Bytes())
		c.TaprootAnnexHashes[key] = hash
	}

	return hash
}

// TxfsOutputsCache is a cache that can be used to speed up the hashing of
// outputs.
type TxfsOutputsCache struct {
	// ScriptPubkeyHashes is a map from the output index to the hash of the
	// scriptPubkey.
	ScriptPubkeyHashes map[string][]byte

	// ValueHashes is a map from the output index to the hash of the value.
	ValueHashes map[string][]byte
}

// NewTxfsOutputsCache creates a new TxfsOutputsCache.
func NewTxfsOutputsCache() *TxfsOutputsCache {
	return &TxfsOutputsCache{
		ScriptPubkeyHashes: make(map[string][]byte),
		ValueHashes:        make(map[string][]byte),
	}
}

// GetScriptPubkeyHash returns the hash of the scriptPubkey. If the hash is not
// in the cache, it is calculated and added to the cache.
func (c *TxfsOutputsCache) GetScriptPubkeyHash(tx *wire.MsgTx,
	outputIndices []int) []byte {

	// Create a string from the tx hash and the output indices. This is
	// used as a key in the cache.
	var buffer bytes.Buffer
	for _, idx := range outputIndices {
		binary.Write(&buffer, binary.LittleEndian, uint32(idx))
	}
	key := hex.EncodeToString(buffer.Bytes())

	hash, ok := c.ScriptPubkeyHashes[key]
	if !ok {
		var buffer bytes.Buffer
		for _, idx := range outputIndices {
			buffer.Write(chainhash.HashB(tx.TxOut[idx].PkScript))
		}
		hash = chainhash.HashB(buffer.Bytes())
		c.ScriptPubkeyHashes[key] = hash
	}

	return hash
}

// GetValueHash returns the hash of the value. If the hash is not in the cache,
// it is calculated and added to the cache.
func (c *TxfsOutputsCache) GetValueHash(tx *wire.MsgTx,
	outputIndices []int) []byte {

	// Create a string from the tx hash and the output indices. This is
	// used as a key in the cache.
	var buffer bytes.Buffer
	for _, idx := range outputIndices {
		binary.Write(&buffer, binary.LittleEndian, uint32(idx))
	}
	key := hex.EncodeToString(buffer.Bytes())

	hash, ok := c.ValueHashes[key]
	if !ok {
		var buffer bytes.Buffer
		for _, idx := range outputIndices {
			binary.Write(
				&buffer, binary.LittleEndian,
				tx.TxOut[idx].Value,
			)
		}
		hash = chainhash.HashB(buffer.Bytes())
		c.ValueHashes[key] = hash
	}

	return hash
}
