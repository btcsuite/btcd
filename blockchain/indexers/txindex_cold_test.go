// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb" // driver registration
	"github.com/btcsuite/btcd/wire/v2"
)

// loadColdFixturesForIndexer returns raw serialized mainnet blocks from
// wire/testdata. It mirrors the helper in database/ffldb/coldblock_test.go but
// lives here so the indexer tests can use the same post-SegWit fixtures without
// crossing package boundaries.
func loadColdFixturesForIndexer(t *testing.T) [][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "wire", "testdata", "block-*.blk"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no block-*.blk fixtures")
	}
	out := make([][]byte, 0, len(paths))
	for _, p := range paths {
		raw, err := loadBlockFixture(p)
		if err != nil {
			t.Fatalf("load %s: %v", p, err)
		}
		out = append(out, raw)
	}
	return out
}

// loadBlockFixture reads a raw block file. Defined separately so the glob
// helper stays readable.
func loadBlockFixture(path string) ([]byte, error) {
	return readFileBytes(path)
}

// readFileBytes is a thin wrapper kept inline to avoid pulling in ioutil.
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// hasWitnessData reports whether the raw block serialization contains witness
// data, by comparing the full and stripped serializations. A block without
// witness has identical full and stripped serializations, so the cold
// compaction offset bug does not manifest for it.
func hasWitnessData(t *testing.T, raw []byte) bool {
	t.Helper()
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	stripped, err := block.BytesNoWitness()
	if err != nil {
		t.Fatalf("BytesNoWitness: %v", err)
	}
	return len(stripped) != len(raw)
}

// setupTxIndexDB creates a fresh ffldb database, stores the given blocks hot,
// creates the txindex buckets, and connects each block to the txindex (building
// txindex entries with witness-relative offsets, exactly as ConnectBlock does
// during normal chain operation). It returns the db, the stored block hashes
// in order, and the TxIndex instance.
func setupTxIndexDB(t *testing.T, rawBlocks [][]byte) (database.DB, []*btcutil.Block, *TxIndex) {
	t.Helper()
	dbPath := t.TempDir()
	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	idx := NewTxIndex(db)

	// Create the txindex buckets and initialize the block ID counter.
	if err := db.Update(func(tx database.Tx) error {
		meta := tx.Metadata()
		if _, err := meta.CreateBucketIfNotExists(indexTipsBucketName); err != nil {
			return err
		}
		if err := idx.Create(tx); err != nil {
			return err
		}
		// Initialize the index tip to "uninitialized" so dbIndexConnectBlock's
		// tip assertion passes for the first block.
		return dbPutIndexerTip(tx, idx.Key(), &chainhash.Hash{}, -1)
	}); err != nil {
		t.Fatalf("Create buckets: %v", err)
	}

	if err := idx.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	var blocks []*btcutil.Block
	for i, raw := range rawBlocks {
		block, err := btcutil.NewBlockFromBytes(raw)
		if err != nil {
			t.Fatalf("block %d: NewBlockFromBytes: %v", i, err)
		}
		block.SetHeight(int32(i))
		if err := db.Update(func(tx database.Tx) error {
			if err := tx.StoreBlock(block); err != nil {
				return err
			}
			// Call ConnectBlock directly (not dbIndexConnectBlock) to skip
			// the tip-chain assertion: the fixture blocks are not a
			// connected sequence starting from genesis, so the assertion
			// would fail. ConnectBlock does the real work — dbAddTxIndexEntries
			// + dbPutBlockIDIndexEntry — which is all we need.
			return idx.ConnectBlock(tx, block, nil)
		}); err != nil {
			t.Fatalf("block %d: StoreBlock+Connect: %v", i, err)
		}
		blocks = append(blocks, block)
	}

	return db, blocks, idx
}

// fetchTxViaIndex looks up a transaction by hash through the txindex and
// FetchBlockRegion, exactly as the getrawtransaction RPC does. It returns the
// raw transaction bytes and the transaction hash derived from them.
func fetchTxViaIndex(t *testing.T, db database.DB, txHash *chainhash.Hash) ([]byte, *chainhash.Hash) {
	t.Helper()
	var got []byte
	err := db.View(func(tx database.Tx) error {
		region, err := dbFetchTxIndexEntry(tx, txHash)
		if err != nil {
			return err
		}
		if region == nil {
			return nil
		}
		got, err = tx.FetchBlockRegion(region)
		return err
	})
	if err != nil {
		t.Fatalf("fetchTxViaIndex %s: %v", txHash, err)
	}
	if got == nil {
		t.Fatalf("fetchTxViaIndex %s: not found", txHash)
	}
	// Deserialize the region bytes as a transaction and re-hash to confirm we
	// read the right bytes. Use Deserialize (WitnessEncoding): it handles both
	// witness-included bytes (hot block) and stripped bytes (cold block)
	// correctly — a stripped tx has no witness marker, so the decoder falls
	// through to the legacy path. TxHash always derives the txid from the
	// non-witness serialization, so it matches regardless of whether witness
	// was present in the region bytes.
	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(got)); err != nil {
		t.Fatalf("fetchTxViaIndex %s: deserialize region bytes: %v",
			txHash, err)
	}
	computed := msgTx.TxHash()
	return got, &computed
}

// TestTxIndexColdCompactionRewrite is the definitive test for the cold
// compaction offset bug. It exercises the full path that getrawtransaction and
// btcwallet rescans use: txindex -> FetchBlockRegion, against a block that has
// been compacted from the hot tier (witness-included) to the cold tier
// (witness-stripped).
//
// Steps:
//  1. Store a post-SegWit mainnet block hot with the txindex enabled.
//  2. Verify every transaction in the block round-trips through the txindex
//     (witness-relative offsets, hot block) — baseline.
//  3. Compact the block to cold via CompactBlockToCold.
//  4. Call RewriteTxOffsetsForColdCompaction to rewrite the txindex entries to
//     stripped-relative offsets.
//  5. Verify every transaction STILL round-trips through the txindex, now
//     reading from the stripped cold block — this is where the bug would
//     manifest without the rewrite.
//
// Without step 4, step 5 fails for any block containing a segwit transaction:
// the witness-relative offsets stored by ConnectBlock point past the real tx
// boundaries in the stripped block, so FetchBlockRegion returns wrong bytes and
// the computed tx hash does not match the looked-up hash.
func TestTxIndexColdCompactionRewrite(t *testing.T) {
	fixtures := loadColdFixturesForIndexer(t)
	if len(fixtures) == 0 {
		t.Skip("no fixtures")
	}

	// Find a fixture that actually has witness data — the bug only manifests
	// for blocks containing segwit transactions (pre-SegWit blocks have
	// identical full and stripped serializations, so the offsets don't
	// change).
	var raw []byte
	for _, f := range fixtures {
		if hasWitnessData(t, f) {
			raw = f
			break
		}
	}
	if raw == nil {
		t.Skip("no witness-containing fixture; cannot exercise the offset bug")
	}

	db, blocks, idx := setupTxIndexDB(t, [][]byte{raw})
	defer db.Close()
	block := blocks[0]

	// Step 2: baseline — every tx round-trips through the txindex while the
	// block is hot. This confirms the index was built correctly.
	verifyAllTxViaIndex(t, db, block, "hot (before compaction)")

	// Step 3: compact the block to cold.
	hash := block.Hash()
	if err := db.Update(func(tx database.Tx) error {
		cc, ok := tx.(database.ColdCompactor)
		if !ok {
			t.Fatal("tx does not implement ColdCompactor")
		}
		return cc.CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	// Step 4: rewrite the txindex offsets to stripped-relative. This happens
	// in the same db tx as the compaction in production; here we do it in a
	// separate tx for test clarity, but the effect is the same because the
	// cold write already committed.
	if err := db.Update(func(tx database.Tx) error {
		return idx.RewriteTxOffsetsForColdCompaction(tx, block, nil)
	}); err != nil {
		t.Fatalf("RewriteTxOffsetsForColdCompaction: %v", err)
	}

	// Step 5: every tx STILL round-trips correctly, now reading from the
	// stripped cold block. Without the rewrite this fails.
	verifyAllTxViaIndex(t, db, block, "cold (after rewrite)")
}

// TestTxIndexColdCompactionWithoutRewrite demonstrates the bug: after
// compaction but WITHOUT the offset rewrite, FetchBlockRegion returns wrong
// bytes for segwit transactions. This test is expected to FAIL on the
// non-segwit txs and potentially error on segwit txs; it serves as a
// regression marker proving the rewrite is necessary, not cosmetic.
//
// We assert that at least one transaction fails to round-trip (wrong hash or
// deserialization error), which proves the bug exists in the unrewritten path.
func TestTxIndexColdCompactionWithoutRewrite(t *testing.T) {
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

	db, blocks, _ := setupTxIndexDB(t, [][]byte{raw})
	defer db.Close()
	block := blocks[0]

	// Baseline: hot round-trip works.
	verifyAllTxViaIndex(t, db, block, "hot baseline")

	// Compact to cold WITHOUT rewriting the txindex offsets.
	hash := block.Hash()
	if err := db.Update(func(tx database.Tx) error {
		return tx.(database.ColdCompactor).CompactBlockToCold(hash)
	}); err != nil {
		t.Fatalf("CompactBlockToCold: %v", err)
	}

	// Now attempt to read each tx through the txindex. At least one must fail
	// (wrong bytes) — this proves the unrewritten offsets are wrong for the
	// cold (stripped) block.
	atLeastOneFailed := false
	for _, tx := range block.Transactions() {
		txHash := tx.Hash()
		err := db.View(func(dbTx database.Tx) error {
			region, err := dbFetchTxIndexEntry(dbTx, txHash)
			if err != nil || region == nil {
				return err
			}
			got, err := dbTx.FetchBlockRegion(region)
			if err != nil {
				return err
			}
			// Try to deserialize and compare the hash. Use Deserialize
			// (WitnessEncoding) — it handles both hot (witness-included)
			// and cold (stripped) region bytes.
			var msgTx wire.MsgTx
			if err := msgTx.Deserialize(bytes.NewReader(got)); err != nil {
				return err
			}
			computed := msgTx.TxHash()
			if !computed.IsEqual(txHash) {
				return errHashMismatch{expected: *txHash, got: computed}
			}
			return nil
		})
		if err != nil {
			atLeastOneFailed = true
			t.Logf("tx %s: correctly failed without rewrite: %v", txHash, err)
		}
	}
	if !atLeastOneFailed {
		t.Fatal("expected at least one tx to fail the cold round-trip " +
			"without the offset rewrite, but all passed — the bug may " +
			"not be exercisable with this fixture (does it contain segwit txs?)")
	}
}

// verifyAllTxViaIndex looks up every transaction in the block through the
// txindex and FetchBlockRegion, and asserts the returned bytes hash to the
// expected transaction hash. stage is a label for failure messages.
func verifyAllTxViaIndex(t *testing.T, db database.DB, block *btcutil.Block, stage string) {
	t.Helper()
	for i, tx := range block.Transactions() {
		txHash := tx.Hash()
		got, computed := fetchTxViaIndex(t, db, txHash)
		if !computed.IsEqual(txHash) {
			t.Errorf("tx %d (%s) [%s]: hash mismatch — expected %s, got %s (raw %d bytes)",
				i, txHash, stage, txHash, computed, len(got))
		}
	}
}

// errHashMismatch is returned by the "without rewrite" test when a transaction
// read from the cold block via stale (witness-relative) offsets deserializes to
// a different hash than expected.
type errHashMismatch struct {
	expected chainhash.Hash
	got      chainhash.Hash
}

func (e errHashMismatch) Error() string {
	return "tx hash mismatch: expected " + e.expected.String() + ", got " + e.got.String()
}

// TestTxIndexColdDirectConnect verifies ConnectBlock writes stripped-relative
// offsets when the block was stored via StoreBlockCold (no later rewrite).
func TestTxIndexColdDirectConnect(t *testing.T) {
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

	dbPath := t.TempDir()
	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer db.Close()

	idx := NewTxIndex(db)
	if err := db.Update(func(tx database.Tx) error {
		meta := tx.Metadata()
		if _, err := meta.CreateBucketIfNotExists(indexTipsBucketName); err != nil {
			return err
		}
		if err := idx.Create(tx); err != nil {
			return err
		}
		return dbPutIndexerTip(tx, idx.Key(), &chainhash.Hash{}, -1)
	}); err != nil {
		t.Fatalf("Create buckets: %v", err)
	}
	if err := idx.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Fatalf("NewBlockFromBytes: %v", err)
	}
	block.SetHeight(0)

	if err := db.Update(func(tx database.Tx) error {
		if err := tx.(database.ColdCompactor).StoreBlockCold(block); err != nil {
			return err
		}
		return idx.ConnectBlock(tx, block, nil)
	}); err != nil {
		t.Fatalf("StoreBlockCold+Connect: %v", err)
	}

	verifyAllTxViaIndex(t, db, block, "cold-direct connect")
}
