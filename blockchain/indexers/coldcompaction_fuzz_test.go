// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build go1.18

// This file provides coverage-guided fuzz targets for the cold compaction
// offset-rewrite path. The core safety property under test — the one that
// protects wallet rescans and getrawtransaction from returning garbage after a
// block is compacted from the hot (witness-included) tier to the cold
// (witness-stripped) tier — is:
//
//	For every transaction in any valid block, after CompactBlockToCold +
//	RewriteTxOffsetsForColdCompaction, the txindex entry resolves via
//	FetchBlockRegion to bytes that deserialize to a transaction with the
//	correct txid.
//
// The fuzz corpus is seeded with the real post-SegWit mainnet fixtures in
// wire/testdata so the fuzzer starts from inputs that exercise the witness
// stripping path, then mutates them to explore neighboring block shapes
// (mixed segwit/legacy transactions, varying witness sizes, edge-case offset
// alignments, etc.).
//
// Run with:
//
//	go test -run='^$' -fuzz=FuzzColdCompactionTxIndex -fuzztime=30s ./blockchain/indexers/
//
// A crashing/failing fuzz input is written to testdata/fuzz/ and becomes a
// permanent regression case when the test is later run without -fuzz.

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

// loadColdFuzzSeeds globs the post-SegWit mainnet block fixtures used by the
// cold-compaction tests and returns their raw bytes, plus a small synthetic
// witness-containing block. It is the fuzz-target analogue of
// loadColdFixturesForIndexer in txindex_cold_test.go; it takes a testing.TB so
// it can be used from both *testing.T (unit tests) and *testing.F (fuzz
// harness) contexts.
//
// The synthetic seed matters for fuzz throughput: the real mainnet fixtures
// are 1.2-2.3 MB, so mutations of them stay large and each fuzz exec is slow.
// The synthetic block is a few hundred bytes with a real witness vector, so
// the coverage-guided fuzzer has small, fast witness-stripping coverage to
// explore from while the real fixtures remain as regression seeds.
func loadColdFuzzSeeds(tb testing.TB) [][]byte {
	tb.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "wire", "testdata", "block-*.blk"))
	if err != nil {
		tb.Fatalf("glob block fixtures: %v", err)
	}
	out := make([][]byte, 0, len(paths)+1)
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			tb.Fatalf("read %s: %v", p, err)
		}
		out = append(out, raw)
	}
	if synth, err := syntheticWitnessBlockBytes(); err != nil {
		tb.Fatalf("build synthetic witness seed: %v", err)
	} else {
		out = append(out, synth)
	}
	return out
}

// syntheticWitnessBlockBytes builds and returns a small block that contains a
// witness vector, serialized with WitnessEncoding. The block is NOT
// consensus-valid — it only needs to deserialize and to carry witness data so
// that BytesNoWitness actually strips something. This gives the fuzzer a
// compact witness-exercising seed without the multi-MB cost of a real mainnet
// block.
func syntheticWitnessBlockBytes() ([]byte, error) {
	// A transaction with a populated witness. The contents are arbitrary; only
	// the structure (marker + flag + witness stack) matters for the
	// stripping/offset logic.
	witnessTx := &wire.MsgTx{
		Version: 2,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{Hash: chainhash.Hash{}, Index: 0},
			SignatureScript:  []byte{0x00},
			Sequence:         0xffffffff,
			Witness: wire.TxWitness{
				[]byte{0x01, 0x02, 0x03},
				[]byte{0x04, 0x05},
			},
		}},
		TxOut: []*wire.TxOut{{
			Value:    1,
			PkScript: []byte{0x00, 0x14}, // minimal push, not a real script
		}},
		LockTime: 0,
	}
	coinbase := &wire.MsgTx{
		Version: 2,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{Hash: chainhash.Hash{}, Index: 0xffffffff},
			SignatureScript:  []byte{0x00},
			Sequence:         0,
		}},
		TxOut:    []*wire.TxOut{{Value: 50_000, PkScript: []byte{0x00, 0x14}}},
		LockTime: 0,
	}
	block := &wire.MsgBlock{
		Header:       wire.BlockHeader{},
		Transactions: []*wire.MsgTx{coinbase, witnessTx},
	}
	var buf bytes.Buffer
	if err := block.Serialize(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// coldCompactionRoundTrip performs the full hot-store -> index-build ->
// compact -> rewrite -> per-tx verify cycle for a single raw block and returns
// nil if every transaction round-trips through the txindex after the rewrite.
//
// It is the shared body of the fuzz target and the deterministic regression
// tests: any failure here means the offset rewrite is wrong for this block,
// i.e. getrawtransaction on a cold-height transaction would return wrong
// bytes — a fund-safety hazard for wallets.
//
// If reorgModel is set, the function additionally models the reorg path:
// after the first compact+rewrite it calls CompactBlockToCold again (which is
// idempotent and must return nil for an already-cold block) and then re-runs
// the rewrite, verifying the offsets are still correct. This mirrors
// connectBlock's behavior when a reorg disconnects and reconnects a block
// that was already compacted to cold.
func coldCompactionRoundTrip(t *testing.T, raw []byte, reorgModel bool) error {
	// Parse the candidate block. Anything that fails to deserialize is simply
	// not a block we can test — the fuzzer explores a lot of non-block inputs.
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Skipf("not a deserializable block: %v", err)
	}
	if len(block.Transactions()) == 0 {
		t.Skip("block has no transactions")
	}

	// Fresh DB per iteration. This is heavier than an in-memory mock but it
	// exercises the real ffldb compaction + FetchBlockRegion code paths, which
	// is where the offset bug actually lives. t.TempDir() auto-cleans.
	dbPath := t.TempDir()
	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("Create ffldb: %v", err)
	}
	defer db.Close()

	idx := NewTxIndex(db)
	if err := db.Update(func(tx database.Tx) error {
		meta := tx.Metadata()
		if _, err := meta.CreateBucketIfNotExists(indexTipsBucketName); err != nil {
			return err
		}
		return idx.Create(tx)
	}); err != nil {
		t.Fatalf("create txindex buckets: %v", err)
	}
	if err := idx.Init(); err != nil {
		t.Fatalf("Init txindex: %v", err)
	}

	block.SetHeight(0)
	hash := block.Hash()

	// Store the block hot and build the txindex with witness-relative offsets,
	// exactly as connectBlock does during normal chain operation.
	if err := db.Update(func(tx database.Tx) error {
		if err := tx.StoreBlock(block); err != nil {
			return err
		}
		return idx.ConnectBlock(tx, block, nil)
	}); err != nil {
		// StoreBlock/ConnectBlock failing on a parsed block means the input is
		// not a storable block for our purposes, not a bug in compaction.
		t.Skipf("store/connect block: %v", err)
	}

	// Sanity: while hot, every tx must already round-trip. This confirms the
	// index was built correctly before we touch the cold tier. A failure here
	// is a fuzz input that confuses ConnectBlock itself, not the rewrite.
	if err := verifyAllTxViaIndexErr(db, block); err != nil {
		return err
	}

	// Compact to cold (strips witness) and rewrite the offsets to be
	// stripped-relative. In production both run inside connectBlock's dbTx;
	// here we split them across two updates for clarity. The cold write is
	// committed before the rewrite reads it, so the effect is identical.
	if err := db.Update(func(tx database.Tx) error {
		cc, ok := tx.(database.ColdCompactor)
		if !ok {
			return errNoColdCompactor
		}
		return cc.CompactBlockToCold(hash)
	}); err != nil {
		return err
	}
	if err := db.Update(func(tx database.Tx) error {
		return idx.RewriteTxOffsetsForColdCompaction(tx, block, nil)
	}); err != nil {
		return err
	}

	// The core assertion: after compaction + rewrite, every tx in the block
	// round-trips through txindex -> FetchBlockRegion -> deserialize -> txid.
	// This is the getrawtransaction RPC path. Any mismatch means a wallet
	// rescanning this height would see the wrong transaction bytes.
	if err := verifyAllTxViaIndexErr(db, block); err != nil {
		return err
	}

	// Model the reorg path: the block is already cold, connectBlock reconnects
	// it (rebuilding witness-relative index entries), CompactBlockToCold must
	// be a no-op (idempotent, returns nil), and the rewrite must fix the
	// offsets again. If the rewrite were not idempotent this would fail.
	if reorgModel {
		// Rebuild the index entries as connectBlock would on reconnect:
		// witness-relative offsets over a now-cold block.
		if err := db.Update(func(tx database.Tx) error {
			return idx.ConnectBlock(tx, block, nil)
		}); err != nil {
			return err
		}
		// CompactBlockToCold must return nil for an already-cold block.
		if err := db.Update(func(tx database.Tx) error {
			cc, ok := tx.(database.ColdCompactor)
			if !ok {
				return errNoColdCompactor
			}
			return cc.CompactBlockToCold(hash)
		}); err != nil {
			return err
		}
		// Rewrite again; offsets must still come out correct.
		if err := db.Update(func(tx database.Tx) error {
			return idx.RewriteTxOffsetsForColdCompaction(tx, block, nil)
		}); err != nil {
			return err
		}
		if err := verifyAllTxViaIndexErr(db, block); err != nil {
			return err
		}
	}

	return nil
}

// verifyAllTxViaIndexErr is the error-returning analogue of verifyAllTxViaIndex
// (in txindex_cold_test.go). It is used by the fuzz target, which must report
// failures by returning an error rather than calling t.Fatalf — the fuzz
// engine needs the function to return so it can record the failing corpus
// entry. It reports the first failing transaction.
func verifyAllTxViaIndexErr(db database.DB, block *btcutil.Block) error {
	for i, tx := range block.Transactions() {
		txHash := tx.Hash()
		err := db.View(func(dbTx database.Tx) error {
			region, err := dbFetchTxIndexEntry(dbTx, txHash)
			if err != nil {
				return err
			}
			if region == nil {
				return errTxNotFound
			}
			got, err := dbTx.FetchBlockRegion(region)
			if err != nil {
				return err
			}
			// Deserialize with WitnessEncoding: it handles both
			// witness-included (hot) and stripped (cold) region bytes — a
			// stripped tx has no marker/flag so the decoder falls back to the
			// legacy path. TxHash derives the txid from the non-witness
			// serialization, so it matches regardless of witness presence.
			var msgTx wire.MsgTx
			if err := msgTx.Deserialize(bytes.NewReader(got)); err != nil {
				return err
			}
			if computed := msgTx.TxHash(); !computed.IsEqual(txHash) {
				return errHashMismatch{expected: *txHash, got: computed}
			}
			return nil
		})
		if err != nil {
			return &coldRoundTripError{txIndex: i, txHash: *txHash, err: err}
		}
	}
	return nil
}

// coldRoundTripError annotates a round-trip failure with the offending
// transaction so the fuzz failure message is immediately actionable.
type coldRoundTripError struct {
	txIndex int
	txHash  chainhash.Hash
	err     error
}

func (e *coldRoundTripError) Error() string {
	return "cold compaction round-trip failed for tx " + e.txHash.String() +
		" (index " + itoa(e.txIndex) + "): " + e.err.Error()
}

// itoa is a tiny strconv.Itoa replacement to keep the import surface of the
// fuzz target minimal.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// errNoColdCompactor is returned when the database transaction does not
// implement ColdCompactor. It is a sentinel so the fuzz target can surface it
// as an environment problem rather than a compaction bug.
var errNoColdCompactor = errSimple("database tx does not implement ColdCompactor")

// errTxNotFound is returned when a txindex lookup returns no entry for a
// transaction that was just connected — a sign of index corruption.
var errTxNotFound = errSimple("txindex entry not found for a connected transaction")

// errSimple is a string-backed error type used for the sentinel errors above
// so we avoid pulling in errors.New just for one-line messages.
type errSimple string

func (e errSimple) Error() string { return string(e) }

// FuzzColdCompactionTxIndex is the primary fuzz target for the cold compaction
// offset-rewrite. It asserts that for any deserializable block, after
// compaction + rewrite, every transaction resolves correctly through the
// txindex — the property that protects getrawtransaction and wallet rescans
// from returning wrong bytes (and thus wallets from acting on bad data) once a
// block ages out of the witness-preserving hot tier.
//
// The reorg model is enabled for every input so the fuzzer also exercises the
// idempotent-compact + idempotent-rewrite path that connectBlock relies on
// when a reorg reconnects an already-cold block.
//
// To run:
//
//	go test -run='^$' -fuzz=FuzzColdCompactionTxIndex -fuzztime=30s ./blockchain/indexers/
func FuzzColdCompactionTxIndex(f *testing.F) {
	for _, seed := range loadColdFuzzSeeds(f) {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if err := coldCompactionRoundTrip(t, raw, true); err != nil {
			t.Fatalf(err.Error())
		}
	})
}

// strippedOffsetCorrectness checks the pure-data core of the cold compaction
// rewrite WITHOUT a database: for any deserializable block, the stripped
// serialization's TxLocs must point at byte slices that deserialize back to
// transactions with the same txids as the original. This is exactly the
// invariant RewriteTxOffsetsForColdCompaction produces, and exactly what
// FetchBlockRegion relies on when reading a cold block.
//
// It runs at pure-CPU speed (no ffldb, no disk), so the coverage-guided fuzzer
// can explore thousands of block shapes per second — orders of magnitude more
// than the DB-backed FuzzColdCompactionTxIndex. The two targets are
// complementary: this one hunts for offset-derivation bugs cheaply; the
// DB-backed one guards the integrated index read path.
func strippedOffsetCorrectness(t *testing.T, raw []byte) error {
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		t.Skipf("not a deserializable block: %v", err)
	}
	if len(block.Transactions()) == 0 {
		t.Skip("block has no transactions")
	}

	// The rewrite path: BytesNoWitness -> reparse -> TxLoc().
	stripped, err := block.BytesNoWitness()
	if err != nil {
		return err
	}
	strippedBlock, err := btcutil.NewBlockFromBytes(stripped)
	if err != nil {
		return err
	}
	strippedLocs, err := strippedBlock.TxLoc()
	if err != nil {
		return err
	}

	origTxs := block.Transactions()
	if len(strippedLocs) != len(origTxs) {
		return &offsetError{msg: "stripped TxLoc count " + itoa(len(strippedLocs)) +
			" != original tx count " + itoa(len(origTxs))}
	}

	// For each transaction, slice the stripped bytes at the computed TxLoc and
	// confirm the slice deserializes to a tx with the expected txid. This is
	// what FetchBlockRegion + Deserialize does in production.
	for i, loc := range strippedLocs {
		if loc.TxStart < 0 || loc.TxLen <= 0 {
			return &offsetError{msg: "tx " + itoa(i) + " non-positive/empty loc",
				detail: "TxStart=" + itoa(loc.TxStart) + " TxLen=" + itoa(loc.TxLen)}
		}
		end := loc.TxStart + loc.TxLen
		if end > len(stripped) {
			return &offsetError{msg: "tx " + itoa(i) + " stripped loc out of bounds",
				detail: "TxStart=" + itoa(loc.TxStart) + " TxLen=" + itoa(loc.TxLen) +
					" strippedLen=" + itoa(len(stripped))}
		}
		slice := stripped[loc.TxStart:end]
		var msgTx wire.MsgTx
		if err := msgTx.Deserialize(bytes.NewReader(slice)); err != nil {
			return &offsetError{msg: "tx " + itoa(i) + " stripped slice does not deserialize",
				detail: err.Error()}
		}
		want := origTxs[i].Hash()
		got := msgTx.TxHash()
		if !got.IsEqual(want) {
			return &offsetError{msg: "tx " + itoa(i) + " txid mismatch after strip",
				detail: "want=" + want.String() + " got=" + got.String()}
		}
	}
	return nil
}

// offsetError is the failure type for the pure offset-correctness fuzz target.
type offsetError struct {
	msg    string
	detail string
}

func (e *offsetError) Error() string {
	if e.detail == "" {
		return e.msg
	}
	return e.msg + " (" + e.detail + ")"
}

// FuzzColdCompactionOffsets is the fast, pure-data fuzz target for the cold
// compaction offset rewrite. It verifies that for any deserializable block, the
// stripped serialization's transaction locators point at byte ranges that
// deserialize to transactions with the correct txids — the invariant the
// rewrite produces and FetchBlockRegion consumes. Because it does no disk I/O,
// it runs orders of magnitude faster than FuzzColdCompactionTxIndex and is the
// primary target for hunting offset-derivation regressions via coverage.
//
// To run:
//
//	go test -run='^$' -fuzz=FuzzColdCompactionOffsets -fuzztime=30s ./blockchain/indexers/
func FuzzColdCompactionOffsets(f *testing.F) {
	for _, seed := range loadColdFuzzSeeds(f) {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if err := strippedOffsetCorrectness(t, raw); err != nil {
			t.Fatalf(err.Error())
		}
	})
}
