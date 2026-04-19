package indexers

import (
	"encoding/hex"
	"sync"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/txscript"
)

const (
	scriptHashIndexName = "script hash index"
)

var (
	scriptHashIndexKey = []byte("scripthashindex")
)

func dbStoreScriptHashEntry(dbTx database.Tx, scriptHash [32]byte, addr string) error {
	idx := dbTx.Metadata().Bucket(scriptHashIndexKey)
	return idx.Put(scriptHash[:], []byte(addr))
}

func dbFetchScriptHashEntry(dbTx database.Tx, scriptHash [32]byte) string {
	idx := dbTx.Metadata().Bucket(scriptHashIndexKey)
	return string(idx.Get(scriptHash[:]))
}

// ScriptHashIndex maps the a sha256 hash of a sriptHash to the bitcoin address.
// Useful for an electrum server as the requests are not done with bitcoin
// addresses, rather with the script hashes.
type ScriptHashIndex struct {
	db          database.DB
	chainParams *chaincfg.Params

	// Map for checking if the script hash -> address mapping already
	// exists.  This happens as there are a lot of address reuse in bitcoin.
	exists map[chainhash.Hash]struct{}

	unconfirmedLock      sync.RWMutex
	addrByScriptHash     map[string]string
	scriptHashesByTxHash map[chainhash.Hash]map[string]struct{}
}

// Ensure the ScriptHashIndex type implements the Indexer interface.
var _ Indexer = (*ScriptHashIndex)(nil)

func (idx *ScriptHashIndex) Init() error {
	return nil // Nothing to do.
}

// Key returns the database key to use for the index as a byte slice. This is
// part of the Indexer interface.
func (idx *ScriptHashIndex) Key() []byte {
	return scriptHashIndexKey
}

// Name returns the human-readable name of the index. This is part of the
// Indexer interface.
func (idx *ScriptHashIndex) Name() string {
	return scriptHashIndexName
}

// Create is invoked when the indexer manager determines the index needs
// to be created for the first time.
//
// This is part of the Indexer interface.
func (idx *ScriptHashIndex) Create(dbTx database.Tx) error {
	meta := dbTx.Metadata()
	_, err := meta.CreateBucket(scriptHashIndexKey)
	return err
}

func (idx *ScriptHashIndex) ConnectBlock(dbTx database.Tx, block *btcutil.Block,
	stxos []blockchain.SpentTxOut) error {

	for _, tx := range block.Transactions() {
		for _, txOut := range tx.MsgTx().TxOut {
			scriptHash := chainhash.HashH(txOut.PkScript)

			_, found := idx.exists[scriptHash]
			if found {
				continue
			}
			addr := dbFetchScriptHashEntry(dbTx, scriptHash)
			if addr != "" {
				continue
			}

			_, addrs, _, err := txscript.ExtractPkScriptAddrs(
				txOut.PkScript, idx.chainParams)
			if err != nil || len(addrs) != 1 {
				// We skip when there are multiple addresses as
				// that means the pkscript is for a raw multisig
				// output.  Since the address manager isn't
				// keeping track of them anyways, we simply
				// skip.
				continue
			}

			err = dbStoreScriptHashEntry(dbTx, scriptHash, addrs[0].String())
			if err != nil {
				return err
			}

			// Keep it in the cache so that we don't revisit it in the future.
			idx.exists[scriptHash] = struct{}{}
		}
	}

	return nil
}

func (idx *ScriptHashIndex) DisconnectBlock(dbTx database.Tx, block *btcutil.Block,
	stxos []blockchain.SpentTxOut) error {
	// Do nothing as we may unmap a script hash -> address mapping that
	// exists in previous blocks. Because of address reuse, we must check
	// all previous blocks to ensure that an address can be unmapped. Since
	// this cost is expensive and the storage for a mapping is cheap, we
	// just do nothing.
	return nil
}

func (idx *ScriptHashIndex) AddrFromScriptHash(dbTx database.Tx, scriptHash chainhash.Hash) string {
	// Look for it in the map first.
	idx.unconfirmedLock.RLock()
	addr, found := idx.addrByScriptHash[scriptHash.String()]
	log.Infof("scriptHash %v\nmap %v", scriptHash.String(), idx.addrByScriptHash)
	idx.unconfirmedLock.RUnlock()
	if found {
		log.Infof("found addr %v for scripthash %v", addr, scriptHash.String())
		return addr
	}

	// Look for it in the database.
	return dbFetchScriptHashEntry(dbTx, scriptHash)
}

func (idx *ScriptHashIndex) indexindexUnconfirmedScriptHash(txHash *chainhash.Hash, pkScript []byte) {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript,
		idx.chainParams)
	if err != nil || len(addrs) != 1 {
		// We skip when there are multiple addresses as
		// that means the pkscript is for a raw multisig
		// output.  Since the address manager isn't
		// keeping track of them anyways, we simply
		// skip.
		return
	}

	scriptHash := chainhash.HashH(pkScript)

	log.Infof("mapping txhash %v with scriptHash %v\n%v",
		txHash.String(), hex.EncodeToString(scriptHash[:]),
		scriptHash)

	var addr string
	idx.db.View(func(dbTx database.Tx) error {
		addr = dbFetchScriptHashEntry(dbTx, scriptHash)
		return nil
	})
	if addr != "" {
		// No need to keep this in memory if we've already mapped it.
		// This happens because of address re-use.
		log.Infof("scriptHash %v already mapped with %v",
			hex.EncodeToString(scriptHash[:]), addr)
		return
	}

	idx.unconfirmedLock.Lock()

	// Add a mapping from the scriptHash to the address.
	idx.addrByScriptHash[scriptHash.String()] = addrs[0].String()

	// Add a mapping from the tx hash to the scriptHash.
	scriptHashesByTxHash := idx.scriptHashesByTxHash[*txHash]
	if scriptHashesByTxHash == nil {
		scriptHashesByTxHash = make(map[string]struct{})
		idx.scriptHashesByTxHash[*txHash] = scriptHashesByTxHash
	}
	scriptHashesByTxHash[scriptHash.String()] = struct{}{}

	idx.unconfirmedLock.Unlock()
}

func (idx *ScriptHashIndex) MapUnconfirmedTx(tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint) {
	log.Infof("MapUnconfirmedTx called with %v", tx.Hash())
	// Index addresses of all referenced previous transaction outputs.
	//
	// The existence checks are elided since this is only called after the
	// transaction has already been validated and thus all inputs are
	// already known to exist.
	for _, txIn := range tx.MsgTx().TxIn {
		entry := utxoView.LookupEntry(txIn.PreviousOutPoint)
		if entry == nil {
			// Ignore missing entries.  This should never happen
			// in practice since the function comments specifically
			// call out all inputs must be available.
			continue
		}

		idx.indexindexUnconfirmedScriptHash(tx.Hash(), entry.PkScript())
	}

	// Index addresses of all created outputs.
	for _, txOut := range tx.MsgTx().TxOut {
		idx.indexindexUnconfirmedScriptHash(tx.Hash(), txOut.PkScript)
	}
}

func (idx *ScriptHashIndex) RemoveUnconfirmedTxEntry(hash *chainhash.Hash) {
	idx.unconfirmedLock.Lock()
	defer idx.unconfirmedLock.Unlock()

	log.Infof("RemoveUnconfirmedTxEntry called with %v", hash.String())
	scriptHashes := idx.scriptHashesByTxHash[*hash]
	for scriptHash := range scriptHashes {
		delete(idx.addrByScriptHash, scriptHash)
	}
	delete(idx.scriptHashesByTxHash, *hash)
}

func NewScriptHashIndex(db database.DB, chainParams *chaincfg.Params) *ScriptHashIndex {
	return &ScriptHashIndex{
		db:                   db,
		chainParams:          chainParams,
		exists:               make(map[chainhash.Hash]struct{}),
		addrByScriptHash:     make(map[string]string),
		scriptHashesByTxHash: make(map[chainhash.Hash]map[string]struct{}),
	}
}

func DropScriptHashIndex(db database.DB, interrupt <-chan struct{}) error {
	return dropIndex(db, scriptHashIndexKey, scriptHashIndexName, interrupt)
}
