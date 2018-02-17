// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/decred/dcrd/blockchain/internal/progresslog"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

// errInterruptRequested indicates that an operation was cancelled due
// to a user-requested interrupt.
var errInterruptRequested = errors.New("interrupt requested")

// errBatchFinished indicates that a foreach database loop was exited due to
// reaching the maximum batch size.
var errBatchFinished = errors.New("batch finished")

// interruptRequested returns true when the provided channel has been closed.
// This simplifies early shutdown slightly since the caller can just use an if
// statement instead of a select.
func interruptRequested(interrupted <-chan struct{}) bool {
	select {
	case <-interrupted:
		return true
	default:
	}

	return false
}

// deserializeDatabaseInfoV2 deserializes a database information struct from the
// passed serialized byte slice according to the legacy version 2 format.
//
// The legacy format is as follows:
//
//   Field      Type     Size      Description
//   version    uint32   4 bytes   The version of the database
//   compVer    uint32   4 bytes   The script compression version of the database
//   created    uint32   4 bytes   The date of the creation of the database
//
// The high bit (0x80000000) is used on version to indicate that an upgrade
// is in progress and used to confirm the database fidelity on start up.
func deserializeDatabaseInfoV2(dbInfoBytes []byte) (*databaseInfo, error) {
	// upgradeStartedBit if the bit flag for whether or not a database
	// upgrade is in progress. It is used to determine if the database
	// is in an inconsistent state from the update.
	const upgradeStartedBit = 0x80000000

	byteOrder := binary.LittleEndian

	rawVersion := byteOrder.Uint32(dbInfoBytes[0:4])
	upgradeStarted := (upgradeStartedBit & rawVersion) > 0
	version := rawVersion &^ upgradeStartedBit
	compVer := byteOrder.Uint32(dbInfoBytes[4:8])
	ts := byteOrder.Uint32(dbInfoBytes[8:12])

	if upgradeStarted {
		return nil, AssertError("database is in the upgrade started " +
			"state before resumable upgrades were supported - " +
			"delete the database and resync the blockchain")
	}

	return &databaseInfo{
		version: version,
		compVer: compVer,
		created: time.Unix(int64(ts), 0),
	}, nil
}

// ticketsVotedInBlock fetches a list of tickets that were voted in the
// block.
func ticketsVotedInBlock(bl *dcrutil.Block) []chainhash.Hash {
	var tickets []chainhash.Hash
	for _, stx := range bl.MsgBlock().STransactions {
		if stake.IsSSGen(stx) {
			tickets = append(tickets, stx.TxIn[1].PreviousOutPoint.Hash)
		}
	}

	return tickets
}

// ticketsRevokedInBlock fetches a list of tickets that were revoked in the
// block.
func ticketsRevokedInBlock(bl *dcrutil.Block) []chainhash.Hash {
	var tickets []chainhash.Hash
	for _, stx := range bl.MsgBlock().STransactions {
		if stake.DetermineTxType(stx) == stake.TxTypeSSRtx {
			tickets = append(tickets, stx.TxIn[0].PreviousOutPoint.Hash)
		}
	}

	return tickets
}

// upgradeToVersion2 upgrades a version 1 blockchain to version 2, allowing
// use of the new on-disk ticket database.
func upgradeToVersion2(db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo) error {
	// Hardcoded so updates to the global values do not affect old upgrades.
	chainStateKeyName := []byte("chainstate")

	log.Infof("Initializing upgrade to database version 2")
	progressLogger := progresslog.NewBlockProgressLogger("Upgraded", log)

	// The upgrade is atomic, so there is no need to set the flag that
	// the database is undergoing an upgrade here.  Get the stake node
	// for the genesis block, and then begin connecting stake nodes
	// incrementally.
	err := db.Update(func(dbTx database.Tx) error {
		// Fetch the stored best chain state from the database metadata.
		serializedData := dbTx.Metadata().Get(chainStateKeyName)
		best, err := deserializeBestChainState(serializedData)
		if err != nil {
			return err
		}

		bestStakeNode, errLocal := stake.InitDatabaseState(dbTx, chainParams)
		if errLocal != nil {
			return errLocal
		}

		parent, errLocal := dbFetchBlockByHeight(dbTx, 0)
		if errLocal != nil {
			return errLocal
		}

		for i := int64(1); i <= int64(best.height); i++ {
			block, errLocal := dbFetchBlockByHeight(dbTx, i)
			if errLocal != nil {
				return errLocal
			}

			// If we need the tickets, fetch them too.
			var newTickets []chainhash.Hash
			if i >= chainParams.StakeEnabledHeight {
				matureHeight := i - int64(chainParams.TicketMaturity)
				matureBlock, errLocal := dbFetchBlockByHeight(dbTx, matureHeight)
				if errLocal != nil {
					return errLocal
				}
				for _, stx := range matureBlock.MsgBlock().STransactions {
					if stake.IsSStx(stx) {
						h := stx.TxHash()
						newTickets = append(newTickets, h)
					}
				}
			}

			// Iteratively connect the stake nodes in memory.
			header := block.MsgBlock().Header
			hB, errLocal := header.Bytes()
			if errLocal != nil {
				return errLocal
			}
			bestStakeNode, errLocal = bestStakeNode.ConnectNode(
				stake.CalcHash256PRNGIV(hB), ticketsVotedInBlock(block),
				ticketsRevokedInBlock(block), newTickets)
			if errLocal != nil {
				return errLocal
			}

			// Write the top block stake node to the database.
			errLocal = stake.WriteConnectedBestNode(dbTx, bestStakeNode,
				best.hash)
			if errLocal != nil {
				return errLocal
			}

			progressLogger.LogBlockHeight(block.MsgBlock(), parent.MsgBlock())
			parent = block
		}

		// Write the new database version.
		dbInfo.version = 2
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	log.Infof("Upgrade to new stake database was successful!")

	return nil
}

// migrateBlockIndex migrates all block entries from the v1 block index bucket
// manged by ffldb to the v2 bucket managed by this package.  The v1 bucket
// stored all block entries keyed by block hash, whereas the v2 bucket stores
// them keyed by block height + hash.  Also, the old block index only stored the
// header, while the new one stores all info needed to recreate block nodes.
//
// The new block index is guaranteed to be fully updated if this returns without
// failure.
func migrateBlockIndex(db database.DB, interrupt <-chan struct{}) error {
	// blkHdrOffset defines the offsets into a v1 block index row for the block
	// header.
	//
	// The serialized block index row format is:
	//   <blocklocation><blockheader>
	const blkHdrOffset = 12

	// blkHdrHeightStart is the offset of the height in the serialized block
	// header bytes as it existed at the time of this migration.  It is hard
	// coded here so potential future changes do not affect old upgrades.
	const blkHdrHeightStart = 128

	// Hardcoded bucket names so updates to the global values do not affect old
	// upgrades.
	v1BucketName := []byte("ffldb-blockidx")
	v2BucketName := []byte("blockidx")
	hashIdxBucketName := []byte("hashidx")

	log.Info("Reindexing block information in the database.  This will take " +
		"a while...")
	start := time.Now()

	// Create the new block index bucket as needed.
	err := db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(v2BucketName)
		return err
	})
	if err != nil {
		return err
	}

	// doBatch contains the primary logic for upgrading the block index from
	// version 1 to 2 in batches.  This is done because attempting to migrate in
	// a single database transaction could result in massive memory usage and
	// could potentially crash on many systems due to ulimits.
	//
	// It returns the number of entries processed.
	const maxEntries = 20000
	var resumeOffset uint32
	doBatch := func(dbTx database.Tx) (uint32, error) {
		meta := dbTx.Metadata()
		v1BlockIdxBucket := meta.Bucket(v1BucketName)
		if v1BlockIdxBucket == nil {
			return 0, fmt.Errorf("bucket %s does not exist", v1BucketName)
		}

		v2BlockIdxBucket := meta.Bucket(v2BucketName)
		if v2BlockIdxBucket == nil {
			return 0, fmt.Errorf("bucket %s does not exist", v2BucketName)
		}

		hashIdxBucket := meta.Bucket(hashIdxBucketName)
		if hashIdxBucket == nil {
			return 0, fmt.Errorf("bucket %s does not exist", hashIdxBucketName)
		}

		// Migrate block index entries so long as the max number of entries for
		// this batch has not been exceeded.
		var numMigrated, numIterated uint32
		err := v1BlockIdxBucket.ForEach(func(hashBytes, blockRow []byte) error {
			if numMigrated >= maxEntries {
				return errBatchFinished
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				return nil
			}
			resumeOffset++

			// Skip entries that have already been migrated in previous
			// interrupted upgrades.
			var blockHash chainhash.Hash
			copy(blockHash[:], hashBytes)
			endOffset := blkHdrOffset + blockHdrSize
			headerBytes := blockRow[blkHdrOffset:endOffset:endOffset]
			heightBytes := headerBytes[blkHdrHeightStart : blkHdrHeightStart+4]
			height := binary.LittleEndian.Uint32(heightBytes)
			key := blockIndexKey(&blockHash, height)
			if v2BlockIdxBucket.Get(key) != nil {
				return nil
			}

			// Load the raw full block from the database.
			blockBytes, err := dbTx.FetchBlock(&blockHash)
			if err != nil {
				return err
			}

			// Deserialize the block bytes.
			var block wire.MsgBlock
			err = block.Deserialize(bytes.NewReader(blockBytes))
			if err != nil {
				return err
			}

			// Construct a block node from the block.
			blockNode := newBlockNode(&block.Header, nil)
			blockNode.populateTicketInfo(stake.FindSpentTicketsInBlock(&block))
			blockNode.status = statusDataStored

			// Mark the block as valid if it's part of the main chain.  While it
			// is possible side chain blocks were validated too, there was
			// previously no tracking of that information, so there is no way to
			// know for sure.  It's better to be safe and just assume side chain
			// blocks were never validated.
			if hashIdxBucket.Get(blockHash[:]) != nil {
				blockNode.status |= statusValid
			}

			// Write the serialized block node to the new bucket keyed by its
			// hash and height.
			serialized, err := serializeBlockNode(blockNode)
			if err != nil {
				return err
			}
			err = v2BlockIdxBucket.Put(key, serialized)
			if err != nil {
				return err
			}

			numMigrated++

			if interruptRequested(interrupt) {
				return errInterruptRequested
			}

			return nil
		})
		return numMigrated, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	var totalMigrated uint64
	for {
		var numMigrated uint32
		err := db.Update(func(dbTx database.Tx) error {
			var err error
			numMigrated, err = doBatch(dbTx)
			if err == errInterruptRequested || err == errBatchFinished {
				// No error here so the database transaction is not cancelled
				// and therefore outstanding work is written to disk.  The
				// outer function will exit with an interrupted error below due
				// to another interrupted check.
				err = nil
			}
			return err
		})
		if err != nil {
			return err
		}

		if interruptRequested(interrupt) {
			return errInterruptRequested
		}

		if numMigrated == 0 {
			break
		}

		totalMigrated += uint64(numMigrated)
		log.Infof("Migrated %d entries (%d total)", numMigrated, totalMigrated)
	}

	seconds := int64(time.Since(start) / time.Second)
	log.Infof("Done upgrading block index.  Total entries: %d in %d seconds",
		totalMigrated, seconds)
	return nil
}

// upgradeToVersion3 upgrades a version 2 blockchain to version 3 along with
// upgrading the block index to version 2.
func upgradeToVersion3(db database.DB, dbInfo *databaseInfo, interrupt <-chan struct{}) error {
	if err := migrateBlockIndex(db, interrupt); err != nil {
		return err
	}

	// Update and persist the updated database versions.
	dbInfo.version = 3
	dbInfo.bidxVer = 2
	return db.Update(func(dbTx database.Tx) error {
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
}

// upgradeDB upgrades old database versions to the newest version by applying
// all possible upgrades iteratively.
//
// NOTE: The passed database info will be updated with the latest versions.
func upgradeDB(db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo, interrupt <-chan struct{}) error {
	if dbInfo.version == 1 {
		if err := upgradeToVersion2(db, chainParams, dbInfo); err != nil {
			return err
		}
	}

	// Migrate to the new v2 block index format if needed.  That database
	// version was bumped because prior versions of the software did not have
	// a block index version.
	if dbInfo.version == 2 && dbInfo.bidxVer < 2 {
		if err := upgradeToVersion3(db, dbInfo, interrupt); err != nil {
			return err
		}
	}

	return nil
}
