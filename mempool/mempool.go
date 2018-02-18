// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"container/list"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/indexers"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/mining"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

const (
	// DefaultBlockPrioritySize is the default size in bytes for high-
	// priority / low-fee transactions.  It is used to help determine which
	// are allowed into the mempool and consequently affects their relay and
	// inclusion when generating block templates.
	DefaultBlockPrioritySize = 20000

	// MinHighPriority is the minimum priority value that allows a
	// transaction to be considered high priority.
	MinHighPriority = dcrutil.AtomsPerCoin * 144.0 / 250

	// mempoolHeight is the height used for the "block" height field of the
	// contextual transaction information provided in a transaction view.
	mempoolHeight = 0x7fffffff

	// maxRelayFeeMultiplier is the factor that we disallow fees / kB above the
	// minimum tx fee.  At the current default minimum relay fee of 0.001
	// DCR/kB, this results in a maximum allowed high fee of 1 DCR/kB.
	maxRelayFeeMultiplier = 1000

	// maxSSGensDoubleSpends is the maximum number of SSGen double spends
	// allowed in the pool.
	maxSSGensDoubleSpends = 5

	// heightDiffToPruneTicket is the number of blocks to pass by in terms
	// of height before old tickets are pruned.
	// TODO Set this based up the stake difficulty retargeting interval?
	heightDiffToPruneTicket = 288

	// heightDiffToPruneVotes is the number of blocks to pass by in terms
	// of height before SSGen relating to that block are pruned.
	heightDiffToPruneVotes = 10

	// If a vote is on a block whose height is before tip minus this
	// amount, reject it from being added to the mempool.
	maximumVoteAgeDelta = 1440

	// maxNullDataOutputs is the maximum number of OP_RETURN null data
	// pushes in a transaction, after which it is considered non-standard.
	maxNullDataOutputs = 4
)

// VoteTx is a struct describing a block vote (SSGen).
type VoteTx struct {
	SsgenHash chainhash.Hash // Vote
	SstxHash  chainhash.Hash // Ticket
	Vote      bool
}

// Config is a descriptor containing the memory pool configuration.
type Config struct {
	// Policy defines the various mempool configuration options related
	// to policy.
	Policy Policy

	// ChainParams identifies which chain parameters the txpool is
	// associated with.
	ChainParams *chaincfg.Params

	// NextStakeDifficulty defines the function to retrieve the stake
	// difficulty for the block after the current best block.
	//
	// This function must be safe for concurrent access.
	NextStakeDifficulty func() (int64, error)

	// FetchUtxoView defines the function to use to fetch unspent
	// transaction output information.
	FetchUtxoView func(*dcrutil.Tx, bool) (*blockchain.UtxoViewpoint, error)

	// BlockByHash defines the function use to fetch the block identified
	// by the given hash.
	BlockByHash func(*chainhash.Hash) (*dcrutil.Block, error)

	// BestHash defines the function to use to access the block hash of
	// the current best chain.
	BestHash func() *chainhash.Hash

	// BestHeight defines the function to use to access the block height of
	// the current best chain.
	BestHeight func() int64

	// PastMedianTime defines the function to use in order to access the
	// median time calculated from the point-of-view of the current chain
	// tip within the best chain.
	PastMedianTime func() time.Time

	// CalcSequenceLock defines the function to use in order to generate
	// the current sequence lock for the given transaction using the passed
	// utxo view.
	CalcSequenceLock func(*dcrutil.Tx, *blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error)

	// SubsidyCache defines a subsidy cache to use.
	SubsidyCache *blockchain.SubsidyCache

	// SigCache defines a signature cache to use.
	SigCache *txscript.SigCache

	// AddrIndex defines the optional address index instance to use for
	// indexing the unconfirmed transactions in the memory pool.
	// This can be nil if the address index is not enabled.
	AddrIndex *indexers.AddrIndex

	// ExistsAddrIndex defines the optional exists address index instance
	// to use for indexing the unconfirmed transactions in the memory pool.
	// This can be nil if the address index is not enabled.
	ExistsAddrIndex *indexers.ExistsAddrIndex
}

// Policy houses the policy (configuration parameters) which is used to
// control the mempool.
type Policy struct {
	// MaxTxVersion is the max transaction version that the mempool should
	// accept.  All transactions above this version are rejected as
	// non-standard.
	MaxTxVersion uint16

	// DisableRelayPriority defines whether to relay free or low-fee
	// transactions that do not have enough priority to be relayed.
	DisableRelayPriority bool

	// AcceptNonStd defines whether to accept and relay non-standard
	// transactions to the network. If true, non-standard transactions
	// will be accepted into the mempool and relayed to the rest of the
	// network. Otherwise, all non-standard transactions will be rejected.
	AcceptNonStd bool

	// FreeTxRelayLimit defines the given amount in thousands of bytes
	// per minute that transactions with no fee are rate limited to.
	FreeTxRelayLimit float64

	// MaxOrphanTxs is the maximum number of orphan transactions
	// that can be queued.
	MaxOrphanTxs int

	// MaxOrphanTxSize is the maximum size allowed for orphan transactions.
	// This helps prevent memory exhaustion attacks from sending a lot of
	// of big orphans.
	MaxOrphanTxSize int

	// MaxSigOpsPerTx is the maximum number of signature operations
	// in a single transaction we will relay or mine.  It is a fraction
	// of the max signature operations for a block.
	MaxSigOpsPerTx int

	// MinRelayTxFee defines the minimum transaction fee in BTC/kB to be
	// considered a non-zero fee.
	MinRelayTxFee dcrutil.Amount

	// AllowOldVotes defines whether or not votes on old blocks will be
	// admitted and relayed.
	AllowOldVotes bool

	// StandardVerifyFlags defines the function to retrieve the flags to
	// use for verifying scripts for the block after the current best block.
	// It must set the verification flags properly depending on the result
	// of any agendas that affect them.
	//
	// This function must be safe for concurrent access.
	StandardVerifyFlags func() (txscript.ScriptFlags, error)
}

// TxDesc is a descriptor containing a transaction in the mempool along with
// additional metadata.
type TxDesc struct {
	mining.TxDesc

	// StartingPriority is the priority of the transaction when it was added
	// to the pool.
	StartingPriority float64
}

// TxPool is used as a source of transactions that need to be mined into blocks
// and relayed to other peers.  It is safe for concurrent access from multiple
// peers.
type TxPool struct {
	// The following variables must only be used atomically.
	lastUpdated int64 // last time pool was updated.

	mtx           sync.RWMutex
	cfg           Config
	pool          map[chainhash.Hash]*TxDesc
	orphans       map[chainhash.Hash]*dcrutil.Tx
	orphansByPrev map[chainhash.Hash]map[chainhash.Hash]*dcrutil.Tx
	outpoints     map[wire.OutPoint]*dcrutil.Tx

	// Votes on blocks.
	votesMtx sync.RWMutex
	votes    map[chainhash.Hash][]VoteTx

	pennyTotal    float64 // exponentially decaying total for penny spends.
	lastPennyUnix int64   // unix time of last ``penny spend''
}

// insertVote inserts a vote into the map of block votes.
//
// This function MUST be called with the vote mutex locked (for writes).
func (mp *TxPool) insertVote(ssgen *dcrutil.Tx) error {
	msgTx := ssgen.MsgTx()
	ticketHash := &msgTx.TxIn[1].PreviousOutPoint.Hash

	// Get the block it is voting on; here we're agnostic of height.
	blockHash, blockHeight := stake.SSGenBlockVotedOn(msgTx)

	// If there are currently no votes for this block,
	// start a new buffered slice and store it.
	vts, exists := mp.votes[blockHash]
	if !exists {
		vts = make([]VoteTx, 0, mp.cfg.ChainParams.TicketsPerBlock)
	}

	// Nothing to do if a vote for the ticket is already known.
	for _, vt := range vts {
		if vt.SstxHash.IsEqual(ticketHash) {
			return nil
		}
	}

	voteHash := ssgen.Hash()
	voteBits := stake.SSGenVoteBits(msgTx)
	vote := dcrutil.IsFlagSet16(voteBits, dcrutil.BlockValid)
	voteTx := VoteTx{
		SsgenHash: *voteHash,
		SstxHash:  *ticketHash,
		Vote:      vote,
	}

	// Append the new vote.
	mp.votes[blockHash] = append(vts, voteTx)

	log.Debugf("Accepted vote %v for block hash %v (height %v), voting "+
		"%v on the transaction tree", voteHash, blockHash, blockHeight,
		vote)

	return nil
}

// VoteHashesForBlock returns the hashes for all votes on the provided block
// hash that are currently available in the mempool.
//
// This function is safe for concurrent access.
func (mp *TxPool) VoteHashesForBlock(blockHash chainhash.Hash) []chainhash.Hash {
	mp.votesMtx.RLock()
	vts, exists := mp.votes[blockHash]
	mp.votesMtx.RUnlock()

	// Lookup the vote metadata for the block.
	if !exists || len(vts) == 0 {
		return nil
	}

	// Copy the vote hashes from the vote metadata.
	hashes := make([]chainhash.Hash, 0, len(vts))
	for _, vt := range vts {
		hashes = append(hashes, vt.SsgenHash)
	}

	return hashes
}

// VotesForBlocks returns the vote metadata for all votes on the provided
// block hashes that are currently available in the mempool.
//
// This function is safe for concurrent access.
func (mp *TxPool) VotesForBlocks(hashes []chainhash.Hash) [][]VoteTx {
	result := make([][]VoteTx, 0, len(hashes))

	mp.votesMtx.RLock()
	for _, hash := range hashes {
		votes := mp.votes[hash]
		result = append(result, votes)
	}
	mp.votesMtx.RUnlock()

	return result
}

// TODO Pruning of the votes map DECRED

// Ensure the TxPool type implements the mining.TxSource interface.
var _ mining.TxSource = (*TxPool)(nil)

// removeOrphan is the internal function which implements the public
// RemoveOrphan.  See the comment for RemoveOrphan for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) removeOrphan(txHash *chainhash.Hash) {
	log.Tracef("Removing orphan transaction %v", txHash)

	// Nothing to do if passed tx is not an orphan.
	tx, exists := mp.orphans[*txHash]
	if !exists {
		return
	}

	// Remove the reference from the previous orphan index.
	for _, txIn := range tx.MsgTx().TxIn {
		originTxHash := txIn.PreviousOutPoint.Hash
		if orphans, exists := mp.orphansByPrev[originTxHash]; exists {
			delete(orphans, *tx.Hash())

			// Remove the map entry altogether if there are no
			// longer any orphans which depend on it.
			if len(orphans) == 0 {
				delete(mp.orphansByPrev, originTxHash)
			}
		}
	}

	// Remove the transaction from the orphan pool.
	delete(mp.orphans, *txHash)
}

// RemoveOrphan removes the passed orphan transaction from the orphan pool and
// previous orphan index.
//
// This function is safe for concurrent access.
func (mp *TxPool) RemoveOrphan(txHash *chainhash.Hash) {
	mp.mtx.Lock()
	mp.removeOrphan(txHash)
	mp.mtx.Unlock()
}

// limitNumOrphans limits the number of orphan transactions by evicting a random
// orphan if adding a new one would cause it to overflow the max allowed.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) limitNumOrphans() error {
	if len(mp.orphans)+1 > mp.cfg.Policy.MaxOrphanTxs &&
		mp.cfg.Policy.MaxOrphanTxs > 0 {

		// Generate a cryptographically random hash.
		randHashBytes := make([]byte, chainhash.HashSize)
		_, err := rand.Read(randHashBytes)
		if err != nil {
			return err
		}
		randHashNum := new(big.Int).SetBytes(randHashBytes)

		// Try to find the first entry that is greater than the random
		// hash.  Use the first entry (which is already pseudorandom due
		// to Go's range statement over maps) as a fallback if none of
		// the hashes in the orphan pool are larger than the random
		// hash.
		var foundHash *chainhash.Hash
		for txHash := range mp.orphans {
			if foundHash == nil {
				foundHash = &txHash
			}
			txHashNum := blockchain.HashToBig(&txHash)
			if txHashNum.Cmp(randHashNum) > 0 {
				foundHash = &txHash
				break
			}
		}

		mp.removeOrphan(foundHash)
	}

	return nil
}

// addOrphan adds an orphan transaction to the orphan pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) addOrphan(tx *dcrutil.Tx) {
	// Limit the number orphan transactions to prevent memory exhaustion.  A
	// random orphan is evicted to make room if needed.
	mp.limitNumOrphans()

	mp.orphans[*tx.Hash()] = tx
	for _, txIn := range tx.MsgTx().TxIn {
		originTxHash := txIn.PreviousOutPoint.Hash
		if _, exists := mp.orphansByPrev[originTxHash]; !exists {
			mp.orphansByPrev[originTxHash] =
				make(map[chainhash.Hash]*dcrutil.Tx)
		}
		mp.orphansByPrev[originTxHash][*tx.Hash()] = tx
	}

	log.Debugf("Stored orphan transaction %v (total: %d)", tx.Hash(),
		len(mp.orphans))
}

// maybeAddOrphan potentially adds an orphan to the orphan pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) maybeAddOrphan(tx *dcrutil.Tx) error {
	// Ignore orphan transactions that are too large.  This helps avoid
	// a memory exhaustion attack based on sending a lot of really large
	// orphans.  In the case there is a valid transaction larger than this,
	// it will ultimtely be rebroadcast after the parent transactions
	// have been mined or otherwise received.
	//
	// Note that the number of orphan transactions in the orphan pool is
	// also limited, so this equates to a maximum memory used of
	// mp.cfg.Policy.MaxOrphanTxSize * mp.cfg.Policy.MaxOrphanTxs (which is ~5MB
	// using the default values at the time this comment was written).
	serializedLen := tx.MsgTx().SerializeSize()
	if serializedLen > mp.cfg.Policy.MaxOrphanTxSize {
		str := fmt.Sprintf("orphan transaction size of %d bytes is "+
			"larger than max allowed size of %d bytes",
			serializedLen, mp.cfg.Policy.MaxOrphanTxSize)
		return txRuleError(wire.RejectNonstandard, str)
	}

	// Add the orphan if the none of the above disqualified it.
	mp.addOrphan(tx)

	return nil
}

// isTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) isTransactionInPool(hash *chainhash.Hash) bool {
	if _, exists := mp.pool[*hash]; exists {
		return true
	}

	return false
}

// IsTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) IsTransactionInPool(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.mtx.RLock()
	inPool := mp.isTransactionInPool(hash)
	mp.mtx.RUnlock()

	return inPool
}

// isOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) isOrphanInPool(hash *chainhash.Hash) bool {
	if _, exists := mp.orphans[*hash]; exists {
		return true
	}

	return false
}

// IsOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) IsOrphanInPool(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.mtx.RLock()
	inPool := mp.isOrphanInPool(hash)
	mp.mtx.RUnlock()

	return inPool
}

// haveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) haveTransaction(hash *chainhash.Hash) bool {
	return mp.isTransactionInPool(hash) || mp.isOrphanInPool(hash)
}

// HaveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) HaveTransaction(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.mtx.RLock()
	haveTx := mp.haveTransaction(hash)
	mp.mtx.RUnlock()

	return haveTx
}

// haveTransactions returns whether or not the passed transactions already exist
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) haveTransactions(hashes []*chainhash.Hash) []bool {
	have := make([]bool, len(hashes))
	for i := range hashes {
		have[i] = mp.haveTransaction(hashes[i])
	}
	return have
}

// HaveTransactions returns whether or not the passed transactions already exist
// in the main pool or in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) HaveTransactions(hashes []*chainhash.Hash) []bool {
	// Protect concurrent access.
	mp.mtx.RLock()
	haveTxns := mp.haveTransactions(hashes)
	mp.mtx.RUnlock()
	return haveTxns
}

// removeTransaction is the internal function which implements the public
// RemoveTransaction.  See the comment for RemoveTransaction for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) removeTransaction(tx *dcrutil.Tx, removeRedeemers bool) {
	log.Tracef("Removing transaction %v", tx.Hash())

	msgTx := tx.MsgTx()
	txHash := tx.Hash()
	var txType stake.TxType
	if removeRedeemers {
		// Remove any transactions which rely on this one.
		txType = stake.DetermineTxType(msgTx)
		tree := wire.TxTreeRegular
		if txType != stake.TxTypeRegular {
			tree = wire.TxTreeStake
		}
		for i := uint32(0); i < uint32(len(msgTx.TxOut)); i++ {
			outpoint := wire.NewOutPoint(txHash, i, tree)
			if txRedeemer, exists := mp.outpoints[*outpoint]; exists {
				mp.removeTransaction(txRedeemer, true)
			}
		}
	}

	// Remove the transaction if needed.
	if txDesc, exists := mp.pool[*txHash]; exists {
		// Remove unconfirmed address index entries associated with the
		// transaction if enabled.
		if mp.cfg.AddrIndex != nil {
			mp.cfg.AddrIndex.RemoveUnconfirmedTx(txHash)
		}

		// Mark the referenced outpoints as unspent by the pool.

		for _, txIn := range txDesc.Tx.MsgTx().TxIn {
			delete(mp.outpoints, txIn.PreviousOutPoint)
		}
		delete(mp.pool, *txHash)
		atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())
	}
}

// RemoveTransaction removes the passed transaction from the mempool. When the
// removeRedeemers flag is set, any transactions that redeem outputs from the
// removed transaction will also be removed recursively from the mempool, as
// they would otherwise become orphans.
//
// This function is safe for concurrent access.
func (mp *TxPool) RemoveTransaction(tx *dcrutil.Tx, removeRedeemers bool) {
	// Protect concurrent access.
	mp.mtx.Lock()
	mp.removeTransaction(tx, removeRedeemers)
	mp.mtx.Unlock()
}

// RemoveDoubleSpends removes all transactions which spend outputs spent by the
// passed transaction from the memory pool.  Removing those transactions then
// leads to removing all transactions which rely on them, recursively.  This is
// necessary when a block is connected to the main chain because the block may
// contain transactions which were previously unknown to the memory pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) RemoveDoubleSpends(tx *dcrutil.Tx) {
	// Protect concurrent access.
	mp.mtx.Lock()
	for _, txIn := range tx.MsgTx().TxIn {
		if txRedeemer, ok := mp.outpoints[txIn.PreviousOutPoint]; ok {
			if !txRedeemer.Hash().IsEqual(tx.Hash()) {
				mp.removeTransaction(txRedeemer, true)
			}
		}
	}
	mp.mtx.Unlock()
}

// addTransaction adds the passed transaction to the memory pool.  It should
// not be called directly as it doesn't perform any validation.  This is a
// helper for maybeAcceptTransaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) addTransaction(utxoView *blockchain.UtxoViewpoint,
	tx *dcrutil.Tx, txType stake.TxType, height int64, fee int64) {

	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.
	msgTx := tx.MsgTx()
	mp.pool[*tx.Hash()] = &TxDesc{
		TxDesc: mining.TxDesc{
			Tx:     tx,
			Type:   txType,
			Added:  time.Now(),
			Height: height,
			Fee:    fee,
		},
		StartingPriority: CalcPriority(msgTx, utxoView, height),
	}
	for _, txIn := range msgTx.TxIn {
		mp.outpoints[txIn.PreviousOutPoint] = tx
	}
	atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())

	// Add unconfirmed address index entries associated with the transaction
	// if enabled.
	if mp.cfg.AddrIndex != nil {
		mp.cfg.AddrIndex.AddUnconfirmedTx(tx, utxoView)
	}
	if mp.cfg.ExistsAddrIndex != nil {
		mp.cfg.ExistsAddrIndex.AddUnconfirmedTx(msgTx)
	}
}

// checkPoolDoubleSpend checks whether or not the passed transaction is
// attempting to spend coins already spent by other transactions in the pool.
// Note it does not check for double spends against transactions already in the
// main chain.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) checkPoolDoubleSpend(tx *dcrutil.Tx, txType stake.TxType) error {
	for i, txIn := range tx.MsgTx().TxIn {
		// We don't care about double spends of stake bases.
		if (txType == stake.TxTypeSSGen || txType == stake.TxTypeSSRtx) &&
			(i == 0) {
			continue
		}

		if txR, exists := mp.outpoints[txIn.PreviousOutPoint]; exists {
			str := fmt.Sprintf("transaction %v in the pool "+
				"already spends the same coins", txR.Hash())
			return txRuleError(wire.RejectDuplicate, str)
		}
	}

	return nil
}

// IsTxTreeValid checks the map of votes for a block to see if the tx
// tree regular for the block at HEAD is valid.
//
// The function is safe for concurrent access.
func (mp *TxPool) IsTxTreeValid(best *chainhash.Hash) bool {
	mp.votesMtx.RLock()
	vts := mp.votes[*best]
	mp.votesMtx.RUnlock()

	if len(vts) == 0 {
		return true
	}

	// There are not possibly enough votes to tell if the txTree is valid;
	// assume it's valid.
	if len(vts) <= int(mp.cfg.ChainParams.TicketsPerBlock/2) {
		return true
	}

	// Otherwise, tally the votes and determine if it's valid or not.
	yea := 0
	nay := 0
	for _, vote := range vts {
		if vote.Vote {
			yea++
		} else {
			nay++
		}
	}

	return yea > nay
}

// fetchInputUtxos loads utxo details about the input transactions referenced by
// the passed transaction.  First, it loads the details form the viewpoint of
// the main chain, then it adjusts them based upon the contents of the
// transaction pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) fetchInputUtxos(tx *dcrutil.Tx) (*blockchain.UtxoViewpoint, error) {
	tv := mp.IsTxTreeValid(mp.cfg.BestHash())
	utxoView, err := mp.cfg.FetchUtxoView(tx, tv)
	if err != nil {
		return nil, err
	}

	// Attempt to populate any missing inputs from the transaction pool.
	for originHash, entry := range utxoView.Entries() {
		if entry != nil && !entry.IsFullySpent() {
			continue
		}

		if poolTxDesc, exists := mp.pool[originHash]; exists {
			utxoView.AddTxOuts(poolTxDesc.Tx, mempoolHeight,
				wire.NullBlockIndex)
		}
	}

	return utxoView, nil
}

// FetchTransaction returns the requested transaction from the transaction pool.
// This only fetches from the main transaction pool and does not include
// orphans.
//
// This function is safe for concurrent access.
func (mp *TxPool) FetchTransaction(txHash *chainhash.Hash, includeRecentBlock bool) (*dcrutil.Tx, error) {
	// Protect concurrent access.
	mp.mtx.RLock()
	txDesc, exists := mp.pool[*txHash]
	mp.mtx.RUnlock()

	if exists {
		return txDesc.Tx, nil
	}

	// For Decred, the latest block is considered "unconfirmed"
	// for the regular transaction tree. Search that if the
	// user indicates too, as well.
	if includeRecentBlock {
		bl, err := mp.cfg.BlockByHash(mp.cfg.BestHash())
		if err != nil {
			return nil, err
		}

		for _, tx := range bl.Transactions() {
			if tx.Hash().IsEqual(txHash) {
				return tx, nil
			}
		}
	}

	return nil, fmt.Errorf("transaction is not in the pool")
}

// maybeAcceptTransaction is the internal function which implements the public
// MaybeAcceptTransaction.  See the comment for MaybeAcceptTransaction for
// more details.
//
// This function MUST be called with the mempool lock held (for writes).
// DECRED - TODO
// We need to make sure thing also assigns the TxType after it evaluates the tx,
// so that we can easily pick different stake tx types from the mempool later.
// This should probably be done at the bottom using "IsSStx" etc functions.
// It should also set the dcrutil tree type for the tx as well.
func (mp *TxPool) maybeAcceptTransaction(tx *dcrutil.Tx, isNew, rateLimit, allowHighFees bool) ([]*chainhash.Hash, error) {
	msgTx := tx.MsgTx()
	txHash := tx.Hash()
	// Don't accept the transaction if it already exists in the pool.  This
	// applies to orphan transactions as well.  This check is intended to
	// be a quick check to weed out duplicates.
	if mp.haveTransaction(txHash) {
		str := fmt.Sprintf("already have transaction %v", txHash)
		return nil, txRuleError(wire.RejectDuplicate, str)
	}

	// Perform preliminary sanity checks on the transaction.  This makes
	// use of chain which contains the invariant rules for what
	// transactions are allowed into blocks.
	err := blockchain.CheckTransactionSanity(msgTx, mp.cfg.ChainParams)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// A standalone transaction must not be a coinbase transaction.
	if blockchain.IsCoinBase(tx) {
		str := fmt.Sprintf("transaction %v is an individual coinbase",
			txHash)
		return nil, txRuleError(wire.RejectInvalid, str)
	}

	// Don't accept transactions with a lock time after the maximum int32
	// value for now.  This is an artifact of older bitcoind clients which
	// treated this field as an int32 and would treat anything larger
	// incorrectly (as negative).
	if msgTx.LockTime > math.MaxInt32 {
		str := fmt.Sprintf("transaction %v has a lock time after "+
			"2038 which is not accepted yet", txHash)
		return nil, txRuleError(wire.RejectNonstandard, str)
	}

	// Get the current height of the main chain.  A standalone transaction
	// will be mined into the next block at best, so its height is at least
	// one more than the current height.
	bestHeight := mp.cfg.BestHeight()
	nextBlockHeight := bestHeight + 1

	// Determine what type of transaction we're dealing with (regular or stake).
	// Then, be sure to set the tx tree correctly as it's possible a use submitted
	// it to the network with TxTreeUnknown.
	txType := stake.DetermineTxType(msgTx)
	if txType == stake.TxTypeRegular {
		tx.SetTree(wire.TxTreeRegular)
	} else {
		tx.SetTree(wire.TxTreeStake)
	}

	// Don't allow non-standard transactions if the mempool config forbids
	// their acceptance and relaying.
	medianTime := mp.cfg.PastMedianTime()
	if !mp.cfg.Policy.AcceptNonStd {
		err := checkTransactionStandard(tx, txType, nextBlockHeight,
			medianTime, mp.cfg.Policy.MinRelayTxFee,
			mp.cfg.Policy.MaxTxVersion)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = wire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction %v is not standard: %v",
				txHash, err)
			return nil, txRuleError(rejectCode, str)
		}
	}

	// If the transaction is a ticket, ensure that it meets the next
	// stake difficulty.
	if txType == stake.TxTypeSStx {
		sDiff, err := mp.cfg.NextStakeDifficulty()
		if err != nil {
			// This is an unexpected error so don't turn it into a
			// rule error.
			return nil, err
		}

		if msgTx.TxOut[0].Value < sDiff {
			str := fmt.Sprintf("transaction %v has not enough funds "+
				"to meet stake difficulty (ticket diff %v < next diff %v)",
				txHash, msgTx.TxOut[0].Value, sDiff)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	// Handle stake transaction double spending exceptions.
	if (txType == stake.TxTypeSSGen) || (txType == stake.TxTypeSSRtx) {
		if txType == stake.TxTypeSSGen {
			ssGenAlreadyFound := 0
			for _, mpTx := range mp.pool {
				if mpTx.Type == stake.TxTypeSSGen {
					if mpTx.Tx.MsgTx().TxIn[1].PreviousOutPoint ==
						msgTx.TxIn[1].PreviousOutPoint {
						ssGenAlreadyFound++
					}
				}
				if ssGenAlreadyFound > maxSSGensDoubleSpends {
					str := fmt.Sprintf("transaction %v in the pool "+
						"with more than %v ssgens",
						msgTx.TxIn[1].PreviousOutPoint,
						maxSSGensDoubleSpends)
					return nil, txRuleError(wire.RejectDuplicate, str)
				}
			}
		}

		if txType == stake.TxTypeSSRtx {
			for _, mpTx := range mp.pool {
				if mpTx.Type == stake.TxTypeSSRtx {
					if mpTx.Tx.MsgTx().TxIn[0].PreviousOutPoint ==
						msgTx.TxIn[0].PreviousOutPoint {
						str := fmt.Sprintf("transaction %v in the pool "+
							" as a ssrtx. Only one ssrtx allowed.",
							msgTx.TxIn[0].PreviousOutPoint)
						return nil, txRuleError(wire.RejectDuplicate, str)
					}
				}
			}
		}
	} else {
		// The transaction may not use any of the same outputs as other
		// transactions already in the pool as that would ultimately result in a
		// double spend.  This check is intended to be quick and therefore only
		// detects double spends within the transaction pool itself.  The
		// transaction could still be double spending coins from the main chain
		// at this point.  There is a more in-depth check that happens later
		// after fetching the referenced transaction inputs from the main chain
		// which examines the actual spend data and prevents double spends.
		err = mp.checkPoolDoubleSpend(tx, txType)
		if err != nil {
			return nil, err
		}
	}

	// Votes that are on too old of blocks are rejected.
	if txType == stake.TxTypeSSGen {
		_, voteHeight := stake.SSGenBlockVotedOn(msgTx)
		if (int64(voteHeight) < nextBlockHeight-maximumVoteAgeDelta) &&
			!mp.cfg.Policy.AllowOldVotes {
			str := fmt.Sprintf("transaction %v votes on old "+
				"block height of %v which is before the "+
				"current cutoff height of %v",
				tx.Hash(), voteHeight, nextBlockHeight-maximumVoteAgeDelta)
			return nil, txRuleError(wire.RejectNonstandard, str)
		}
	}

	// Fetch all of the unspent transaction outputs referenced by the inputs
	// to this transaction.  This function also attempts to fetch the
	// transaction itself to be used for detecting a duplicate transaction
	// without needing to do a separate lookup.
	utxoView, err := mp.fetchInputUtxos(tx)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow the transaction if it exists in the main chain and is not
	// not already fully spent.
	txEntry := utxoView.LookupEntry(txHash)
	if txEntry != nil && !txEntry.IsFullySpent() {
		return nil, txRuleError(wire.RejectDuplicate,
			"transaction already exists")
	}
	delete(utxoView.Entries(), *txHash)

	// Transaction is an orphan if any of the inputs don't exist.
	var missingParents []*chainhash.Hash
	for i, txIn := range msgTx.TxIn {
		if i == 0 && txType == stake.TxTypeSSGen {
			continue
		}

		entry := utxoView.LookupEntry(&txIn.PreviousOutPoint.Hash)
		if entry == nil || entry.IsFullySpent() {
			// Must make a copy of the hash here since the iterator
			// is replaced and taking its address directly would
			// result in all of the entries pointing to the same
			// memory location and thus all be the final hash.
			hashCopy := txIn.PreviousOutPoint.Hash
			missingParents = append(missingParents, &hashCopy)

			// Prevent a panic in the logger by continuing here if the
			// transaction input is nil.
			if entry == nil {
				log.Tracef("Transaction %v uses unknown input %v "+
					"and will be considered an orphan", txHash,
					txIn.PreviousOutPoint.Hash)
				continue
			}
			if entry.IsFullySpent() {
				log.Tracef("Transaction %v uses full spent input %v "+
					"and will be considered an orphan", txHash,
					txIn.PreviousOutPoint.Hash)
			}
		}
	}

	if len(missingParents) > 0 {
		return missingParents, nil
	}

	// Don't allow the transaction into the mempool unless its sequence
	// lock is active, meaning that it'll be allowed into the next block
	// with respect to its defined relative lock times.
	seqLock, err := mp.cfg.CalcSequenceLock(tx, utxoView)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}
	if !blockchain.SequenceLockActive(seqLock, nextBlockHeight, medianTime) {
		return nil, txRuleError(wire.RejectNonstandard,
			"transaction sequence locks on inputs not met")
	}

	// Perform several checks on the transaction inputs using the invariant
	// rules in chain for what transactions are allowed into blocks.
	// Also returns the fees associated with the transaction which will be
	// used later.  The fraud proof is not checked because it will be
	// filled in by the miner.
	txFee, err := blockchain.CheckTransactionInputs(mp.cfg.SubsidyCache,
		tx, nextBlockHeight, utxoView, false, mp.cfg.ChainParams)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow transactions with non-standard inputs if the mempool config
	// forbids their acceptance and relaying.
	if !mp.cfg.Policy.AcceptNonStd {
		err := checkInputsStandard(tx, txType, utxoView)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = wire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction %v has a non-standard "+
				"input: %v", txHash, err)
			return nil, txRuleError(rejectCode, str)
		}
	}

	// NOTE: if you modify this code to accept non-standard transactions,
	// you should add code here to check that the transaction does a
	// reasonable number of ECDSA signature verifications.

	// Don't allow transactions with an excessive number of signature
	// operations which would result in making it impossible to mine.  Since
	// the coinbase address itself can contain signature operations, the
	// maximum allowed signature operations per transaction is less than
	// the maximum allowed signature operations per block.
	numSigOps, err := blockchain.CountP2SHSigOps(tx, false,
		(txType == stake.TxTypeSSGen), utxoView)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	numSigOps += blockchain.CountSigOps(tx, false, (txType == stake.TxTypeSSGen))
	if numSigOps > mp.cfg.Policy.MaxSigOpsPerTx {
		str := fmt.Sprintf("transaction %v has too many sigops: %d > %d",
			txHash, numSigOps, mp.cfg.Policy.MaxSigOpsPerTx)
		return nil, txRuleError(wire.RejectNonstandard, str)
	}

	// Don't allow transactions with fees too low to get into a mined block.
	//
	// Most miners allow a free transaction area in blocks they mine to go
	// alongside the area used for high-priority transactions as well as
	// transactions with fees.  A transaction size of up to 1000 bytes is
	// considered safe to go into this section.  Further, the minimum fee
	// calculated below on its own would encourage several small
	// transactions to avoid fees rather than one single larger transaction
	// which is more desirable.  Therefore, as long as the size of the
	// transaction does not exceeed 1000 less than the reserved space for
	// high-priority transactions, don't require a fee for it.
	// This applies to non-stake transactions only.
	serializedSize := int64(msgTx.SerializeSize())
	minFee := calcMinRequiredTxRelayFee(serializedSize,
		mp.cfg.Policy.MinRelayTxFee)
	if txType == stake.TxTypeRegular { // Non-stake only
		if serializedSize >= (DefaultBlockPrioritySize-1000) &&
			txFee < minFee {

			str := fmt.Sprintf("transaction %v has %v fees which "+
				"is under the required amount of %v", txHash,
				txFee, minFee)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	// Require that free transactions have sufficient priority to be mined
	// in the next block.  Transactions which are being added back to the
	// memory pool from blocks that have been disconnected during a reorg
	// are exempted.
	//
	// This applies to non-stake transactions only.
	if isNew && !mp.cfg.Policy.DisableRelayPriority && txFee < minFee &&
		txType == stake.TxTypeRegular {

		currentPriority := CalcPriority(msgTx, utxoView,
			nextBlockHeight)
		if currentPriority <= MinHighPriority {
			str := fmt.Sprintf("transaction %v has insufficient "+
				"priority (%g <= %g)", txHash,
				currentPriority, MinHighPriority)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	// Free-to-relay transactions are rate limited here to prevent
	// penny-flooding with tiny transactions as a form of attack.
	// This applies to non-stake transactions only.
	if rateLimit && txFee < minFee && txType == stake.TxTypeRegular {
		nowUnix := time.Now().Unix()
		// Decay passed data with an exponentially decaying ~10 minute
		// window.
		mp.pennyTotal *= math.Pow(1.0-1.0/600.0,
			float64(nowUnix-mp.lastPennyUnix))
		mp.lastPennyUnix = nowUnix

		// Are we still over the limit?
		if mp.pennyTotal >= mp.cfg.Policy.FreeTxRelayLimit*10*1000 {
			str := fmt.Sprintf("transaction %v has been rejected "+
				"by the rate limiter due to low fees", txHash)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
		oldTotal := mp.pennyTotal

		mp.pennyTotal += float64(serializedSize)
		log.Tracef("rate limit: curTotal %v, nextTotal: %v, "+
			"limit %v", oldTotal, mp.pennyTotal,
			mp.cfg.Policy.FreeTxRelayLimit*10*1000)
	}

	// Check that tickets also pay the minimum of the relay fee.  This fee is
	// also performed on regular transactions above, but fees lower than the
	// miniumum may be allowed when there is sufficient priority, and these
	// checks aren't desired for ticket purchases.
	if txType == stake.TxTypeSStx {
		minTicketFee := calcMinRequiredTxRelayFee(serializedSize,
			mp.cfg.Policy.MinRelayTxFee)
		if txFee < minTicketFee {
			str := fmt.Sprintf("ticket purchase transaction %v has a %v "+
				"fee which is under the required threshold amount of %d",
				txHash, txFee, minTicketFee)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	// Check whether allowHighFees is set to false (default), if so, then make
	// sure the current fee is sensible.  If people would like to avoid this
	// check then they can AllowHighFees = true
	if !allowHighFees {
		maxFee := calcMinRequiredTxRelayFee(serializedSize*maxRelayFeeMultiplier,
			mp.cfg.Policy.MinRelayTxFee)
		if txFee > maxFee {
			err = fmt.Errorf("transaction %v has %v fee which is above the "+
				"allowHighFee check threshold amount of %v", txHash,
				txFee, maxFee)
			return nil, err
		}
	}

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	flags, err := mp.cfg.Policy.StandardVerifyFlags()
	if err != nil {
		return nil, err
	}
	err = blockchain.ValidateTransactionScripts(tx, utxoView, flags,
		mp.cfg.SigCache)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Add to transaction pool.
	mp.addTransaction(utxoView, tx, txType, bestHeight, txFee)

	// If it's an SSGen (vote), insert it into the list of
	// votes.
	if txType == stake.TxTypeSSGen {
		mp.votesMtx.Lock()
		err := mp.insertVote(tx)
		mp.votesMtx.Unlock()
		if err != nil {
			return nil, err
		}
	}

	log.Debugf("Accepted transaction %v (pool size: %v)", txHash,
		len(mp.pool))

	return nil, nil
}

// MaybeAcceptTransaction is the main workhorse for handling insertion of new
// free-standing transactions into a memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.  The
// isOrphan parameter can be nil if the caller does not need to know whether
// or not the transaction is an orphan.
//
// This function is safe for concurrent access.
func (mp *TxPool) MaybeAcceptTransaction(tx *dcrutil.Tx, isNew, rateLimit bool) ([]*chainhash.Hash, error) {
	// Protect concurrent access.
	mp.mtx.Lock()
	hashes, err := mp.maybeAcceptTransaction(tx, isNew, rateLimit, true)
	mp.mtx.Unlock()

	return hashes, err
}

// processOrphans is the internal function which implements the public
// ProcessOrphans.  See the comment for ProcessOrphans for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) processOrphans(hash *chainhash.Hash) []*dcrutil.Tx {
	var acceptedTxns []*dcrutil.Tx

	// Start with processing at least the passed hash.
	processHashes := list.New()
	processHashes.PushBack(hash)
	for processHashes.Len() > 0 {
		// Pop the first hash to process.
		firstElement := processHashes.Remove(processHashes.Front())
		processHash := firstElement.(*chainhash.Hash)

		// Look up all orphans that are referenced by the transaction we
		// just accepted.  This will typically only be one, but it could
		// be multiple if the referenced transaction contains multiple
		// outputs.  Skip to the next item on the list of hashes to
		// process if there are none.
		orphans, exists := mp.orphansByPrev[*processHash]
		if !exists || orphans == nil {
			continue
		}

		for _, tx := range orphans {
			// Remove the orphan from the orphan pool.  Current
			// behavior requires that all saved orphans with
			// a newly accepted parent are removed from the orphan
			// pool and potentially added to the memory pool, but
			// transactions which cannot be added to memory pool
			// (including due to still being orphans) are expunged
			// from the orphan pool.
			//
			// TODO(jrick): The above described behavior sounds
			// like a bug, and I think we should investigate
			// potentially moving orphans to the memory pool, but
			// leaving them in the orphan pool if not all parent
			// transactions are known yet.
			orphanHash := tx.Hash()
			mp.removeOrphan(orphanHash)

			// Potentially accept the transaction into the
			// transaction pool.
			missingParents, err := mp.maybeAcceptTransaction(tx,
				true, true, true)
			if err != nil {
				// TODO: Remove orphans that depend on this
				// failed transaction.
				log.Debugf("Unable to move orphan transaction "+
					"%v to mempool: %v", tx.Hash(), err)
				continue
			}

			if len(missingParents) > 0 {
				// Transaction is still an orphan, so add it
				// back.
				mp.addOrphan(tx)
				continue
			}

			// Add this transaction to the list of transactions
			// that are no longer orphans.
			acceptedTxns = append(acceptedTxns, tx)

			// Add this transaction to the list of transactions to
			// process so any orphans that depend on this one are
			// handled too.
			//
			// TODO(jrick): In the case that this is still an orphan,
			// we know that any other transactions in the orphan
			// pool with this orphan as their parent are still
			// orphans as well, and should be removed.  While
			// recursively calling removeOrphan and
			// maybeAcceptTransaction on these transactions is not
			// wrong per se, it is overkill if all we care about is
			// recursively removing child transactions of this
			// orphan.
			processHashes.PushBack(orphanHash)
		}
	}

	return acceptedTxns
}

// PruneStakeTx is the function which is called every time a new block is
// processed.  The idea is any outstanding SStx that hasn't been mined in a
// certain period of time (CoinbaseMaturity) and the submitted SStx's
// stake difficulty is below the current required stake difficulty should be
// pruned from mempool since they will never be mined.  The same idea stands
// for SSGen and SSRtx
func (mp *TxPool) PruneStakeTx(requiredStakeDifficulty, height int64) {
	// Protect concurrent access.
	mp.mtx.Lock()
	mp.pruneStakeTx(requiredStakeDifficulty, height)
	mp.mtx.Unlock()
}

func (mp *TxPool) pruneStakeTx(requiredStakeDifficulty, height int64) {
	for _, tx := range mp.pool {
		txType := stake.DetermineTxType(tx.Tx.MsgTx())
		if txType == stake.TxTypeSStx &&
			tx.Height+int64(heightDiffToPruneTicket) < height {
			mp.removeTransaction(tx.Tx, true)
		}
		if txType == stake.TxTypeSStx &&
			tx.Tx.MsgTx().TxOut[0].Value < requiredStakeDifficulty {
			mp.removeTransaction(tx.Tx, true)
		}
		if (txType == stake.TxTypeSSRtx || txType == stake.TxTypeSSGen) &&
			tx.Height+int64(heightDiffToPruneVotes) < height {
			mp.removeTransaction(tx.Tx, true)
		}
	}
}

// PruneExpiredTx prunes expired transactions from the mempool that may no longer
// be able to be included into a block.
func (mp *TxPool) PruneExpiredTx(height int64) {
	// Protect concurrent access.
	mp.mtx.Lock()
	mp.pruneExpiredTx(height)
	mp.mtx.Unlock()
}

func (mp *TxPool) pruneExpiredTx(height int64) {
	for _, tx := range mp.pool {
		if tx.Tx.MsgTx().Expiry != 0 {
			if height >= int64(tx.Tx.MsgTx().Expiry) {
				log.Debugf("Pruning expired transaction %v "+
					"from the mempool", tx.Tx.Hash())
				mp.removeTransaction(tx.Tx, true)
			}
		}
	}
}

// ProcessOrphans determines if there are any orphans which depend on the passed
// transaction hash (it is possible that they are no longer orphans) and
// potentially accepts them to the memory pool.  It repeats the process for the
// newly accepted transactions (to detect further orphans which may no longer be
// orphans) until there are no more.
//
// It returns a slice of transactions added to the mempool.  A nil slice means
// no transactions were moved from the orphan pool to the mempool.
//
// This function is safe for concurrent access.
func (mp *TxPool) ProcessOrphans(hash *chainhash.Hash) []*dcrutil.Tx {
	mp.mtx.Lock()
	acceptedTxns := mp.processOrphans(hash)
	mp.mtx.Unlock()
	return acceptedTxns
}

// ProcessTransaction is the main workhorse for handling insertion of new
// free-standing transactions into the memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
//
// It returns a slice of transactions added to the mempool.  When the
// error is nil, the list will include the passed transaction itself along
// with any additional orphan transaactions that were added as a result of
// the passed one being accepted.
//
// This function is safe for concurrent access.
func (mp *TxPool) ProcessTransaction(tx *dcrutil.Tx, allowOrphan, rateLimit, allowHighFees bool) ([]*dcrutil.Tx, error) {
	// Protect concurrent access.
	mp.mtx.Lock()
	defer mp.mtx.Unlock()
	var err error
	defer func() {
		if err != nil {
			log.Tracef("Failed to process transaction %v: %s",
				tx.Hash(), err.Error())
		}
	}()

	// Potentially accept the transaction to the memory pool.
	var missingParents []*chainhash.Hash
	missingParents, err = mp.maybeAcceptTransaction(tx, true, rateLimit,
		allowHighFees)
	if err != nil {
		return nil, err
	}

	// If len(missingParents) == 0 then we know the tx is NOT an orphan.
	if len(missingParents) == 0 {
		// Accept any orphan transactions that depend on this
		// transaction (they are no longer orphans if all inputs are
		// now available) and repeat for those accepted transactions
		// until there are no more.
		newTxs := mp.processOrphans(tx.Hash())
		acceptedTxs := make([]*dcrutil.Tx, len(newTxs)+1)

		// Add the parent transaction first so remote nodes
		// do not add orphans.
		acceptedTxs[0] = tx
		copy(acceptedTxs[1:], newTxs)

		return acceptedTxs, nil
	}

	// The transaction is an orphan (has inputs missing).  Reject
	// it if the flag to allow orphans is not set.
	if !allowOrphan {
		// Only use the first missing parent transaction in
		// the error message.
		//
		// NOTE: RejectDuplicate is really not an accurate
		// reject code here, but it matches the reference
		// implementation and there isn't a better choice due
		// to the limited number of reject codes.  Missing
		// inputs is assumed to mean they are already spent
		// which is not really always the case.
		str := fmt.Sprintf("orphan transaction %v references "+
			"outputs of unknown or fully-spent "+
			"transaction %v", tx.Hash(), missingParents[0])
		return nil, txRuleError(wire.RejectDuplicate, str)
	}

	// Potentially add the orphan transaction to the orphan pool.
	err = mp.maybeAddOrphan(tx)
	return nil, err
}

// Count returns the number of transactions in the main pool.  It does not
// include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) Count() int {
	mp.mtx.RLock()
	count := len(mp.pool)
	mp.mtx.RUnlock()

	return count
}

// TxHashes returns a slice of hashes for all of the transactions in the memory
// pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) TxHashes() []*chainhash.Hash {
	mp.mtx.RLock()
	hashes := make([]*chainhash.Hash, len(mp.pool))
	i := 0
	for hash := range mp.pool {
		hashCopy := hash
		hashes[i] = &hashCopy
		i++
	}
	mp.mtx.RUnlock()

	return hashes
}

// TxDescs returns a slice of descriptors for all the transactions in the pool.
// The descriptors are to be treated as read only.
//
// This function is safe for concurrent access.
func (mp *TxPool) TxDescs() []*TxDesc {
	mp.mtx.RLock()
	descs := make([]*TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = desc
		i++
	}
	mp.mtx.RUnlock()

	return descs
}

// MiningDescs returns a slice of mining descriptors for all the transactions
// in the pool.
//
// This is part of the mining.TxSource interface implementation and is safe for
// concurrent access as required by the interface contract.
func (mp *TxPool) MiningDescs() []*mining.TxDesc {
	mp.mtx.RLock()
	descs := make([]*mining.TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = &desc.TxDesc
		i++
	}
	mp.mtx.RUnlock()

	return descs
}

// RawMempoolVerbose returns all of the entries in the mempool filtered by the
// provided stake type as a fully populated JSON result.  The filter type can be
// nil in which case all transactions will be returned.
//
// This function is safe for concurrent access.
func (mp *TxPool) RawMempoolVerbose(filterType *stake.TxType) map[string]*dcrjson.GetRawMempoolVerboseResult {
	mp.mtx.RLock()
	defer mp.mtx.RUnlock()

	result := make(map[string]*dcrjson.GetRawMempoolVerboseResult,
		len(mp.pool))
	bestHeight := mp.cfg.BestHeight()

	for _, desc := range mp.pool {
		// Skip entries that don't match the requested stake type if
		// specified.
		if filterType != nil && desc.Type != *filterType {
			continue
		}

		// Calculate the current priority based on the inputs to
		// the transaction.  Use zero if one or more of the
		// input transactions can't be found for some reason.
		tx := desc.Tx
		var currentPriority float64
		utxos, err := mp.fetchInputUtxos(tx)
		if err == nil {
			currentPriority = CalcPriority(tx.MsgTx(), utxos,
				bestHeight+1)
		}

		mpd := &dcrjson.GetRawMempoolVerboseResult{
			Size:             int32(tx.MsgTx().SerializeSize()),
			Fee:              dcrutil.Amount(desc.Fee).ToCoin(),
			Time:             desc.Added.Unix(),
			Height:           desc.Height,
			StartingPriority: desc.StartingPriority,
			CurrentPriority:  currentPriority,
			Depends:          make([]string, 0),
		}
		for _, txIn := range tx.MsgTx().TxIn {
			hash := &txIn.PreviousOutPoint.Hash
			if mp.haveTransaction(hash) {
				mpd.Depends = append(mpd.Depends,
					hash.String())
			}
		}

		result[tx.Hash().String()] = mpd
	}

	return result
}

// LastUpdated returns the last time a transaction was added to or removed from
// the main pool.  It does not include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) LastUpdated() time.Time {
	return time.Unix(atomic.LoadInt64(&mp.lastUpdated), 0)
}

// CheckIfTxsExist checks a list of transaction hashes against the mempool
// and returns true if they all exist in the mempool, otherwise false.
//
// This function is safe for concurrent access.
func (mp *TxPool) CheckIfTxsExist(hashes []chainhash.Hash) bool {
	mp.mtx.RLock()
	inPool := true
	for _, h := range hashes {
		if _, exists := mp.pool[h]; !exists {
			inPool = false
			break
		}
	}
	mp.mtx.RUnlock()
	return inPool
}

// New returns a new memory pool for validating and storing standalone
// transactions until they are mined into a block.
func New(cfg *Config) *TxPool {
	return &TxPool{
		cfg:           *cfg,
		pool:          make(map[chainhash.Hash]*TxDesc),
		orphans:       make(map[chainhash.Hash]*dcrutil.Tx),
		orphansByPrev: make(map[chainhash.Hash]map[chainhash.Hash]*dcrutil.Tx),
		outpoints:     make(map[wire.OutPoint]*dcrutil.Tx),
		votes:         make(map[chainhash.Hash][]VoteTx),
	}
}
