// Copyright (c) 2016-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"sync"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

var (
	// existsAddressIndexName is the human-readable name for the index.
	existsAddressIndexName = "exists address index"

	// existsAddrIndexKey is the key of the ever seen address index and
	// the db bucket used to house it.
	existsAddrIndexKey = []byte("existsaddridx")
)

// ExistsAddrIndex implements an "ever seen" address index.  Any address that
// is ever seen in a block or in the mempool is stored here as a key. Values
// are empty.  Once an address is seen, it is never removed from this store.
// This results in a local version of this database that is consistent only
// for this peer, but at minimum contains all the addresses seen on the
// blockchain itself.
//
// In addition, support is provided for a memory-only index of unconfirmed
// transactions such as those which are kept in the memory pool before inclusion
// in a block.
type ExistsAddrIndex struct {
	// The following fields are set when the instance is created and can't
	// be changed afterwards, so there is no need to protect them with a
	// separate mutex.
	db          database.DB
	chainParams *chaincfg.Params

	// The following fields are used to quickly link transactions and
	// addresses that have not been included into a block yet when an
	// address index is being maintained.  The are protected by the
	// unconfirmedLock field.
	//
	// The txnsByAddr field is used to keep an index of all transactions
	// which either create an output to a given address or spend from a
	// previous output to it keyed by the address.
	//
	// The addrsByTx field is essentially the reverse and is used to
	// keep an index of all addresses which a given transaction involves.
	// This allows fairly efficient updates when transactions are removed
	// once they are included into a block.
	unconfirmedLock sync.RWMutex
	mpExistsAddr    map[[addrKeySize]byte]struct{}
}

// NewExistsAddrIndex returns a new instance of an indexer that is used to
// create a mapping of all addresses ever seen.
//
// It implements the Indexer interface which plugs into the IndexManager that in
// turn is used by the blockchain package.  This allows the index to be
// seamlessly maintained along with the chain.
func NewExistsAddrIndex(db database.DB, chainParams *chaincfg.Params) *ExistsAddrIndex {
	return &ExistsAddrIndex{
		db:           db,
		chainParams:  chainParams,
		mpExistsAddr: make(map[[addrKeySize]byte]struct{}),
	}
}

// Ensure the ExistsAddrIndex type implements the Indexer interface.
var _ Indexer = (*ExistsAddrIndex)(nil)

// Ensure the ExistsAddrIndex type implements the NeedsInputser interface.
var _ NeedsInputser = (*ExistsAddrIndex)(nil)

// NeedsInputs signals that the index requires the referenced inputs in order
// to properly create the index.
//
// This implements the NeedsInputser interface.
func (idx *ExistsAddrIndex) NeedsInputs() bool {
	return false
}

// Init is only provided to satisfy the Indexer interface as there is nothing to
// initialize for this index.
//
// This is part of the Indexer interface.
func (idx *ExistsAddrIndex) Init() error {
	// Nothing to do.
	return nil
}

// Key returns the database key to use for the index as a byte slice.
//
// This is part of the Indexer interface.
func (idx *ExistsAddrIndex) Key() []byte {
	return existsAddrIndexKey
}

// Name returns the human-readable name of the index.
//
// This is part of the Indexer interface.
func (idx *ExistsAddrIndex) Name() string {
	return existsAddressIndexName
}

// Create is invoked when the indexer manager determines the index needs
// to be created for the first time.  It creates the bucket for the address
// index.
//
// This is part of the Indexer interface.
func (idx *ExistsAddrIndex) Create(dbTx database.Tx) error {
	_, err := dbTx.Metadata().CreateBucket(existsAddrIndexKey)
	return err
}

// dbPutExistsAddr uses an existing database transaction to update or add a
// used address index to the database.
func dbPutExistsAddr(bucket database.Bucket, addrKey [addrKeySize]byte) error {
	return bucket.Put(addrKey[:], nil)
}

// existsAddress takes a bucket and key for an address and responds with
// whether or not the key exists in the database.
func (idx *ExistsAddrIndex) existsAddress(bucket database.Bucket, k [addrKeySize]byte) bool {
	if bucket.Get(k[:]) != nil {
		return true
	}

	idx.unconfirmedLock.RLock()
	_, exists := idx.mpExistsAddr[k]
	idx.unconfirmedLock.RUnlock()

	return exists
}

// ExistsAddress is the concurrency safe, exported function that returns
// whether or not an address has been seen before.
func (idx *ExistsAddrIndex) ExistsAddress(addr dcrutil.Address) (bool, error) {
	k, err := addrToKey(addr, idx.chainParams)
	if err != nil {
		return false, err
	}

	var exists bool
	err = idx.db.View(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		existsAddrIndex := meta.Bucket(existsAddrIndexKey)
		exists = existsAddrIndex.Get(k[:]) != nil

		return nil
	})
	if err != nil {
		return false, err
	}

	// Only check the in memory map if needed.
	if !exists {
		idx.unconfirmedLock.RLock()
		_, exists = idx.mpExistsAddr[k]
		idx.unconfirmedLock.RUnlock()
	}

	return exists, nil
}

// ExistsAddresses is the concurrency safe, exported function that returns
// whether or not each address in a slice of addresses has been seen before.
func (idx *ExistsAddrIndex) ExistsAddresses(addrs []dcrutil.Address) ([]bool, error) {
	exists := make([]bool, len(addrs))
	addrKeys := make([][addrKeySize]byte, len(addrs))
	for i := range addrKeys {
		var err error
		addrKeys[i], err = addrToKey(addrs[i], idx.chainParams)
		if err != nil {
			return nil, err
		}
	}

	err := idx.db.View(func(dbTx database.Tx) error {
		for i := range addrKeys {
			meta := dbTx.Metadata()
			existsAddrIndex := meta.Bucket(existsAddrIndexKey)

			exists[i] = existsAddrIndex.Get(addrKeys[i][:]) != nil
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	idx.unconfirmedLock.RLock()
	for i := range addrKeys {
		if !exists[i] {
			_, exists[i] = idx.mpExistsAddr[addrKeys[i]]
		}
	}
	idx.unconfirmedLock.RUnlock()

	return exists, nil
}

// ConnectBlock is invoked by the index manager when a new block has been
// connected to the main chain.  This indexer adds a key for each address
// the transactions in the block involve.
//
// This is part of the Indexer interface.
func (idx *ExistsAddrIndex) ConnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, view *blockchain.UtxoViewpoint) error {
	var parentTxs []*dcrutil.Tx
	if approvesParent(block) && block.Height() > 1 {
		parentTxs = parent.Transactions()
	}
	blockTxns := block.STransactions()
	allTxns := append(parentTxs, blockTxns...)

	usedAddrs := make(map[[addrKeySize]byte]struct{})

	for _, tx := range allTxns {
		msgTx := tx.MsgTx()
		isSStx := stake.IsSStx(msgTx)
		for _, txIn := range msgTx.TxIn {
			if txscript.IsMultisigSigScript(txIn.SignatureScript) {
				rs, err :=
					txscript.MultisigRedeemScriptFromScriptSig(
						txIn.SignatureScript)
				if err != nil {
					continue
				}

				class, addrs, _, err := txscript.ExtractPkScriptAddrs(
					txscript.DefaultScriptVersion, rs, idx.chainParams)
				if err != nil {
					// Non-standard outputs are skipped.
					continue
				}
				if class != txscript.MultiSigTy {
					// This should never happen, but be paranoid.
					continue
				}

				for _, addr := range addrs {
					k, err := addrToKey(addr, idx.chainParams)
					if err != nil {
						continue
					}

					usedAddrs[k] = struct{}{}
				}
			}
		}

		for _, txOut := range tx.MsgTx().TxOut {
			class, addrs, _, err := txscript.ExtractPkScriptAddrs(
				txOut.Version, txOut.PkScript, idx.chainParams)
			if err != nil {
				// Non-standard outputs are skipped.
				continue
			}

			if isSStx && class == txscript.NullDataTy {
				addr, err := stake.AddrFromSStxPkScrCommitment(txOut.PkScript,
					idx.chainParams)
				if err != nil {
					// Ignore unsupported address types.
					continue
				}

				addrs = append(addrs, addr)
			}

			for _, addr := range addrs {
				k, err := addrToKey(addr, idx.chainParams)
				if err != nil {
					// Ignore unsupported address types.
					continue
				}

				usedAddrs[k] = struct{}{}
			}
		}
	}

	// Write all the newly used addresses to the database,
	// skipping any keys that already exist. Write any
	// addresses we see in mempool at this time, too,
	// then remove them from the unconfirmed map drop
	// dropping the old map and reassigning a new map.
	idx.unconfirmedLock.Lock()
	for k := range idx.mpExistsAddr {
		usedAddrs[k] = struct{}{}
	}
	idx.mpExistsAddr = make(map[[addrKeySize]byte]struct{})
	idx.unconfirmedLock.Unlock()

	meta := dbTx.Metadata()
	existsAddrIndex := meta.Bucket(existsAddrIndexKey)
	newUsedAddrs := make(map[[addrKeySize]byte]struct{})
	for k := range usedAddrs {
		if !idx.existsAddress(existsAddrIndex, k) {
			newUsedAddrs[k] = struct{}{}
		}
	}

	for k := range newUsedAddrs {
		err := dbPutExistsAddr(existsAddrIndex, k)
		if err != nil {
			return err
		}
	}

	return nil
}

// DisconnectBlock is invoked by the index manager when a block has been
// disconnected from the main chain. Note that the exists address manager
// never removes addresses.
//
// This is part of the Indexer interface.
func (idx *ExistsAddrIndex) DisconnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, view *blockchain.UtxoViewpoint) error {
	return nil
}

// addUnconfirmedTx adds all addresses related to the transaction to the
// unconfirmed (memory-only) exists address index.
func (idx *ExistsAddrIndex) addUnconfirmedTx(tx *wire.MsgTx) {
	isSStx := stake.IsSStx(tx)
	for _, txIn := range tx.TxIn {
		if txscript.IsMultisigSigScript(txIn.SignatureScript) {
			rs, err :=
				txscript.MultisigRedeemScriptFromScriptSig(
					txIn.SignatureScript)
			if err != nil {
				continue
			}

			class, addrs, _, err := txscript.ExtractPkScriptAddrs(
				txscript.DefaultScriptVersion, rs, idx.chainParams)
			if err != nil {
				// Non-standard outputs are skipped.
				continue
			}
			if class != txscript.MultiSigTy {
				// This should never happen, but be paranoid.
				continue
			}

			for _, addr := range addrs {
				k, err := addrToKey(addr, idx.chainParams)
				if err != nil {
					continue
				}

				if _, exists := idx.mpExistsAddr[k]; !exists {
					idx.mpExistsAddr[k] = struct{}{}
				}
			}
		}
	}

	for _, txOut := range tx.TxOut {
		class, addrs, _, err := txscript.ExtractPkScriptAddrs(txOut.Version,
			txOut.PkScript, idx.chainParams)
		if err != nil {
			// Non-standard outputs are skipped.
			continue
		}

		if isSStx && class == txscript.NullDataTy {
			addr, err := stake.AddrFromSStxPkScrCommitment(txOut.PkScript,
				idx.chainParams)
			if err != nil {
				// Ignore unsupported address types.
				continue
			}

			addrs = append(addrs, addr)
		}

		for _, addr := range addrs {
			k, err := addrToKey(addr, idx.chainParams)
			if err != nil {
				// Ignore unsupported address types.
				continue
			}

			if _, exists := idx.mpExistsAddr[k]; !exists {
				idx.mpExistsAddr[k] = struct{}{}
			}
		}
	}
}

// AddUnconfirmedTx is the exported form of addUnconfirmedTx.
//
// This function is safe for concurrent access.
func (idx *ExistsAddrIndex) AddUnconfirmedTx(tx *wire.MsgTx) {
	idx.unconfirmedLock.Lock()
	defer idx.unconfirmedLock.Unlock()

	idx.addUnconfirmedTx(tx)
}

// DropExistsAddrIndex drops the exists address index from the provided
// database if it exists.
func DropExistsAddrIndex(db database.DB, interrupt <-chan struct{}) error {
	return dropIndex(db, existsAddrIndexKey, existsAddressIndexName,
		interrupt)
}
