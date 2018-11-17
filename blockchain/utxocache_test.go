package blockchain

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btclog"
	"github.com/btcsuite/btcutil"
	"github.com/davecgh/go-spew/spew"
)

var (
	// opTrueScript is simply a public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	opTrueScript = []byte{txscript.OP_TRUE}

	// lowFee is a single satoshi and exists to make the test code more
	// readable.
	lowFee = btcutil.Amount(1)
)

// uniqueOpReturnScript returns a standard provably-pruneable OP_RETURN script
// with a random uint64 encoded as the data.
func uniqueOpReturnScript() []byte {
	rand, err := wire.RandomUint64()
	if err != nil {
		panic(err)
	}

	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data[0:8], rand)

	builder := txscript.NewScriptBuilder()
	script, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// solveBlock attempts to find a nonce which makes the passed block header hash
// to a value less than the target difficulty.  When a successful solution is
// found true is returned and the nonce field of the passed header is updated
// with the solution.  False is returned if no solution exists.
func solveBlock(header *wire.BlockHeader) bool {
	// sbResult is used by the solver goroutines to send results.
	type sbResult struct {
		found bool
		nonce uint32
	}

	// solver accepts a block header and a nonce range to test. It is
	// intended to be run as a goroutine.
	targetDifficulty := CompactToBig(header.Bits)
	quit := make(chan bool)
	results := make(chan sbResult)
	solver := func(hdr wire.BlockHeader, startNonce, stopNonce uint32) {
		// We need to modify the nonce field of the header, so make sure
		// we work with a copy of the original header.
		for i := startNonce; i >= startNonce && i <= stopNonce; i++ {
			select {
			case <-quit:
				return
			default:
				hdr.Nonce = i
				hash := hdr.BlockHash()
				if HashToBig(&hash).Cmp(
					targetDifficulty) <= 0 {

					results <- sbResult{true, i}
					return
				}
			}
		}
		results <- sbResult{false, 0}
	}

	startNonce := uint32(1)
	stopNonce := uint32(math.MaxUint32)
	numCores := uint32(runtime.NumCPU())
	noncesPerCore := (stopNonce - startNonce) / numCores
	for i := uint32(0); i < numCores; i++ {
		rangeStart := startNonce + (noncesPerCore * i)
		rangeStop := startNonce + (noncesPerCore * (i + 1)) - 1
		if i == numCores-1 {
			rangeStop = stopNonce
		}
		go solver(*header, rangeStart, rangeStop)
	}
	for i := uint32(0); i < numCores; i++ {
		result := <-results
		if result.found {
			close(quit)
			header.Nonce = result.nonce
			return true
		}
	}

	return false
}

// spendableOut represents a transaction output that is spendable along with
// additional metadata such as the block its in and how much it pays.
type spendableOut struct {
	prevOut wire.OutPoint
	amount  btcutil.Amount
}

// makeSpendableOutForTx returns a spendable output for the given transaction
// and transaction output index within the transaction.
func makeSpendableOutForTx(tx *wire.MsgTx, txOutIndex uint32) spendableOut {
	return spendableOut{
		prevOut: wire.OutPoint{
			Hash:  tx.TxHash(),
			Index: txOutIndex,
		},
		amount: btcutil.Amount(tx.TxOut[txOutIndex].Value),
	}
}

// addBlock adds a block to the blockchain that succeeds prev.  The blocks spends
// all the provided spendable outputs.  The new block is returned, together with
// the new spendable outputs created in the block.
//
// Panics on errors.
func addBlock(chain *BlockChain, prev *btcutil.Block, spends []*spendableOut) (*btcutil.Block, []*spendableOut) {
	blockHeight := prev.Height() + 1
	txns := make([]*wire.MsgTx, 0, 1+len(spends))

	// Create and add coinbase tx.
	coinbaseScript, err := txscript.NewScriptBuilder().
		AddInt64(int64(blockHeight)).
		AddInt64(int64(0)).Script()
	if err != nil {
		panic(err)
	}
	cb := wire.NewMsgTx(1)
	cb.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex),
		Sequence:        wire.MaxTxInSequenceNum,
		SignatureScript: coinbaseScript,
	})
	cb.AddTxOut(&wire.TxOut{
		Value:    CalcBlockSubsidy(blockHeight, chain.chainParams),
		PkScript: opTrueScript,
	})
	txns = append(txns, cb)

	// Spend all txs to be spent.
	for _, spend := range spends {
		spendTx := wire.NewMsgTx(1)
		spendTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: spend.prevOut,
			Sequence:         wire.MaxTxInSequenceNum,
			SignatureScript:  nil,
		})
		spendTx.AddTxOut(wire.NewTxOut(int64(spend.amount-lowFee),
			opTrueScript))
		// Add a random (prunable) OP_RETURN output to make the txid unique.
		spendTx.AddTxOut(wire.NewTxOut(0, uniqueOpReturnScript()))

		cb.TxOut[0].Value += int64(lowFee)
		txns = append(txns, spendTx)
	}

	// Create spendable outs to return at the end.
	outs := make([]*spendableOut, len(txns))
	for i, tx := range txns {
		out := makeSpendableOutForTx(tx, 0)
		outs[i] = &out
	}

	// Build the block.

	// Use a timestamp that is one second after the previous block unless
	// this is the first block in which case the current time is used.
	var ts time.Time
	if blockHeight == 1 {
		ts = time.Unix(time.Now().Unix(), 0)
	} else {
		ts = prev.MsgBlock().Header.Timestamp.Add(time.Second)
	}

	// Calculate merkle root.
	utilTxns := make([]*btcutil.Tx, 0, len(txns))
	for _, tx := range txns {
		utilTxns = append(utilTxns, btcutil.NewTx(tx))
	}
	merkles := BuildMerkleTreeStore(utilTxns, false)

	block := btcutil.NewBlock(&wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    1,
			PrevBlock:  *prev.Hash(),
			MerkleRoot: *merkles[len(merkles)-1],
			Bits:       chain.chainParams.PowLimitBits,
			Timestamp:  ts,
			Nonce:      0, // To be solved.
		},
		Transactions: txns,
	})
	block.SetHeight(blockHeight)

	// Solve the block.
	if !solveBlock(&block.MsgBlock().Header) {
		panic(fmt.Sprintf("Unable to solve block at height %d", blockHeight))
	}

	_, _, err = chain.ProcessBlock(block, BFNone)
	if err != nil {
		panic(err)
	}

	return block, outs
}

// assertConsistencyState asserts the utxo consistency states of the blockchain.
func assertConsistencyState(t *testing.T, chain *BlockChain, code byte, hash *chainhash.Hash) {
	var actualCode byte
	var actualHash *chainhash.Hash
	err := chain.db.View(func(dbTx database.Tx) (err error) {
		actualCode, actualHash, err = dbFetchUtxoStateConsistency(dbTx)
		return
	})
	if err != nil {
		t.Fatalf("Error fetching utxo state consistency: %v", err)
	}
	if actualCode != code {
		t.Fatalf("Unexpected consistency code: %d instead of %d",
			actualCode, code)
	}
	if !actualHash.IsEqual(hash) {
		t.Fatalf("Unexpected consistency hash: %v instead of %v",
			actualHash, hash)
	}
}

func parseOutpointKey(b []byte) wire.OutPoint {
	op := wire.OutPoint{}
	copy(op.Hash[:], b[:chainhash.HashSize])
	idx, _ := deserializeVLQ(b[chainhash.HashSize:])
	op.Index = uint32(idx)
	return op
}

// assertNbEntriesOnDisk asserts that the total number of utxo entries on the
// disk is equal to the given expected number.
func assertNbEntriesOnDisk(t *testing.T, chain *BlockChain, expectedNumber int) {
	t.Log("Asserting nb entries on disk...")
	var nb int
	err := chain.db.View(func(dbTx database.Tx) error {
		cursor := dbTx.Metadata().Bucket(utxoSetBucketName).Cursor()
		nb = 0
		for b := cursor.First(); b; b = cursor.Next() {
			nb++
			entry, err := deserializeUtxoEntry(cursor.Value())
			if err != nil {
				t.Fatalf("Failed to deserialize entry: %v", err)
			}
			outpoint := parseOutpointKey(cursor.Key())
			t.Log(outpoint.String())
			t.Log(spew.Sdump(entry))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Error fetching utxo entries: %v", err)
	}
	if nb != expectedNumber {
		t.Fatalf("Expected %d elements in the UTXO set, but found %d",
			expectedNumber, nb)
	}
	t.Log("Assertion done")
}

func init() {
	//TODO(stevenroose) remove
	UseLogger(btclog.NewBackend(os.Stdout).Logger("TST"))
	log.SetLevel(btclog.LevelTrace)
}

// utxoCacheTestChain creates a test BlockChain to be used for utxo cache tests.
// It uses the regression test parameters, a coin matutiry of 1 block and sets
// the cache size limit to 10 MiB.
func utxoCacheTestChain(testName string) (*BlockChain, *chaincfg.Params, func()) {
	params := chaincfg.RegressionNetParams
	chain, tearDown, err := chainSetup(testName, &params)
	if err != nil {
		panic(fmt.Sprintf("error loading blockchain with database: %v", err))
	}

	chain.TstSetCoinbaseMaturity(1)
	chain.utxoCache.maxTotalMemoryUsage = 10 * 1024 * 1024

	return chain, &params, tearDown
}

func TestUtxoCache_SimpleFlush(t *testing.T) {
	t.Parallel()

	chain, params, tearDown := utxoCacheTestChain("TestUtxoCache_SimpleFlush")
	defer tearDown()
	cache := chain.utxoCache
	tip := btcutil.NewBlock(params.GenesisBlock)

	// The chainSetup init triggered write of consistency status of genesis.
	assertConsistencyState(t, chain, ucsConsistent, params.GenesisHash)
	assertNbEntriesOnDisk(t, chain, 0)

	// LastFlushHash starts with genesis.
	if cache.lastFlushHash != *params.GenesisHash {
		t.Fatalf("lastFlushHash before first flush expected to be "+
			"genesis block hash, instead was %v", cache.lastFlushHash)
	}

	// First, add 10 utxos without flushing.
	for i := 0; i < 10; i++ {
		tip, _ = addBlock(chain, tip, nil)
	}
	if len(cache.cachedEntries) != 10 {
		t.Fatalf("Expected 10 entries, has %d instead", len(cache.cachedEntries))
	}

	// All elements should be fresh and modified.
	for outpoint, elem := range cache.cachedEntries {
		if elem == nil {
			t.Fatalf("Unexpected nil entry found for %v", outpoint)
		}
		if elem.packedFlags&tfModified == 0 {
			t.Fatal("Entry should be marked mofified")
		}
		if elem.packedFlags&tfFresh == 0 {
			t.Fatal("Entry should be marked fresh")
		}
	}

	// Not flushed yet.
	assertConsistencyState(t, chain, ucsConsistent, params.GenesisHash)
	assertNbEntriesOnDisk(t, chain, 0)

	// Flush.
	if err := chain.FlushCachedState(FlushRequired); err != nil {
		t.Fatalf("unexpected error while flushing cache: %v", err)
	}
	if len(cache.cachedEntries) != 0 {
		t.Fatalf("Expected 0 entries, has %d instead", len(cache.cachedEntries))
	}
	assertConsistencyState(t, chain, ucsConsistent, tip.Hash())
	assertNbEntriesOnDisk(t, chain, 10)
}

func TestUtxoCache_ThresholdPeriodicFlush(t *testing.T) {
	t.Parallel()

	chain, params, tearDown := utxoCacheTestChain("TestUtxoCache_ThresholdPeriodicFlush")
	defer tearDown()
	cache := chain.utxoCache
	tip := btcutil.NewBlock(params.GenesisBlock)

	// Set the limit to the size of 10 empty elements.  This will trigger
	// flushing when adding 10 non-empty elements.
	cache.maxTotalMemoryUsage = (*UtxoEntry)(nil).memoryUsage() * 10

	// Add 10 elems and let it exceed the threshold.
	var flushedAt *chainhash.Hash
	for i := 0; i < 10; i++ {
		tip, _ = addBlock(chain, tip, nil)
		if len(cache.cachedEntries) == 0 {
			flushedAt = tip.Hash()
		}
	}

	// Should have flushed in the meantime.
	if flushedAt == nil {
		t.Fatal("should have flushed")
	}
	assertConsistencyState(t, chain, ucsConsistent, flushedAt)
	if len(cache.cachedEntries) >= 10 {
		t.Fatalf("Expected less than 10 entries, has %d instead",
			len(cache.cachedEntries))
	}

	// Make sure flushes on periodic.
	cache.maxTotalMemoryUsage = (90*cache.totalMemoryUsage())/100 - 1
	if err := chain.FlushCachedState(FlushPeriodic); err != nil {
		t.Fatalf("unexpected error while flushing cache: %v", err)
	}
	if len(cache.cachedEntries) != 0 {
		t.Fatalf("Expected 0 entries, has %d instead", len(cache.cachedEntries))
	}
	assertConsistencyState(t, chain, ucsConsistent, tip.Hash())
	assertNbEntriesOnDisk(t, chain, 10)
}

func TestUtxoCache_Reorg(t *testing.T) {
	t.Parallel()

	chain, params, tearDown := utxoCacheTestChain("TestUtxoCache_Reorg")
	defer tearDown()
	tip := btcutil.NewBlock(params.GenesisBlock)

	// Create base blocks 1 and 2 that will not be reorged.
	// Spend the outputs of block 1.
	var emptySpendableOuts []*spendableOut
	b1, spendableOuts1 := addBlock(chain, tip, emptySpendableOuts)
	b2, spendableOuts2 := addBlock(chain, b1, spendableOuts1)
	t.Log(spew.Sdump(spendableOuts2))
	//                 db       cache
	// block 1:                  stxo
	// block 2:                  utxo

	// Commit the two base blocks to DB
	if err := chain.FlushCachedState(FlushRequired); err != nil {
		t.Fatalf("unexpected error while flushing cache: %v", err)
	}
	//                 db       cache
	// block 1:       stxo
	// block 2:       utxo
	assertConsistencyState(t, chain, ucsConsistent, b2.Hash())
	assertNbEntriesOnDisk(t, chain, len(spendableOuts2))

	// Add blocks 3 and 4 that will be orphaned.
	// Spend the outputs of block 2 and 3a.
	b3a, spendableOuts3 := addBlock(chain, b2, spendableOuts2)
	addBlock(chain, b3a, spendableOuts3)
	//                 db       cache
	// block 1:       stxo
	// block 2:       utxo       stxo      << these are left spent without flush
	// ---
	// block 3a:                 stxo
	// block 4a:                 utxo

	// Build an alternative chain of blocks 3 and 4 + new 5th, spending none of the outputs
	b3b, altSpendableOuts3 := addBlock(chain, b2, nil)
	b4b, altSpendableOuts4 := addBlock(chain, b3b, nil)
	b5b, altSpendableOuts5 := addBlock(chain, b4b, nil)
	totalSpendableOuts := spendableOuts2[:]
	totalSpendableOuts = append(totalSpendableOuts, altSpendableOuts3...)
	totalSpendableOuts = append(totalSpendableOuts, altSpendableOuts4...)
	totalSpendableOuts = append(totalSpendableOuts, altSpendableOuts5...)
	t.Log(spew.Sdump(totalSpendableOuts))
	//                 db       cache
	// block 1:       stxo
	// block 2:       utxo       utxo     << now they should become utxo
	// ---
	// block 3b:                 utxo
	// block 4b:                 utxo
	// block 5b:                 utxo

	if err := chain.FlushCachedState(FlushRequired); err != nil {
		t.Fatalf("unexpected error while flushing cache: %v", err)
	}
	//                 db       cache
	// block 1:       stxo
	// block 2:       utxo
	// ---
	// block 3b:      utxo
	// block 4b:      utxo
	// block 5b:      utxo
	assertConsistencyState(t, chain, ucsConsistent, b5b.Hash())
	assertNbEntriesOnDisk(t, chain, len(totalSpendableOuts))
}
