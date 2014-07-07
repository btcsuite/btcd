// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package memdb_test

import (
	"reflect"
	"testing"

	"github.com/conformal/btcdb"
	"github.com/conformal/btcdb/memdb"
	"github.com/conformal/btcnet"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// TestClosed ensure calling the interface functions on a closed database
// returns appropriate errors for the interface functions that return errors
// and does not panic or otherwise misbehave for functions which do not return
// errors.
func TestClosed(t *testing.T) {
	db, err := btcdb.CreateDB("memdb")
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	_, err = db.InsertBlock(btcutil.NewBlock(btcnet.MainNetParams.GenesisBlock))
	if err != nil {
		t.Errorf("InsertBlock: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("Close: unexpected error %v", err)
	}

	genesisHash := btcnet.MainNetParams.GenesisHash
	if err := db.DropAfterBlockBySha(genesisHash); err != memdb.ErrDbClosed {
		t.Errorf("DropAfterBlockBySha: unexpected error %v", err)
	}

	if _, err := db.ExistsSha(genesisHash); err != memdb.ErrDbClosed {
		t.Errorf("ExistsSha: Unexpected error: %v", err)
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

	genesisCoinbaseTx := btcnet.MainNetParams.GenesisBlock.Transactions[0]
	coinbaseHash, err := genesisCoinbaseTx.TxSha()
	if err != nil {
		t.Errorf("TxSha: unexpected error %v", err)
	}
	if _, err := db.ExistsTxSha(&coinbaseHash); err != memdb.ErrDbClosed {
		t.Errorf("ExistsTxSha: unexpected error %v", err)
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

	if err := db.Sync(); err != memdb.ErrDbClosed {
		t.Errorf("Sync: unexpected error %v", err)
	}

	if err := db.RollbackClose(); err != memdb.ErrDbClosed {
		t.Errorf("RollbackClose: unexpected error %v", err)
	}

	if err := db.Close(); err != memdb.ErrDbClosed {
		t.Errorf("Close: unexpected error %v", err)
	}
}
