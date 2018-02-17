// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/decred/dcrd/blockchain/internal/dbnamespace"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

var (
	// byteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	byteOrder = binary.LittleEndian
)

const (
	// currentDatabaseVersion indicates what the current database
	// version is.
	currentDatabaseVersion = 3

	// currentBlockIndexVersion indicates what the current block index
	// database version.
	currentBlockIndexVersion = 2

	// blockHdrSize is the size of a block header.  This is simply the
	// constant from wire and is only provided here for convenience since
	// wire.MaxBlockHeaderPayload is quite long.
	blockHdrSize = wire.MaxBlockHeaderPayload
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
// The staking system requires some extra information to be stored for tickets
// to maintain consensus rules. The full set of minimal outputs are thus required
// in order for the chain to work correctly. A 'minimal output' is simply the
// script version, pubkey script, and amount.

// serializeSizeForMinimalOutputs calculates the number of bytes needed to
// serialize a transaction to its minimal outputs.
func serializeSizeForMinimalOutputs(tx *dcrutil.Tx) int {
	sz := serializeSizeVLQ(uint64(len(tx.MsgTx().TxOut)))
	for _, out := range tx.MsgTx().TxOut {
		sz += serializeSizeVLQ(compressTxOutAmount(uint64(out.Value)))
		sz += serializeSizeVLQ(uint64(out.Version))
		sz += serializeSizeVLQ(uint64(len(out.PkScript)))
		sz += len(out.PkScript)
	}

	return sz
}

// putTxToMinimalOutputs serializes a transaction to its minimal outputs.
// It returns the amount of data written. The function will panic if it writes
// beyond the bounds of the passed memory.
func putTxToMinimalOutputs(target []byte, tx *dcrutil.Tx) int {
	offset := putVLQ(target, uint64(len(tx.MsgTx().TxOut)))
	for _, out := range tx.MsgTx().TxOut {
		offset += putVLQ(target[offset:], compressTxOutAmount(uint64(out.Value)))
		offset += putVLQ(target[offset:], uint64(out.Version))
		offset += putVLQ(target[offset:], uint64(len(out.PkScript)))
		copy(target[offset:], out.PkScript)
		offset += len(out.PkScript)
	}

	return offset
}

// deserializeToMinimalOutputs deserializes a series of minimal outputs to their
// decompressed, deserialized state and stores them in a slice. It also returns
// the amount of data read. The function will panic if it reads beyond the bounds
// of the passed memory.
func deserializeToMinimalOutputs(serialized []byte) ([]*stake.MinimalOutput, int) {
	numOutputs, offset := deserializeVLQ(serialized)
	minOuts := make([]*stake.MinimalOutput, int(numOutputs))
	for i := 0; i < int(numOutputs); i++ {
		amountComp, bytesRead := deserializeVLQ(serialized[offset:])
		amount := decompressTxOutAmount(amountComp)
		offset += bytesRead

		version, bytesRead := deserializeVLQ(serialized[offset:])
		offset += bytesRead

		scriptSize, bytesRead := deserializeVLQ(serialized[offset:])
		offset += bytesRead

		pkScript := make([]byte, int(scriptSize))
		copy(pkScript, serialized[offset:offset+int(scriptSize)])
		offset += int(scriptSize)

		minOuts[i] = &stake.MinimalOutput{
			Value:    int64(amount),
			Version:  uint16(version),
			PkScript: pkScript,
		}
	}

	return minOuts, offset
}

// readDeserializeSizeOfMinimalOutputs reads the size of the stored set of
// minimal outputs without allocating memory for the structs themselves. It
// will panic if the function reads outside of memory bounds.
func readDeserializeSizeOfMinimalOutputs(serialized []byte) int {
	numOutputs, offset := deserializeVLQ(serialized)
	for i := 0; i < int(numOutputs); i++ {
		// Amount
		_, bytesRead := deserializeVLQ(serialized[offset:])
		offset += bytesRead

		// Script version
		_, bytesRead = deserializeVLQ(serialized[offset:])
		offset += bytesRead

		// Script
		var scriptSize uint64
		scriptSize, bytesRead = deserializeVLQ(serialized[offset:])
		offset += bytesRead
		offset += int(scriptSize)
	}

	return offset
}

// ConvertUtxosToMinimalOutputs converts the contents of a UTX to a series of
// minimal outputs. It does this so that these can be passed to stake subpackage
// functions, where they will be evaluated for correctness.
func ConvertUtxosToMinimalOutputs(entry *UtxoEntry) []*stake.MinimalOutput {
	minOuts, _ := deserializeToMinimalOutputs(entry.stakeExtra)

	return minOuts
}

// -----------------------------------------------------------------------------
// The block index consists of an entry for every known block.  It consists of
// information such as the block header and hashes of tickets voted and revoked.
//
// The serialized key format is:
//
//   <block height><block hash>
//
//   Field           Type              Size
//   block height    uint32            4 bytes
//   block hash      chainhash.Hash    chainhash.HashSize
//
// The serialized value format is:
//
//   <block header><status><num votes><votes info><num revoked><revoked tickets>
//
//   Field              Type                Size
//   block header       wire.BlockHeader    180 bytes
//   status             blockStatus         1 byte
//   num votes          VLQ                 variable
//   vote info
//     ticket hash      chainhash.Hash      chainhash.HashSize
//     vote version     VLQ                 variable
//     vote bits        VLQ                 variable
//   num revoked        VLQ                 variable
//   revoked tickets
//     ticket hash      chainhash.Hash      chainhash.HashSize
// -----------------------------------------------------------------------------

// blockIndexKey generates the binary key for an entry in the block index
// bucket.  The key is composed of the block height encoded as a big-endian
// 32-bit unsigned int followed by the 32 byte block hash.  Big endian is used
// here so the entries can easily be iterated by height.
func blockIndexKey(blockHash *chainhash.Hash, blockHeight uint32) []byte {
	indexKey := make([]byte, chainhash.HashSize+4)
	binary.BigEndian.PutUint32(indexKey[0:4], blockHeight)
	copy(indexKey[4:chainhash.HashSize+4], blockHash[:])
	return indexKey
}

// blockNodeSerializeSize returns the number of bytes it would take to serialize
// the passed block node according to the format described above.
func blockNodeSerializeSize(node *blockNode) int {
	voteInfoSize := 0
	for i := range node.votes {
		voteInfoSize += chainhash.HashSize +
			serializeSizeVLQ(uint64(node.votes[i].Version)) +
			serializeSizeVLQ(uint64(node.votes[i].Bits))
	}

	return blockHdrSize + 1 + serializeSizeVLQ(uint64(len(node.votes))) +
		voteInfoSize + serializeSizeVLQ(uint64(len(node.ticketsRevoked))) +
		chainhash.HashSize*len(node.ticketsRevoked)
}

// putBlockNode serializes the passed block node according to the format
// described above directly into the passed target byte slice.  The target byte
// slice must be at least large enough to handle the number of bytes returned by
// the blockNodeSerializeSize function or it will panic.
func putBlockNode(target []byte, node *blockNode) (int, error) {
	if len(node.ticketsVoted) != len(node.votes) {
		return 0, AssertError("putBlockNode called with a block node " +
			"that has a mismatched number of tickets voted and " +
			"votes")
	}

	// Serialize the entire block header.
	w := bytes.NewBuffer(target[0:0])
	header := node.Header()
	if err := header.Serialize(w); err != nil {
		return 0, err
	}

	// Serialize the status.
	offset := blockHdrSize
	target[offset] = byte(node.status)
	offset++

	// Serialize the number of votes and associated vote information.
	offset += putVLQ(target[offset:], uint64(len(node.votes)))
	for i := range node.votes {
		offset += copy(target[offset:], node.ticketsVoted[i][:])
		offset += putVLQ(target[offset:], uint64(node.votes[i].Version))
		offset += putVLQ(target[offset:], uint64(node.votes[i].Bits))
	}

	// Serialize the number of revocations and associated revocation
	// information.
	offset += putVLQ(target[offset:], uint64(len(node.ticketsRevoked)))
	for i := range node.ticketsRevoked {
		offset += copy(target[offset:], node.ticketsRevoked[i][:])
	}

	return offset, nil
}

// serializeBlockNode serializes the passed block node into a single byte slice
// according to the format described in detail above.
func serializeBlockNode(node *blockNode) ([]byte, error) {
	serialized := make([]byte, blockNodeSerializeSize(node))
	_, err := putBlockNode(serialized, node)
	return serialized, err
}

// decodeBlockNode decodes the passed serialized block node into the passed
// struct according to the format described above.  It returns the number of
// bytes read.
func decodeBlockNode(serialized []byte, node *blockNode) (int, error) {
	// Ensure there are enough bytes to decode header.
	if len(serialized) < blockHdrSize {
		return 0, errDeserialize("unexpected end of data while " +
			"reading block header")
	}
	hB := serialized[0:blockHdrSize]

	// Deserialize the header.
	var header wire.BlockHeader
	if err := header.Deserialize(bytes.NewReader(hB)); err != nil {
		return 0, err
	}
	offset := blockHdrSize

	// Deserialize the status.
	if offset+1 > len(serialized) {
		return offset, errDeserialize("unexpected end of data while " +
			"reading status")
	}
	status := blockStatus(serialized[offset])
	offset++

	// Deserialize the number of tickets spent.
	var ticketsVoted []chainhash.Hash
	var votes []stake.VoteVersionTuple
	numVotes, bytesRead := deserializeVLQ(serialized[offset:])
	if bytesRead == 0 {
		return offset, errDeserialize("unexpected end of data while " +
			"reading num votes")
	}
	offset += bytesRead
	if numVotes > 0 {
		ticketsVoted = make([]chainhash.Hash, numVotes)
		votes = make([]stake.VoteVersionTuple, numVotes)
		for i := uint64(0); i < numVotes; i++ {
			// Deserialize the ticket hash associated with the vote.
			if offset+chainhash.HashSize > len(serialized) {
				return offset, errDeserialize(fmt.Sprintf("unexpected "+
					"end of data while reading vote #%d hash",
					i))
			}
			copy(ticketsVoted[i][:], serialized[offset:])
			offset += chainhash.HashSize

			// Deserialize the vote version.
			version, bytesRead := deserializeVLQ(serialized[offset:])
			if bytesRead == 0 {
				return offset, errDeserialize(fmt.Sprintf("unexpected "+
					"end of data while reading vote #%d version",
					i))
			}
			offset += bytesRead

			// Deserialize the vote bits.
			voteBits, bytesRead := deserializeVLQ(serialized[offset:])
			if bytesRead == 0 {
				return offset, errDeserialize(fmt.Sprintf("unexpected "+
					"end of data while reading vote #%d bits",
					i))
			}
			offset += bytesRead

			votes[i].Version = uint32(version)
			votes[i].Bits = uint16(voteBits)
		}
	}

	// Deserialize the number of tickets revoked.
	var ticketsRevoked []chainhash.Hash
	numTicketsRevoked, bytesRead := deserializeVLQ(serialized[offset:])
	if bytesRead == 0 {
		return offset, errDeserialize("unexpected end of data while " +
			"reading num tickets revoked")
	}
	offset += bytesRead
	if numTicketsRevoked > 0 {
		ticketsRevoked = make([]chainhash.Hash, numTicketsRevoked)
		for i := uint64(0); i < numTicketsRevoked; i++ {
			// Deserialize the ticket hash associated with the
			// revocation.
			if offset+chainhash.HashSize > len(serialized) {
				return offset, errDeserialize(fmt.Sprintf("unexpected "+
					"end of data while reading revocation "+
					"#%d", i))
			}
			copy(ticketsRevoked[i][:], serialized[offset:])
			offset += chainhash.HashSize
		}
	}

	initBlockNode(node, &header, nil)
	node.status = status
	node.populateTicketInfo(&stake.SpentTicketsInBlock{
		VotedTickets:   ticketsVoted,
		RevokedTickets: ticketsRevoked,
		Votes:          votes,
	})

	return offset, nil
}

// deserializeBlockNode decodes the passed serialized byte slice into a block
// node according to the format described above.
func deserializeBlockNode(serialized []byte) (*blockNode, error) {
	var node blockNode
	if _, err := decodeBlockNode(serialized, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

// dbPutBlockNode stores the information needed to reconstruct the provided
// block node in the block index according to the format described above.
func dbPutBlockNode(dbTx database.Tx, node *blockNode) error {
	serialized, err := serializeBlockNode(node)
	if err != nil {
		return err
	}

	bucket := dbTx.Metadata().Bucket(dbnamespace.BlockIndexBucketName)
	key := blockIndexKey(&node.hash, uint32(node.height))
	return bucket.Put(key, serialized)
}

// dbFetchBlockNode fetches the block node for the passed hash and height from
// the block index.
func dbFetchBlockNode(dbTx database.Tx, hash *chainhash.Hash, height uint32) (*blockNode, error) {
	bucket := dbTx.Metadata().Bucket(dbnamespace.BlockIndexBucketName)
	key := blockIndexKey(hash, height)
	serialized := bucket.Get(key)
	if serialized == nil {
		return nil, AssertError(fmt.Sprintf("missing block node %s "+
			"(height %d)", hash, height))
	}

	return deserializeBlockNode(serialized)
}

// dbMaybeStoreBlock stores the provided block in the database if it's not
// already there.
func dbMaybeStoreBlock(dbTx database.Tx, block *dcrutil.Block) error {
	// Store the block in ffldb if not already done.
	hasBlock, err := dbTx.HasBlock(block.Hash())
	if err != nil {
		return err
	}
	if hasBlock {
		return nil
	}

	return dbTx.StoreBlock(block)
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
//   [<flags><script version><compressed pk script>],...
//   OPTIONAL: [<txVersion>]
//
//   Field                Type           Size
//   flags                VLQ            byte
//   scriptVersion        uint16         2 bytes
//   pkScript             VLQ+[]byte     variable
//
//   OPTIONAL
//     txVersion          VLQ            variable
//     stakeExtra         []byte         variable
//
// The serialized flags code format is:
//   bit  0   - containing transaction is a coinbase
//   bit  1   - containing transaction has an expiry
//   bits 2-3 - transaction type
//   bit  4   - is fully spent
//
// The stake extra field contains minimally encoded outputs for all
// consensus-related outputs in the stake transaction. It is only
// encoded for tickets.
//
//   NOTE: The transaction version and flags are only encoded when the spent
//   txout was the final unspent output of the containing transaction.
//   Otherwise, the header code will be 0 and the version is not serialized at
//   all. This is  done because that information is only needed when the utxo
//   set no longer has it.
//
// Example:
//   TODO
// -----------------------------------------------------------------------------

// spentTxOut contains a spent transaction output and potentially additional
// contextual information such as whether or not it was contained in a coinbase
// transaction, the txVersion of the transaction it was contained in, and which
// block height the containing transaction was included in.  As described in
// the comments above, the additional contextual information will only be valid
// when this spent txout is spending the last unspent output of the containing
// transaction.
//
// The struct is aligned for memory efficiency.
type spentTxOut struct {
	pkScript   []byte // The public key script for the output.
	stakeExtra []byte // Extra information for the staking system.

	amount        int64        // The amount of the output.
	txType        stake.TxType // The stake type of the transaction.
	height        uint32       // Height of the the block containing the tx.
	index         uint32       // Index in the block of the transaction.
	scriptVersion uint16       // The version of the scripting language.
	txVersion     uint16       // The version of creating tx.

	txFullySpent bool // Whether or not the transaction is fully spent.
	isCoinBase   bool // Whether creating tx is a coinbase.
	hasExpiry    bool // The expiry of the creating tx.
	compressed   bool // Whether or not the script is compressed.
}

// spentTxOutSerializeSize returns the number of bytes it would take to
// serialize the passed stxo according to the format described above.
// The amount is never encoded into spent transaction outputs in Decred
// because they're already encoded into the transactions, so skip them when
// determining the serialization size.
func spentTxOutSerializeSize(stxo *spentTxOut) int {
	flags := encodeFlags(stxo.isCoinBase, stxo.hasExpiry, stxo.txType,
		stxo.txFullySpent)
	size := serializeSizeVLQ(uint64(flags))

	// false below indicates that the txOut does not specify an amount.
	size += compressedTxOutSize(uint64(stxo.amount), stxo.scriptVersion,
		stxo.pkScript, currentCompressionVersion, stxo.compressed, false)

	// The transaction was fully spent, so we need to store some extra
	// data for UTX resurrection.
	if stxo.txFullySpent {
		size += serializeSizeVLQ(uint64(stxo.txVersion))
		if stxo.txType == stake.TxTypeSStx {
			size += len(stxo.stakeExtra)
		}
	}

	return size
}

// putSpentTxOut serializes the passed stxo according to the format described
// above directly into the passed target byte slice.  The target byte slice must
// be at least large enough to handle the number of bytes returned by the
// spentTxOutSerializeSize function or it will panic.
func putSpentTxOut(target []byte, stxo *spentTxOut) int {
	flags := encodeFlags(stxo.isCoinBase, stxo.hasExpiry, stxo.txType,
		stxo.txFullySpent)
	offset := putVLQ(target, uint64(flags))

	// false below indicates that the txOut does not specify an amount.
	offset += putCompressedTxOut(target[offset:], 0, stxo.scriptVersion,
		stxo.pkScript, currentCompressionVersion, stxo.compressed, false)

	// The transaction was fully spent, so we need to store some extra
	// data for UTX resurrection.
	if stxo.txFullySpent {
		offset += putVLQ(target[offset:], uint64(stxo.txVersion))
		if stxo.txType == stake.TxTypeSStx {
			copy(target[offset:], stxo.stakeExtra)
			offset += len(stxo.stakeExtra)
		}
	}
	return offset
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
func decodeSpentTxOut(serialized []byte, stxo *spentTxOut, amount int64, height uint32, index uint32) (int, error) {
	// Ensure there are bytes to decode.
	if len(serialized) == 0 {
		return 0, errDeserialize("no serialized bytes")
	}

	// Deserialize the header code.
	flags, offset := deserializeVLQ(serialized)
	if offset >= len(serialized) {
		return offset, errDeserialize("unexpected end of data after " +
			"spent tx out flags")
	}

	// Decode the flags. If the flags are non-zero, it means that the
	// transaction was fully spent at this spend.
	if decodeFlagsFullySpent(byte(flags)) {
		isCoinBase, hasExpiry, txType, _ := decodeFlags(byte(flags))

		stxo.isCoinBase = isCoinBase
		stxo.hasExpiry = hasExpiry
		stxo.txType = txType
		stxo.txFullySpent = true
	}

	// Decode the compressed txout. We pass false for the amount flag,
	// since in Decred we only need pkScript at most due to fraud proofs
	// already storing the decompressed amount.
	_, scriptVersion, compScript, bytesRead, err :=
		decodeCompressedTxOut(serialized[offset:], currentCompressionVersion,
			false)
	offset += bytesRead
	if err != nil {
		return offset, errDeserialize(fmt.Sprintf("unable to decode "+
			"txout: %v", err))
	}
	stxo.scriptVersion = scriptVersion
	stxo.amount = amount
	stxo.pkScript = compScript
	stxo.compressed = true
	stxo.height = height
	stxo.index = index

	// Deserialize the containing transaction if the flags indicate that
	// the transaction has been fully spent.
	if decodeFlagsFullySpent(byte(flags)) {
		txVersion, bytesRead := deserializeVLQ(serialized[offset:])
		offset += bytesRead
		if offset == 0 || offset > len(serialized) {
			return offset, errDeserialize("unexpected end of data " +
				"after version")
		}

		stxo.txVersion = uint16(txVersion)

		if stxo.txType == stake.TxTypeSStx {
			sz := readDeserializeSizeOfMinimalOutputs(serialized[offset:])
			if sz == 0 || sz > len(serialized[offset:]) {
				return offset, errDeserialize("corrupt data for ticket " +
					"fully spent stxo stakeextra")
			}

			stakeExtra := make([]byte, sz)
			copy(stakeExtra, serialized[offset:offset+sz])
			stxo.stakeExtra = stakeExtra
			offset += sz
		}
	}

	return offset, nil
}

// deserializeSpendJournalEntry decodes the passed serialized byte slice into a
// slice of spent txouts according to the format described in detail above.
//
// Since the serialization format is not self describing, as noted in the
// format comments, this function also requires the transactions that spend the
// txouts and a utxo view that contains any remaining existing utxos in the
// transactions referenced by the inputs to the passed transasctions.
func deserializeSpendJournalEntry(serialized []byte, txns []*wire.MsgTx) ([]spentTxOut, error) {
	// Calculate the total number of stxos.
	var numStxos int
	for _, tx := range txns {
		txType := stake.DetermineTxType(tx)

		if txType == stake.TxTypeSSGen {
			numStxos++
			continue
		}
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
	offset := 0
	stxos := make([]spentTxOut, numStxos)
	for txIdx := len(txns) - 1; txIdx > -1; txIdx-- {
		tx := txns[txIdx]
		txType := stake.DetermineTxType(tx)

		// Loop backwards through all of the transaction inputs and read
		// the associated stxo.
		for txInIdx := len(tx.TxIn) - 1; txInIdx > -1; txInIdx-- {
			// Skip stakebase.
			if txInIdx == 0 && txType == stake.TxTypeSSGen {
				continue
			}

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
			n, err := decodeSpentTxOut(serialized[offset:], stxo, txIn.ValueIn,
				txIn.BlockHeight, txIn.BlockIndex)
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
func serializeSpendJournalEntry(stxos []spentTxOut) ([]byte, error) {
	if len(stxos) == 0 {
		return nil, nil
	}

	// Calculate the size needed to serialize the entire journal entry.
	var size int
	var sizes []int
	for i := range stxos {
		sz := spentTxOutSerializeSize(&stxos[i])
		sizes = append(sizes, sz)
		size += sz
	}
	serialized := make([]byte, size)

	// Serialize each individual stxo directly into the slice in reverse
	// order one after the other.
	var offset int
	for i := len(stxos) - 1; i > -1; i-- {
		oldOffset := offset
		offset += putSpentTxOut(serialized[offset:], &stxos[i])

		if offset-oldOffset != sizes[i] {
			return nil, AssertError(fmt.Sprintf("bad write; expect sz %v, "+
				"got sz %v (wrote %x)", sizes[i], offset-oldOffset,
				serialized[oldOffset:offset]))
		}
	}

	return serialized, nil
}

// dbFetchSpendJournalEntry fetches the spend journal entry for the passed
// block and deserializes it into a slice of spent txout entries.  The provided
// view MUST have the utxos referenced by all of the transactions available for
// the passed block since that information is required to reconstruct the spent
// txouts.
func dbFetchSpendJournalEntry(dbTx database.Tx, block *dcrutil.Block, parent *dcrutil.Block) ([]spentTxOut, error) {
	// Exclude the coinbase transaction since it can't spend anything.
	spendBucket := dbTx.Metadata().Bucket(dbnamespace.SpendJournalBucketName)
	serialized := spendBucket.Get(block.Hash()[:])

	var blockTxns []*wire.MsgTx
	if headerApprovesParent(&block.MsgBlock().Header) {
		blockTxns = append(blockTxns, parent.MsgBlock().Transactions[1:]...)
	}
	blockTxns = append(blockTxns, block.MsgBlock().STransactions...)

	if len(blockTxns) > 0 && len(serialized) == 0 {
		return nil, AssertError("missing spend journal data")
	}

	stxos, err := deserializeSpendJournalEntry(serialized, blockTxns)
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
	spendBucket := dbTx.Metadata().Bucket(dbnamespace.SpendJournalBucketName)
	serialized, err := serializeSpendJournalEntry(stxos)
	if err != nil {
		return err
	}
	return spendBucket.Put(blockHash[:], serialized)
}

// dbRemoveSpendJournalEntry uses an existing database transaction to remove the
// spend journal entry for the passed block hash.
func dbRemoveSpendJournalEntry(dbTx database.Tx, blockHash *chainhash.Hash) error {
	spendBucket := dbTx.Metadata().Bucket(dbnamespace.SpendJournalBucketName)
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
//   Field                 Type     Size
//   transaction version   VLQ      variable
//   block height          VLQ      variable
//   block index           VLQ      variable
//   flags                 VLQ      variable (currently 1 byte)
//   header code           VLQ      variable
//   unspentness bitmap    []byte   variable
//   compressed txouts
//     compressed amount   VLQ      variable
//     compressed version  VLQ      variable
//     compressed script   []byte   variable
//   stakeExtra            []byte   variable
//
// The serialized flags code format is:
//   bit  0   - containing transaction is a coinbase
//   bit  1   - containing transaction has an expiry
//   bits 2-3 - transaction type
//   bits 4-7 - unused
//
// The serialized header code format is:
//   bit 0 - output zero is unspent
//   bit 1 - output one is unspent
//   bits 2-x - number of bytes in unspentness bitmap.  When both bits 1 and 2
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
//   - Encoding N-1 bytes when both bits 0 and 1 are unset allows an additional
//     8 outpoints to be encoded before causing the header code to require an
//     additional byte.
//
// The stake extra field contains minimally encoded outputs for all
// consensus-related outputs in the stake transaction. It is only
// encoded for tickets.
//
// Example 1: TODO
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
	// encode the value by shifting it over 2 bits.
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
	headerCode := uint64(numBitmapBytes-numBitmapBytesAdjustment) << 2

	// Set the output 0 and output 1 bits in the header code
	// accordingly.
	if output0Unspent {
		headerCode |= 0x01 // bit 0
	}
	if output1Unspent {
		headerCode |= 0x02 // bit 1
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
	flags := encodeFlags(entry.isCoinBase, entry.hasExpiry, entry.txType, false)
	size := serializeSizeVLQ(uint64(entry.txVersion)) +
		serializeSizeVLQ(uint64(entry.height)) +
		serializeSizeVLQ(uint64(entry.index)) +
		serializeSizeVLQ(uint64(flags)) +
		serializeSizeVLQ(headerCode) + numBitmapBytes
	for _, outputIndex := range outputOrder {
		out := entry.sparseOutputs[uint32(outputIndex)]
		if out.spent {
			continue
		}
		size += compressedTxOutSize(uint64(out.amount), out.scriptVersion,
			out.pkScript, currentCompressionVersion, out.compressed, true)
	}
	if entry.txType == stake.TxTypeSStx {
		size += len(entry.stakeExtra)
	}

	// Serialize the version, block height, block index, and flags of the
	// containing transaction, and "header code" which is a complex bitmap
	// of spentness.
	serialized := make([]byte, size)
	offset := putVLQ(serialized, uint64(entry.txVersion))
	offset += putVLQ(serialized[offset:], uint64(entry.height))
	offset += putVLQ(serialized[offset:], uint64(entry.index))
	offset += putVLQ(serialized[offset:], uint64(flags))
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
			uint64(out.amount), out.scriptVersion, out.pkScript,
			currentCompressionVersion, out.compressed, true)
	}

	if entry.txType == stake.TxTypeSStx {
		copy(serialized[offset:], entry.stakeExtra)
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

	// Deserialize the block index.
	blockIndex, bytesRead := deserializeVLQ(serialized[offset:])
	offset += bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after index")
	}

	// Deserialize the flags.
	flags, bytesRead := deserializeVLQ(serialized[offset:])
	offset += bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after flags")
	}
	isCoinBase, hasExpiry, txType, _ := decodeFlags(byte(flags))

	// Deserialize the header code.
	code, bytesRead := deserializeVLQ(serialized[offset:])
	offset += bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after header")
	}

	// Decode the header code.
	//
	// Bit 0 indicates output 0 is unspent.
	// Bit 1 indicates output 1 is unspent.
	// Bits 2-x encodes the number of non-zero unspentness bitmap bytes that
	// follow.  When both output 0 and 1 are spent, it encodes N-1.
	output0Unspent := code&0x01 != 0
	output1Unspent := code&0x02 != 0
	numBitmapBytes := code >> 2
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
	entry := newUtxoEntry(uint16(version), uint32(blockHeight),
		uint32(blockIndex), isCoinBase, hasExpiry, txType)

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
		//
		// 'true' below instructs the method to deserialize a stored
		// amount.
		amount, scriptVersion, compScript, bytesRead, err :=
			decodeCompressedTxOut(serialized[offset:], currentCompressionVersion,
				true)
		if err != nil {
			return nil, errDeserialize(fmt.Sprintf("unable to "+
				"decode utxo at index %d: %v", i, err))
		}
		offset += bytesRead

		entry.sparseOutputs[outputIndex] = &utxoOutput{
			spent:         false,
			compressed:    true,
			scriptVersion: scriptVersion,
			pkScript:      compScript,
			amount:        amount,
		}
	}

	// Copy the stake extra data if this was a ticket.
	if entry.txType == stake.TxTypeSStx {
		stakeExtra := make([]byte, len(serialized[offset:]))
		copy(stakeExtra, serialized[offset:])
		entry.stakeExtra = stakeExtra
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
	utxoBucket := dbTx.Metadata().Bucket(dbnamespace.UtxoSetBucketName)
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
	utxoBucket := dbTx.Metadata().Bucket(dbnamespace.UtxoSetBucketName)
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
// The main chain index consists of two buckets with an entry for every block in
// the main chain.  One bucket is for the hash to height mapping and the other
// is for the height to hash mapping.
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

// dbPutMainChainIndex uses an existing database transaction to update or add
// index entries for the hash to height and height to hash mappings for the
// provided values.
func dbPutMainChainIndex(dbTx database.Tx, hash *chainhash.Hash, height int64) error {
	// Serialize the height for use in the index entries.
	var serializedHeight [4]byte
	dbnamespace.ByteOrder.PutUint32(serializedHeight[:], uint32(height))

	// Add the block hash to height mapping to the index.
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(dbnamespace.HashIndexBucketName)
	if err := hashIndex.Put(hash[:], serializedHeight[:]); err != nil {
		return err
	}

	// Add the block height to hash mapping to the index.
	heightIndex := meta.Bucket(dbnamespace.HeightIndexBucketName)
	return heightIndex.Put(serializedHeight[:], hash[:])
}

// dbRemoveMainChainIndex uses an existing database transaction remove main
// chain index entries from the hash to height and height to hash mappings for
// the provided values.
func dbRemoveMainChainIndex(dbTx database.Tx, hash *chainhash.Hash, height int64) error {
	// Remove the block hash to height mapping.
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(dbnamespace.HashIndexBucketName)
	if err := hashIndex.Delete(hash[:]); err != nil {
		return err
	}

	// Remove the block height to hash mapping.
	var serializedHeight [4]byte
	dbnamespace.ByteOrder.PutUint32(serializedHeight[:], uint32(height))
	heightIndex := meta.Bucket(dbnamespace.HeightIndexBucketName)
	return heightIndex.Delete(serializedHeight[:])
}

// dbFetchHeightByHash uses an existing database transaction to retrieve the
// height for the provided hash from the index.
func dbFetchHeightByHash(dbTx database.Tx, hash *chainhash.Hash) (int64, error) {
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(dbnamespace.HashIndexBucketName)
	serializedHeight := hashIndex.Get(hash[:])
	if serializedHeight == nil {
		str := fmt.Sprintf("block %s is not in the main chain", hash)
		return 0, errNotInMainChain(str)
	}

	return int64(dbnamespace.ByteOrder.Uint32(serializedHeight)), nil
}

// dbFetchHashByHeight uses an existing database transaction to retrieve the
// hash for the provided height from the index.
func dbFetchHashByHeight(dbTx database.Tx, height int64) (*chainhash.Hash, error) {
	var serializedHeight [4]byte
	dbnamespace.ByteOrder.PutUint32(serializedHeight[:], uint32(height))

	meta := dbTx.Metadata()
	heightIndex := meta.Bucket(dbnamespace.HeightIndexBucketName)
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
// The database information contains information about the version and date
// of the blockchain database.
//
// It consists of a separate key for each individual piece of information:
//
//   Key        Value    Size      Description
//   version    uint32   4 bytes   The version of the database
//   compver    uint32   4 bytes   The script compression version of the database
//   bidxver    uint32   4 bytes   The block index version of the database
//   created    uint64   8 bytes   The date of the creation of the database
// -----------------------------------------------------------------------------

// databaseInfo is the structure for a database.
type databaseInfo struct {
	version uint32
	compVer uint32
	bidxVer uint32
	created time.Time
}

// dbPutDatabaseInfo uses an existing database transaction to store the database
// information.
func dbPutDatabaseInfo(dbTx database.Tx, dbi *databaseInfo) error {
	// uint32Bytes is a helper function to convert a uint32 to a byte slice
	// using the byte order specified by the database namespace.
	uint32Bytes := func(ui32 uint32) []byte {
		var b [4]byte
		dbnamespace.ByteOrder.PutUint32(b[:], ui32)
		return b[:]
	}

	// uint64Bytes is a helper function to convert a uint64 to a byte slice
	// using the byte order specified by the database namespace.
	uint64Bytes := func(ui64 uint64) []byte {
		var b [8]byte
		dbnamespace.ByteOrder.PutUint64(b[:], ui64)
		return b[:]
	}

	// Store the database version.
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.BCDBInfoBucketName)
	err := bucket.Put(dbnamespace.BCDBInfoVersionKeyName,
		uint32Bytes(dbi.version))
	if err != nil {
		return err
	}

	// Store the compression version.
	err = bucket.Put(dbnamespace.BCDBInfoCompressionVersionKeyName,
		uint32Bytes(dbi.compVer))
	if err != nil {
		return err
	}

	// Store the block index version.
	err = bucket.Put(dbnamespace.BCDBInfoBlockIndexVersionKeyName,
		uint32Bytes(dbi.bidxVer))
	if err != nil {
		return err
	}

	// Store the database creation date.
	return bucket.Put(dbnamespace.BCDBInfoCreatedKeyName,
		uint64Bytes(uint64(dbi.created.Unix())))
}

// dbFetchDatabaseInfo uses an existing database transaction to fetch the
// database versioning and creation information.
func dbFetchDatabaseInfo(dbTx database.Tx) (*databaseInfo, error) {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.BCDBInfoBucketName)

	// Uninitialized state.
	if bucket == nil {
		return nil, nil
	}

	// Load the database version.
	var version uint32
	versionBytes := bucket.Get(dbnamespace.BCDBInfoVersionKeyName)
	if versionBytes != nil {
		version = dbnamespace.ByteOrder.Uint32(versionBytes)
	}

	// Load the database compression version.
	var compVer uint32
	compVerBytes := bucket.Get(dbnamespace.BCDBInfoCompressionVersionKeyName)
	if compVerBytes != nil {
		compVer = dbnamespace.ByteOrder.Uint32(compVerBytes)
	}

	// Load the database block index version.
	var bidxVer uint32
	bidxVerBytes := bucket.Get(dbnamespace.BCDBInfoBlockIndexVersionKeyName)
	if bidxVerBytes != nil {
		bidxVer = dbnamespace.ByteOrder.Uint32(bidxVerBytes)
	}

	// Load the database creation date.
	var created time.Time
	createdBytes := bucket.Get(dbnamespace.BCDBInfoCreatedKeyName)
	if createdBytes != nil {
		ts := dbnamespace.ByteOrder.Uint64(createdBytes)
		created = time.Unix(int64(ts), 0)
	}

	return &databaseInfo{
		version: version,
		compVer: compVer,
		bidxVer: bidxVer,
		created: created,
	}, nil
}

// -----------------------------------------------------------------------------
// The best chain state consists of the best block hash and height, the total
// number of transactions up to and including those in the best block, the
// total coin supply, the subsidy at the current block, the subsidy of the
// block prior (for rollbacks), and the accumulated work sum up to and
// including the best block.
//
// The serialized format is:
//
//   <block hash><block height><total txns><total subsidy><work sum length><work sum>
//
//   Field             Type             Size
//   block hash        chainhash.Hash   chainhash.HashSize
//   block height      uint32           4 bytes
//   total txns        uint64           8 bytes
//   total subsidy     int64            8 bytes
//   work sum length   uint32           4 bytes
//   work sum          big.Int          work sum length
// -----------------------------------------------------------------------------

// bestChainState represents the data to be stored the database for the current
// best chain state.
type bestChainState struct {
	hash         chainhash.Hash
	height       uint32
	totalTxns    uint64
	totalSubsidy int64
	workSum      *big.Int
}

// serializeBestChainState returns the serialization of the passed block best
// chain state.  This is data to be stored in the chain state bucket.
func serializeBestChainState(state bestChainState) []byte {
	// Calculate the full size needed to serialize the chain state.
	workSumBytes := state.workSum.Bytes()
	workSumBytesLen := uint32(len(workSumBytes))
	serializedLen := chainhash.HashSize + 4 + 8 + 8 + 4 + workSumBytesLen

	// Serialize the chain state.
	serializedData := make([]byte, serializedLen)
	copy(serializedData[0:chainhash.HashSize], state.hash[:])
	offset := uint32(chainhash.HashSize)
	dbnamespace.ByteOrder.PutUint32(serializedData[offset:], state.height)
	offset += 4
	dbnamespace.ByteOrder.PutUint64(serializedData[offset:], state.totalTxns)
	offset += 8
	dbnamespace.ByteOrder.PutUint64(serializedData[offset:],
		uint64(state.totalSubsidy))
	offset += 8
	dbnamespace.ByteOrder.PutUint32(serializedData[offset:], workSumBytesLen)
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
	// the hash, height, total transactions, total subsidy, current subsidy,
	// and work sum length.
	expectedMinLen := chainhash.HashSize + 4 + 8 + 8 + 4
	if len(serializedData) < expectedMinLen {
		return bestChainState{}, database.Error{
			ErrorCode: database.ErrCorruption,
			Description: fmt.Sprintf("corrupt best chain state size; min %v "+
				"got %v", expectedMinLen, len(serializedData)),
		}
	}

	state := bestChainState{}
	copy(state.hash[:], serializedData[0:chainhash.HashSize])
	offset := uint32(chainhash.HashSize)
	state.height = dbnamespace.ByteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	state.totalTxns = dbnamespace.ByteOrder.Uint64(
		serializedData[offset : offset+8])
	offset += 8
	state.totalSubsidy = int64(dbnamespace.ByteOrder.Uint64(
		serializedData[offset : offset+8]))
	offset += 8
	workSumBytesLen := dbnamespace.ByteOrder.Uint32(
		serializedData[offset : offset+4])
	offset += 4

	// Ensure the serialized data has enough bytes to deserialize the work
	// sum.
	if uint32(len(serializedData[offset:])) < workSumBytesLen {
		return bestChainState{}, database.Error{
			ErrorCode: database.ErrCorruption,
			Description: fmt.Sprintf("corrupt work sum size; want %v "+
				"got %v", workSumBytesLen, uint32(len(serializedData[offset:]))),
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
		hash:         snapshot.Hash,
		height:       uint32(snapshot.Height),
		totalTxns:    snapshot.TotalTxns,
		totalSubsidy: snapshot.TotalSubsidy,
		workSum:      workSum,
	})

	// Store the current best chain state into the database.
	return dbTx.Metadata().Put(dbnamespace.ChainStateKeyName, serializedData)
}

// createChainState initializes both the database and the chain state to the
// genesis block.  This includes creating the necessary buckets and inserting
// the genesis block, so it must only be called on an uninitialized database.
func (b *BlockChain) createChainState() error {
	// Create a new node from the genesis block and set it as the best node.
	genesisBlock := dcrutil.NewBlock(b.chainParams.GenesisBlock)
	header := &genesisBlock.MsgBlock().Header
	node := newBlockNode(header, nil)
	node.status = statusDataStored | statusValid
	node.inMainChain = true

	// Initialize the state related to the best block.  Since it is the
	// genesis block, use its timestamp for the median time.
	numTxns := uint64(len(genesisBlock.MsgBlock().Transactions))
	blockSize := uint64(genesisBlock.MsgBlock().SerializeSize())
	stateSnapshot := newBestState(node, blockSize, numTxns, numTxns,
		time.Unix(node.timestamp, 0), 0)

	// Create the initial the database chain state including creating the
	// necessary index buckets and inserting the genesis block.
	err := b.db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()

		// Create the bucket that houses information about the database's
		// creation and version.
		_, err := meta.CreateBucket(dbnamespace.BCDBInfoBucketName)
		if err != nil {
			return err
		}

		b.dbInfo = &databaseInfo{
			version: currentDatabaseVersion,
			compVer: currentCompressionVersion,
			bidxVer: currentBlockIndexVersion,
			created: time.Now(),
		}
		err = dbPutDatabaseInfo(dbTx, b.dbInfo)
		if err != nil {
			return err
		}

		// Create the bucket that houses the block index data.
		_, err = meta.CreateBucket(dbnamespace.BlockIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the chain block hash to height
		// index.
		_, err = meta.CreateBucket(dbnamespace.HashIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the chain block height to hash
		// index.
		_, err = meta.CreateBucket(dbnamespace.HeightIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the spend journal data.
		_, err = meta.CreateBucket(dbnamespace.SpendJournalBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the utxo set.  Note that the
		// genesis block coinbase transaction is intentionally not
		// inserted here since it is not spendable by consensus rules.
		_, err = meta.CreateBucket(dbnamespace.UtxoSetBucketName)
		if err != nil {
			return err
		}

		// Add the genesis block to the block index.
		err = dbPutBlockNode(dbTx, node)
		if err != nil {
			return err
		}

		// Add the genesis block hash to height and height to hash
		// mappings to the index.
		err = dbPutMainChainIndex(dbTx, &node.hash, node.height)
		if err != nil {
			return err
		}

		// Store the current best chain state into the database.
		err = dbPutBestState(dbTx, stateSnapshot, node.workSum)
		if err != nil {
			return err
		}

		// Initialize the stake buckets in the database, along with
		// the best state for the stake database.
		_, err = stake.InitDatabaseState(dbTx, b.chainParams)
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
func (b *BlockChain) initChainState(interrupt <-chan struct{}) error {
	// Update database versioning scheme if needed.
	err := b.db.Update(func(dbTx database.Tx) error {
		// No versioning upgrade is needed if the dbinfo bucket does not
		// exist or the legacy key does not exist.
		bucket := dbTx.Metadata().Bucket(dbnamespace.BCDBInfoBucketName)
		if bucket == nil {
			return nil
		}
		legacyBytes := bucket.Get(dbnamespace.BCDBInfoBucketName)
		if legacyBytes == nil {
			return nil
		}

		// No versioning upgrade is needed if the new version key exists.
		if bucket.Get(dbnamespace.BCDBInfoVersionKeyName) != nil {
			return nil
		}

		// Load and deserialize the legacy version information.
		log.Infof("Migrating versioning scheme...")
		dbi, err := deserializeDatabaseInfoV2(legacyBytes)
		if err != nil {
			return err
		}

		// Store the database version info using the new format.
		if err := dbPutDatabaseInfo(dbTx, dbi); err != nil {
			return err
		}

		// Remove the legacy version information.
		return bucket.Delete(dbnamespace.BCDBInfoBucketName)
	})
	if err != nil {
		return err
	}

	// Determine the state of the database.
	var isStateInitialized bool
	err = b.db.View(func(dbTx database.Tx) error {
		// Fetch the database versioning information.
		dbInfo, err := dbFetchDatabaseInfo(dbTx)
		if err != nil {
			return err
		}

		// The database bucket for the versioning information is missing.
		if dbInfo == nil && err == nil {
			return nil
		}

		// Don't allow downgrades of the blockchain database.
		if dbInfo.version > currentDatabaseVersion {
			return fmt.Errorf("the current blockchain database is "+
				"no longer compatible with this version of "+
				"the software (%d > %d)", dbInfo.version,
				currentDatabaseVersion)
		}

		// Don't allow downgrades of the database compression version.
		if dbInfo.compVer > currentCompressionVersion {
			return fmt.Errorf("the current database compression "+
				"version is no longer compatible with this "+
				"version of the software (%d > %d)",
				dbInfo.compVer, currentCompressionVersion)
		}

		// Don't allow downgrades of the block index.
		if dbInfo.bidxVer > currentBlockIndexVersion {
			return fmt.Errorf("the current database block index "+
				"version is no longer compatible with this "+
				"version of the software (%d > %d)",
				dbInfo.bidxVer, currentBlockIndexVersion)
		}

		b.dbInfo = dbInfo
		isStateInitialized = true
		return nil
	})
	if err != nil {
		return err
	}

	// Initialize the database if it has not already been done.
	if !isStateInitialized {
		if err := b.createChainState(); err != nil {
			return err
		}
	}

	// Upgrade the database as needed.
	err = upgradeDB(b.db, b.chainParams, b.dbInfo, interrupt)
	if err != nil {
		return err
	}

	// Attempt to load the chain state from the database.
	err = b.db.View(func(dbTx database.Tx) error {
		// Fetch the stored chain state from the database metadata.
		// When it doesn't exist, it means the database hasn't been
		// initialized for use with chain yet, so break out now to allow
		// that to happen under a writable database transaction.
		serializedData := dbTx.Metadata().Get(dbnamespace.ChainStateKeyName)
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
		node := newBlockNode(header, nil)
		node.populateTicketInfo(stake.FindSpentTicketsInBlock(&block))
		node.status = statusDataStored | statusValid
		node.inMainChain = true
		node.workSum = state.workSum

		// Exception for version 1 blockchains: skip loading the stake
		// node, as the upgrade path handles ensuring this is correctly
		// set.
		if b.dbInfo.version >= 2 {
			node.stakeNode, err = stake.LoadBestNode(dbTx, uint32(node.height),
				node.hash, *header, b.chainParams)
			if err != nil {
				return err
			}
			node.stakeUndoData = node.stakeNode.UndoData()
			node.newTickets = node.stakeNode.NewTickets()
		}

		b.bestNode = node

		// Add the new node to the indices for faster lookups.
		b.index.AddNode(node)

		// Calculate the median time for the block.
		medianTime, err := b.index.CalcPastMedianTime(node)
		if err != nil {
			return err
		}

		// Initialize the state related to the best block.
		blockSize := uint64(len(blockBytes))
		numTxns := uint64(len(block.Transactions))
		b.stateSnapshot = newBestState(b.bestNode, blockSize, numTxns,
			state.totalTxns, medianTime, state.totalSubsidy)

		return nil
	})
	return err
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
func dbFetchHeaderByHeight(dbTx database.Tx, height int64) (*wire.BlockHeader, error) {
	hash, err := dbFetchHashByHeight(dbTx, height)
	if err != nil {
		return nil, err
	}

	return dbFetchHeaderByHash(dbTx, hash)
}

// DBFetchHeaderByHeight is the exported version of dbFetchHeaderByHeight.
func DBFetchHeaderByHeight(dbTx database.Tx, height int64) (*wire.BlockHeader, error) {
	return dbFetchHeaderByHeight(dbTx, height)
}

// HeaderByHeight is the exported version of dbFetchHeaderByHeight that
// internally creates a database transaction to do the lookup.
func (b *BlockChain) HeaderByHeight(height int64) (*wire.BlockHeader, error) {
	var header *wire.BlockHeader
	err := b.db.View(func(dbTx database.Tx) error {
		var errLocal error
		header, errLocal = dbFetchHeaderByHeight(dbTx, height)
		return errLocal
	})
	if err != nil {
		return nil, err
	}

	return header, nil
}

// dbFetchBlockByHash uses an existing database transaction to retrieve the raw
// block for the provided hash, deserialize it, retrieve the appropriate height
// from the index, and return a dcrutil.Block with the height set.
func dbFetchBlockByHash(dbTx database.Tx, hash *chainhash.Hash) (*dcrutil.Block, error) {
	// Check if the block is in the main chain.
	if !dbMainChainHasBlock(dbTx, hash) {
		str := fmt.Sprintf("block %s is not in the main chain", hash)
		return nil, errNotInMainChain(str)
	}

	// Load the raw block bytes from the database.
	blockBytes, err := dbTx.FetchBlock(hash)
	if err != nil {
		return nil, err
	}

	// Create the encapsulated block and set the height appropriately.
	block, err := dcrutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// dbFetchBlockByHeight uses an existing database transaction to retrieve the
// raw block for the provided height, deserialize it, and return a dcrutil.Block
// with the height set.
func dbFetchBlockByHeight(dbTx database.Tx, height int64) (*dcrutil.Block, error) {
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
	block, err := dcrutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// DBFetchBlockByHeight is the exported version of dbFetchBlockByHeight.
func DBFetchBlockByHeight(dbTx database.Tx, height int64) (*dcrutil.Block, error) {
	return dbFetchBlockByHeight(dbTx, height)
}

// dbMainChainHasBlock uses an existing database transaction to return whether
// or not the main chain contains the block identified by the provided hash.
func dbMainChainHasBlock(dbTx database.Tx, hash *chainhash.Hash) bool {
	hashIndex := dbTx.Metadata().Bucket(dbnamespace.HashIndexBucketName)
	return hashIndex.Get(hash[:]) != nil
}

// DBMainChainHasBlock is the exported version of dbMainChainHasBlock.
func DBMainChainHasBlock(dbTx database.Tx, hash *chainhash.Hash) bool {
	return dbMainChainHasBlock(dbTx, hash)
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
func (b *BlockChain) BlockHeightByHash(hash *chainhash.Hash) (int64, error) {
	var height int64
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
func (b *BlockChain) BlockHashByHeight(blockHeight int64) (*chainhash.Hash, error) {
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
func (b *BlockChain) BlockByHeight(blockHeight int64) (*dcrutil.Block, error) {
	var block *dcrutil.Block
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		block, err = dbFetchBlockByHeight(dbTx, blockHeight)
		return err
	})
	return block, err
}

// BlockByHash returns the block from the main chain with the given hash.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	return b.fetchMainChainBlockByHash(hash)
}

// HeightRange returns a range of block hashes for the given start and end
// heights.  It is inclusive of the start height and exclusive of the end
// height.  The end height will be limited to the current main chain height.
//
// This function is safe for concurrent access.
func (b *BlockChain) HeightRange(startHeight, endHeight int64) ([]chainhash.Hash, error) {
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

// -----------------------------------------------------------------------------
// The threshold state consists of individual threshold cache buckets for each
// cache id under one main threshold state bucket.  Each threshold cache bucket
// contains entries keyed by the block hash for the final block in each window
// and their associated threshold states as well as the associated deployment
// parameters.
//
// The serialized value format is for each cache entry keyed by hash is:
//
//   <thresholdstate>
//
//   Field             Type      Size
//   threshold state   uint8     1 byte
//
//
// In addition, the threshold cache buckets for deployments contain the specific
// deployment parameters they were created with.  This allows the cache
// invalidation when there any changes to their definitions.
//
// The serialized value format for the deployment parameters is:
//
//   <bit number><start time><expire time>
//
//   Field            Type      Size
//   mask             uint16    2 bytes
//   start time       uint64    8 bytes
//   expire time      uint64    8 bytes
//   num choices      uint16    2 bytes
//   choice[0..N]     uint32    4 bytes
//
// The serialized value format for the choice array is:
//
//   <bits><isAbstain><isNo>
//
//   Field            Type      Size
//   bits             uint16    2 bytes
//   isAbstain        uint8     1 byte (bool)
//   isNo             uint8     1 byte (bool)
//
// Finally, the main threshold bucket also contains the number of stored
// deployment buckets as described above.
//
// The serialized value format for the number of stored deployment buckets is:
//
//   <num deployments>
//
//   Field             Type      Size
//   num deployments   uint32    4 bytes
// -----------------------------------------------------------------------------

// serializeDeploymentCacheParams serializes the parameters for the passed
// deployment into a single byte slice according to the format described in
// detail above.
func serializeDeploymentCacheParams(deployment *chaincfg.ConsensusDeployment) []byte {
	serialized := make([]byte, 2+8+8+2+len(deployment.Vote.Choices)*4)
	byteOrder.PutUint16(serialized[0:], deployment.Vote.Mask)
	byteOrder.PutUint64(serialized[2:], deployment.StartTime)
	byteOrder.PutUint64(serialized[10:], deployment.ExpireTime)
	byteOrder.PutUint16(serialized[18:],
		uint16(len(deployment.Vote.Choices)))
	for i := 0; i < len(deployment.Vote.Choices); i++ {
		byteOrder.PutUint16(serialized[20+i*4:],
			deployment.Vote.Choices[i].Bits)
		if deployment.Vote.Choices[i].IsAbstain {
			serialized[20+i*4+2] = 1
		}
		if deployment.Vote.Choices[i].IsNo {
			serialized[20+i*4+3] = 1
		}
	}
	return serialized
}

// deserializeDeploymentCacheParams deserializes the passed serialized
// deployment cache parameters into a deployment struct.
func deserializeDeploymentCacheParams(serialized []byte) (chaincfg.ConsensusDeployment, error) {
	// Ensure the serialized data has enough bytes to properly deserialize
	// the bit number, start time, and expire time.
	if len(serialized) < 2+8+8+2 {
		return chaincfg.ConsensusDeployment{}, database.Error{
			ErrorCode:   database.ErrCorruption,
			Description: "corrupt deployment cache state",
		}
	}

	var deployment chaincfg.ConsensusDeployment
	deployment.Vote.Mask = byteOrder.Uint16(serialized[0:])
	deployment.StartTime = byteOrder.Uint64(serialized[2:])
	deployment.ExpireTime = byteOrder.Uint64(serialized[10:])
	choicesLen := byteOrder.Uint16(serialized[18:])

	// make sure we have enough bytes
	if len(serialized) != 2+8+8+2+int(choicesLen)*4 {
		return chaincfg.ConsensusDeployment{}, database.Error{
			ErrorCode:   database.ErrCorruption,
			Description: "corrupt deployment choices cache state",
		}
	}

	// Recreate array.
	deployment.Vote.Choices = make([]chaincfg.Choice, choicesLen)
	for i := 0; i < int(choicesLen); i++ {
		deployment.Vote.Choices[i].Bits =
			byteOrder.Uint16(serialized[20+i*4:])
		if serialized[20+i*4+2] != 0 {
			deployment.Vote.Choices[i].IsAbstain = true
		}
		if serialized[20+i*4+3] != 0 {
			deployment.Vote.Choices[i].IsNo = true
		}
	}

	return deployment, nil
}
