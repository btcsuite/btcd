package mempool

import (
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/mock"
)

// MockTxMempool is a mock implementation of the TxMempool interface.
type MockTxMempool struct {
	mock.Mock
}

// Ensure the MockTxMempool implements the TxMemPool interface.
var _ TxMempool = (*MockTxMempool)(nil)

// LastUpdated returns the last time a transaction was added to or removed from
// the source pool.
func (m *MockTxMempool) LastUpdated() time.Time {
	args := m.Called()
	return args.Get(0).(time.Time)
}

// TxDescs returns a slice of descriptors for all the transactions in the pool.
func (m *MockTxMempool) TxDescs() []*TxDesc {
	args := m.Called()
	return args.Get(0).([]*TxDesc)
}

// RawMempoolVerbose returns all the entries in the mempool as a fully
// populated btcjson result.
func (m *MockTxMempool) RawMempoolVerbose() map[string]*btcjson.
	GetRawMempoolVerboseResult {

	args := m.Called()
	return args.Get(0).(map[string]*btcjson.GetRawMempoolVerboseResult)
}

// Count returns the number of transactions in the main pool. It does not
// include the orphan pool.
func (m *MockTxMempool) Count() int {
	args := m.Called()
	return args.Get(0).(int)
}

// FetchTransaction returns the requested transaction from the transaction
// pool. This only fetches from the main transaction pool and does not include
// orphans.
func (m *MockTxMempool) FetchTransaction(
	txHash *chainhash.Hash) (*btcutil.Tx, error) {

	args := m.Called(txHash)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*btcutil.Tx), args.Error(1)
}

// HaveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
func (m *MockTxMempool) HaveTransaction(hash *chainhash.Hash) bool {
	args := m.Called(hash)
	return args.Get(0).(bool)
}

// ProcessTransaction is the main workhorse for handling insertion of new
// free-standing transactions into the memory pool. It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
func (m *MockTxMempool) ProcessTransaction(tx *btcutil.Tx, allowOrphan,
	rateLimit bool, tag Tag) ([]*TxDesc, error) {

	args := m.Called(tx, allowOrphan, rateLimit, tag)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]*TxDesc), args.Error(1)
}

// RemoveTransaction removes the passed transaction from the mempool.  When the
// removeRedeemers flag is set, any transactions that redeem outputs from the
// removed transaction will also be removed recursively from the mempool, as
// they would otherwise become orphans.
func (m *MockTxMempool) RemoveTransaction(tx *btcutil.Tx,
	removeRedeemers bool) {

	m.Called(tx, removeRedeemers)
}

// CheckMempoolAcceptance behaves similarly to bitcoind's `testmempoolaccept`
// RPC method. It will perform a series of checks to decide whether this
// transaction can be accepted to the mempool. If not, the specific error is
// returned and the caller needs to take actions based on it.
func (m *MockTxMempool) CheckMempoolAcceptance(
	tx *btcutil.Tx) (*MempoolAcceptResult, error) {

	args := m.Called(tx)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*MempoolAcceptResult), args.Error(1)
}

// CheckSpend checks whether the passed outpoint is already spent by a
// transaction in the mempool. If that's the case the spending transaction will
// be returned, if not nil will be returned.
func (m *MockTxMempool) CheckSpend(op wire.OutPoint) *btcutil.Tx {
	args := m.Called(op)

	if args.Get(0) == nil {
		return nil
	}

	return args.Get(0).(*btcutil.Tx)
}
