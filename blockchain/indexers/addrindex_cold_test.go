// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb" // driver registration
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// setupAddrIndexDB creates a fresh ffldb, stores the given blocks hot, and
// builds the address index for them (writing witness-relative offsets, exactly
// as ConnectBlock does during normal chain operation). It returns the db, the
// stored blocks, and the AddrIndex instance.
func setupAddrIndexDB(t *testing.T, rawBlocks [][]byte) (database.DB, []*btcutil.Block, *AddrIndex) {
	t.Helper()
	dbPath := t.TempDir()
	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	idx := NewAddrIndex(db, &chaincfg.MainNetParams)

	if err := db.Update(func(tx database.Tx) error {
		meta := tx.Metadata()
		if _, err := meta.CreateBucketIfNotExists(indexTipsBucketName); err != nil {
			return err
		}
		// The address index shares the block-ID index with the transaction
		// index. In production the Manager creates both indexers together and
		// TxIndex.Create makes these buckets; here we create them directly so
		// AddrIndex.ConnectBlock's dbFetchBlockIDByHash has a place to read
		// from, and we seed the block-ID entries ourselves.
		if _, err := meta.CreateBucketIfNotExists(idByHashIndexBucketName); err != nil {
			return err
		}
		if _, err := meta.CreateBucketIfNotExists(hashByIDIndexBucketName); err != nil {
			return err
		}
		return idx.Create(tx)
	}); err != nil {
		t.Fatalf("create addrindex buckets: %v", err)
	}
	if err := idx.Init(); err != nil {
		t.Fatalf("Init addrindex: %v", err)
	}

	var blocks []*btcutil.Block
	for i, raw := range rawBlocks {
		block, err := btcutil.NewBlockFromBytes(raw)
		if err != nil {
			t.Fatalf("block %d: NewBlockFromBytes: %v", i, err)
		}
		block.SetHeight(int32(i))
		// Assign a block ID (1-based, mirroring TxIndex.ConnectBlock) and write
		// the id<->hash mapping before connecting the address index, which
		// reads the ID via dbFetchBlockIDByHash.
		blockID := uint32(i + 1)
		// AddrIndex.indexBlock walks the spent outputs (stxos) to extract
		// input addresses. The fixture blocks are standalone mainnet blocks,
		// not a chain we have validated, so we do not have the real prior
		// output scripts. Build a dummy stxos slice with one entry per
		// non-coinbase input (in block order) and an empty PkScript so
		// indexPkScript extracts no input addresses — only output-address
		// entries are created, which is exactly the case we exercise and
		// verify (searchrawtransactions by address). This matches the
		// production behavior when the spend journal has been pruned.
		stxos := dummyStxosForBlock(block)
		if err := db.Update(func(tx database.Tx) error {
			if err := dbPutBlockIDIndexEntry(tx, block.Hash(), blockID); err != nil {
				return err
			}
			if err := tx.StoreBlock(block); err != nil {
				return err
			}
			// ConnectBlock directly (skip dbIndexConnectBlock's tip-chain
			// assertion) — the fixture blocks are not a connected sequence
			// from genesis. With dummy stxos (empty PkScripts) only
			// output-address entries are written, which is sufficient to
			// exercise the offset rewrite for output addresses — the case
			// that matters for searchrawtransactions.
			return idx.ConnectBlock(tx, block, stxos)
		}); err != nil {
			t.Fatalf("block %d: StoreBlock+Connect: %v", i, err)
		}
		blocks = append(blocks, block)
	}

	return db, blocks, idx
}

// blockOutputAddresses extracts every address that appears in the OUTPUT scripts
// of the block's transactions. These are the addresses for which ConnectBlock
// (with nil/dummy stxos) writes addrindex entries, and thus the addresses we
// can round-trip through TxRegionsForAddress. It returns a map keyed by the
// address's string encoding (deterministic — ExtractPkScriptAddrs returns a
// fresh pointer per call, so keying by the address.Address interface would
// treat the same address as distinct entries) to a representative address
// object and the set of tx indices in the block that pay to it.
func blockOutputAddresses(t *testing.T, block *btcutil.Block,
	params *chaincfg.Params) map[string]addrTxIdxs {

	t.Helper()
	out := make(map[string]addrTxIdxs)
	for txIdx, tx := range block.Transactions() {
		for _, txOut := range tx.MsgTx().TxOut {
			_, addrs, _, err := txscript.ExtractPkScriptAddrs(txOut.PkScript, params)
			if err != nil || len(addrs) == 0 {
				continue
			}
			for _, a := range addrs {
				key := a.String()
				entry := out[key]
				entry.addr = a
				entry.txIdxs = appendUnique(entry.txIdxs, txIdx)
				out[key] = entry
			}
		}
	}
	return out
}

// addrTxIdxs pairs a representative address object with the block tx indices
// that pay to it.
type addrTxIdxs struct {
	addr   address.Address
	txIdxs []int
}

// appendUnique appends txIdx to s only if it is not already the last element
// (indexBlock dedups the same way — consecutive duplicate tx indices are
// skipped because a single tx can pay the same address in multiple outputs).
func appendUnique(s []int, txIdx int) []int {
	if n := len(s); n > 0 && s[n-1] == txIdx {
		return s
	}
	return append(s, txIdx)
}

// dummyStxosForBlock returns a SpentTxOut slice with one entry per non-coinbase
// input in the block (in the order indexBlock walks them: tx-by-tx, input-by-
// input, skipping the coinbase). Each entry has an empty PkScript so
// indexPkScript extracts no input addresses — only output-address entries are
// produced. This lets us build the address index for a standalone fixture
// block whose real prior outputs we do not have.
func dummyStxosForBlock(block *btcutil.Block) []blockchain.SpentTxOut {
	txs := block.Transactions()
	if len(txs) == 0 {
		return nil
	}
	var stxos []blockchain.SpentTxOut
	for txIdx, tx := range txs {
		if txIdx == 0 {
			continue // coinbase has no real inputs
		}
		for range tx.MsgTx().TxIn {
			stxos = append(stxos, blockchain.SpentTxOut{PkScript: nil})
		}
	}
	return stxos
}

// addrTxRegionsAndBytes looks up the block regions for addr via the addrindex
// and reads each region's tx bytes through FetchBlockRegions — exactly the path
// handleSearchRawTransactions uses. It returns the (txHash, txBytes) pairs.
func addrTxRegionsAndBytes(t *testing.T, db database.DB, idx *AddrIndex, addr address.Address) []txBytesEntry {
	t.Helper()
	var entries []txBytesEntry
	err := db.View(func(dbTx database.Tx) error {
		regions, _, err := idx.TxRegionsForAddress(dbTx, addr, 0, 1<<20, false)
		if err != nil {
			return err
		}
		if len(regions) == 0 {
			return nil
		}
		serialized, err := dbTx.FetchBlockRegions(regions)
		if err != nil {
			return err
		}
		for i, raw := range serialized {
			var msgTx wire.MsgTx
			if err := msgTx.Deserialize(bytes.NewReader(raw)); err != nil {
				return err
			}
			h := msgTx.TxHash()
			entries = append(entries, txBytesEntry{
				hash:    &h,
				blkHash: regions[i].Hash,
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("addrTxRegionsAndBytes %s: %v", addr, err)
	}
	return entries
}

type txBytesEntry struct {
	hash    *chainhash.Hash
	blkHash *chainhash.Hash
}

// TestAddrIndexColdCompactionRewrite is the address-index analogue of
// TestTxIndexColdCompactionRewrite. It exercises the path that the
// searchrawtransactions RPC and btcwallet's address-history rescan use:
// TxRegionsForAddress -> FetchBlockRegions, against a block that has been
// compacted from the hot tier (witness-included) to the cold tier
// (witness-stripped).
//
// Steps:
//  1. Store a post-SegWit mainnet block hot with addrindex enabled.
//  2. For every address paid by the block, verify the address's tx regions
//     round-trip to the correct tx hashes while the block is hot (baseline).
//  3. Compact the block to cold.
//  4. Rewrite the addrindex offsets to stripped-relative.
//  5. Re-verify every address's tx regions still resolve to the SAME tx hashes,
//     now reading from the stripped cold block.
//
// Without step 4, step 5 fails for any address paid by a segwit transaction
// that appears after a witness-bearing tx in the block: the witness-relative
// offsets stored by ConnectBlock point past the real tx boundaries in the
// stripped block, so FetchBlockRegions reads wrong bytes and the derived tx
// hash does not match.
func TestAddrIndexColdCompactionRewrite(t *testing.T) {
	fixtures := loadColdFixturesForIndexer(t)
	if len(fixtures) == 0 {
		t.Skip("no fixtures")
	}

	var raw []byte
	for _, f := range fixtures {
		if hasWitnessData(t, f) {
			raw = f
			break
		}
	}
	if raw == nil {
		t.Skip("no witness-containing fixture")
	}

	params := &chaincfg.MainNetParams
	db, blocks, idx := setupAddrIndexDB(t, [][]byte{raw})
	defer db.Close()
	block := blocks[0]

	addrs := blockOutputAddresses(t, block, params)
	if len(addrs) == 0 {
		t.Skip("fixture block has no parseable output addresses")
	}
	t.Logf("fixture block has %d distinct output address(es)", len(addrs))

	// Step 2: baseline — every address's tx regions resolve to the expected
	// tx hashes while the block is hot.
	expected := collectExpectedAddrTxns(t, db, idx, addrs, block, params, "hot baseline")

	// Step 3: compact to cold.
	hash := block.Hash()
	if err := db.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	// Step 4: rewrite addrindex offsets to stripped-relative.
	if err := db.Update(func(tx database.Tx) error {
		return idx.RewriteTxOffsetsForColdCompaction(tx, block, dummyStxosForBlock(block))
	}); err != nil {
		t.Fatalf("RewriteTxOffsetsForColdCompaction: %v", err)
	}

	// Step 5: every address's tx regions STILL resolve to the same tx hashes,
	// now reading from the stripped cold block.
	got := collectExpectedAddrTxns(t, db, idx, addrs, block, params, "cold (after rewrite)")

	// The set of (address -> tx hashes) must be identical before and after.
	compareAddrTxns(t, expected, got)
}

// TestAddrIndexRewriteWithStrippedRefetchLeavesStaleOffsets is the regression
// for the production age-out bug: after CompactBlockToCold, FetchBlock returns
// stripped bytes (pending cold). Passing that re-parsed block to
// RewriteTxOffsetsForColdCompaction makes oldToNew an identity map keyed by
// stripped offsets, so witness-relative stored entries are not updated.
// searchrawtransactions then reads stale offsets into the cold block.
func TestAddrIndexRewriteWithStrippedRefetchLeavesStaleOffsets(t *testing.T) {
	fixtures := loadColdFixturesForIndexer(t)
	var raw []byte
	for _, f := range fixtures {
		if hasWitnessData(t, f) {
			raw = f
			break
		}
	}
	if raw == nil {
		t.Skip("no witness-containing fixture")
	}

	params := &chaincfg.MainNetParams
	db, blocks, idx := setupAddrIndexDB(t, [][]byte{raw})
	defer db.Close()
	block := blocks[0]
	addrs := blockOutputAddresses(t, block, params)
	if len(addrs) == 0 {
		t.Skip("no parseable output addresses")
	}

	hash := block.Hash()
	if err := db.Update(func(tx database.Tx) error {
		cc := tx.(database.ColdCompactor)
		if err := cc.CompactBlockToCold(hash); err != nil {
			return err
		}
		// Simulate the buggy chain.go re-fetch after scheduling compaction.
		strippedBytes, err := tx.FetchBlock(hash)
		if err != nil {
			return err
		}
		strippedBlock, err := btcutil.NewBlockFromBytes(strippedBytes)
		if err != nil {
			return err
		}
		return idx.RewriteTxOffsetsForColdCompaction(tx, strippedBlock,
			dummyStxosForBlock(block))
	}); err != nil {
		t.Fatalf("compact+stripped-rewrite: %v", err)
	}

	atLeastOneFailed := false
	for key, entry := range addrs {
		var regionsFailed bool
		err := db.View(func(dbTx database.Tx) error {
			regions, _, err := idx.TxRegionsForAddress(dbTx, entry.addr, 0, 1<<20, false)
			if err != nil {
				return err
			}
			if len(regions) == 0 {
				regionsFailed = true
				return nil
			}
			serialized, err := dbTx.FetchBlockRegions(regions)
			if err != nil {
				regionsFailed = true
				return nil
			}
			wantSet := make(map[chainhash.Hash]struct{}, len(entry.txIdxs))
			for _, txIdx := range entry.txIdxs {
				wantSet[*block.Transactions()[txIdx].Hash()] = struct{}{}
			}
			if len(serialized) != len(wantSet) {
				regionsFailed = true
				return nil
			}
			for _, raw := range serialized {
				var msgTx wire.MsgTx
				if err := msgTx.Deserialize(bytes.NewReader(raw)); err != nil {
					regionsFailed = true
					return nil
				}
				h := msgTx.TxHash()
				if _, ok := wantSet[h]; !ok {
					regionsFailed = true
					return nil
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("view %s: %v", key, err)
		}
		if regionsFailed {
			atLeastOneFailed = true
			t.Logf("addr %s: stale offsets after stripped-refetch rewrite", key)
		}
	}
	if !atLeastOneFailed {
		t.Fatal("expected stripped-refetch rewrite to leave stale offsets " +
			"for at least one address; bug may not be exercisable")
	}
}

// TestAddrIndexRewriteFullBlockSameTxAsCompact is the corrected production
// path: CompactBlockToCold then Rewrite with the ORIGINAL full in-memory
// block (never re-fetch). Offsets must round-trip after commit.
func TestAddrIndexRewriteFullBlockSameTxAsCompact(t *testing.T) {
	fixtures := loadColdFixturesForIndexer(t)
	var raw []byte
	for _, f := range fixtures {
		if hasWitnessData(t, f) {
			raw = f
			break
		}
	}
	if raw == nil {
		t.Skip("no witness-containing fixture")
	}

	params := &chaincfg.MainNetParams
	db, blocks, idx := setupAddrIndexDB(t, [][]byte{raw})
	defer db.Close()
	block := blocks[0]
	addrs := blockOutputAddresses(t, block, params)
	if len(addrs) == 0 {
		t.Skip("no parseable output addresses")
	}

	expected := collectExpectedAddrTxns(t, db, idx, addrs, block, params, "hot")
	hash := block.Hash()
	if err := db.Update(func(tx database.Tx) error {
		if err := tx.(database.ColdCompactor).CompactBlockToCold(hash); err != nil {
			return err
		}
		return idx.RewriteTxOffsetsForColdCompaction(tx, block,
			dummyStxosForBlock(block))
	}); err != nil {
		t.Fatalf("compact+full-rewrite: %v", err)
	}
	got := collectExpectedAddrTxns(t, db, idx, addrs, block, params, "cold")
	compareAddrTxns(t, expected, got)
}

// TestAddrIndexColdCompactionWithoutRewrite demonstrates the addrindex bug:
// after compaction but WITHOUT the offset rewrite, at least one address's tx
// region resolves to wrong bytes (a tx hash that is not in the block, or a
// deserialization error). This is the searchrawtransactions corruption path.
func TestAddrIndexColdCompactionWithoutRewrite(t *testing.T) {
	fixtures := loadColdFixturesForIndexer(t)
	if len(fixtures) == 0 {
		t.Skip("no fixtures")
	}

	var raw []byte
	for _, f := range fixtures {
		if hasWitnessData(t, f) {
			raw = f
			break
		}
	}
	if raw == nil {
		t.Skip("no witness-containing fixture")
	}

	params := &chaincfg.MainNetParams
	db, blocks, idx := setupAddrIndexDB(t, [][]byte{raw})
	defer db.Close()
	block := blocks[0]

	addrs := blockOutputAddresses(t, block, params)
	if len(addrs) == 0 {
		t.Skip("fixture block has no parseable output addresses")
	}

	// Baseline hot.
	expected := collectExpectedAddrTxns(t, db, idx, addrs, block, params, "hot baseline")

	// Compact WITHOUT rewriting.
	hash := block.Hash()
	if err := db.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	// Now attempt to read each address's tx regions. At least one must produce
	// a tx hash not in the block or fail to deserialize — proving the
	// unrewritten offsets are wrong for the stripped block.
	blockTxIDs := make(map[chainhash.Hash]struct{}, len(block.Transactions()))
	for _, tx := range block.Transactions() {
		blockTxIDs[*tx.Hash()] = struct{}{}
	}

	atLeastOneFailed := false
	for _, info := range addrs {
		addr := info.addr
		err := db.View(func(dbTx database.Tx) error {
			regions, _, err := idx.TxRegionsForAddress(dbTx, addr, 0, 1<<20, false)
			if err != nil {
				return err
			}
			serialized, err := dbTx.FetchBlockRegions(regions)
			if err != nil {
				return err
			}
			for _, raw := range serialized {
				var msgTx wire.MsgTx
				if err := msgTx.Deserialize(bytes.NewReader(raw)); err != nil {
					return err
				}
				h := msgTx.TxHash()
				if _, ok := blockTxIDs[h]; !ok {
					return errHashMismatch{expected: chainhash.Hash{}, got: h}
				}
			}
			return nil
		})
		if err != nil {
			atLeastOneFailed = true
			t.Logf("address %s: correctly failed without rewrite: %v", addr, err)
		}
	}
	if !atLeastOneFailed {
		t.Fatal("expected at least one address's tx region to fail the " +
			"cold round-trip without the offset rewrite, but all passed — " +
			"the bug may not be exercisable with this fixture")
	}

	// Keep expected used so the linter doesn't complain about the baseline
	// being collected but not compared in this negative test.
	_ = expected
}

// collectExpectedAddrTxns returns, for each address, the set of tx hashes that
// its addrindex entries resolve to at the current DB state. stage labels the
// state for error messages.
func collectExpectedAddrTxns(t *testing.T, db database.DB, idx *AddrIndex,
	addrs map[string]addrTxIdxs, block *btcutil.Block,
	params *chaincfg.Params, stage string) map[string]map[chainhash.Hash]struct{} {

	t.Helper()
	out := make(map[string]map[chainhash.Hash]struct{}, len(addrs))
	for key, info := range addrs {
		addr := info.addr
		entries := addrTxRegionsAndBytes(t, db, idx, addr)
		got := make(map[chainhash.Hash]struct{}, len(entries))
		for _, e := range entries {
			got[*e.hash] = struct{}{}
		}
		// Sanity: the block hash on every region must be this block.
		bh := block.Hash()
		for _, e := range entries {
			if !e.blkHash.IsEqual(bh) {
				t.Errorf("[%s] address %s: region block hash %s != block %s",
					stage, addr, e.blkHash, bh)
			}
		}
		// While hot, every resolved tx must be one of the expected tx indices.
		if stage == "hot baseline" {
			for h := range got {
				found := false
				for _, txIdx := range info.txIdxs {
					if block.Transactions()[txIdx].Hash().IsEqual(&h) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("[%s] address %s: resolved tx %s is not among "+
						"the block txs that pay this address",
						stage, addr, h)
				}
			}
			// Also confirm we found at least the expected count.
			if len(got) < len(info.txIdxs) {
				t.Errorf("[%s] address %s: resolved %d txs but expected at "+
					"least %d (output occurrences)",
					stage, addr, len(got), len(info.txIdxs))
			}
		}
		out[key] = got
	}
	return out
}

// compareAddrTxns asserts that the cold (post-rewrite) address->tx-set is
// identical to the hot baseline. Any difference means the rewrite corrupted or
// lost addrindex entries — searchrawtransactions would return wrong results.
func compareAddrTxns(t *testing.T, hot, cold map[string]map[chainhash.Hash]struct{}) {
	t.Helper()
	if len(hot) != len(cold) {
		t.Fatalf("address count changed across compaction: hot=%d cold=%d",
			len(hot), len(cold))
	}
	for key, hotTxns := range hot {
		coldTxns, ok := cold[key]
		if !ok {
			t.Errorf("address %s present hot but missing cold after rewrite", key)
			continue
		}
		if len(hotTxns) != len(coldTxns) {
			t.Errorf("address %s: tx count changed across compaction: "+
				"hot=%d cold=%d", key, len(hotTxns), len(coldTxns))
			continue
		}
		for h := range hotTxns {
			if _, ok := coldTxns[h]; !ok {
				t.Errorf("address %s: tx %s present hot but missing cold "+
					"after rewrite — searchrawtransactions would lose "+
					"this tx for the address", key, h)
			}
		}
		for h := range coldTxns {
			if _, ok := hotTxns[h]; !ok {
				t.Errorf("address %s: tx %s absent hot but present cold "+
					"after rewrite — searchrawtransactions would return a "+
					"spurious tx for the address", key, h)
			}
		}
	}
}

// Ensure blockchain is referenced for the SpentTxOut type used by the
// RewriteTxOffsetsForColdCompaction signature in the parent package.
var _ = blockchain.SpentTxOut{}

// TestAddrIndexRewriteNilStxosRefused verifies that cold-compaction rewrite
// refuses a nil spend journal when the block has non-coinbase inputs. Leaving
// stale input-address offsets would make searchrawtransactions return garbage;
// the chain layer skips compaction when the journal is missing, and this method
// also hard-errors if called directly with nil stxos.
func TestAddrIndexRewriteNilStxosRefused(t *testing.T) {
	fixtures := loadColdFixturesForIndexer(t)
	if len(fixtures) == 0 {
		t.Skip("no fixtures")
	}
	var raw []byte
	for _, f := range fixtures {
		if hasWitnessData(t, f) {
			raw = f
			break
		}
	}
	if raw == nil {
		t.Skip("no witness-containing fixture")
	}

	db, blocks, idx := setupAddrIndexDB(t, [][]byte{raw})
	defer db.Close()
	block := blocks[0]

	hasNonCoinbaseInputs := false
	for i, tx := range block.Transactions() {
		if i != 0 && len(tx.MsgTx().TxIn) > 0 {
			hasNonCoinbaseInputs = true
			break
		}
	}
	if !hasNonCoinbaseInputs {
		t.Skip("fixture has no non-coinbase inputs; nil-stxos path not exercised")
	}

	hash := block.Hash()
	if err := db.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	err := db.Update(func(tx database.Tx) error {
		return idx.RewriteTxOffsetsForColdCompaction(tx, block, nil)
	})
	if err == nil {
		t.Fatal("expected RewriteTxOffsetsForColdCompaction to refuse nil stxos")
	}
}
