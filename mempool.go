// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"container/list"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	// mempoolHeight is the height used for the "block" height field of the
	// contextual transaction information provided in a transaction store.
	mempoolHeight = 0x7fffffff

	// maxOrphanTransactions is the maximum number of orphan transactions
	// that can be queued.
	maxOrphanTransactions = 1000

	// maxOrphanTxSize is the maximum size allowed for orphan transactions.
	// This helps prevent memory exhaustion attacks from sending a lot of
	// of big orphans.
	maxOrphanTxSize = 5000

	// maxSigOpsPerTx is the maximum number of signature operations
	// in a single transaction we will relay or mine.  It is a fraction
	// of the max signature operations for a block.
	maxSigOpsPerTx = blockchain.MaxSigOpsPerBlock / 5

	// maxStandardTxSize is the maximum size allowed for transactions that
	// are considered standard and will therefore be relayed and considered
	// for mining.
	maxStandardTxSize = 100000

	// maxStandardSigScriptSize is the maximum size allowed for a
	// transaction input signature script to be considered standard.  This
	// value allows for a 15-of-15 CHECKMULTISIG pay-to-script-hash with
	// compressed keys.
	//
	// The form of the overall script is: OP_0 <15 signatures> OP_PUSHDATA2
	// <2 bytes len> [OP_15 <15 pubkeys> OP_15 OP_CHECKMULTISIG]
	//
	// For the p2sh script portion, each of the 15 compressed pubkeys are
	// 33 bytes (plus one for the OP_DATA_33 opcode), and the thus it totals
	// to (15*34)+3 = 513 bytes.  Next, each of the 15 signatures is a max
	// of 73 bytes (plus one for the OP_DATA_73 opcode).  Also, there is one
	// extra byte for the initial extra OP_0 push and 3 bytes for the
	// OP_PUSHDATA2 needed to specify the 513 bytes for the script push.
	// That brings the total to 1+(15*74)+3+513 = 1627.  This value also
	// adds a few extra bytes to provide a little buffer.
	// (1 + 15*74 + 3) + (15*34 + 3) + 23 = 1650
	maxStandardSigScriptSize = 1650

	// maxStandardMultiSigKeys is the maximum number of public keys allowed
	// in a multi-signature transaction output script for it to be
	// considered standard.
	maxStandardMultiSigKeys = 3

	// minTxRelayFeeMainNet is the minimum fee in atoms that is required for a
	// transaction to be treated as free for relay and mining purposes.  It
	// is also used to help determine if a transaction is considered dust
	// and as a base for calculating minimum required fees for larger
	// transactions.  This value is in Atom/1000 bytes.
	minTxRelayFeeMainNet = 1e5

	// minTxRelayFeeTestNet is the minimum relay fee for the Test and Simulation
	// networks.
	minTxRelayFeeTestNet = 1e3

	// minTxFeeForMempoolMainNet is the minimum fee in atoms that is required
	// for a transaction to enter the mempool on MainNet.
	minTxFeeForMempoolMainNet = 1e6

	// minTxFeeForMempoolMainNet is the minimum fee in atoms that is required
	// for a transaction to enter the mempool on TestNet or SimNet.
	minTxFeeForMempoolTestNet = 1e3

	// maxSSGensDoubleSpends is the maximum number of SSGen double spends
	// allowed in the pool.
	maxSSGensDoubleSpends = 64

	// heightDiffToPruneTicket is the number of blocks to pass by in terms
	// of height before old tickets are pruned.
	// TODO Set this based up the stake difficulty retargeting interval?
	heightDiffToPruneTicket = 288

	// heightDiffToPruneVotes is the number of blocks to pass by in terms
	// of height before SSGen relating to that block are pruned.
	heightDiffToPruneVotes = 10

	// maxNullDataOutputs is the maximum number of OP_RETURN null data
	// pushes in a transaction, after which it is considered non-standard.
	maxNullDataOutputs = 4
)

// TxDesc is a descriptor containing a transaction in the mempool and the
// metadata we store about it.
type TxDesc struct {
	Tx               *dcrutil.Tx  // Transaction.
	Type             stake.TxType // Transcation type.
	Added            time.Time    // Time when added to pool.
	Height           int64        // Blockheight when added to pool.
	Fee              int64        // Transaction fees.
	startingPriority float64      // Priority when added to the pool.
}

// GetType returns what TxType a given TxDesc is.
func (td *TxDesc) GetType() stake.TxType {
	return td.Type
}

// VoteTx is a struct describing a block vote (SSGen).
type VoteTx struct {
	SsgenHash chainhash.Hash // Vote
	SstxHash  chainhash.Hash // Ticket
	Vote      bool
}

// txMemPool is used as a source of transactions that need to be mined into
// blocks and relayed to other peers.  It is safe for concurrent access from
// multiple peers.
type txMemPool struct {
	sync.RWMutex
	server        *server
	pool          map[chainhash.Hash]*TxDesc
	orphans       map[chainhash.Hash]*dcrutil.Tx
	orphansByPrev map[chainhash.Hash]*list.List
	addrindex     map[string]map[chainhash.Hash]struct{} // maps address to txs
	outpoints     map[wire.OutPoint]*dcrutil.Tx

	// Votes on blocks.
	votes    map[chainhash.Hash][]*VoteTx
	votesMtx sync.Mutex

	lastUpdated   time.Time // last time pool was updated.
	pennyTotal    float64   // exponentially decaying total for penny spends.
	lastPennyUnix int64     // unix time of last ``penny spend''
}

// insertVote inserts a vote into the map of block votes.
// This function is safe for concurrent access.
func (mp *txMemPool) insertVote(ssgen *dcrutil.Tx) error {
	voteHash := ssgen.Sha()
	msgTx := ssgen.MsgTx()
	ticketHash := &msgTx.TxIn[1].PreviousOutPoint.Hash

	// Get the block it is voting on; here we're agnostic of height.
	blockHash, blockHeight, err := stake.GetSSGenBlockVotedOn(ssgen)
	if err != nil {
		return err
	}

	voteBits := stake.GetSSGenVoteBits(ssgen)
	vote := dcrutil.IsFlagSet16(voteBits, dcrutil.BlockValid)

	voteTx := &VoteTx{*voteHash, *ticketHash, vote}
	vts, exists := mp.votes[blockHash]

	// If there are currently no votes for this block,
	// start a new buffered slice and store it.
	if !exists {
		minrLog.Debugf("Accepted vote %v for block hash %v (height %v), "+
			"voting %v on the transaction tree",
			voteHash, blockHash, blockHeight, vote)

		slice := make([]*VoteTx, int(mp.server.chainParams.TicketsPerBlock),
			int(mp.server.chainParams.TicketsPerBlock))
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
	hashes := make([]chainhash.Hash, 0)
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
	minimumVotesRequired := uint16((mp.server.chainParams.TicketsPerBlock / 2) + 1)
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

// isDust returns whether or not the passed transaction output amount is
// considered dust or not.  Dust is defined in terms of the minimum transaction
// relay fee.  In particular, if the cost to the network to spend coins is more
// than 1/3 of the minimum transaction relay fee, it is considered dust.
func isDust(txOut *wire.TxOut, params *chaincfg.Params) bool {
	// The total serialized size consists of the output and the associated
	// input script to redeem it.  Since there is no input script
	// to redeem it yet, use the minimum size of a typical input script.
	//
	// Pay-to-pubkey-hash bytes breakdown:
	//
	//  Output to hash (38 bytes):
	//   2 script version, 8 value, 1 script len, 25 script
	//   [1 OP_DUP, 1 OP_HASH_160, 1 OP_DATA_20, 20 hash,
	//   1 OP_EQUALVERIFY, 1 OP_CHECKSIG]
	//
	//  Input with compressed pubkey (165 bytes):
	//   37 prev outpoint, 16 fraud proof, 1 script len,
	//   107 script [1 OP_DATA_72, 72 sig, 1 OP_DATA_33,
	//   33 compressed pubkey], 4 sequence
	//
	//  Input with uncompressed pubkey (198 bytes):
	//   37 prev outpoint,  16 fraud proof, 1 script len,
	//   139 script [1 OP_DATA_72, 72 sig, 1 OP_DATA_65,
	//   65 compressed pubkey], 4 sequence, 1 witness
	//   append
	//
	// Pay-to-pubkey bytes breakdown:
	//
	//  Output to compressed pubkey (46 bytes):
	//   2 script version, 8 value, 1 script len, 35 script
	//   [1 OP_DATA_33, 33 compressed pubkey, 1 OP_CHECKSIG]
	//
	//  Output to uncompressed pubkey (76 bytes):
	//   2 script version, 8 value, 1 script len, 67 script
	//   [1 OP_DATA_65, 65 pubkey, 1 OP_CHECKSIG]
	//
	//  Input (133 bytes):
	//   37 prev outpoint, 16 fraud proof, 1 script len, 73
	//   script [1 OP_DATA_72, 72 sig], 4 sequence, 1 witness
	//   append
	//
	// Theoretically this could examine the script type of the output script
	// and use a different size for the typical input script size for
	// pay-to-pubkey vs pay-to-pubkey-hash inputs per the above breakdowns,
	// but the only combinination which is less than the value chosen is
	// a pay-to-pubkey script with a compressed pubkey, which is not very
	// common.
	//
	// The most common scripts are pay-to-pubkey-hash, and as per the above
	// breakdown, the minimum size of a p2pkh input script is 165 bytes.  So
	// that figure is used.
	totalSize := txOut.SerializeSize() + 165

	// The output is considered dust if the cost to the network to spend the
	// coins is more than 1/3 of the minimum free transaction relay fee.
	// minFreeTxRelayFee is in Atom/KB, so multiply by 1000 to
	// convert to bytes.
	//
	// Using the typical values for a pay-to-pubkey-hash transaction from
	// the breakdown above and the default minimum free transaction relay
	// fee of 5000000, this equates to values less than 546 atoms being
	// considered dust.
	//
	// The following is equivalent to (value/totalSize) * (1/3) * 1000
	// without needing to do floating point math.
	var minTxRelayFee dcrutil.Amount
	switch {
	case params == &chaincfg.MainNetParams:
		minTxRelayFee = minTxRelayFeeMainNet
	case params == &chaincfg.MainNetParams:
		minTxRelayFee = minTxRelayFeeTestNet
	default:
		minTxRelayFee = minTxRelayFeeTestNet
	}
	return txOut.Value*1000/(3*int64(totalSize)) < int64(minTxRelayFee)
}

// checkPkScriptStandard performs a series of checks on a transaction ouput
// script (public key script) to ensure it is a "standard" public key script.
// A standard public key script is one that is a recognized form, and for
// multi-signature scripts, only contains from 1 to maxStandardMultiSigKeys
// public keys.
func checkPkScriptStandard(version uint16, pkScript []byte,
	scriptClass txscript.ScriptClass) error {
	// Only default Bitcoin-style script is standard except for
	// null data outputs.
	if version != wire.DefaultPkScriptVersion {
		str := fmt.Sprintf("versions other than default pkscript version " +
			"are currently non-standard except for provably unspendable " +
			"outputs")
		return txRuleError(wire.RejectNonstandard, str)
	}

	switch scriptClass {
	case txscript.MultiSigTy:
		numPubKeys, numSigs, err := txscript.CalcMultiSigStats(pkScript)
		if err != nil {
			str := fmt.Sprintf("multi-signature script parse "+
				"failure: %v", err)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// A standard multi-signature public key script must contain
		// from 1 to maxStandardMultiSigKeys public keys.
		if numPubKeys < 1 {
			str := "multi-signature script with no pubkeys"
			return txRuleError(wire.RejectNonstandard, str)
		}
		if numPubKeys > maxStandardMultiSigKeys {
			str := fmt.Sprintf("multi-signature script with %d "+
				"public keys which is more than the allowed "+
				"max of %d", numPubKeys, maxStandardMultiSigKeys)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// A standard multi-signature public key script must have at
		// least 1 signature and no more signatures than available
		// public keys.
		if numSigs < 1 {
			return txRuleError(wire.RejectNonstandard,
				"multi-signature script with no signatures")
		}
		if numSigs > numPubKeys {
			str := fmt.Sprintf("multi-signature script with %d "+
				"signatures which is more than the available "+
				"%d public keys", numSigs, numPubKeys)
			return txRuleError(wire.RejectNonstandard, str)
		}

	case txscript.NonStandardTy:
		return txRuleError(wire.RejectNonstandard,
			"non-standard script form")
	}

	return nil
}

// checkTransactionStandard performs a series of checks on a transaction to
// ensure it is a "standard" transaction.  A standard transaction is one that
// conforms to several additional limiting cases over what is considered a
// "sane" transaction such as having a version in the supported range, being
// finalized, conforming to more stringent size constraints, having scripts
// of recognized forms, and not containing "dust" outputs (those that are
// so small it costs more to process them than they are worth).
func (mp *txMemPool) checkTransactionStandard(tx *dcrutil.Tx, txType stake.TxType,
	height int64) error {
	msgTx := tx.MsgTx()

	// The transaction must be a currently supported version.
	if !wire.IsSupportedMsgTxVersion(msgTx) {
		str := fmt.Sprintf("transaction version %d is not in the "+
			"valid range of %d-%d", msgTx.Version, 1,
			wire.TxVersion)
		return txRuleError(wire.RejectNonstandard, str)
	}

	// The transaction must be finalized to be standard and therefore
	// considered for inclusion in a block.
	adjustedTime := mp.server.timeSource.AdjustedTime()
	if !blockchain.IsFinalizedTransaction(tx, height, adjustedTime) {
		return txRuleError(wire.RejectNonstandard,
			"transaction is not finalized")
	}

	// Since extremely large transactions with a lot of inputs can cost
	// almost as much to process as the sender fees, limit the maximum
	// size of a transaction.  This also helps mitigate CPU exhaustion
	// attacks.
	serializedLen := msgTx.SerializeSize()
	if serializedLen > maxStandardTxSize {
		str := fmt.Sprintf("transaction size of %v is larger than max "+
			"allowed size of %v", serializedLen, maxStandardTxSize)
		return txRuleError(wire.RejectNonstandard, str)
	}

	for i, txIn := range msgTx.TxIn {
		// Each transaction input signature script must not exceed the
		// maximum size allowed for a standard transaction.  See
		// the comment on maxStandardSigScriptSize for more details.
		sigScriptLen := len(txIn.SignatureScript)
		if sigScriptLen > maxStandardSigScriptSize {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script size of %d bytes is large than max "+
				"allowed size of %d bytes", i, sigScriptLen,
				maxStandardSigScriptSize)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// Each transaction input signature script must only contain
		// opcodes which push data onto the stack.
		if !txscript.IsPushOnlyScript(txIn.SignatureScript) {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script is not push only", i)
			return txRuleError(wire.RejectNonstandard, str)
		}

	}

	// None of the output public key scripts can be a non-standard script or
	// be "dust" (except when the script is a null data script).
	numNullDataOutputs := 0
	for i, txOut := range msgTx.TxOut {
		scriptClass := txscript.GetScriptClass(txOut.Version, txOut.PkScript)
		err := checkPkScriptStandard(txOut.Version, txOut.PkScript, scriptClass)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained.  When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = wire.RejectNonstandard
			}
			str := fmt.Sprintf("transaction output %d: %v", i, err)
			return txRuleError(rejectCode, str)
		}

		// Accumulate the number of outputs which only carry data.  For
		// all other script types, ensure the output value is not
		// "dust".
		if scriptClass == txscript.NullDataTy {
			numNullDataOutputs++
		} else if isDust(txOut, mp.server.chainParams) &&
			txType != stake.TxTypeSStx {
			str := fmt.Sprintf("transaction output %d: payment "+
				"of %d is dust", i, txOut.Value)
			return txRuleError(wire.RejectDust, str)
		}
	}

	// A standard transaction must not have more than one output script that
	// only carries data. However, certain types of standard stake transactions
	// are allowed to have multiple OP_RETURN outputs, so only throw an error here
	// if the tx is TxTypeRegular.
	if numNullDataOutputs > maxNullDataOutputs && txType == stake.TxTypeRegular {
		str := "more than one transaction output in a nulldata script for a " +
			"regular type tx"
		return txRuleError(wire.RejectNonstandard, str)
	}

	return nil
}

// checkInputsStandard performs a series of checks on a transaction's inputs
// to ensure they are "standard".  A standard transaction input is one that
// that consumes the expected number of elements from the stack and that number
// is the same as the output script pushes.  This help prevent resource
// exhaustion attacks by "creative" use of scripts that are super expensive to
// process like OP_DUP OP_CHECKSIG OP_DROP repeated a large number of times
// followed by a final OP_TRUE.
// Decred TODO: I think this is okay, but we'll see with simnet.
func checkInputsStandard(tx *dcrutil.Tx,
	txType stake.TxType,
	txStore blockchain.TxStore) error {
	// NOTE: The reference implementation also does a coinbase check here,
	// but coinbases have already been rejected prior to calling this
	// function so no need to recheck.

	for i, txIn := range tx.MsgTx().TxIn {
		if i == 0 && txType == stake.TxTypeSSGen {
			continue
		}

		// It is safe to elide existence and index checks here since
		// they have already been checked prior to calling this
		// function.
		prevOut := txIn.PreviousOutPoint
		originTx := txStore[prevOut.Hash].Tx.MsgTx()
		originPkScript := originTx.TxOut[prevOut.Index].PkScript

		// Calculate stats for the script pair.
		scriptInfo, err := txscript.CalcScriptInfo(txIn.SignatureScript,
			originPkScript, true)
		if err != nil {
			str := fmt.Sprintf("transaction input #%d script parse "+
				"failure: %v", i, err)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// A negative value for expected inputs indicates the script is
		// non-standard in some way.
		if scriptInfo.ExpectedInputs < 0 {
			str := fmt.Sprintf("transaction input #%d expects %d "+
				"inputs", i, scriptInfo.ExpectedInputs)
			return txRuleError(wire.RejectNonstandard, str)
		}

		// The script pair is non-standard if the number of available
		// inputs does not match the number of expected inputs.
		if scriptInfo.NumInputs != scriptInfo.ExpectedInputs {
			str := fmt.Sprintf("transaction input #%d expects %d "+
				"inputs, but referenced output script provides "+
				"%d", i, scriptInfo.ExpectedInputs,
				scriptInfo.NumInputs)
			return txRuleError(wire.RejectNonstandard, str)
		}
	}

	return nil
}

// calcMinRequiredTxRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
func calcMinRequiredTxRelayFee(serializedSize int64,
	params *chaincfg.Params) int64 {
	// Calculate the minimum fee for a transaction to be allowed into the
	// mempool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee).  minTxRelayFee is in Atom/KB, so
	// divide the transaction size by 1000 to convert to kilobytes.  Also,
	// integer division is used so fees only increase on full kilobyte
	// boundaries.
	var minTxRelayFee dcrutil.Amount
	switch {
	case params == &chaincfg.MainNetParams:
		minTxRelayFee = minTxRelayFeeMainNet
	case params == &chaincfg.MainNetParams:
		minTxRelayFee = minTxRelayFeeTestNet
	default:
		minTxRelayFee = minTxRelayFeeTestNet
	}
	minFee := (1 + serializedSize/1000) * int64(minTxRelayFee)

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	if minFee < 0 || minFee > dcrutil.MaxAmount {
		minFee = dcrutil.MaxAmount
	}

	return minFee
}

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
			for e := orphans.Front(); e != nil; e = e.Next() {
				if e.Value.(*dcrutil.Tx) == tx {
					orphans.Remove(e)
					break
				}
			}

			// Remove the map entry altogether if there are no
			// longer any orphans which depend on it.
			if orphans.Len() == 0 {
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
	if len(mp.orphans)+1 > cfg.MaxOrphanTxs && cfg.MaxOrphanTxs > 0 {
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
		if mp.orphansByPrev[originTxHash] == nil {
			mp.orphansByPrev[originTxHash] = list.New()
		}
		mp.orphansByPrev[originTxHash].PushBack(tx)
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
	// maxOrphanTxSize * cfg.MaxOrphanTxs (which is ~5MB using the default
	// values at the time this comment was written).
	serializedLen := tx.MsgTx().SerializeSize()
	if serializedLen > maxOrphanTxSize {
		str := fmt.Sprintf("orphan transaction size of %d bytes is "+
			"larger than max allowed size of %d bytes",
			serializedLen, maxOrphanTxSize)
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

// removeTransaction is the internal function which implements the public
// RemoveTransaction.  See the comment for RemoveTransaction for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) removeTransaction(tx *dcrutil.Tx, removeRedeemers bool) {
	txmpLog.Tracef("Removing transaction %v", tx.Sha())

	txHash := tx.Sha()
	if removeRedeemers {
		// Remove any transactions which rely on this one.
		txType := stake.DetermineTxType(tx)
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
		for _, txIn := range txDesc.Tx.MsgTx().TxIn {
			delete(mp.outpoints, txIn.PreviousOutPoint)
		}
		delete(mp.pool, *txHash)
		mp.lastUpdated = time.Now()
	}
}

// RemoveTransaction removes the passed transaction from the mempool. If
// removeRedeemers flag is set, any transactions that redeem outputs from the
// removed transaction will also be removed recursively from the mempool, as
// they would otherwise become orphan.
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
func (mp *txMemPool) addTransaction(
	tx *dcrutil.Tx,
	txType stake.TxType,
	height,
	fee int64) {
	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.
	mp.pool[*tx.Sha()] = &TxDesc{
		Tx:     tx,
		Type:   txType,
		Added:  time.Now(),
		Height: height,
		Fee:    fee,
	}
	for _, txIn := range tx.MsgTx().TxIn {
		mp.outpoints[txIn.PreviousOutPoint] = tx
	}
	mp.lastUpdated = time.Now()
}

// fetchReferencedOutputScripts looks up and returns all the scriptPubKeys
// referenced by inputs of the passed transaction.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) fetchReferencedOutputScripts(tx *dcrutil.Tx) ([][]byte,
	error) {
	txStore, err := mp.fetchInputTransactions(tx)
	if err != nil || len(txStore) == 0 {
		return nil, err
	}

	previousOutScripts := make([][]byte, 0, len(tx.MsgTx().TxIn))
	for _, txIn := range tx.MsgTx().TxIn {
		outPoint := txIn.PreviousOutPoint
		if txStore[outPoint.Hash].Err == nil {
			referencedOutPoint :=
				txStore[outPoint.Hash].Tx.MsgTx().TxOut[outPoint.Index]
			previousOutScripts =
				append(previousOutScripts, referencedOutPoint.PkScript)
		}
	}
	return previousOutScripts, nil
}

// indexScriptByAddress alters our address index by indexing the payment address
// encoded by the passed scriptPubKey to the passed transaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) indexScriptAddressToTx(pkVersion uint16, pkScript []byte,
	tx *dcrutil.Tx) error {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(pkVersion, pkScript,
		activeNetParams.Params)
	if err != nil {
		txmpLog.Errorf("Unable to extract encoded addresses from script "+
			"for addrindex: %v", err)
		return err
	}

	for _, addr := range addresses {
		if mp.addrindex[addr.EncodeAddress()] == nil {
			mp.addrindex[addr.EncodeAddress()] = make(map[chainhash.Hash]struct{})
		}
		mp.addrindex[addr.EncodeAddress()][*tx.Sha()] = struct{}{}
	}

	return nil
}

// calcInputValueAge is a helper function used to calculate the input age of
// a transaction.  The input age for a txin is the number of confirmations
// since the referenced txout multiplied by its output value.  The total input
// age is the sum of this value for each txin.  Any inputs to the transaction
// which are currently in the mempool and hence not mined into a block yet,
// contribute no additional input age to the transaction.
func calcInputValueAge(txDesc *TxDesc, txStore blockchain.TxStore,
	nextBlockHeight int64) float64 {
	var totalInputAge float64
	for _, txIn := range txDesc.Tx.MsgTx().TxIn {
		originHash := &txIn.PreviousOutPoint.Hash
		originIndex := txIn.PreviousOutPoint.Index

		// Don't attempt to accumulate the total input age if the txIn
		// in question doesn't exist.
		if txData, exists := txStore[*originHash]; exists && txData.Tx != nil {
			// Inputs with dependencies currently in the mempool
			// have their block height set to a special constant.
			// Their input age should computed as zero since their
			// parent hasn't made it into a block yet.
			var inputAge int64
			if txData.BlockHeight == mempoolHeight {
				inputAge = 0
			} else {
				inputAge = nextBlockHeight - txData.BlockHeight
			}

			// Sum the input value times age.
			originTxOut := txData.Tx.MsgTx().TxOut[originIndex]
			inputValue := originTxOut.Value
			totalInputAge += float64(inputValue * inputAge)
		}
	}

	return totalInputAge
}

// minInt is a helper function to return the minimum of two ints.  This avoids
// a math import and the need to cast to floats.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// calcPriority returns a transaction priority given a transaction and the sum
// of each of its input values multiplied by their age (# of confirmations).
// Thus, the final formula for the priority is:
// sum(inputValue * inputAge) / adjustedTxSize
func calcPriority(tx *dcrutil.Tx, inputValueAge float64) float64 {
	// In order to encourage spending multiple old unspent transaction
	// outputs thereby reducing the total set, don't count the constant
	// overhead for each input as well as enough bytes of the signature
	// script to cover a pay-to-script-hash redemption with a compressed
	// pubkey.  This makes additional inputs free by boosting the priority
	// of the transaction accordingly.  No more incentive is given to avoid
	// encouraging gaming future transactions through the use of junk
	// outputs.  This is the same logic used in the reference
	// implementation.
	//
	// The constant overhead for a txin is 41 bytes since the previous
	// outpoint is 36 bytes + 4 bytes for the sequence + 1 byte the
	// signature script length.
	//
	// A compressed pubkey pay-to-script-hash redemption with a maximum len
	// signature is of the form:
	// [OP_DATA_73 <73-byte sig> + OP_DATA_35 + {OP_DATA_33
	// <33 byte compresed pubkey> + OP_CHECKSIG}]
	//
	// Thus 1 + 73 + 1 + 1 + 33 + 1 = 110
	overhead := 0
	for _, txIn := range tx.MsgTx().TxIn {
		// Max inputs + size can't possibly overflow here.
		overhead += 41 + minInt(110, len(txIn.SignatureScript))
	}

	serializedTxSize := tx.MsgTx().SerializeSize()
	if overhead >= serializedTxSize {
		return 0.0
	}

	return inputValueAge / float64(serializedTxSize-overhead)
}

// StartingPriority calculates the priority of this tx descriptor's underlying
// transaction relative to when it was first added to the mempool.  The result
// is lazily computed and then cached for subsequent function calls.
func (txD *TxDesc) StartingPriority(txStore blockchain.TxStore) float64 {
	// Return our cached result.
	if txD.startingPriority != float64(0) {
		return txD.startingPriority
	}

	// Compute our starting priority caching the result.
	inputAge := calcInputValueAge(txD, txStore, txD.Height)
	txD.startingPriority = calcPriority(txD.Tx, inputAge)

	return txD.startingPriority
}

// CurrentPriority calculates the current priority of this tx descriptor's
// underlying transaction relative to the next block height.
func (txD *TxDesc) CurrentPriority(txStore blockchain.TxStore,
	nextBlockHeight int64) float64 {
	inputAge := calcInputValueAge(txD, txStore, nextBlockHeight)
	return calcPriority(txD.Tx, inputAge)
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
		int(mp.server.chainParams.TicketsPerBlock/2) {
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

// fetchInputTransactions fetches the input transactions referenced by the
// passed transaction.  First, it fetches from the main chain, then it tries to
// fetch any missing inputs from the transaction pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *txMemPool) fetchInputTransactions(tx *dcrutil.Tx) (blockchain.TxStore,
	error) {
	tv := mp.IsTxTreeValid(mp.server.blockManager.chainState.newestHash)
	txStore, err := mp.server.blockManager.blockChain.FetchTransactionStore(tx,
		tv)
	if err != nil {
		return nil, err
	}

	// Attempt to populate any missing inputs from the transaction pool.
	for _, txD := range txStore {
		if txD.Err == database.ErrTxShaMissing || txD.Tx == nil {
			if poolTxDesc, exists := mp.pool[*txD.Hash]; exists {
				poolTx := poolTxDesc.Tx
				txD.Tx = poolTx
				txD.BlockHeight = mempoolHeight
				txD.BlockIndex = wire.NullBlockIndex
				txD.Spent = make([]bool, len(poolTx.MsgTx().TxOut))
				txD.Err = nil
			}
		}
	}

	return txStore, nil
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

// FilterTransactionsByAddress returns all transactions currently in the
// mempool that either create an output to the passed address or spend a
// previously created ouput to the address.
func (mp *txMemPool) FilterTransactionsByAddress(
	addr dcrutil.Address) ([]*dcrutil.Tx, error) {
	// Protect concurrent access.
	mp.RLock()
	defer mp.RUnlock()

	if txs, exists := mp.addrindex[addr.EncodeAddress()]; exists {
		addressTxs := make([]*dcrutil.Tx, 0, len(txs))
		for txHash := range txs {
			if tx, exists := mp.pool[txHash]; exists {
				addressTxs = append(addressTxs, tx.Tx)
			}
		}
		return addressTxs, nil
	}

	return nil, fmt.Errorf("address does not have any transactions in the pool")
}

// This function detects whether or not a transaction is a stake transaction and,
// if it is, also returns the type of stake transaction.
func detectTxType(tx *dcrutil.Tx) stake.TxType {
	// Check to see if it's an SStx
	if pass, _ := stake.IsSStx(tx); pass {
		return stake.TxTypeSStx
	}

	// Check to see if it's an SSGen
	if pass, _ := stake.IsSSGen(tx); pass {
		return stake.TxTypeSSGen
	}

	// Check to see if it's an SSGen
	if pass, _ := stake.IsSSRtx(tx); pass {
		return stake.TxTypeSSRtx
	}

	// If it's none of these things, it's a malformed or non-standard stake tx
	// which will be rejected during other checks or a regular tx.
	return stake.TxTypeRegular
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
	rateLimit bool) ([]*chainhash.Hash, error) {
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
	err := blockchain.CheckTransactionSanity(tx, mp.server.chainParams)
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
	// will be mined into the next block at best, so it's height is at least
	// one more than the current height.
	_, curHeight, err := mp.server.db.NewestSha()
	if err != nil {
		// This is an unexpected error so don't turn it into a rule
		// error.
		return nil, err
	}
	nextBlockHeight := curHeight + 1

	// Determine what type of transaction we're dealing with (regular or stake).
	// Then, be sure to set the tx tree correctly as it's possible a use submitted
	// it to the network with TxTreeUnknown.
	txType := detectTxType(tx)
	if txType == stake.TxTypeRegular {
		tx.SetTree(dcrutil.TxTreeRegular)
	} else {
		tx.SetTree(dcrutil.TxTreeStake)
	}

	// Don't allow non-standard transactions if the network parameters
	// forbid their relaying.
	if !activeNetParams.RelayNonStdTxs {
		err := mp.checkTransactionStandard(tx, txType, nextBlockHeight)
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

	isSSGen, _ := stake.IsSSGen(tx)
	isSSRtx, _ := stake.IsSSRtx(tx)
	if isSSGen || isSSRtx {
		if isSSGen {
			ssGenAlreadyFound := 0
			for _, mpTx := range mp.pool {
				if mpTx.GetType() == stake.TxTypeSSGen {
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

		if isSSRtx {
			for _, mpTx := range mp.pool {
				if mpTx.GetType() == stake.TxTypeSSRtx {
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
	// Fetch all of the transactions referenced by the inputs to this
	// transaction.  This function also attempts to fetch the transaction
	// itself to be used for detecting a duplicate transaction without
	// needing to do a separate lookup.
	txStore, err := mp.fetchInputTransactions(tx)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow the transaction if it exists in the main chain and is not
	// not already fully spent.
	if txD, exists := txStore[*txHash]; exists && txD.Err == nil {
		for _, isOutputSpent := range txD.Spent {
			if !isOutputSpent {
				return nil, txRuleError(wire.RejectDuplicate,
					"transaction already exists")
			}
		}
	}
	delete(txStore, *txHash)

	// Transaction is an orphan if any of the inputs don't exist.
	var missingParents []*chainhash.Hash
	for _, txD := range txStore {
		if txD.Err == database.ErrTxShaMissing {
			missingParents = append(missingParents, txD.Hash)
		}
	}

	if len(missingParents) > 0 {
		return missingParents, nil
	}

	// Perform several checks on the transaction inputs using the invariant
	// rules in chain for what transactions are allowed into blocks.
	// Also returns the fees associated with the transaction which will be
	// used later.
	txFee, err := blockchain.CheckTransactionInputs(tx,
		nextBlockHeight,
		txStore,
		false, // Don't check fraud proof; filled in by miner
		mp.server.chainParams)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow transactions with non-standard inputs if the network
	// parameters forbid their relaying.
	if !activeNetParams.RelayNonStdTxs {
		err := checkInputsStandard(tx, txType, txStore)
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
	numSigOps, err := blockchain.CountP2SHSigOps(tx, false, isSSGen, txStore)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	numSigOps += blockchain.CountSigOps(tx, false, isSSGen)
	if numSigOps > maxSigOpsPerTx {
		str := fmt.Sprintf("transaction %v has too many sigops: %d > %d",
			txHash, numSigOps, maxSigOpsPerTx)
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
	serializedSize := int64(tx.MsgTx().SerializeSize())
	minFee := calcMinRequiredTxRelayFee(serializedSize, mp.server.chainParams)
	if txType == stake.TxTypeRegular { // Non-stake only
		if serializedSize >= (defaultBlockPrioritySize-1000) && txFee < minFee {
			str := fmt.Sprintf("transaction %v has %d fees which is under "+
				"the required amount of %d", txHash, txFee,
				minFee)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	// Set an absolute threshold for rejection and obey it. This prevents
	// unnecessary transaction spam. We only enforce this for transactions
	// we expect to have fees. Votes are mandatory, so we skip the check
	// on them.
	var feeThreshold int64
	switch {
	case mp.server.chainParams == &chaincfg.MainNetParams:
		feeThreshold = minTxFeeForMempoolMainNet
	case mp.server.chainParams == &chaincfg.TestNetParams:
		feeThreshold = minTxFeeForMempoolTestNet
	default:
		feeThreshold = minTxFeeForMempoolTestNet
	}
	feePerKB := float64(txFee) / (float64(serializedSize) / 1000.0)
	if (float64(feePerKB) < float64(feeThreshold)) &&
		txType != stake.TxTypeSSGen {
		str := fmt.Sprintf("transaction %v has %d fees per kb which "+
			"is under the required threshold amount of %d", txHash, feePerKB,
			feeThreshold)
		return nil, txRuleError(wire.RejectInsufficientFee, str)
	}

	// Require that free transactions have sufficient priority to be mined
	// in the next block.  Transactions which are being added back to the
	// memory pool from blocks that have been disconnected during a reorg
	// are exempted.
	if isNew && !cfg.NoRelayPriority && txFee < minFee &&
		txType == stake.TxTypeRegular {
		txD := &TxDesc{
			Tx:     tx,
			Added:  time.Now(),
			Height: curHeight,
			Fee:    txFee,
		}
		currentPriority := txD.CurrentPriority(txStore, nextBlockHeight)
		if currentPriority <= minHighPriority {
			str := fmt.Sprintf("transaction %v has insufficient "+
				"priority (%g <= %g)", txHash,
				currentPriority, minHighPriority)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
	}

	// Free-to-relay transactions are rate limited here to prevent
	// penny-flooding with tiny transactions as a form of attack.
	if rateLimit && txFee < minFee && txType == stake.TxTypeRegular {
		nowUnix := time.Now().Unix()
		// we decay passed data with an exponentially decaying ~10
		// minutes window - matches bitcoind handling.
		mp.pennyTotal *= math.Pow(1.0-1.0/600.0,
			float64(nowUnix-mp.lastPennyUnix))
		mp.lastPennyUnix = nowUnix

		// Are we still over the limit?
		if mp.pennyTotal >= cfg.FreeTxRelayLimit*10*1000 {
			str := fmt.Sprintf("transaction %v has been rejected "+
				"by the rate limiter due to low fees", txHash)
			return nil, txRuleError(wire.RejectInsufficientFee, str)
		}
		oldTotal := mp.pennyTotal

		mp.pennyTotal += float64(serializedSize)
		txmpLog.Tracef("rate limit: curTotal %v, nextTotal: %v, "+
			"limit %v", oldTotal, mp.pennyTotal,
			cfg.FreeTxRelayLimit*10*1000)
	}

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = blockchain.ValidateTransactionScripts(tx, txStore,
		txscript.StandardVerifyFlags)
	if err != nil {
		if cerr, ok := err.(blockchain.RuleError); ok {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Add to transaction pool.
	mp.addTransaction(tx, txType, curHeight, txFee)

	// If it's an SSGen (vote), insert it into the list of
	// votes.
	if txType == stake.TxTypeSSGen {
		err := mp.InsertVote(tx)
		if err != nil {
			return nil, err
		}
	}

	txmpLog.Debugf("Accepted transaction %v (pool size: %v)", txHash,
		len(mp.pool))

	if mp.server.rpcServer != nil {
		// Notify websocket clients about mempool transactions.
		mp.server.rpcServer.ntfnMgr.NotifyMempoolTx(tx, isNew)

		// Potentially notify any getblocktemplate long poll clients
		// about stale block templates due to the new transaction.
		mp.server.rpcServer.gbtWorkState.NotifyMempoolTx(mp.lastUpdated)
	}

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
func (mp *txMemPool) MaybeAcceptTransaction(tx *dcrutil.Tx, isNew,
	rateLimit bool) ([]*chainhash.Hash, error) {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	return mp.maybeAcceptTransaction(tx, isNew, rateLimit)
}

// processOrphans is the internal function which implements the public
// ProcessOrphans.  See the comment for ProcessOrphans for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *txMemPool) processOrphans(hash *chainhash.Hash) {
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

		var enext *list.Element
		for e := orphans.Front(); e != nil; e = enext {
			enext = e.Next()
			tx := e.Value.(*dcrutil.Tx)

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
			missingParents, err := mp.maybeAcceptTransaction(tx, true, true)
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

			// Generate and relay the inventory vector for the
			// newly accepted transaction.
			iv := wire.NewInvVect(wire.InvTypeTx, tx.Sha())
			mp.server.RelayInventory(iv, tx)

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
}

// PruneStakeTx is the function which is called everytime a new block is
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
		txType := detectTxType(tx.Tx)
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
// This function is safe for concurrent access.
func (mp *txMemPool) ProcessOrphans(hash *chainhash.Hash) {
	mp.Lock()
	mp.processOrphans(hash)
	mp.Unlock()
}

// ProcessTransaction is the main workhorse for handling insertion of new
// free-standing transactions into the memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) ProcessTransaction(tx *dcrutil.Tx, allowOrphan,
	rateLimit bool) error {
	// Protect concurrent access.
	mp.Lock()
	defer mp.Unlock()

	txmpLog.Tracef("Processing transaction %v", tx.Sha())

	// Potentially accept the transaction to the memory pool.
	var isOrphan bool
	_, err := mp.maybeAcceptTransaction(tx, true, rateLimit)
	if err != nil {
		return err
	}

	if !isOrphan {
		// Generate the inventory vector and relay it.
		iv := wire.NewInvVect(wire.InvTypeTx, tx.Sha())
		mp.server.RelayInventory(iv, tx)

		// Accept any orphan transactions that depend on this
		// transaction (they are no longer orphans) and repeat for those
		// accepted transactions until there are no more.
		mp.processOrphans(tx.Sha())
	} else {
		// The transaction is an orphan (has inputs missing).  Reject
		// it if the flag to allow orphans is not set.
		if !allowOrphan {
			// NOTE: RejectDuplicate is really not an accurate
			// reject code here, but it matches the reference
			// implementation and there isn't a better choice due
			// to the limited number of reject codes.  Missing
			// inputs is assumed to mean they are already spent
			// which is not really always the case.
			var buf bytes.Buffer
			buf.WriteString("transaction spends unknown inputs; includes " +
				"inputs: \n")
			lenIn := len(tx.MsgTx().TxIn)
			for i, txIn := range tx.MsgTx().TxIn {
				str := fmt.Sprintf("[%v]: %v, %v, %v",
					i,
					txIn.PreviousOutPoint.Hash,
					txIn.PreviousOutPoint.Index,
					txIn.PreviousOutPoint.Tree)
				buf.WriteString(str)
				if i != lenIn-1 {
					buf.WriteString("\n")
				}
			}
			txmpLog.Debugf("%v", buf.String())

			return txRuleError(wire.RejectDuplicate,
				"transaction spends unknown inputs")
		}

		// Potentially add the orphan transaction to the orphan pool.
		err := mp.maybeAddOrphan(tx)
		if err != nil {
			return err
		}
	}

	return nil
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
func (mp *txMemPool) TxDescs() []*TxDesc {
	mp.RLock()
	defer mp.RUnlock()

	descs := make([]*TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = desc
		i++
	}

	return descs
}

// LastUpdated returns the last time a transaction was added to or removed from
// the main pool.  It does not include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *txMemPool) LastUpdated() time.Time {
	mp.RLock()
	defer mp.RUnlock()

	return mp.lastUpdated
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
func newTxMemPool(server *server) *txMemPool {
	memPool := &txMemPool{
		server:        server,
		pool:          make(map[chainhash.Hash]*TxDesc),
		orphans:       make(map[chainhash.Hash]*dcrutil.Tx),
		orphansByPrev: make(map[chainhash.Hash]*list.List),
		outpoints:     make(map[wire.OutPoint]*dcrutil.Tx),
		votes:         make(map[chainhash.Hash][]*VoteTx),
	}

	if !cfg.NoAddrIndex {
		memPool.addrindex = make(map[string]map[chainhash.Hash]struct{})
	}
	return memPool
}
