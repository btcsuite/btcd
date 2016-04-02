// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

var (
	// hashIndexBucketName is the name of the db bucket used to house to the
	// block hash -> block height index.
	hashIndexBucketName = []byte("hashidx")

	// heightIndexBucketName is the name of the db bucket used to house to
	// the block height -> block hash index.
	heightIndexBucketName = []byte("heightidx")

	// chainStateKeyName is the name of the db key used to store the best
	// chain state.
	chainStateKeyName = []byte("chainstate")

	// spendJournalBucketName is the name of the db bucket used to house
	// transactions outputs that are spent in each block.
	spendJournalBucketName = []byte("spendjournal")

	// utxoSetBucketName is the name of the db bucket used to house the
	// unspent transaction output set.
	utxoSetBucketName = []byte("utxoset")

	// byteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	byteOrder = binary.LittleEndian
)

// errNotInMainChain signifies that a block hash or height that is not in the
// main chain was requested.
type errNotInMainChain string

// Error implements the error interface.
func (e errNotInMainChain) Error() string {
	return string(e)
}

// isNotInMainChainErr returns whether or not the passed error is an
// errNotInMainChain error.
func isNotInMainChainErr(err error) bool {
	_, ok := err.(errNotInMainChain)
	return ok
}

// errDeserialize signifies that a problem was encountered when deserializing
// data.
type errDeserialize string

// Error implements the error interface.
func (e errDeserialize) Error() string {
	return string(e)
}

// isDeserializeErr returns whether or not the passed error is an errDeserialize
// error.
func isDeserializeErr(err error) bool {
	_, ok := err.(errDeserialize)
	return ok
}

// -----------------------------------------------------------------------------
// The transaction spend journal consists of an entry for each block connected
// to the main chain which contains the transaction outputs the block spends
// serialized such that the order is the reverse of the order they were spent.
//
// This is required because reorganizing the chain necessarily entails
// disconnecting blocks to get back to the point of the fork which implies
// unspending all of the transaction outputs that each block previously spent.
// Since the utxo set, by definition, only contains unspent transaction outputs,
// the spent transaction outputs must be resurrected from somewhere.  There is
// more than one way this could be done, however this is the most straight
// forward method that does not require having a transaction index and unpruned
// blockchain.
//
// NOTE: This format is NOT self describing.  The additional details such as
// the number of entries (transaction inputs) are expected to come from the
// block itself and the utxo set.  The rationale in doing this is to save a
// significant amount of space.  This is also the reason the spent outputs are
// serialized in the reverse order they are spent because later transactions
// are allowed to spend outputs from earlier ones in the same block.
//
// The serialized format is:
//
//   [<header code><version><compressed txout>],...
//
//   Field                Type     Size
//   header code          VLQ      variable
//   version              VLQ      variable
//   compressed txout
//     compressed amount  VLQ      variable
//     compressed script  []byte   variable
//
// The serialized header code format is:
//   bit 0 - containing transaction is a coinbase
//   bits 1-x - height of the block that contains the spent txout
//
//   NOTE: The header code and version are only encoded when the spent txout was
//   the final unspent output of the containing transaction.  Otherwise, the
//   header code will be 0 and the version is not serialized at all.  This is
//   done because that information is only needed when the utxo set no longer
//   has it.
//
// Example 1:
// From block 170 in main blockchain.
//
//    1301320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c
//    <><><------------------------------------------------------------------>
//     | |                                  |
//     | version                   compressed txout
//    header code
//
//  - header code: 0x13 (coinbase, height 9)
//  - transaction version: 1
//  - compressed txout 0:
//    - 0x32: VLQ-encoded compressed amount for 5000000000 (50 BTC)
//    - 0x05: special script type pay-to-pubkey
//    - 0x11...5c: x-coordinate of the pubkey
//
// Example 2:
// Adapted from block 100025 in main blockchain.
//
//    0091f20f006edbc6c4d31bae9f1ccc38538a114bf42de65e868b99700186c64700b2fb57eadf61e106a100a7445a8c3f67898841ec
//    <><----------------------------------------------><----><><---------------------------------------------->
//     |                                |                  |   |                            |
//     |                       compressed txout            |   version             compressed txout
//    header code                                      header code
//
//  - Last spent output:
//    - header code: 0x00 (was not the final unspent output for containing tx)
//    - transaction version: Nothing since header code is 0
//    - compressed txout:
//      - 0x91f20f: VLQ-encoded compressed amount for 34405000000 (344.05 BTC)
//      - 0x00: special script type pay-to-pubkey-hash
//      - 0x6e...86: pubkey hash
//  - Second to last spent output:
//    - header code: 0x8b9970 (not coinbase, height 100024)
//    - transaction version: 1
//    - compressed txout:
//      - 0x86c647: VLQ-encoded compressed amount for 13761000000 (137.61 BTC)
//      - 0x00: special script type pay-to-pubkey-hash
//      - 0xb2...ec: pubkey hash
// -----------------------------------------------------------------------------

// spentTxOut contains a spent transaction output and potentially additional
// contextual information such as whether or not it was contained in a coinbase
// transaction, the version of the transaction it was contained in, and which
// block height the containing transaction was included in.  As described in
// the comments above, the additional contextual information will only be valid
// when this spent txout is spending the last unspent output of the containing
// transaction.
type spentTxOut struct {
	compressed bool   // The amount and public key script are compressed.
	version    int32  // The version of creating tx.
	amount     int64  // The amount of the output.
	pkScript   []byte // The public key script for the output.

	// These fields are only set when this is spending the final output of
	// the creating tx.
	height     int32 // Height of the the block containing the creating tx.
	isCoinBase bool  // Whether creating tx is a coinbase.
}

// spentTxOutHeaderCode returns the calculated header code to be used when
// serializing the provided stxo entry.
func spentTxOutHeaderCode(stxo *spentTxOut) uint64 {
	// The header code is 0 when there is no height set for the stxo.
	if stxo.height == 0 {
		return 0
	}

	// As described in the serialization format comments, the header code
	// encodes the height shifted over one bit and the coinbase flag in the
	// lowest bit.
	headerCode := uint64(stxo.height) << 1
	if stxo.isCoinBase {
		headerCode |= 0x01
	}

	return headerCode
}

// spentTxOutSerializeSize returns the number of bytes it would take to
// serialize the passed stxo according to the format described above.
func spentTxOutSerializeSize(stxo *spentTxOut) int {
	headerCode := spentTxOutHeaderCode(stxo)
	size := serializeSizeVLQ(headerCode)
	if headerCode != 0 {
		size += serializeSizeVLQ(uint64(stxo.version))
	}
	return size + compressedTxOutSize(uint64(stxo.amount), stxo.pkScript,
		stxo.version, stxo.compressed)
}

// putSpentTxOut serializes the passed stxo according to the format described
// above directly into the passed target byte slice.  The target byte slice must
// be at least large enough to handle the number of bytes returned by the
// spentTxOutSerializeSize function or it will panic.
func putSpentTxOut(target []byte, stxo *spentTxOut) int {
	headerCode := spentTxOutHeaderCode(stxo)
	offset := putVLQ(target, headerCode)
	if headerCode != 0 {
		offset += putVLQ(target[offset:], uint64(stxo.version))
	}
	return offset + putCompressedTxOut(target[offset:], uint64(stxo.amount),
		stxo.pkScript, stxo.version, stxo.compressed)
}

// decodeSpentTxOut decodes the passed serialized stxo entry, possibly followed
// by other data, into the passed stxo struct.  It returns the number of bytes
// read.
//
// Since the serialized stxo entry does not contain the height, version, or
// coinbase flag of the containing transaction when it still has utxos, the
// caller is responsible for passing in the containing transaction version in
// that case.  The provided version is ignore when it is serialized as a part of
// the stxo.
//
// An error will be returned if the version is not serialized as a part of the
// stxo and is also not provided to the function.
func decodeSpentTxOut(serialized []byte, stxo *spentTxOut, txVersion int32) (int, error) {
	// Ensure there are bytes to decode.
	if len(serialized) == 0 {
		return 0, errDeserialize("no serialized bytes")
	}

	// Deserialize the header code.
	code, offset := deserializeVLQ(serialized)
	if offset >= len(serialized) {
		return offset, errDeserialize("unexpected end of data after " +
			"header code")
	}

	// Decode the header code and deserialize the containing transaction
	// version if needed.
	//
	// Bit 0 indicates containing transaction is a coinbase.
	// Bits 1-x encode height of containing transaction.
	if code != 0 {
		version, bytesRead := deserializeVLQ(serialized[offset:])
		offset += bytesRead
		if offset >= len(serialized) {
			return offset, errDeserialize("unexpected end of data " +
				"after version")
		}

		stxo.isCoinBase = code&0x01 != 0
		stxo.height = int32(code >> 1)
		stxo.version = int32(version)
	} else {
		// Ensure a tx version was specified if the stxo did not encode
		// it.  This should never happen unless there is database
		// corruption or this function is being called without the
		// proper state.
		if txVersion == 0 {
			return offset, AssertError("decodeSpentTxOut called " +
				"without a containing tx version when the " +
				"serialized stxo that does not encode the " +
				"version")
		}
		stxo.version = txVersion
	}

	// Decode the compressed txout.
	compAmount, compScript, bytesRead, err := decodeCompressedTxOut(
		serialized[offset:], stxo.version)
	offset += bytesRead
	if err != nil {
		return offset, errDeserialize(fmt.Sprintf("unable to decode "+
			"txout: %v", err))
	}
	stxo.amount = int64(compAmount)
	stxo.pkScript = compScript
	stxo.compressed = true
	return offset, nil
}

// deserializeSpendJournalEntry decodes the passed serialized byte slice into a
// slice of spent txouts according to the format described in detail above.
//
// Since the serialization format is not self describing, as noted in the
// format comments, this function also requires the transactions that spend the
// txouts and a utxo view that contains any remaining existing utxos in the
// transactions referenced by the inputs to the passed transasctions.
func deserializeSpendJournalEntry(serialized []byte, txns []*wire.MsgTx, view *UtxoViewpoint) ([]spentTxOut, error) {
	// Calculate the total number of stxos.
	var numStxos int
	for _, tx := range txns {
		numStxos += len(tx.TxIn)
	}

	// When a block has no spent txouts there is nothing to serialize.
	if len(serialized) == 0 {
		// Ensure the block actually has no stxos.  This should never
		// happen unless there is database corruption or an empty entry
		// erroneously made its way into the database.
		if numStxos != 0 {
			return nil, AssertError(fmt.Sprintf("mismatched spend "+
				"journal serialization - no serialization for "+
				"expected %d stxos", numStxos))
		}

		return nil, nil
	}

	// Loop backwards through all transactions so everything is read in
	// reverse order to match the serialization order.
	stxoIdx := numStxos - 1
	stxoInFlight := make(map[chainhash.Hash]int)
	offset := 0
	stxos := make([]spentTxOut, numStxos)
	for txIdx := len(txns) - 1; txIdx > -1; txIdx-- {
		tx := txns[txIdx]

		// Loop backwards through all of the transaction inputs and read
		// the associated stxo.
		for txInIdx := len(tx.TxIn) - 1; txInIdx > -1; txInIdx-- {
			txIn := tx.TxIn[txInIdx]
			stxo := &stxos[stxoIdx]
			stxoIdx--

			// Get the transaction version for the stxo based on
			// whether or not it should be serialized as a part of
			// the stxo.  Recall that it is only serialized when the
			// stxo spends the final utxo of a transaction.  Since
			// they are deserialized in reverse order, this means
			// the first time an entry for a given containing tx is
			// encountered that is not already in the utxo view it
			// must have been the final spend and thus the extra
			// data will be serialized with the stxo.  Otherwise,
			// the version must be pulled from the utxo entry.
			//
			// Since the view is not actually modified as the stxos
			// are read here and it's possible later entries
			// reference earlier ones, an inflight map is maintained
			// to detect this case and pull the tx version from the
			// entry that contains the version information as just
			// described.
			var txVersion int32
			originHash := &txIn.PreviousOutPoint.Hash
			entry := view.LookupEntry(originHash)
			if entry != nil {
				txVersion = entry.Version()
			} else if idx, ok := stxoInFlight[*originHash]; ok {
				txVersion = stxos[idx].version
			} else {
				stxoInFlight[*originHash] = stxoIdx + 1
			}

			n, err := decodeSpentTxOut(serialized[offset:], stxo,
				txVersion)
			offset += n
			if err != nil {
				return nil, errDeserialize(fmt.Sprintf("unable "+
					"to decode stxo for %v: %v",
					txIn.PreviousOutPoint, err))
			}
		}
	}

	return stxos, nil
}

// serializeSpendJournalEntry serializes all of the passed spent txouts into a
// single byte slice according to the format described in detail above.
func serializeSpendJournalEntry(stxos []spentTxOut) []byte {
	if len(stxos) == 0 {
		return nil
	}

	// Calculate the size needed to serialize the entire journal entry.
	var size int
	for i := range stxos {
		size += spentTxOutSerializeSize(&stxos[i])
	}
	serialized := make([]byte, size)

	// Serialize each individual stxo directly into the slice in reverse
	// order one after the other.
	var offset int
	for i := len(stxos) - 1; i > -1; i-- {
		offset += putSpentTxOut(serialized[offset:], &stxos[i])
	}

	return serialized
}

// dbFetchSpendJournalEntry fetches the spend journal entry for the passed
// block and deserializes it into a slice of spent txout entries.  The provided
// view MUST have the utxos referenced by all of the transactions available for
// the passed block since that information is required to reconstruct the spent
// txouts.
func dbFetchSpendJournalEntry(dbTx database.Tx, block *btcutil.Block, view *UtxoViewpoint) ([]spentTxOut, error) {
	// Exclude the coinbase transaction since it can't spend anything.
	spendBucket := dbTx.Metadata().Bucket(spendJournalBucketName)
	serialized := spendBucket.Get(block.Hash()[:])
	blockTxns := block.MsgBlock().Transactions[1:]
	stxos, err := deserializeSpendJournalEntry(serialized, blockTxns, view)
	if err != nil {
		// Ensure any deserialization errors are returned as database
		// corruption errors.
		if isDeserializeErr(err) {
			return nil, database.Error{
				ErrorCode: database.ErrCorruption,
				Description: fmt.Sprintf("corrupt spend "+
					"information for %v: %v", block.Hash(),
					err),
			}
		}

		return nil, err
	}

	return stxos, nil
}

// dbPutSpendJournalEntry uses an existing database transaction to update the
// spend journal entry for the given block hash using the provided slice of
// spent txouts.   The spent txouts slice must contain an entry for every txout
// the transactions in the block spend in the order they are spent.
func dbPutSpendJournalEntry(dbTx database.Tx, blockHash *chainhash.Hash, stxos []spentTxOut) error {
	spendBucket := dbTx.Metadata().Bucket(spendJournalBucketName)
	serialized := serializeSpendJournalEntry(stxos)
	return spendBucket.Put(blockHash[:], serialized)
}

// dbRemoveSpendJournalEntry uses an existing database transaction to remove the
// spend journal entry for the passed block hash.
func dbRemoveSpendJournalEntry(dbTx database.Tx, blockHash *chainhash.Hash) error {
	spendBucket := dbTx.Metadata().Bucket(spendJournalBucketName)
	return spendBucket.Delete(blockHash[:])
}

// -----------------------------------------------------------------------------
// The unspent transaction output (utxo) set consists of an entry for each
// transaction which contains a utxo serialized using a format that is highly
// optimized to reduce space using domain specific compression algorithms.  This
// format is a slightly modified version of the format used in Bitcoin Core.
//
// The serialized format is:
//
//   <version><height><header code><unspentness bitmap>[<compressed txouts>,...]
//
//   Field                Type     Size
//   version              VLQ      variable
//   block height         VLQ      variable
//   header code          VLQ      variable
//   unspentness bitmap   []byte   variable
//   compressed txouts
//     compressed amount  VLQ      variable
//     compressed script  []byte   variable
//
// The serialized header code format is:
//   bit 0 - containing transaction is a coinbase
//   bit 1 - output zero is unspent
//   bit 2 - output one is unspent
//   bits 3-x - number of bytes in unspentness bitmap.  When both bits 1 and 2
//     are unset, it encodes N-1 since there must be at least one unspent
//     output.
//
// The rationale for the header code scheme is as follows:
//   - Transactions which only pay to a single output and a change output are
//     extremely common, thus an extra byte for the unspentness bitmap can be
//     avoided for them by encoding those two outputs in the low order bits.
//   - Given it is encoded as a VLQ which can encode values up to 127 with a
//     single byte, that leaves 4 bits to represent the number of bytes in the
//     unspentness bitmap while still only consuming a single byte for the
//     header code.  In other words, an unspentness bitmap with up to 120
//     transaction outputs can be encoded with a single-byte header code.
//     This covers the vast majority of transactions.
//   - Encoding N-1 bytes when both bits 1 and 2 are unset allows an additional
//     8 outpoints to be encoded before causing the header code to require an
//     additional byte.
//
// Example 1:
// From tx in main blockchain:
// Blk 1, 0e3e2357e806b6cdb1f70b54c3a3a17b6714ee1f0e68bebb44a74b1efd512098
//
//    010103320496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52
//    <><><><------------------------------------------------------------------>
//     | | \--------\                               |
//     | height     |                      compressed txout 0
//  version    header code
//
//  - version: 1
//  - height: 1
//  - header code: 0x03 (coinbase, output zero unspent, 0 bytes of unspentness)
//  - unspentness: Nothing since it is zero bytes
//  - compressed txout 0:
//    - 0x32: VLQ-encoded compressed amount for 5000000000 (50 BTC)
//    - 0x04: special script type pay-to-pubkey
//    - 0x96...52: x-coordinate of the pubkey
//
// Example 2:
// From tx in main blockchain:
// Blk 113931, 4a16969aa4764dd7507fc1de7f0baa4850a246de90c45e59a3207f9a26b5036f
//
//    0185f90b0a011200e2ccd6ec7c6e2e581349c77e067385fa8236bf8a800900b8025be1b3efc63b0ad48e7f9f10e87544528d58
//    <><----><><><------------------------------------------><-------------------------------------------->
//     |    |  | \-------------------\            |                            |
//  version |  \--------\       unspentness       |                    compressed txout 2
//        height     header code          compressed txout 0
//
//  - version: 1
//  - height: 113931
//  - header code: 0x0a (output zero unspent, 1 byte in unspentness bitmap)
//  - unspentness: [0x01] (bit 0 is set, so output 0+2 = 2 is unspent)
//    NOTE: It's +2 since the first two outputs are encoded in the header code
//  - compressed txout 0:
//    - 0x12: VLQ-encoded compressed amount for 20000000 (0.2 BTC)
//    - 0x00: special script type pay-to-pubkey-hash
//    - 0xe2...8a: pubkey hash
//  - compressed txout 2:
//    - 0x8009: VLQ-encoded compressed amount for 15000000 (0.15 BTC)
//    - 0x00: special script type pay-to-pubkey-hash
//    - 0xb8...58: pubkey hash
//
// Example 3:
// From tx in main blockchain:
// Blk 338156, 1b02d1c8cfef60a189017b9a420c682cf4a0028175f2f563209e4ff61c8c3620
//
//    0193d06c100000108ba5b9e763011dd46a006572d820e448e12d2bbb38640bc718e6
//    <><----><><----><-------------------------------------------------->
//     |    |  |   \-----------------\            |
//  version |  \--------\       unspentness       |
//        height     header code          compressed txout 22
//
//  - version: 1
//  - height: 338156
//  - header code: 0x10 (2+1 = 3 bytes in unspentness bitmap)
//    NOTE: It's +1 since neither bit 1 nor 2 are set, so N-1 is encoded.
//  - unspentness: [0x00 0x00 0x10] (bit 20 is set, so output 20+2 = 22 is unspent)
//    NOTE: It's +2 since the first two outputs are encoded in the header code
//  - compressed txout 22:
//    - 0x8ba5b9e763: VLQ-encoded compressed amount for 366875659 (3.66875659 BTC)
//    - 0x01: special script type pay-to-script-hash
//    - 0x1d...e6: script hash
// -----------------------------------------------------------------------------

// utxoEntryHeaderCode returns the calculated header code to be used when
// serializing the provided utxo entry and the number of bytes needed to encode
// the unspentness bitmap.
func utxoEntryHeaderCode(entry *UtxoEntry, highestOutputIndex uint32) (uint64, int, error) {
	// The first two outputs are encoded separately, so offset the index
	// accordingly to calculate the correct number of bytes needed to encode
	// up to the highest unspent output index.
	numBitmapBytes := int((highestOutputIndex + 6) / 8)

	// As previously described, one less than the number of bytes is encoded
	// when both output 0 and 1 are spent because there must be at least one
	// unspent output.  Adjust the number of bytes to encode accordingly and
	// encode the value by shifting it over 3 bits.
	output0Unspent := !entry.IsOutputSpent(0)
	output1Unspent := !entry.IsOutputSpent(1)
	var numBitmapBytesAdjustment int
	if !output0Unspent && !output1Unspent {
		if numBitmapBytes == 0 {
			return 0, 0, AssertError("attempt to serialize utxo " +
				"header for fully spent transaction")
		}
		numBitmapBytesAdjustment = 1
	}
	headerCode := uint64(numBitmapBytes-numBitmapBytesAdjustment) << 3

	// Set the coinbase, output 0, and output 1 bits in the header code
	// accordingly.
	if entry.isCoinBase {
		headerCode |= 0x01 // bit 0
	}
	if output0Unspent {
		headerCode |= 0x02 // bit 1
	}
	if output1Unspent {
		headerCode |= 0x04 // bit 2
	}

	return headerCode, numBitmapBytes, nil
}

// serializeUtxoEntry returns the entry serialized to a format that is suitable
// for long-term storage.  The format is described in detail above.
func serializeUtxoEntry(entry *UtxoEntry) ([]byte, error) {
	// Fully spent entries have no serialization.
	if entry.IsFullySpent() {
		return nil, nil
	}

	// Determine the output order by sorting the sparse output index keys.
	outputOrder := make([]int, 0, len(entry.sparseOutputs))
	for outputIndex := range entry.sparseOutputs {
		outputOrder = append(outputOrder, int(outputIndex))
	}
	sort.Ints(outputOrder)

	// Encode the header code and determine the number of bytes the
	// unspentness bitmap needs.
	highIndex := uint32(outputOrder[len(outputOrder)-1])
	headerCode, numBitmapBytes, err := utxoEntryHeaderCode(entry, highIndex)
	if err != nil {
		return nil, err
	}

	// Calculate the size needed to serialize the entry.
	size := serializeSizeVLQ(uint64(entry.version)) +
		serializeSizeVLQ(uint64(entry.blockHeight)) +
		serializeSizeVLQ(headerCode) + numBitmapBytes
	for _, outputIndex := range outputOrder {
		out := entry.sparseOutputs[uint32(outputIndex)]
		if out.spent {
			continue
		}
		size += compressedTxOutSize(uint64(out.amount), out.pkScript,
			entry.version, out.compressed)
	}

	// Serialize the version, block height of the containing transaction,
	// and header code.
	serialized := make([]byte, size)
	offset := putVLQ(serialized, uint64(entry.version))
	offset += putVLQ(serialized[offset:], uint64(entry.blockHeight))
	offset += putVLQ(serialized[offset:], headerCode)

	// Serialize the unspentness bitmap.
	for i := uint32(0); i < uint32(numBitmapBytes); i++ {
		unspentBits := byte(0)
		for j := uint32(0); j < 8; j++ {
			// The first 2 outputs are encoded via the header code,
			// so adjust the output index accordingly.
			if !entry.IsOutputSpent(2 + i*8 + j) {
				unspentBits |= 1 << uint8(j)
			}
		}
		serialized[offset] = unspentBits
		offset++
	}

	// Serialize the compressed unspent transaction outputs.  Outputs that
	// are already compressed are serialized without modifications.
	for _, outputIndex := range outputOrder {
		out := entry.sparseOutputs[uint32(outputIndex)]
		if out.spent {
			continue
		}

		offset += putCompressedTxOut(serialized[offset:],
			uint64(out.amount), out.pkScript, entry.version,
			out.compressed)
	}

	return serialized, nil
}

// deserializeUtxoEntry decodes a utxo entry from the passed serialized byte
// slice into a new UtxoEntry using a format that is suitable for long-term
// storage.  The format is described in detail above.
func deserializeUtxoEntry(serialized []byte) (*UtxoEntry, error) {
	// Deserialize the version.
	version, bytesRead := deserializeVLQ(serialized)
	offset := bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after version")
	}

	// Deserialize the block height.
	blockHeight, bytesRead := deserializeVLQ(serialized[offset:])
	offset += bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after height")
	}

	// Deserialize the header code.
	code, bytesRead := deserializeVLQ(serialized[offset:])
	offset += bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after header")
	}

	// Decode the header code.
	//
	// Bit 0 indicates whether the containing transaction is a coinbase.
	// Bit 1 indicates output 0 is unspent.
	// Bit 2 indicates output 1 is unspent.
	// Bits 3-x encodes the number of non-zero unspentness bitmap bytes that
	// follow.  When both output 0 and 1 are spent, it encodes N-1.
	isCoinBase := code&0x01 != 0
	output0Unspent := code&0x02 != 0
	output1Unspent := code&0x04 != 0
	numBitmapBytes := code >> 3
	if !output0Unspent && !output1Unspent {
		numBitmapBytes++
	}

	// Ensure there are enough bytes left to deserialize the unspentness
	// bitmap.
	if uint64(len(serialized[offset:])) < numBitmapBytes {
		return nil, errDeserialize("unexpected end of data for " +
			"unspentness bitmap")
	}

	// Create a new utxo entry with the details deserialized above to house
	// all of the utxos.
	entry := newUtxoEntry(int32(version), isCoinBase, int32(blockHeight))

	// Add sparse output for unspent outputs 0 and 1 as needed based on the
	// details provided by the header code.
	var outputIndexes []uint32
	if output0Unspent {
		outputIndexes = append(outputIndexes, 0)
	}
	if output1Unspent {
		outputIndexes = append(outputIndexes, 1)
	}

	// Decode the unspentness bitmap adding a sparse output for each unspent
	// output.
	for i := uint32(0); i < uint32(numBitmapBytes); i++ {
		unspentBits := serialized[offset]
		for j := uint32(0); j < 8; j++ {
			if unspentBits&0x01 != 0 {
				// The first 2 outputs are encoded via the
				// header code, so adjust the output number
				// accordingly.
				outputNum := 2 + i*8 + j
				outputIndexes = append(outputIndexes, outputNum)
			}
			unspentBits >>= 1
		}
		offset++
	}

	// Decode and add all of the utxos.
	for i, outputIndex := range outputIndexes {
		// Decode the next utxo.  The script and amount fields of the
		// utxo output are left compressed so decompression can be
		// avoided on those that are not accessed.  This is done since
		// it is quite common for a redeeming transaction to only
		// reference a single utxo from a referenced transaction.
		compAmount, compScript, bytesRead, err := decodeCompressedTxOut(
			serialized[offset:], int32(version))
		if err != nil {
			return nil, errDeserialize(fmt.Sprintf("unable to "+
				"decode utxo at index %d: %v", i, err))
		}
		offset += bytesRead

		entry.sparseOutputs[outputIndex] = &utxoOutput{
			spent:      false,
			compressed: true,
			pkScript:   compScript,
			amount:     int64(compAmount),
		}
	}

	return entry, nil
}

// dbFetchUtxoEntry uses an existing database transaction to fetch all unspent
// outputs for the provided Bitcoin transaction hash from the utxo set.
//
// When there is no entry for the provided hash, nil will be returned for the
// both the entry and the error.
func dbFetchUtxoEntry(dbTx database.Tx, hash *chainhash.Hash) (*UtxoEntry, error) {
	// Fetch the unspent transaction output information for the passed
	// transaction hash.  Return now when there is no entry.
	utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)
	serializedUtxo := utxoBucket.Get(hash[:])
	if serializedUtxo == nil {
		return nil, nil
	}

	// A non-nil zero-length entry means there is an entry in the database
	// for a fully spent transaction which should never be the case.
	if len(serializedUtxo) == 0 {
		return nil, AssertError(fmt.Sprintf("database contains entry "+
			"for fully spent tx %v", hash))
	}

	// Deserialize the utxo entry and return it.
	entry, err := deserializeUtxoEntry(serializedUtxo)
	if err != nil {
		// Ensure any deserialization errors are returned as database
		// corruption errors.
		if isDeserializeErr(err) {
			return nil, database.Error{
				ErrorCode: database.ErrCorruption,
				Description: fmt.Sprintf("corrupt utxo entry "+
					"for %v: %v", hash, err),
			}
		}

		return nil, err
	}

	return entry, nil
}

// dbPutUtxoView uses an existing database transaction to update the utxo set
// in the database based on the provided utxo view contents and state.  In
// particular, only the entries that have been marked as modified are written
// to the database.
func dbPutUtxoView(dbTx database.Tx, view *UtxoViewpoint) error {
	utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)
	for txHashIter, entry := range view.entries {
		// No need to update the database if the entry was not modified.
		if entry == nil || !entry.modified {
			continue
		}

		// Serialize the utxo entry without any entries that have been
		// spent.
		serialized, err := serializeUtxoEntry(entry)
		if err != nil {
			return err
		}

		// Make a copy of the hash because the iterator changes on each
		// loop iteration and thus slicing it directly would cause the
		// data to change out from under the put/delete funcs below.
		txHash := txHashIter

		// Remove the utxo entry if it is now fully spent.
		if serialized == nil {
			if err := utxoBucket.Delete(txHash[:]); err != nil {
				return err
			}

			continue
		}

		// At this point the utxo entry is not fully spent, so store its
		// serialization in the database.
		err = utxoBucket.Put(txHash[:], serialized)
		if err != nil {
			return err
		}
	}

	return nil
}

// -----------------------------------------------------------------------------
// The block index consists of two buckets with an entry for every block in the
// main chain.  One bucket is for the hash to height mapping and the other is
// for the height to hash mapping.
//
// The serialized format for values in the hash to height bucket is:
//   <height>
//
//   Field      Type     Size
//   height     uint32   4 bytes
//
// The serialized format for values in the height to hash bucket is:
//   <hash>
//
//   Field      Type             Size
//   hash       chainhash.Hash   chainhash.HashSize
// -----------------------------------------------------------------------------

// dbPutBlockIndex uses an existing database transaction to update or add the
// block index entries for the hash to height and height to hash mappings for
// the provided values.
func dbPutBlockIndex(dbTx database.Tx, hash *chainhash.Hash, height int32) error {
	// Serialize the height for use in the index entries.
	var serializedHeight [4]byte
	byteOrder.PutUint32(serializedHeight[:], uint32(height))

	// Add the block hash to height mapping to the index.
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(hashIndexBucketName)
	if err := hashIndex.Put(hash[:], serializedHeight[:]); err != nil {
		return err
	}

	// Add the block height to hash mapping to the index.
	heightIndex := meta.Bucket(heightIndexBucketName)
	return heightIndex.Put(serializedHeight[:], hash[:])
}

// dbRemoveBlockIndex uses an existing database transaction remove block index
// entries from the hash to height and height to hash mappings for the provided
// values.
func dbRemoveBlockIndex(dbTx database.Tx, hash *chainhash.Hash, height int32) error {
	// Remove the block hash to height mapping.
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(hashIndexBucketName)
	if err := hashIndex.Delete(hash[:]); err != nil {
		return err
	}

	// Remove the block height to hash mapping.
	var serializedHeight [4]byte
	byteOrder.PutUint32(serializedHeight[:], uint32(height))
	heightIndex := meta.Bucket(heightIndexBucketName)
	return heightIndex.Delete(serializedHeight[:])
}

// dbFetchHeightByHash uses an existing database transaction to retrieve the
// height for the provided hash from the index.
func dbFetchHeightByHash(dbTx database.Tx, hash *chainhash.Hash) (int32, error) {
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(hashIndexBucketName)
	serializedHeight := hashIndex.Get(hash[:])
	if serializedHeight == nil {
		str := fmt.Sprintf("block %s is not in the main chain", hash)
		return 0, errNotInMainChain(str)
	}

	return int32(byteOrder.Uint32(serializedHeight)), nil
}

// dbFetchHashByHeight uses an existing database transaction to retrieve the
// hash for the provided height from the index.
func dbFetchHashByHeight(dbTx database.Tx, height int32) (*chainhash.Hash, error) {
	var serializedHeight [4]byte
	byteOrder.PutUint32(serializedHeight[:], uint32(height))

	meta := dbTx.Metadata()
	heightIndex := meta.Bucket(heightIndexBucketName)
	hashBytes := heightIndex.Get(serializedHeight[:])
	if hashBytes == nil {
		str := fmt.Sprintf("no block at height %d exists", height)
		return nil, errNotInMainChain(str)
	}

	var hash chainhash.Hash
	copy(hash[:], hashBytes)
	return &hash, nil
}

// -----------------------------------------------------------------------------
// The best chain state consists of the best block hash and height, the total
// number of transactions up to and including those in the best block, and the
// accumulated work sum up to and including the best block.
//
// The serialized format is:
//
//   <block hash><block height><total txns><work sum length><work sum>
//
//   Field             Type             Size
//   block hash        chainhash.Hash   chainhash.HashSize
//   block height      uint32           4 bytes
//   total txns        uint64           8 bytes
//   work sum length   uint32           4 bytes
//   work sum          big.Int          work sum length
// -----------------------------------------------------------------------------

// bestChainState represents the data to be stored the database for the current
// best chain state.
type bestChainState struct {
	hash      chainhash.Hash
	height    uint32
	totalTxns uint64
	workSum   *big.Int
}

// serializeBestChainState returns the serialization of the passed block best
// chain state.  This is data to be stored in the chain state bucket.
func serializeBestChainState(state bestChainState) []byte {
	// Calculate the full size needed to serialize the chain state.
	workSumBytes := state.workSum.Bytes()
	workSumBytesLen := uint32(len(workSumBytes))
	serializedLen := chainhash.HashSize + 4 + 8 + 4 + workSumBytesLen

	// Serialize the chain state.
	serializedData := make([]byte, serializedLen)
	copy(serializedData[0:chainhash.HashSize], state.hash[:])
	offset := uint32(chainhash.HashSize)
	byteOrder.PutUint32(serializedData[offset:], state.height)
	offset += 4
	byteOrder.PutUint64(serializedData[offset:], state.totalTxns)
	offset += 8
	byteOrder.PutUint32(serializedData[offset:], workSumBytesLen)
	offset += 4
	copy(serializedData[offset:], workSumBytes)
	return serializedData[:]
}

// deserializeBestChainState deserializes the passed serialized best chain
// state.  This is data stored in the chain state bucket and is updated after
// every block is connected or disconnected form the main chain.
// block.
func deserializeBestChainState(serializedData []byte) (bestChainState, error) {
	// Ensure the serialized data has enough bytes to properly deserialize
	// the hash, height, total transactions, and work sum length.
	if len(serializedData) < chainhash.HashSize+16 {
		return bestChainState{}, database.Error{
			ErrorCode:   database.ErrCorruption,
			Description: "corrupt best chain state",
		}
	}

	state := bestChainState{}
	copy(state.hash[:], serializedData[0:chainhash.HashSize])
	offset := uint32(chainhash.HashSize)
	state.height = byteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	state.totalTxns = byteOrder.Uint64(serializedData[offset : offset+8])
	offset += 8
	workSumBytesLen := byteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4

	// Ensure the serialized data has enough bytes to deserialize the work
	// sum.
	if uint32(len(serializedData[offset:])) < workSumBytesLen {
		return bestChainState{}, database.Error{
			ErrorCode:   database.ErrCorruption,
			Description: "corrupt best chain state",
		}
	}
	workSumBytes := serializedData[offset : offset+workSumBytesLen]
	state.workSum = new(big.Int).SetBytes(workSumBytes)

	return state, nil
}

// dbPutBestState uses an existing database transaction to update the best chain
// state with the given parameters.
func dbPutBestState(dbTx database.Tx, snapshot *BestState, workSum *big.Int) error {
	// Serialize the current best chain state.
	serializedData := serializeBestChainState(bestChainState{
		hash:      *snapshot.Hash,
		height:    uint32(snapshot.Height),
		totalTxns: snapshot.TotalTxns,
		workSum:   workSum,
	})

	// Store the current best chain state into the database.
	return dbTx.Metadata().Put(chainStateKeyName, serializedData)
}

// createChainState initializes both the database and the chain state to the
// genesis block.  This includes creating the necessary buckets and inserting
// the genesis block, so it must only be called on an uninitialized database.
func (b *BlockChain) createChainState() error {
	// Create a new node from the genesis block and set it as the best node.
	genesisBlock := btcutil.NewBlock(b.chainParams.GenesisBlock)
	header := &genesisBlock.MsgBlock().Header
	node := newBlockNode(header, genesisBlock.Hash(), 0)
	node.inMainChain = true
	b.bestNode = node

	// Add the new node to the index which is used for faster lookups.
	b.index[*node.hash] = node

	// Initialize the state related to the best block.  Since it is the
	// genesis block, use its timestamp for the median time.
	numTxns := uint64(len(genesisBlock.MsgBlock().Transactions))
	blockSize := uint64(genesisBlock.MsgBlock().SerializeSize())
	b.stateSnapshot = newBestState(b.bestNode, blockSize, numTxns, numTxns,
		b.bestNode.timestamp)

	// Create the initial the database chain state including creating the
	// necessary index buckets and inserting the genesis block.
	err := b.db.Update(func(dbTx database.Tx) error {
		// Create the bucket that houses the chain block hash to height
		// index.
		meta := dbTx.Metadata()
		_, err := meta.CreateBucket(hashIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the chain block height to hash
		// index.
		_, err = meta.CreateBucket(heightIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the spend journal data.
		_, err = meta.CreateBucket(spendJournalBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the utxo set.  Note that the
		// genesis block coinbase transaction is intentionally not
		// inserted here since it is not spendable by consensus rules.
		_, err = meta.CreateBucket(utxoSetBucketName)
		if err != nil {
			return err
		}

		// Add the genesis block hash to height and height to hash
		// mappings to the index.
		err = dbPutBlockIndex(dbTx, b.bestNode.hash, b.bestNode.height)
		if err != nil {
			return err
		}

		// Store the current best chain state into the database.
		err = dbPutBestState(dbTx, b.stateSnapshot, b.bestNode.workSum)
		if err != nil {
			return err
		}

		// Store the genesis block into the database.
		return dbTx.StoreBlock(genesisBlock)
	})
	return err
}

// initChainState attempts to load and initialize the chain state from the
// database.  When the db does not yet contain any chain state, both it and the
// chain state are initialized to the genesis block.
func (b *BlockChain) initChainState() error {
	// Attempt to load the chain state from the database.
	var isStateInitialized bool
	err := b.db.View(func(dbTx database.Tx) error {
		// Fetch the stored chain state from the database metadata.
		// When it doesn't exist, it means the database hasn't been
		// initialized for use with chain yet, so break out now to allow
		// that to happen under a writable database transaction.
		serializedData := dbTx.Metadata().Get(chainStateKeyName)
		if serializedData == nil {
			return nil
		}
		log.Tracef("Serialized chain state: %x", serializedData)
		state, err := deserializeBestChainState(serializedData)
		if err != nil {
			return err
		}

		// Load the raw block bytes for the best block.
		blockBytes, err := dbTx.FetchBlock(&state.hash)
		if err != nil {
			return err
		}
		var block wire.MsgBlock
		err = block.Deserialize(bytes.NewReader(blockBytes))
		if err != nil {
			return err
		}

		// Create a new node and set it as the best node.  The preceding
		// nodes will be loaded on demand as needed.
		header := &block.Header
		node := newBlockNode(header, &state.hash, int32(state.height))
		node.inMainChain = true
		node.workSum = state.workSum
		b.bestNode = node

		// Add the new node to the indices for faster lookups.
		prevHash := node.parentHash
		b.index[*node.hash] = node
		b.depNodes[*prevHash] = append(b.depNodes[*prevHash], node)

		// Calculate the median time for the block.
		medianTime, err := b.calcPastMedianTime(node)
		if err != nil {
			return err
		}

		// Initialize the state related to the best block.
		blockSize := uint64(len(blockBytes))
		numTxns := uint64(len(block.Transactions))
		b.stateSnapshot = newBestState(b.bestNode, blockSize, numTxns,
			state.totalTxns, medianTime)

		isStateInitialized = true
		return nil
	})
	if err != nil {
		return err
	}

	// There is nothing more to do if the chain state was initialized.
	if isStateInitialized {
		return nil
	}

	// At this point the database has not already been initialized, so
	// initialize both it and the chain state to the genesis block.
	return b.createChainState()
}

// dbFetchHeaderByHash uses an existing database transaction to retrieve the
// block header for the provided hash.
func dbFetchHeaderByHash(dbTx database.Tx, hash *chainhash.Hash) (*wire.BlockHeader, error) {
	headerBytes, err := dbTx.FetchBlockHeader(hash)
	if err != nil {
		return nil, err
	}

	var header wire.BlockHeader
	err = header.Deserialize(bytes.NewReader(headerBytes))
	if err != nil {
		return nil, err
	}

	return &header, nil
}

// dbFetchHeaderByHeight uses an existing database transaction to retrieve the
// block header for the provided height.
func dbFetchHeaderByHeight(dbTx database.Tx, height int32) (*wire.BlockHeader, error) {
	hash, err := dbFetchHashByHeight(dbTx, height)
	if err != nil {
		return nil, err
	}

	return dbFetchHeaderByHash(dbTx, hash)
}

// dbFetchBlockByHash uses an existing database transaction to retrieve the raw
// block for the provided hash, deserialize it, retrieve the appropriate height
// from the index, and return a btcutil.Block with the height set.
func dbFetchBlockByHash(dbTx database.Tx, hash *chainhash.Hash) (*btcutil.Block, error) {
	// First find the height associated with the provided hash in the index.
	blockHeight, err := dbFetchHeightByHash(dbTx, hash)
	if err != nil {
		return nil, err
	}

	// Load the raw block bytes from the database.
	blockBytes, err := dbTx.FetchBlock(hash)
	if err != nil {
		return nil, err
	}

	// Create the encapsulated block and set the height appropriately.
	block, err := btcutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		return nil, err
	}
	block.SetHeight(blockHeight)

	return block, nil
}

// dbFetchBlockByHeight uses an existing database transaction to retrieve the
// raw block for the provided height, deserialize it, and return a btcutil.Block
// with the height set.
func dbFetchBlockByHeight(dbTx database.Tx, height int32) (*btcutil.Block, error) {
	// First find the hash associated with the provided height in the index.
	hash, err := dbFetchHashByHeight(dbTx, height)
	if err != nil {
		return nil, err
	}

	// Load the raw block bytes from the database.
	blockBytes, err := dbTx.FetchBlock(hash)
	if err != nil {
		return nil, err
	}

	// Create the encapsulated block and set the height appropriately.
	block, err := btcutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		return nil, err
	}
	block.SetHeight(height)

	return block, nil
}

// dbMainChainHasBlock uses an existing database transaction to return whether
// or not the main chain contains the block identified by the provided hash.
func dbMainChainHasBlock(dbTx database.Tx, hash *chainhash.Hash) bool {
	hashIndex := dbTx.Metadata().Bucket(hashIndexBucketName)
	return hashIndex.Get(hash[:]) != nil
}

// MainChainHasBlock returns whether or not the block with the given hash is in
// the main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) MainChainHasBlock(hash *chainhash.Hash) (bool, error) {
	var exists bool
	err := b.db.View(func(dbTx database.Tx) error {
		exists = dbMainChainHasBlock(dbTx, hash)
		return nil
	})
	return exists, err
}

// BlockHeightByHash returns the height of the block with the given hash in the
// main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockHeightByHash(hash *chainhash.Hash) (int32, error) {
	var height int32
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		height, err = dbFetchHeightByHash(dbTx, hash)
		return err
	})
	return height, err
}

// BlockHashByHeight returns the hash of the block at the given height in the
// main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockHashByHeight(blockHeight int32) (*chainhash.Hash, error) {
	var hash *chainhash.Hash
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		hash, err = dbFetchHashByHeight(dbTx, blockHeight)
		return err
	})
	return hash, err
}

// BlockByHeight returns the block at the given height in the main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockByHeight(blockHeight int32) (*btcutil.Block, error) {
	var block *btcutil.Block
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		block, err = dbFetchBlockByHeight(dbTx, blockHeight)
		return err
	})
	return block, err
}

// BlockByHash returns the block from the main chain with the given hash with
// the appropriate chain height set.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockByHash(hash *chainhash.Hash) (*btcutil.Block, error) {
	var block *btcutil.Block
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		block, err = dbFetchBlockByHash(dbTx, hash)
		return err
	})
	return block, err
}

// HeightRange returns a range of block hashes for the given start and end
// heights.  It is inclusive of the start height and exclusive of the end
// height.  The end height will be limited to the current main chain height.
//
// This function is safe for concurrent access.
func (b *BlockChain) HeightRange(startHeight, endHeight int32) ([]chainhash.Hash, error) {
	// Ensure requested heights are sane.
	if startHeight < 0 {
		return nil, fmt.Errorf("start height of fetch range must not "+
			"be less than zero - got %d", startHeight)
	}
	if endHeight < startHeight {
		return nil, fmt.Errorf("end height of fetch range must not "+
			"be less than the start height - got start %d, end %d",
			startHeight, endHeight)
	}

	// There is nothing to do when the start and end heights are the same,
	// so return now to avoid the chain lock and a database transaction.
	if startHeight == endHeight {
		return nil, nil
	}

	// Grab a lock on the chain to prevent it from changing due to a reorg
	// while building the hashes.
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	// When the requested start height is after the most recent best chain
	// height, there is nothing to do.
	latestHeight := b.bestNode.height
	if startHeight > latestHeight {
		return nil, nil
	}

	// Limit the ending height to the latest height of the chain.
	if endHeight > latestHeight+1 {
		endHeight = latestHeight + 1
	}

	// Fetch as many as are available within the specified range.
	var hashList []chainhash.Hash
	err := b.db.View(func(dbTx database.Tx) error {
		hashes := make([]chainhash.Hash, 0, endHeight-startHeight)
		for i := startHeight; i < endHeight; i++ {
			hash, err := dbFetchHashByHeight(dbTx, i)
			if err != nil {
				return err
			}
			hashes = append(hashes, *hash)
		}

		// Set the list to be returned to the constructed list.
		hashList = hashes
		return nil
	})
	return hashList, err
}
