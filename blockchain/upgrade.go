// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"encoding/binary"
	"time"

	"github.com/decred/dcrd/blockchain/internal/progresslog"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
)

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

// upgradeDB upgrades old database versions to the newest version by applying
// all possible upgrades iteratively.
//
// NOTE: The passed database info will be updated with the latest versions.
func upgradeDB(db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo) error {
	if dbInfo.version == 1 {
		if err := upgradeToVersion2(db, chainParams, dbInfo); err != nil {
			return err
		}
	}
	return nil
}
