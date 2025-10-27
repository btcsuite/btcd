package mempool

import (
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/wire"
)

// TxMempool defines an interface that's used by other subsystems to interact
// with the mempool.
type TxMempool interface {
	// LastUpdated returns the last time a transaction was added to or
	// removed from the source pool.
	LastUpdated() time.Time

	// TxDescs returns a slice of descriptors for all the transactions in
	// the pool.
	TxDescs() []*TxDesc

	// RawMempoolVerbose returns all the entries in the mempool as a fully
	// populated btcjson result.
	RawMempoolVerbose() map[string]*btcjson.GetRawMempoolVerboseResult

	// Count returns the number of transactions in the main pool. It does
	// not include the orphan pool.
	Count() int

	// FetchTransaction returns the requested transaction from the
	// transaction pool. This only fetches from the main transaction pool
	// and does not include orphans.
	FetchTransaction(txHash *chainhash.Hash) (*btcutil.Tx, error)

	// HaveTransaction returns whether or not the passed transaction
	// already exists in the main pool or in the orphan pool.
	HaveTransaction(hash *chainhash.Hash) bool

	// ProcessTransaction is the main workhorse for handling insertion of
	// new free-standing transactions into the memory pool. It includes
	// functionality such as rejecting duplicate transactions, ensuring
	// transactions follow all rules, orphan transaction handling, and
	// insertion into the memory pool.
	//
	// It returns a slice of transactions added to the mempool. When the
	// error is nil, the list will include the passed transaction itself
	// along with any additional orphan transactions that were added as a
	// result of the passed one being accepted.
	ProcessTransaction(tx *btcutil.Tx, allowOrphan,
		rateLimit bool, tag Tag) ([]*TxDesc, error)

	// RemoveTransaction removes the passed transaction from the mempool.
	// When the removeRedeemers flag is set, any transactions that redeem
	// outputs from the removed transaction will also be removed
	// recursively from the mempool, as they would otherwise become
	// orphans.
	RemoveTransaction(tx *btcutil.Tx, removeRedeemers bool)

	// CheckMempoolAcceptance behaves similarly to bitcoind's
	// `testmempoolaccept` RPC method. It will perform a series of checks
	// to decide whether this transaction can be accepted to the mempool.
	// If not, the specific error is returned and the caller needs to take
	// actions based on it.
	CheckMempoolAcceptance(tx *btcutil.Tx) (*MempoolAcceptResult, error)

	// CheckSpend checks whether the passed outpoint is already spent by
	// a transaction in the mempool. If that's the case the spending
	// transaction will be returned, if not nil will be returned.
	CheckSpend(op wire.OutPoint) *btcutil.Tx

	// RemoveOrphansByTag removes all orphan transactions tagged with the
	// provided identifier. Returns the number of orphans removed.
	RemoveOrphansByTag(tag Tag) uint64

	// MiningDescs returns a slice of mining descriptors for all the
	// transactions in the source pool.
	MiningDescs() []*mining.TxDesc

	// RemoveDoubleSpends removes all transactions that spend outputs spent
	// by the passed transaction from the mempool.
	RemoveDoubleSpends(tx *btcutil.Tx)

	// RemoveOrphan removes the passed orphan transaction from the orphan
	// pool.
	RemoveOrphan(tx *btcutil.Tx)

	// ProcessOrphans processes orphan transactions that now have a valid
	// ancestor after the provided transaction was accepted. Returns a slice
	// of transaction descriptors for any orphans that were accepted.
	ProcessOrphans(acceptedTx *btcutil.Tx) []*TxDesc

	// MaybeAcceptTransaction validates and potentially accepts a
	// transaction to the memory pool. It returns a slice of hashes for all
	// transactions that were accepted, the transaction descriptor for the
	// primary transaction (if accepted), and an error if the transaction
	// was rejected. The isNew parameter indicates whether this is a new
	// transaction or one being added from a reorganization. The rateLimit
	// parameter indicates whether to apply rate limiting for relay.
	MaybeAcceptTransaction(tx *btcutil.Tx, isNew, rateLimit bool) ([]*chainhash.Hash, *TxDesc, error)
}
