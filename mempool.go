// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/mining"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	// mempoolHeight is the height used for the "block" height field of the
	// contextual transaction information provided in a transaction view.
	mempoolHeight = 0x7fffffff

	// minTicketFee is the minimum fee per KB in atoms that is required for
	// a ticket to enter the mempool.
	minTicketFee = 1e6

	// maxRelayFeeMultiplier is the factor that we disallow fees / kb above
	// the minimum tx fee.
	maxRelayFeeMultiplier = 100

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

// mempoolTxDesc is a descriptor containing a transaction in the mempool along
// with additional metadata.
type mempoolTxDesc struct {
	mining.TxDesc

	// StartingPriority is the priority of the transaction when it was added
	// to the pool.
	StartingPriority float64
}

// mempoolConfig is a descriptor containing the memory pool configuration.
type mempoolConfig struct {
	// ChainParams identifies which chain parameters the mempool is
	// associated with.
	ChainParams *chaincfg.Params

	// NewestSha defines the function to retrieve the newest sha.
	NewestSha func() (*chainhash.Hash, int64, error)

	// NextStakeDifficulty defines the function to retrieve the stake
	// difficulty for the block after the current best block.
	//
	// This function must be safe for concurrent access.
	NextStakeDifficulty func() (int64, error)

	// Policy defines the various mempool configuration options related
	// to policy.
	Policy mempoolPolicy

	// FetchUtxoView defines the function to use to fetch unspent
	// transaction output information.
	FetchUtxoView func(*dcrutil.Tx, bool) (*blockchain.UtxoViewpoint, error)

	// Chain defines the concurrent safe block chain instance which houses
	// the current best chain.
	Chain *blockchain.BlockChain

	// SigCache defines a signature cache to use.
	SigCache *txscript.SigCache

	// TimeSource defines the timesource to use.
	TimeSource blockchain.MedianTimeSource
}

// mempoolPolicy houses the policy (configuration parameters) which is used to
// control the mempool.
type mempoolPolicy struct {
	// DisableRelayPriority defines whether to relay free or low-fee
	// transactions that do not have enough priority to be relayed.
	DisableRelayPriority bool

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
}

// txMemPool is used as a source of transactions that need to be mined into
// blocks and relayed to other peers.  It is safe for concurrent access from
// multiple peers.
type txMemPool struct {
	// The following variables must only be used atomically.
	lastUpdated int64 // last time pool was updated.

	sync.RWMutex
	cfg           mempoolConfig
	pool          map[chainhash.Hash]*mempoolTxDesc
	orphans       map[chainhash.Hash]*dcrutil.Tx
	orphansByPrev map[chainhash.Hash]map[chainhash.Hash]*dcrutil.Tx
	addrindex     map[string]map[chainhash.Hash]struct{} // maps address to txs
	outpoints     map[wire.OutPoint]*dcrutil.Tx

	// Votes on blocks.
	votes    map[chainhash.Hash][]*VoteTx
	votesMtx sync.Mutex

	// A declared subsidy cache as passed from the blockchain.
	subsidyCache *blockchain.SubsidyCache

	pennyTotal    float64 // exponentially decaying total for penny spends.
	lastPennyUnix int64   // unix time of last ``penny spend''
}

// insertVote inserts a vote into the map of block votes.
// This function is safe for concurrent access.
func (mp *txMemPool) insertVote(ssgen *dcrutil.Tx) error {
	voteHash := ssgen.Sha()
	msgTx := ssgen.MsgTx()
	ticketHash := &msgTx.TxIn[1].PreviousOutPoint.Hash

	// Get the block it is voting on; here we're agnostic of height.
	blockHash, blockHeight, err := stake.SSGenBlockVotedOn(ssgen)
	if err != nil {
		return err
	}

	voteBits := stake.SSGenVoteBits(ssgen)
	vote := dcrutil.IsFlagSet16(voteBits, dcrutil.BlockValid)

	voteTx := &VoteTx{*voteHash, *ticketHash, vote}
	vts, exists := mp.votes[blockHash]

	// If there are currently no votes for this block,
	// start a new buffered slice and store it.
	if !exists {
		minrLog.Debugf("Accepted vote %v for block hash %v (height %v), "+
			"voting %v on the transaction tree",
			voteHash, blockHash, blockHeight, vote)

		slice := make([]*VoteTx, int(mp.cfg.ChainParams.TicketsPerBlock),
			int(mp.cfg.ChainParams.TicketsPerBlock))
		slice[0] = voteTx
		mp.votes[blockHash] = slice
		return nil
	}

	// We already have a vote for this ticket; break.
	for _, vt := range vts {
		// At the end.
		if vt == nil {
			break
		}

		if vt.SstxHash.IsEqual(ticketHash) {
			return nil
		}
	}

	// Add the new vote in. Find where the first empty
	// slot is and insert it.
	for i, vt := range vts {
		// At the end.
		if vt == nil {
			mp.votes[blockHash][i] = voteTx
			break
		}
	}

	minrLog.Debugf("Accepted vote %v for block hash %v (height %v), "+
		"voting %v on the transaction tree",
		voteHash, blockHash, blockHeight, vote)

	return nil
}

// InsertVote calls insertVote, but makes it safe for concurrent access.
func (mp *txMemPool) InsertVote(ssgen *dcrutil.Tx) error {
	mp.votesMtx.Lock()
	defer mp.votesMtx.Unlock()

	err := mp.insertVote(ssgen)

	return err
}

// getVoteHashesForBlock gets the transaction hashes of all the known votes for
// some block on the blockchain.
func (mp *txMemPool) getVoteHashesForBlock(block chainhash.Hash) ([]chainhash.Hash,
	error) {
	var hashes []chainhash.Hash
	vts, exists := mp.votes[block]
	if !exists {
		return nil, fmt.Errorf("couldn't find block requested in mp.votes")
	}

	if len(vts) == 0 {
		return nil, fmt.Errorf("block found in mp.votes, but contains no votes")
	}

	zeroHash := &chainhash.Hash{}
	for _, vt := range vts {
		if vt == nil {
			break
		}

		if vt.SsgenHash.IsEqual(zeroHash) {
			return nil, fmt.Errorf("unset vote hash in vote info")
		}
		hashes = append(hashes, vt.SsgenHash)
	}

	return hashes, nil
}

// GetVoteHashesForBlock calls getVoteHashesForBlock, but makes it safe for
// concurrent access.
func (mp *txMemPool) GetVoteHashesForBlock(block chainhash.Hash) ([]chainhash.Hash,
	error) {
	mp.votesMtx.Lock()
	defer mp.votesMtx.Unlock()

	hashes, err := mp.getVoteHashesForBlock(block)

	return hashes, err
}

// TODO Pruning of the votes map DECRED

func getNumberOfVotesOnBlock(blockVoteTxs []*VoteTx) int {
	numVotes := 0
	for _, vt := range blockVoteTxs {
		if vt == nil {
			break
		}

		numVotes++
	}

	return numVotes
}

// blockWithLenVotes is a block with the number of votes currently present
// for that block. Just used for sorting.
type blockWithLenVotes struct {
	Block chainhash.Hash
	Votes uint16
}

// ByNumberOfVotes defines the methods needed to satisify sort.Interface to
// sort a slice of Blocks by their number of votes.
type ByNumberOfVotes []*blockWithLenVotes

func (b ByNumberOfVotes) Len() int           { return len(b) }
func (b ByNumberOfVotes) Less(i, j int) bool { return b[i].Votes < b[j].Votes }
func (b ByNumberOfVotes) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

// sortParentsByVotes takes a list of block header hashes and sorts them
// by the number of votes currently available for them in the votes map of
// mempool. It then returns all blocks that are eligible to be used (have
// at least a majority number of votes) sorted by number of votes, descending.
func (mp *txMemPool) sortParentsByVotes(currentTopBlock chainhash.Hash,
	blocks []chainhash.Hash) ([]chainhash.Hash, error) {
	lenBlocks := len(blocks)
	if lenBlocks == 0 {
		return nil, fmt.Errorf("no blocks to sort")
	}

	bwlvs := make([]*blockWithLenVotes, lenBlocks, lenBlocks)

	for i, blockHash := range blocks {
		votes, exists := mp.votes[blockHash]
		if exists {
			bwlv := &blockWithLenVotes{
				blockHash,
				uint16(getNumberOfVotesOnBlock(votes)),
			}
			bwlvs[i] = bwlv
		} else {
			bwlv := &blockWithLenVotes{
				blockHash,
				uint16(0),
			}
			bwlvs[i] = bwlv
		}
	}

	// Blocks with the most votes appear at the top of the list.
	sort.Sort(sort.Reverse(ByNumberOfVotes(bwlvs)))

	var sortedUsefulBlocks []chainhash.Hash
	minimumVotesRequired := uint16((mp.cfg.ChainParams.TicketsPerBlock / 2) + 1)
	for _, bwlv := range bwlvs {
		if bwlv.Votes >= minimumVotesRequired {
			sortedUsefulBlocks = append(sortedUsefulBlocks, bwlv.Block)
		}
	}

	if sortedUsefulBlocks == nil {
		return nil, miningRuleError(ErrNotEnoughVoters,
			"no block had enough votes to build on top of")
	}

	// Make sure we don't reorganize the chain needlessly if the top block has
	// the same amount of votes as the current leader after the sort. After this
	// point, all blocks listed in sortedUsefulBlocks definitely also have the
	// minimum number of votes required.
	topBlockVotes, exists := mp.votes[currentTopBlock]
	topBlockVotesLen := 0
	if exists {
		topBlockVotesLen = getNumberOfVotesOnBlock(topBlockVotes)
	}
	if bwlvs[0].Votes == uint16(topBlockVotesLen) {
		if !bwlvs[0].Block.IsEqual(&currentTopBlock) {
			// Find our block in the list.
			pos := 0
			for i, bwlv := range bwlvs {
				if bwlv.Block.IsEqual(&currentTopBlock) {
					pos = i
					break
				}
			}

			if pos == 0 { // Should never happen...
				return nil, fmt.Errorf("couldn't find top block in list")
			}

			// Swap the top block into the first position. We directly access
			// sortedUsefulBlocks useful blocks here with the assumption that
			// since the values were accumulated from blvs, they should be
			// in the same positions and we shouldn't be able to access anything
			// out of bounds.
			sortedUsefulBlocks[0], sortedUsefulBlocks[pos] =
				sortedUsefulBlocks[pos], sortedUsefulBlocks[0]
		}
	}

	return sortedUsefulBlocks, nil
}

// SortParentsByVotes is the concurrency safe exported version of
// sortParentsByVotes.
func (mp *txMemPool) SortParentsByVotes(currentTopBlock chainhash.Hash,
	blocks []chainhash.Hash) ([]chainhash.Hash, error) {
	mp.votesMtx.Lock()
	defer mp.votesMtx.Unlock()

	sortedBlocks, err := mp.sortParentsByVotes(currentTopBlock, blocks)

	return sortedBlocks, err
}

// Ensure the txMemPool type implements the mining.TxSource interface.
var _ mining.TxSource = (*txMemPool)(nil)

// removeOrphan is the internal function which implements the public
// RemoveOrphan.  See the comment for RemoveOrphan for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeOrphan(txHash *chainhash.Hash) {
	txmpLog.Tracef("Removing orphan transaction %v", txHash)

	// Nothing to do if passed tx is not an orphan.
	tx, exists := mp.orphans[*txHash]
	if !exists {
		return
	}

	// Remove the reference from the previous orphan index.
	for _, txIn := range tx.MsgTx().TxIn {
		originTxHash := txIn.PreviousOutPoint.Hash
		if orphans, exists := mp.orphansByPrev[originTxHash]; exists {
			delete(orphans, *tx.Sha())

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
func (mp *txMemPool) RemoveOrphan(txHash *chainhash.Hash) {
	mp.Lock()
	mp.removeOrphan(txHash)
	mp.Unlock()
}

// limitNumOrphans limits the number of orphan transactions by evicting a random
// orphan if adding a new one would cause it to overflow the max allowed.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) limitNumOrphans() error {
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
			txHashNum := blockchain.ShaHashToBig(&txHash)
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
func (mp *txMemPool) addOrphan(tx *dcrutil.Tx) {
	// Limit the number orphan transactions to prevent memory exhaustion.  A
	// random orphan is evicted to make room if needed.
	mp.limitNumOrphans()

	mp.orphans[*tx.Sha()] = tx
	for _, txIn := range tx.MsgTx().TxIn {
		originTxHash := txIn.PreviousOutPoint.Hash
		if _, exists := mp.orphansByPrev[originTxHash]; !exists {
			mp.orphansByPrev[originTxHash] =
				make(map[chainhash.Hash]*dcrutil.Tx)
		}
		mp.orphansByPrev[originTxHash][*tx.Sha()] = tx
	}

	txmpLog.Debugf("Stored orphan transaction %v (total: %d)", tx.Sha(),
		len(mp.orphans))
}

// maybeAddOrphan potentially adds an orphan to the orphan pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) maybeAddOrphan(tx *dcrutil.Tx) error {
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
func (mp *txMemPool) isTransactionInPool(hash *chainhash.Hash) bool {
	if _, exists := mp.pool[*hash]; exists {
		return true
	}

	return false
}

// IsTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) IsTransactionInPool(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.isTransactionInPool(hash)
}

// isOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) isOrphanInPool(hash *chainhash.Hash) bool {
	if _, exists := mp.orphans[*hash]; exists {
		return true
	}

	return false
}

// IsOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) IsOrphanInPool(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.isOrphanInPool(hash)
}

// haveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) haveTransaction(hash *chainhash.Hash) bool {
	return mp.isTransactionInPool(hash) || mp.isOrphanInPool(hash)
}

// HaveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) HaveTransaction(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.haveTransaction(hash)
}

// haveTransactions returns whether or not the passed transactions already exist
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) haveTransactions(hashes []*chainhash.Hash) []bool {
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
func (mp *txMemPool) HaveTransactions(hashes []*chainhash.Hash) []bool {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	return mp.haveTransactions(hashes)
}

// removeTransaction is the internal function which implements the public
// RemoveTransaction.  See the comment for RemoveTransaction for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeTransaction(tx *dcrutil.Tx, removeRedeemers bool) {
	txmpLog.Tracef("Removing transaction %v", tx.Sha())

	txHash := tx.Sha()
	var txType stake.TxType
	if removeRedeemers {
		// Remove any transactions which rely on this one.
		txType = stake.DetermineTxType(tx)
		tree := dcrutil.TxTreeRegular
		if txType != stake.TxTypeRegular {
			tree = dcrutil.TxTreeStake
		}
		for i := uint32(0); i < uint32(len(tx.MsgTx().TxOut)); i++ {
			outpoint := wire.NewOutPoint(txHash, i, tree)
			if txRedeemer, exists := mp.outpoints[*outpoint]; exists {
				mp.removeTransaction(txRedeemer, true)
			}
		}
	}

	// Remove the transaction and mark the referenced outpoints as unspent
	// by the pool.
	if txDesc, exists := mp.pool[*txHash]; exists {
		// Remove the transaction and its addresses from the address
		// index if it's enabled.
		/*
			TODO New address index
			if !mp.cfg.NoAddrIndex {
				mp.pruneTxFromAddrIndex(tx, txType)
			}
		*/

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
func (mp *txMemPool) RemoveTransaction(tx *dcrutil.Tx, removeRedeemers bool) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	mp.removeTransaction(tx, removeRedeemers)
}

// RemoveDoubleSpends removes all transactions which spend outputs spent by the
// passed transaction from the memory pool.  Removing those transactions then
// leads to removing all transactions which rely on them, recursively.  This is
// necessary when a block is connected to the main chain because the block may
// contain transactions which were previously unknown to the memory pool
//
// This function is safe for concurrent access.
func (mp *txMemPool) RemoveDoubleSpends(tx *dcrutil.Tx) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	for _, txIn := range tx.MsgTx().TxIn {
		if txRedeemer, ok := mp.outpoints[txIn.PreviousOutPoint]; ok {
			if !txRedeemer.Sha().IsEqual(tx.Sha()) {
				mp.removeTransaction(txRedeemer, true)
			}
		}
	}
}

// addTransaction adds the passed transaction to the memory pool.  It should
// not be called directly as it doesn't perform any validation.  This is a
// helper for maybeAcceptTransaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) addTransaction(utxoView *blockchain.UtxoViewpoint,
	tx *dcrutil.Tx, txType stake.TxType, height int64, fee int64) {
	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.
	mp.pool[*tx.Sha()] = &mempoolTxDesc{
		TxDesc: mining.TxDesc{
			Tx:     tx,
			Type:   txType,
			Added:  time.Now(),
			Height: height,
			Fee:    fee,
		},
		StartingPriority: calcPriority(tx.MsgTx(), utxoView, height),
	}
	for _, txIn := range tx.MsgTx().TxIn {
		mp.outpoints[txIn.PreviousOutPoint] = tx
	}
	atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())

	// Add the addresses associated with the transaction to the address
	// index if it's enabled.
	/*
		TODO New address index
		if !mp.cfg.NoAddrIndex {
			mp.addTransactionToAddrIndex(tx, txType)
		}
	*/
}

// addTransactionToAddrIndex adds all addresses related to the transaction to
// our in-memory address index. Note that this address is only populated when
// we're running with the optional address index activated.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) addTransactionToAddrIndex(tx *dcrutil.Tx,
	txType stake.TxType) error {

	// Insert the addresses into the mempool address index.
	for _, txOut := range tx.MsgTx().TxOut {
		err := mp.indexScriptAddressToTx(txOut.Version, txOut.PkScript,
			tx, txType)
		if err != nil {
			return err
		}
	}

	return nil
}

// indexScriptByAddress alters our address index by indexing the payment address
// encoded by the passed scriptPubKey to the passed transaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) indexScriptAddressToTx(pkVersion uint16, pkScript []byte,
	tx *dcrutil.Tx, txType stake.TxType) error {
	/*
		class, addresses, _, err := txscript.ExtractPkScriptAddrs(pkVersion, pkScript,
			activeNetParams.Params)
		if err != nil {
			txmpLog.Tracef("Unable to extract encoded addresses from script "+
				"for addrindex: %v", err)
			return err
		}

		// An exception is SStx commitments. Handle these manually.
		if txType == stake.TxTypeSStx && class == txscript.NullDataTy {
			addr, err := stake.AddrFromSStxPkScrCommitment(pkScript,
				mp.cfg.ChainParams)
			if err != nil {
				txmpLog.Tracef("Unable to extract encoded addresses "+
					"from sstx commitment script for addrindex: %v", err)
				return err
			}

			addresses = []dcrutil.Address{addr}
		}

		for _, addr := range addresses {
			if mp.addrindex[addr.EncodeAddress()] == nil {
				mp.addrindex[addr.EncodeAddress()] = make(map[chainhash.Hash]struct{})
			}
			mp.addrindex[addr.EncodeAddress()][*tx.Sha()] = struct{}{}
		}
	*/

	return nil
}

// pruneTxFromAddrIndex deletes references to the transaction in the address
// index by searching first for the address from an output and second for the
// transaction itself.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) pruneTxFromAddrIndex(tx *dcrutil.Tx, txType stake.TxType) {
	/*
		txHash := tx.Sha()

		for _, txOut := range tx.MsgTx().TxOut {
			class, addresses, _, err := txscript.ExtractPkScriptAddrs(txOut.Version,
				txOut.PkScript, activeNetParams.Params)
			if err != nil {
				// If we couldn't extract addresses, skip this output.
				continue
			}

			// An exception is SStx commitments. Handle these manually.
			if txType == stake.TxTypeSStx && class == txscript.NullDataTy {
				addr, err := stake.AddrFromSStxPkScrCommitment(txOut.PkScript,
					mp.cfg.ChainParams)
				if err != nil {
					// If we couldn't extract addresses, skip this output.
					continue
				}

				addresses = []dcrutil.Address{addr}
			}

			for _, addr := range addresses {
				if mp.addrindex[addr.EncodeAddress()] != nil {
					// First remove all references to the transaction hash.
					for thisTxHash := range mp.addrindex[addr.EncodeAddress()] {
						if thisTxHash == *txHash {
							delete(mp.addrindex[addr.EncodeAddress()], thisTxHash)
						}
					}

					// Then, if the address has no transactions referenced,
					// remove it too.
					if len(mp.addrindex[addr.EncodeAddress()]) == 0 {
						delete(mp.addrindex, addr.EncodeAddress())
					}
				}
			}
		}
	*/
}

// findTxForAddr searches for all referenced transactions for a given address
// that are currently stored in the mempool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) findTxForAddr(addr dcrutil.Address) []*dcrutil.Tx {
	/*
		var txs []*dcrutil.Tx
		if mp.addrindex[addr.EncodeAddress()] != nil {
			// Lookup all relevant transactions and append them.
			for thisTxHash := range mp.addrindex[addr.EncodeAddress()] {
				txDesc, exists := mp.pool[thisTxHash]
				if !exists {
					txmpLog.Warnf("Failed to find transaction %v in mempool "+
						"that was referenced in the mempool addrIndex",
						thisTxHash)
					continue
				}

				txs = append(txs, txDesc.Tx)
			}
		}

		return txs
	*/
	return nil
}

// FindTxForAddr is the exported and concurrency safe version of findTxForAddr.
//
// This function is safe for concurrent access.
func (mp *txMemPool) FindTxForAddr(addr dcrutil.Address) []*dcrutil.Tx {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	return mp.findTxForAddr(addr)
}

// checkPoolDoubleSpend checks whether or not the passed transaction is
// attempting to spend coins already spent by other transactions in the pool.
// Note it does not check for double spends against transactions already in the
// main chain.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) checkPoolDoubleSpend(tx *dcrutil.Tx,
	txType stake.TxType) error {

	for i, txIn := range tx.MsgTx().TxIn {
		// We don't care about double spends of stake bases.
		if (txType == stake.TxTypeSSGen || txType == stake.TxTypeSSRtx) &&
			(i == 0) {
			continue
		}

		if txR, exists := mp.outpoints[txIn.PreviousOutPoint]; exists {
			str := fmt.Sprintf("transaction %v in the pool "+
				"already spends the same coins", txR.Sha())
			return txRuleError(wire.RejectDuplicate, str)
		}
	}

	return nil
}

// isTxTreeValid checks the map of votes for a block to see if the tx
// tree regular for the block at HEAD is valid.
func (mp *txMemPool) isTxTreeValid(newestHash *chainhash.Hash) bool {
	// There are no votes on the block currently; assume it's valid.
	if mp.votes[*newestHash] == nil {
		return true
	}

	// There are not possibly enough votes to tell if the txTree is valid;
	// assume it's valid.
	if len(mp.votes[*newestHash]) <=
		int(mp.cfg.ChainParams.TicketsPerBlock/2) {
		return true
	}

	// Otherwise, tally the votes and determine if it's valid or not.
	yea := 0
	nay := 0

	for _, vote := range mp.votes[*newestHash] {
		// End of list, break.
		if vote == nil {
			break
		}

		if vote.Vote == true {
			yea++
		} else {
			nay++
		}
	}

	if yea > nay {
		return true
	}
	return false
}

// IsTxTreeValid calls isTxTreeValid, but makes it safe for concurrent access.
func (mp *txMemPool) IsTxTreeValid(best *chainhash.Hash) bool {
	mp.votesMtx.Lock()
	defer mp.votesMtx.Unlock()
	isValid := mp.isTxTreeValid(best)

	return isValid
}

// fetchInputUtxos loads utxo details about the input transactions referenced by
// the passed transaction.  First, it loads the details form the viewpoint of
// the main chain, then it adjusts them based upon the contents of the
// transaction pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) fetchInputUtxos(tx *dcrutil.Tx) (*blockchain.UtxoViewpoint, error) {
	best := mp.cfg.Chain.BestSnapshot()
	tv := mp.IsTxTreeValid(best.Hash)
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
func (mp *txMemPool) FetchTransaction(txHash *chainhash.Hash) (*dcrutil.Tx,
	error) {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	if txDesc, exists := mp.pool[*txHash]; exists {
		return txDesc.Tx, nil
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
func (mp *txMemPool) maybeAcceptTransaction(tx *dcrutil.Tx, isNew,
	rateLimit, allowHighFees bool) ([]*chainhash.Hash, error) {
	txHash := tx.Sha()
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
	err := blockchain.CheckTransactionSanity(tx, mp.cfg.ChainParams)
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
	if tx.MsgTx().LockTime > math.MaxInt32 {
		str := fmt.Sprintf("transaction %v has a lock time after "+
			"2038 which is not accepted yet", txHash)
		return nil, txRuleError(wire.RejectNonstandard, str)
	}

	// Get the current height of the main chain.  A standalone transaction
	// will be mined into the next block at best, so its height is at least
	// one more than the current height.
	best := mp.cfg.Chain.BestSnapshot()
	nextBlockHeight := best.Height + 1

	// Determine what type of transaction we're dealing with (regular or stake).
	// Then, be sure to set the tx tree correctly as it's possible a use submitted
	// it to the network with TxTreeUnknown.
	txType := stake.DetermineTxType(tx)
	if txType == stake.TxTypeRegular {
		tx.SetTree(dcrutil.TxTreeRegular)
	} else {
		tx.SetTree(dcrutil.TxTreeStake)
	}

	// Don't allow non-standard transactions if the network parameters
	// forbid their relaying.
	if !activeNetParams.RelayNonStdTxs {
		err := checkTransactionStandard(tx, txType, nextBlockHeight,
			mp.cfg.TimeSource, mp.cfg.Policy.MinRelayTxFee)
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

		if tx.MsgTx().TxOut[0].Value < sDiff {
			str := fmt.Sprintf("transaction %v has not enough funds "+
				"to meet stake difficuly (ticket diff %v < next diff %v)",
				txHash, tx.MsgTx().TxOut[0].Value, sDiff)
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
						tx.MsgTx().TxIn[1].PreviousOutPoint {
						ssGenAlreadyFound++
					}
				}
				if ssGenAlreadyFound > maxSSGensDoubleSpends {
					str := fmt.Sprintf("transaction %v in the pool "+
						"with more than %v ssgens",
						tx.MsgTx().TxIn[1].PreviousOutPoint,
						maxSSGensDoubleSpends)
					return nil, txRuleError(wire.RejectDuplicate, str)
				}
			}
		}

		if txType == stake.TxTypeSSRtx {
			for _, mpTx := range mp.pool {
				if mpTx.Type == stake.TxTypeSSRtx {
					if mpTx.Tx.MsgTx().TxIn[0].PreviousOutPoint ==
						tx.MsgTx().TxIn[0].PreviousOutPoint {
						str := fmt.Sprintf("transaction %v in the pool "+
							" as a ssrtx. Only one ssrtx allowed.",
							tx.MsgTx().TxIn[0].PreviousOutPoint)
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
		_, voteHeight, err := stake.SSGenBlockVotedOn(tx)
		if err != nil {
			return nil, err
		}

		if (int64(voteHeight) < nextBlockHeight-maximumVoteAgeDelta) &&
			!cfg.AllowOldVotes {
			str := fmt.Sprintf("transaction %v votes on old "+
				"block height of %v which is before the "+
				"current cutoff height of %v",
				tx.Sha(), voteHeight, nextBlockHeight-maximumVoteAgeDelta)
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
	for i, txIn := range tx.MsgTx().TxIn {
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
				txmpLog.Tracef("Transaction %v uses unknown input %v "+
					"and will be considered an orphan", txHash,
					txIn.PreviousOutPoint.Hash)
				continue
			}
			if entry.IsFullySpent() {
				txmpLog.Tracef("Transaction %v uses full spent input %v "+
					"and will be considered an orphan", txHash,
					txIn.PreviousOutPoint.Hash)
			}
		}
	}

	if len(missingParents) > 0 {
		return missingParents, nil
	}

	// Perform several checks on the transaction inputs using the invariant
	// rules in chain for what transactions are allowed into blocks.
	// Also returns the fees associated with the transaction which will be
	// used later.
	txFee, err := blockchain.CheckTransactionInputs(mp.subsidyCache,
		tx,
		nextBlockHeight,
		utxoView,
		false, // Don't check fraud proof; filled in by miner
		mp.cfg.ChainParams)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow transactions with non-standard inputs if the network
	// parameters forbid their relaying.
	if !activeNetParams.RelayNonStdTxs {
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
	serializedSize := int64(tx.MsgTx().SerializeSize())
	minFee := calcMinRequiredTxRelayFee(serializedSize,
		mp.cfg.Policy.MinRelayTxFee)
	if txType == stake.TxTypeRegular { // Non-stake only
		if serializedSize >= (defaultBlockPrioritySize-1000) && txFee < minFee {
			str := fmt.Sprintf("transaction %v has %v fees which is under "+
				"the required amount of %v", txHash, txFee,
				minFee)
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

		currentPriority := calcPriority(tx.MsgTx(), utxoView,
			nextBlockHeight)
		if currentPriority <= minHighPriority {
			str := fmt.Sprintf("transaction %v has insufficient "+
				"priority (%g <= %g)", txHash,
				currentPriority, minHighPriority)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	// Free-to-relay transactions are rate limited here to prevent
	// penny-flooding with tiny transactions as a form of attack.
	// This applies to non-stake transactions only.
	if rateLimit && txFee < minFee && txType == stake.TxTypeRegular {
		nowUnix := time.Now().Unix()
		// we decay passed data with an exponentially decaying ~10
		// minutes window - matches bitcoind handling.
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
		txmpLog.Tracef("rate limit: curTotal %v, nextTotal: %v, "+
			"limit %v", oldTotal, mp.pennyTotal,
			mp.cfg.Policy.FreeTxRelayLimit*10*1000)
	}

	// Set an absolute threshold for ticket rejection and obey it. Tickets
	// are treated differently in the mempool because they have a set
	// difficulty and generally a window in which they expire.
	//
	// This applies to tickets transactions only.
	minTicketFee := calcMinRequiredTxRelayFee(serializedSize, minTicketFee)
	if (txFee < minTicketFee) && txType == stake.TxTypeSStx {
		str := fmt.Sprintf("transaction %v has a %v fee which "+
			"is under the required threshold amount of %d", txHash, txFee,
			minTicketFee)
		return nil, txRuleError(wire.RejectInsufficientFee, str)
	}

	// Check whether allowHighFees is set to false (default), if so, then make sure
	// the current fee is sensible.  100 * above the minimum fee/kb seems to be a
	// reasonable amount to check.  If people would like to avoid this check
	// then they can AllowHighFees = true
	if !allowHighFees {
		maxFee := calcMinRequiredTxRelayFee(serializedSize*maxRelayFeeMultiplier,
			cfg.minRelayTxFee)
		if txFee > maxFee {
			err = fmt.Errorf("transaction %v has %v fee which is above the "+
				"allowHighFee check threshold amount of %v", txHash,
				txFee, maxFee)
			return nil, err
		}
	}

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = blockchain.ValidateTransactionScripts(tx, utxoView,
		txscript.StandardVerifyFlags, mp.cfg.SigCache)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Add to transaction pool.
	mp.addTransaction(utxoView, tx, txType, best.Height, txFee)

	// If it's an SSGen (vote), insert it into the list of
	// votes.
	if txType == stake.TxTypeSSGen {
		err := mp.InsertVote(tx)
		if err != nil {
			return nil, err
		}
	}

	// Insert the address into the mempool address index.
	for _, txOut := range tx.MsgTx().TxOut {
		// This function returns an error, but we don't really care
		// if the script was non-standard or otherwise malformed.
		mp.indexScriptAddressToTx(txOut.Version, txOut.PkScript, tx, txType)
	}

	txmpLog.Debugf("Accepted transaction %v (pool size: %v)", txHash,
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
func (mp *txMemPool) MaybeAcceptTransaction(tx *dcrutil.Tx, isNew, rateLimit bool) ([]*chainhash.Hash, error) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	return mp.maybeAcceptTransaction(tx, isNew, rateLimit, true)
}

// processOrphans is the internal function which implements the public
// ProcessOrphans.  See the comment for ProcessOrphans for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) processOrphans(hash *chainhash.Hash) []*dcrutil.Tx {
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
			orphanHash := tx.Sha()
			mp.removeOrphan(orphanHash)

			// Potentially accept the transaction into the
			// transaction pool.
			missingParents, err := mp.maybeAcceptTransaction(tx,
				true, true, true)
			if err != nil {
				// TODO: Remove orphans that depend on this
				// failed transaction.
				txmpLog.Debugf("Unable to move "+
					"orphan transaction %v to mempool: %v",
					tx.Sha(), err)
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
func (mp *txMemPool) PruneStakeTx(requiredStakeDifficulty, height int64) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	mp.pruneStakeTx(requiredStakeDifficulty, height)
}

func (mp *txMemPool) pruneStakeTx(requiredStakeDifficulty, height int64) {
	for _, tx := range mp.pool {
		txType := stake.DetermineTxType(tx.Tx)
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
func (mp *txMemPool) PruneExpiredTx(height int64) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	mp.pruneExpiredTx(height)
}

func (mp *txMemPool) pruneExpiredTx(height int64) {
	for _, tx := range mp.pool {
		if tx.Tx.MsgTx().Expiry != 0 {
			if height >= int64(tx.Tx.MsgTx().Expiry) {
				txmpLog.Debugf("Pruning expired transaction %v from the "+
					"mempool", tx.Tx.Sha())
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
func (mp *txMemPool) ProcessOrphans(hash *chainhash.Hash) []*dcrutil.Tx {
	mp.Lock()
	acceptedTxns := mp.processOrphans(hash)
	mp.Unlock()

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
func (mp *txMemPool) ProcessTransaction(tx *dcrutil.Tx, allowOrphan,
	rateLimit, allowHighFees bool) ([]*dcrutil.Tx, error) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()
	var err error
	defer func() {
		if err != nil {
			txmpLog.Tracef("Failed to process transaction %v: %s",
				tx.Sha(), err.Error())
		}
	}()

	txmpLog.Tracef("Processing transaction %v", tx.Sha())

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
		newTxs := mp.processOrphans(tx.Sha())
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
			"transaction %v", tx.Sha(), missingParents[0])
		return nil, txRuleError(wire.RejectDuplicate, str)
	}

	// Potentially add the orphan transaction to the orphan pool.
	err = mp.maybeAddOrphan(tx)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// Count returns the number of transactions in the main pool.  It does not
// include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) Count() int {
	mp.RLock()
	defer mp.RUnlock()

	return len(mp.pool)
}

// TxShas returns a slice of hashes for all of the transactions in the memory
// pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) TxShas() []*chainhash.Hash {
	mp.RLock()
	defer mp.RUnlock()

	hashes := make([]*chainhash.Hash, len(mp.pool))
	i := 0
	for hash := range mp.pool {
		hashCopy := hash
		hashes[i] = &hashCopy
		i++
	}

	return hashes
}

// TxDescs returns a slice of descriptors for all the transactions in the pool.
// The descriptors are to be treated as read only.
//
// This function is safe for concurrent access.
func (mp *txMemPool) TxDescs() []*mempoolTxDesc {
	mp.RLock()
	defer mp.RUnlock()

	descs := make([]*mempoolTxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = desc
		i++
	}

	return descs
}

// MiningDescs returns a slice of mining descriptors for all the transactions
// in the pool.
//
// This is part of the mining.TxSource interface implementation and is safe for
// concurrent access as required by the interface contract.
func (mp *txMemPool) MiningDescs() []*mining.TxDesc {
	mp.RLock()
	defer mp.RUnlock()

	descs := make([]*mining.TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = &desc.TxDesc
		i++
	}

	return descs
}

// LastUpdated returns the last time a transaction was added to or removed from
// the main pool.  It does not include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) LastUpdated() time.Time {
	return time.Unix(atomic.LoadInt64(&mp.lastUpdated), 0)
}

// CheckIfTxsExist checks a list of transaction hashes against the mempool
// and returns true if they all exist in the mempool, otherwise false.
//
// This function is safe for concurrent access.
func (mp *txMemPool) CheckIfTxsExist(hashes []chainhash.Hash) bool {
	mp.RLock()
	defer mp.RUnlock()

	inPool := true
	for _, h := range hashes {
		if _, exists := mp.pool[h]; !exists {
			inPool = false
			break
		}
	}

	return inPool
}

// newTxMemPool returns a new memory pool for validating and storing standalone
// transactions until they are mined into a block.
func newTxMemPool(cfg *mempoolConfig) *txMemPool {
	memPool := &txMemPool{
		cfg:           *cfg,
		pool:          make(map[chainhash.Hash]*mempoolTxDesc),
		orphans:       make(map[chainhash.Hash]*dcrutil.Tx),
		orphansByPrev: make(map[chainhash.Hash]map[chainhash.Hash]*dcrutil.Tx),
		outpoints:     make(map[wire.OutPoint]*dcrutil.Tx),
		votes:         make(map[chainhash.Hash][]*VoteTx),
		subsidyCache:  cfg.Chain.FetchSubsidyCache(),
	}

	/*
		TODO New address index
		if cfg.EnableAddrIndex {
			memPool.addrindex = make(map[string]map[chainhash.Hash]struct{})
		}
	*/
	return memPool
}
