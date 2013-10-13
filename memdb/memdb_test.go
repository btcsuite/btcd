// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package memdb_test

import (
	"github.com/conformal/btcdb"
	"github.com/conformal/btcdb/memdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"reflect"
	"testing"
)

// TestClosed ensure calling the interface functions on a closed database
// returns appropriate errors for the interface functions that return errors
// and does not panic or otherwise misbehave for functions which do not return
// errors.
func TestClosed(t *testing.T) {
	db, err := btcdb.CreateDB("memdb", "")
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	_, err = db.InsertBlock(btcutil.NewBlock(&btcwire.GenesisBlock))
	if err != nil {
		t.Errorf("InsertBlock: %v", err)
	}
	db.Close()

	genesisHash := &btcwire.GenesisHash
	if err := db.DropAfterBlockBySha(genesisHash); err != memdb.ErrDbClosed {
		t.Errorf("DropAfterBlockBySha: unexpected error %v", err)
	}

	if exists := db.ExistsSha(genesisHash); exists != false {
		t.Errorf("ExistsSha: genesis hash exists after close")
	}

	if _, err := db.FetchBlockBySha(genesisHash); err != memdb.ErrDbClosed {
		t.Errorf("FetchBlockBySha: unexpected error %v", err)
	}

	if _, err := db.FetchBlockShaByHeight(0); err != memdb.ErrDbClosed {
		t.Errorf("FetchBlockShaByHeight: unexpected error %v", err)
	}

	if _, err := db.FetchHeightRange(0, 1); err != memdb.ErrDbClosed {
		t.Errorf("FetchHeightRange: unexpected error %v", err)
	}

	genesisMerkleRoot := &btcwire.GenesisMerkleRoot
	if exists := db.ExistsTxSha(genesisMerkleRoot); exists != false {
		t.Errorf("ExistsTxSha: hash %v exists when it shouldn't",
			genesisMerkleRoot)
	}

	if _, err := db.FetchTxBySha(genesisHash); err != memdb.ErrDbClosed {
		t.Errorf("FetchTxBySha: unexpected error %v", err)
	}

	requestHashes := []*btcwire.ShaHash{genesisHash}
	reply := db.FetchTxByShaList(requestHashes)
	if len(reply) != len(requestHashes) {
		t.Errorf("FetchUnSpentTxByShaList unexpected number of replies "+
			"got: %d, want: %d", len(reply), len(requestHashes))
	}
	for i, txLR := range reply {
		wantReply := &btcdb.TxListReply{
			Sha: requestHashes[i],
			Err: memdb.ErrDbClosed,
		}
		if !reflect.DeepEqual(wantReply, txLR) {
			t.Errorf("FetchTxByShaList unexpected reply\ngot: %v\n"+
				"want: %v", txLR, wantReply)
		}
	}

	reply = db.FetchUnSpentTxByShaList(requestHashes)
	if len(reply) != len(requestHashes) {
		t.Errorf("FetchUnSpentTxByShaList unexpected number of replies "+
			"got: %d, want: %d", len(reply), len(requestHashes))
	}
	for i, txLR := range reply {
		wantReply := &btcdb.TxListReply{
			Sha: requestHashes[i],
			Err: memdb.ErrDbClosed,
		}
		if !reflect.DeepEqual(wantReply, txLR) {
			t.Errorf("FetchUnSpentTxByShaList unexpected reply\n"+
				"got: %v\nwant: %v", txLR, wantReply)
		}
	}

	if _, _, err := db.NewestSha(); err != memdb.ErrDbClosed {
		t.Errorf("NewestSha: unexpected error %v", err)
	}

	// The following calls don't return errors from the interface to be able
	// to detect a closed database, so just call them to ensure there are no
	// panics.
	db.InvalidateBlockCache()
	db.InvalidateCache()
	db.InvalidateTxCache()
	db.NewIterateBlocks()
	db.SetDBInsertMode(btcdb.InsertNormal)
	db.SetDBInsertMode(btcdb.InsertFast)
	db.SetDBInsertMode(btcdb.InsertValidatedInput)
	db.Sync()
	db.RollbackClose()
}
