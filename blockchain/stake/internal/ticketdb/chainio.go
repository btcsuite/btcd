// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ticketdb

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/blockchain/stake/internal/dbnamespace"
	"github.com/decred/dcrd/blockchain/stake/internal/tickettreap"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
)

const (
	// upgradeStartedBit if the bit flag for whether or not a database
	// upgrade is in progress. It is used to determine if the database
	// is in an inconsistent state from the update.
	upgradeStartedBit = 0x80000000

	// currentDatabaseVersion indicates what the current database
	// version is.
	currentDatabaseVersion = 1
)

// Database structure -------------------------------------------------------------
//
//   Buckets
//
// The information about the ticket database is defined by the
// StakeDbInfoBucketName bucket. By default, this bucket contains a single key
// keyed to the contents of StakeDbInfoBucketName which contains the value of
// all the database information, such as the date created and the version of
// the database.
//
// Blockchain state is stored in a root key named StakeChainStateKeyName. This
// contains the current height of the blockchain, which should be equivalent to
// the height of the best chain on start up.
//
// There are 5 buckets from the database reserved for tickets. These are:
// 1. Live
//     Live ticket bucket, for tickets currently in the lottery
//
//     k: ticket hash
//     v: height
//
// 2. Missed
//     Missed tickets bucket, for all tickets that are missed.
//
//     k: ticket hash
//     v: height
//
// 3. Revoked
//     Revoked tickets bucket, for all tickets that are Revoked.
//
//     k: ticket hash
//     v: height
//
// 4. BlockUndo
//     Block removal data, for reverting the the first 3 database buckets to
//     a previous state.
//
//     k: height
//     v: serialized undo ticket data
//
// 5. TicketsToAdd
//     Tickets to add bucket, which tells which tickets will be maturing and
//     entering the (1) in the event that a block at that height is added.
//
//     k: height
//     v: serialized list of ticket hashes
//
// For pruned nodes, both 4 and 5 can be curtailed to include only the most
// recent blocks.
//
// Procedures ---------------------------------------------------------------------
//
//   Adding a block
//
// The steps for the addition of a block are as follows:
// 1. Remove the n (constant, n=5 for all Decred networks) many tickets that were
//     selected this block.  The results of this feed into two database updates:
//         ------> A database entry containing all the data for the block
//            |     required to undo the adding of the block (as serialized
//            |     SpentTicketData and MissedTicketData)
//            \--> All missed tickets must be moved to the missed ticket bucket.
//
// 2. Expire any tickets from this block.
//     The results of this feed into two database updates:
//         ------> A database entry containing all the data for the block
//            |     required to undo the adding of the block (as serialized
//            |     MissedTicketData)
//            \--> All expired tickets must be moved to the missed ticket bucket.
//
// 3. All revocations in the block are processed, and the revoked ticket moved
//     from the missed ticket bucket to the revocations bucket:
//         ------> A database entry containing all the data for the block
//            |     required to undo the adding of the block (as serialized
//            |     MissedTicketData, revoked flag added)
//            \--> All revoked tickets must be moved to the revoked ticket bucket.
//
// 4. All newly maturing tickets must be added to the live ticket bucket. These
//     are previously stored in the "tickets to add" bucket so they can more
//     easily be pulled down when adding a block without having to load the
//     entire block itself and suffer the deserialization overhead. The only
//     things that must be written for this step are newly added tickets to the
//     ticket database, along with their respective heights.
//
//   Removing a block
//
// Steps 1 through 4 above are iterated through in reverse. The newly maturing
//  ticket hashes are fetched from the "tickets to add" bucket for the given
//  height that was used at this block height, and the tickets are dropped from
//  the live ticket bucket. The UndoTicketData is then fetched for the block and
//  iterated through in reverse order (it was stored in forward order) to restore
//  any changes to the relevant buckets made when inserting the block. Finally,
//  the data for the block removed is purged from both the BlockUndo and
//  TicketsToAdd buckets.

// -----------------------------------------------------------------------------
// The database information contains information about the version and date
// of the blockchain database.
//
//   Field      Type     Size      Description
//   version    uint32   4 bytes   The version of the database
//   date       uint32   4 bytes   The date of the creation of the database
//
// The high bit (0x80000000) is used on version to indicate that an upgrade
// is in progress and used to confirm the database fidelity on start up.
// -----------------------------------------------------------------------------

// databaseInfoSize is the serialized size of the best chain state in bytes.
const databaseInfoSize = 8

// DatabaseInfo is the structure for a database.
type DatabaseInfo struct {
	Date           time.Time
	Version        uint32
	UpgradeStarted bool
}

// serializeDatabaseInfo serializes a database information struct.
func serializeDatabaseInfo(dbi *DatabaseInfo) []byte {
	version := dbi.Version
	if dbi.UpgradeStarted {
		version |= upgradeStartedBit
	}

	val := make([]byte, databaseInfoSize)
	versionBytes := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(versionBytes, version)
	copy(val[0:4], versionBytes)
	timestampBytes := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(timestampBytes, uint32(dbi.Date.Unix()))
	copy(val[4:8], timestampBytes)

	return val
}

// DbPutDatabaseInfo uses an existing database transaction to store the database
// information.
func DbPutDatabaseInfo(dbTx database.Tx, dbi *DatabaseInfo) error {
	meta := dbTx.Metadata()
	subsidyBucket := meta.Bucket(dbnamespace.StakeDbInfoBucketName)
	val := serializeDatabaseInfo(dbi)

	// Store the current database info into the database.
	return subsidyBucket.Put(dbnamespace.StakeDbInfoBucketName, val[:])
}

// deserializeDatabaseInfo deserializes a database information struct.
func deserializeDatabaseInfo(dbInfoBytes []byte) (*DatabaseInfo, error) {
	if len(dbInfoBytes) < databaseInfoSize {
		return nil, ticketDBError(ErrDatabaseInfoShortRead,
			"short read when deserializing best chain state data")
	}

	rawVersion := dbnamespace.ByteOrder.Uint32(dbInfoBytes[0:4])
	upgradeStarted := (upgradeStartedBit & rawVersion) > 0
	version := rawVersion &^ upgradeStartedBit
	ts := dbnamespace.ByteOrder.Uint32(dbInfoBytes[4:8])

	return &DatabaseInfo{
		Version:        version,
		Date:           time.Unix(int64(ts), 0),
		UpgradeStarted: upgradeStarted,
	}, nil
}

// DbFetchDatabaseInfo uses an existing database transaction to
// fetch the database versioning and creation information.
func DbFetchDatabaseInfo(dbTx database.Tx) (*DatabaseInfo, error) {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.StakeDbInfoBucketName)

	// Uninitialized state.
	if bucket == nil {
		return nil, nil
	}

	dbInfoBytes := bucket.Get(dbnamespace.StakeDbInfoBucketName)
	if dbInfoBytes == nil {
		return nil, ticketDBError(ErrMissingKey, "missing key for database info")
	}

	return deserializeDatabaseInfo(dbInfoBytes)
}

// -----------------------------------------------------------------------------
// The best chain state consists of the best block hash and height, the total
// number of live tickets, the total number of missed tickets, and the number of
// revoked tickets.
//
// The serialized format is:
//
//   <block hash><block height><live><missed><revoked>
//
//   Field              Type              Size
//   block hash         chainhash.Hash    chainhash.HashSize
//   block height       uint32            4 bytes
//   live tickets       uint32            4 bytes
//   missed tickets     uint64            8 bytes
//   revoked tickets    uint64            8 bytes
//   tickets per block  uint16            2 bytes
//   next winners       []chainhash.Hash  chainhash.hashSize * tickets per block
// -----------------------------------------------------------------------------

// minimumBestChainStateSize is the minimum serialized size of the best chain
// state in bytes.
var minimumBestChainStateSize = chainhash.HashSize + 4 + 4 + 8 + 8 + 2

// BestChainState represents the data to be stored the database for the current
// best chain state.
type BestChainState struct {
	Hash        chainhash.Hash
	Height      uint32
	Live        uint32
	Missed      uint64
	Revoked     uint64
	PerBlock    uint16
	NextWinners []chainhash.Hash
}

// serializeBestChainState returns the serialization of the passed block best
// chain state.  This is data to be stored in the chain state bucket. This
// function will panic if the number of tickets per block is less than the
// size of next winners, which should never happen unless there is memory
// corruption.
func serializeBestChainState(state BestChainState) []byte {
	// Serialize the chain state.
	serializedData := make([]byte, minimumBestChainStateSize)

	offset := 0
	copy(serializedData[offset:offset+chainhash.HashSize], state.Hash[:])
	offset += chainhash.HashSize
	dbnamespace.ByteOrder.PutUint32(serializedData[offset:], state.Height)
	offset += 4
	dbnamespace.ByteOrder.PutUint32(serializedData[offset:], state.Live)
	offset += 4
	dbnamespace.ByteOrder.PutUint64(serializedData[offset:], state.Missed)
	offset += 8
	dbnamespace.ByteOrder.PutUint64(serializedData[offset:], state.Revoked)
	offset += 8
	dbnamespace.ByteOrder.PutUint16(serializedData[offset:], state.PerBlock)
	offset += 2

	// Serialize the next winners.
	ticketBuffer := make([]byte, chainhash.HashSize*int(state.PerBlock))
	serializedData = append(serializedData, ticketBuffer...)
	for i := range state.NextWinners {
		copy(serializedData[offset:offset+chainhash.HashSize],
			state.NextWinners[i][:])
		offset += chainhash.HashSize
	}

	return serializedData[:]
}

// deserializeBestChainState deserializes the passed serialized best chain
// state.  This is data stored in the chain state bucket and is updated after
// every block is connected or disconnected form the main chain.
// block.
func deserializeBestChainState(serializedData []byte) (BestChainState, error) {
	// Ensure the serialized data has enough bytes to properly deserialize
	// the state.
	if len(serializedData) < minimumBestChainStateSize {
		return BestChainState{}, ticketDBError(ErrChainStateShortRead,
			"short read when deserializing best chain state data")
	}

	state := BestChainState{}
	offset := 0
	copy(state.Hash[:], serializedData[offset:offset+chainhash.HashSize])
	offset += chainhash.HashSize
	state.Height = dbnamespace.ByteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	state.Live = dbnamespace.ByteOrder.Uint32(
		serializedData[offset : offset+4])
	offset += 4
	state.Missed = dbnamespace.ByteOrder.Uint64(
		serializedData[offset : offset+8])
	offset += 8
	state.Revoked = dbnamespace.ByteOrder.Uint64(
		serializedData[offset : offset+8])
	offset += 8
	state.PerBlock = dbnamespace.ByteOrder.Uint16(
		serializedData[offset : offset+2])
	offset += 2

	state.NextWinners = make([]chainhash.Hash, int(state.PerBlock))
	for i := 0; i < int(state.PerBlock); i++ {
		copy(state.NextWinners[i][:],
			serializedData[offset:offset+chainhash.HashSize])
		offset += chainhash.HashSize
	}

	return state, nil
}

// DbFetchBestState uses an existing database transaction to fetch the best chain
// state.
func DbFetchBestState(dbTx database.Tx) (BestChainState, error) {
	meta := dbTx.Metadata()
	v := meta.Get(dbnamespace.StakeChainStateKeyName)
	if v == nil {
		return BestChainState{}, ticketDBError(ErrMissingKey,
			"missing key for chain state data")
	}

	return deserializeBestChainState(v)
}

// DbPutBestState uses an existing database transaction to update the best chain
// state with the given parameters.
func DbPutBestState(dbTx database.Tx, bcs BestChainState) error {
	// Serialize the current best chain state.
	serializedData := serializeBestChainState(bcs)

	// Store the current best chain state into the database.
	return dbTx.Metadata().Put(dbnamespace.StakeChainStateKeyName, serializedData)
}

// UndoTicketData is the data for any ticket that has been spent, missed, or
// revoked at some new height.  It is used to roll back the database in the
// event of reorganizations or determining if a side chain block is valid.
// The last 3 are encoded as a single byte of flags.
// The flags describe a particular state for the ticket:
//  1. Missed is set, but revoked and spent are not (0000 0001). The ticket
//      was selected in the lottery at this block height but missed, or the
//      ticket became too old and was missed. The ticket is being moved to the
//      missed ticket bucket from the live ticket bucket.
//  2. Missed and revoked are set (0000 0011). The ticket was missed
//      previously at a block before this one and was revoked, and
//      as such is being moved to the revoked ticket bucket from the
//      missed ticket bucket.
//  3. Spent is set (0000 0100). The ticket has been spent and is removed
//      from the live ticket bucket.
//  4. No flags are set. The ticket was newly added to the live ticket
//      bucket this block as a maturing ticket.
type UndoTicketData struct {
	TicketHash   chainhash.Hash
	TicketHeight uint32
	Missed       bool
	Revoked      bool
	Spent        bool
	Expired      bool
}

// undoTicketDataSize is the serialized size of an UndoTicketData struct in bytes.
const undoTicketDataSize = 37

// undoBitFlagsToByte converts the bools of the UndoTicketData struct into a
// series of bitflags in a single byte.
func undoBitFlagsToByte(missed, revoked, spent, expired bool) byte {
	var b byte
	if missed {
		b |= 1 << 0
	}
	if revoked {
		b |= 1 << 1
	}
	if spent {
		b |= 1 << 2
	}
	if expired {
		b |= 1 << 3
	}

	return b
}

// undoBitFlagsFromByte converts a byte into its relevant flags.
func undoBitFlagsFromByte(b byte) (bool, bool, bool, bool) {
	missed := b&(1<<0) > 0
	revoked := b&(1<<1) > 0
	spent := b&(1<<2) > 0
	expired := b&(1<<3) > 0

	return missed, revoked, spent, expired
}

// serializeBlockUndoData serializes an entire list of relevant tickets for
// undoing tickets at any given height.
func serializeBlockUndoData(utds []UndoTicketData) []byte {
	b := make([]byte, len(utds)*undoTicketDataSize)
	offset := 0
	for _, utd := range utds {
		copy(b[offset:offset+chainhash.HashSize], utd.TicketHash[:])
		offset += chainhash.HashSize
		dbnamespace.ByteOrder.PutUint32(b[offset:offset+4], utd.TicketHeight)
		offset += 4
		b[offset] = undoBitFlagsToByte(utd.Missed, utd.Revoked, utd.Spent,
			utd.Expired)
		offset++
	}

	return b
}

// deserializeBlockUndoData deserializes a list of UndoTicketData for an entire
// block. Empty but non-nil slices are deserialized empty.
func deserializeBlockUndoData(b []byte) ([]UndoTicketData, error) {
	if b != nil && len(b) == 0 {
		return make([]UndoTicketData, 0), nil
	}

	if len(b) < undoTicketDataSize {
		return nil, ticketDBError(ErrUndoDataShortRead, "short read when "+
			"deserializing block undo data")
	}

	if len(b)%undoTicketDataSize != 0 {
		return nil, ticketDBError(ErrUndoDataCorrupt, "corrupt data found "+
			"when deserializing block undo data")
	}

	entries := len(b) / undoTicketDataSize
	utds := make([]UndoTicketData, entries)

	offset := 0
	for i := 0; i < entries; i++ {
		hash, err := chainhash.NewHash(
			b[offset : offset+chainhash.HashSize])
		if err != nil {
			return nil, ticketDBError(ErrUndoDataCorrupt, "corrupt hash found "+
				"when deserializing block undo data")
		}
		offset += chainhash.HashSize

		height := dbnamespace.ByteOrder.Uint32(b[offset : offset+4])
		offset += 4

		missed, revoked, spent, expired := undoBitFlagsFromByte(b[offset])
		offset++

		utds[i] = UndoTicketData{
			TicketHash:   *hash,
			TicketHeight: height,
			Missed:       missed,
			Revoked:      revoked,
			Spent:        spent,
			Expired:      expired,
		}
	}

	return utds, nil
}

// DbFetchBlockUndoData fetches block undo data from the database.
func DbFetchBlockUndoData(dbTx database.Tx, height uint32) ([]UndoTicketData, error) {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.StakeBlockUndoDataBucketName)

	k := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(k, height)
	v := bucket.Get(k)
	if v == nil {
		return nil, ticketDBError(ErrMissingKey,
			fmt.Sprintf("missing key %v for block undo data", height))
	}

	return deserializeBlockUndoData(v)
}

// DbPutBlockUndoData inserts block undo data into the database for a given height.
func DbPutBlockUndoData(dbTx database.Tx, height uint32, utds []UndoTicketData) error {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.StakeBlockUndoDataBucketName)
	k := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(k, height)
	v := serializeBlockUndoData(utds)

	return bucket.Put(k[:], v[:])
}

// DbDropBlockUndoData drops block undo data from the database at a given height.
func DbDropBlockUndoData(dbTx database.Tx, height uint32) error {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.StakeBlockUndoDataBucketName)
	k := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(k, height)

	return bucket.Delete(k)
}

// TicketHashes is a list of ticket hashes that will mature in TicketMaturity
// many blocks from the block in which they were included.
type TicketHashes []chainhash.Hash

// serializeTicketHashes serializes a list of ticket hashes.
func serializeTicketHashes(ths TicketHashes) []byte {
	b := make([]byte, len(ths)*chainhash.HashSize)
	offset := 0
	for _, th := range ths {
		copy(b[offset:offset+chainhash.HashSize], th[:])
		offset += chainhash.HashSize
	}

	return b
}

// deserializeTicketHashes deserializes a list of ticket hashes. Empty but
// non-nil slices are deserialized empty.
func deserializeTicketHashes(b []byte) (TicketHashes, error) {
	if b != nil && len(b) == 0 {
		return make(TicketHashes, 0), nil
	}

	if len(b) < chainhash.HashSize {
		return nil, ticketDBError(ErrTicketHashesShortRead, "short read when "+
			"deserializing ticket hashes")
	}

	if len(b)%chainhash.HashSize != 0 {
		return nil, ticketDBError(ErrTicketHashesCorrupt, "corrupt data found "+
			"when deserializing ticket hashes")
	}

	entries := len(b) / chainhash.HashSize
	ths := make(TicketHashes, entries)

	offset := 0
	for i := 0; i < entries; i++ {
		hash, err := chainhash.NewHash(
			b[offset : offset+chainhash.HashSize])
		if err != nil {
			return nil, ticketDBError(ErrUndoDataCorrupt, "corrupt hash found "+
				"when deserializing block undo data")
		}
		offset += chainhash.HashSize

		ths[i] = *hash
	}

	return ths, nil
}

// DbFetchNewTickets fetches new tickets for a mainchain block from the database.
func DbFetchNewTickets(dbTx database.Tx, height uint32) (TicketHashes, error) {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.TicketsInBlockBucketName)

	k := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(k, height)
	v := bucket.Get(k)
	if v == nil {
		return nil, ticketDBError(ErrMissingKey,
			fmt.Sprintf("missing key %v for new tickets", height))
	}

	return deserializeTicketHashes(v)
}

// DbPutNewTickets inserts new tickets for a mainchain block data into the
// database.
func DbPutNewTickets(dbTx database.Tx, height uint32, ths TicketHashes) error {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.TicketsInBlockBucketName)
	k := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(k, height)
	v := serializeTicketHashes(ths)

	return bucket.Put(k[:], v[:])
}

// DbDropNewTickets drops new tickets for a mainchain block data at some height.
func DbDropNewTickets(dbTx database.Tx, height uint32) error {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.TicketsInBlockBucketName)
	k := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(k, height)

	return bucket.Delete(k)
}

// DbDeleteTicket removes a ticket from one of the ticket database buckets. This
// differs from the bucket deletion method in that it will fail if the value
// itself is missing.
func DbDeleteTicket(dbTx database.Tx, ticketBucket []byte, hash *chainhash.Hash) error {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(ticketBucket)

	// Check to see if the value exists before we delete it.
	v := bucket.Get(hash[:])
	if v == nil {
		return ticketDBError(ErrMissingKey, fmt.Sprintf("missing key %v "+
			"to delete", hash))
	}

	return bucket.Delete(hash[:])
}

// DbPutTicket inserts a ticket into one of the ticket database buckets.
func DbPutTicket(dbTx database.Tx, ticketBucket []byte, hash *chainhash.Hash,
	height uint32, missed, revoked, spent, expired bool) error {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(ticketBucket)
	k := hash[:]
	v := make([]byte, 5)
	dbnamespace.ByteOrder.PutUint32(v, height)
	v[4] = undoBitFlagsToByte(missed, revoked, spent, expired)

	return bucket.Put(k[:], v[:])
}

// DbLoadAllTickets loads all the live tickets from the database into a treap.
func DbLoadAllTickets(dbTx database.Tx, ticketBucket []byte) (*tickettreap.Immutable, error) {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(ticketBucket)

	treap := tickettreap.NewImmutable()
	err := bucket.ForEach(func(k []byte, v []byte) error {
		if len(v) < 5 {
			return ticketDBError(ErrLoadAllTickets, fmt.Sprintf("short "+
				"read for ticket key %x when loading tickets", k))
		}

		h, err := chainhash.NewHash(k)
		if err != nil {
			return err
		}
		treapKey := tickettreap.Key(*h)
		missed, revoked, spent, expired := undoBitFlagsFromByte(v[4])
		treapValue := &tickettreap.Value{
			Height:  dbnamespace.ByteOrder.Uint32(v[0:4]),
			Missed:  missed,
			Revoked: revoked,
			Spent:   spent,
			Expired: expired,
		}

		treap = treap.Put(treapKey, treapValue)
		return nil
	})
	if err != nil {
		return nil, ticketDBError(ErrLoadAllTickets, fmt.Sprintf("failed to "+
			"load all tickets for the bucket %s", string(ticketBucket)))
	}

	return treap, nil
}

// DbCreate initializes all the buckets required for the database and stores
// the current database version information.
func DbCreate(dbTx database.Tx) error {
	meta := dbTx.Metadata()

	// Create the bucket that houses information about the database's
	// creation and version.
	_, err := meta.CreateBucket(dbnamespace.StakeDbInfoBucketName)
	if err != nil {
		return err
	}

	dbInfo := &DatabaseInfo{
		Version:        currentDatabaseVersion,
		Date:           time.Now(),
		UpgradeStarted: false,
	}
	err = DbPutDatabaseInfo(dbTx, dbInfo)
	if err != nil {
		return err
	}

	// Create the bucket that houses the live tickets of the best node.
	_, err = meta.CreateBucket(dbnamespace.LiveTicketsBucketName)
	if err != nil {
		return err
	}

	// Create the bucket that houses the missed tickets of the best node.
	_, err = meta.CreateBucket(dbnamespace.MissedTicketsBucketName)
	if err != nil {
		return err
	}

	// Create the bucket that houses the revoked tickets of the best node.
	_, err = meta.CreateBucket(dbnamespace.RevokedTicketsBucketName)
	if err != nil {
		return err
	}

	// Create the bucket that houses block undo data for stake states on
	// the main chain.
	_, err = meta.CreateBucket(dbnamespace.StakeBlockUndoDataBucketName)
	if err != nil {
		return err
	}

	// Create the bucket that houses the tickets that were added with
	// this block into the main chain.
	_, err = meta.CreateBucket(dbnamespace.TicketsInBlockBucketName)
	return err
}
