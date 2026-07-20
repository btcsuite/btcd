# M1 Test Plan: Witness-Separated Storage

## Overview

The witness-separated storage design (M1) is validated at three levels:
database-level whitebox tests, blockchain-level integration tests, and
real-chain measurement tools. Together they cover correctness, consensus safety,
and the disk-reduction headline.

## Test Matrix

| Test | Location | What it verifies |
|---|---|---|
| `TestWitnessExcisionEndToEnd` | `database/ffldb/witness_excision_test.go` | Full lifecycle: store hot → compact to cold → FetchBlock returns stripped → no witness on any tx → block hash unchanged → cold file smaller → reclaim hot space → block still readable |
| `TestWitnessExcisionMultipleBlocks` | `database/ffldb/witness_excision_test.go` | Multiple blocks compacted together, hot window preserved, all blocks readable after reclaim |
| `TestColdBlockRoundTrip` | `database/ffldb/coldblock_test.go` | Cold block reads back as stripped serialization, byte-identical, deserializes via `DeserializeNoWitness` |
| `TestColdBlockRegion` | `database/ffldb/coldblock_test.go` | `readBlockRegion` on cold blocks (header reads from decompressed stripped block) |
| `TestHotPathUnchanged` | `database/ffldb/coldblock_test.go` | Hot-tier write/read path untouched by cold-tier code |
| `TestMixedHotAndCold` | `database/ffldb/coldblock_test.go` | Coexistence of hot and cold blocks in the same store |
| `TestColdCorruptionDetection` | `database/ffldb/coldblock_test.go` | CRC catches corrupted cold blocks |
| `TestColdSavingsOnDisk` | `database/ffldb/coldblock_test.go` | Cold file is smaller than hot file on disk |
| `TestCompactBlockToCold` | `database/ffldb/coldblock_test.go` | `CompactBlockToCold` through full `database.Tx`: store hot → compact → FetchBlock returns stripped, block index has cold flag |
| `TestStoreBlockCold` | `database/ffldb/coldblock_test.go` | Cold-direct store: no hot write, FetchBlock stripped, cold flag set, rollback leaves no orphan |
| `TestCompactBlockToColdRollback` | `database/ffldb/coldblock_test.go` | Rolled-back compaction leaves block in hot tier, no orphaned cold data |
| `TestCompactAlreadyCold` | `database/ffldb/coldblock_test.go` | Compacting an already-cold block is an idempotent no-op (returns nil) |
| `TestReclaimHotSpace` | `database/ffldb/coldblock_test.go` | Hot file deleted after reclaim, block still readable from cold |
| `TestReclaimHotSpacePartial` | `database/ffldb/coldblock_test.go` | Hot file with remaining hot blocks is NOT deleted |
| `TestColdCacheHit` | `database/ffldb/coldblock_test.go` | LRU cache returns cached decompressed block without file I/O |
| `TestColdCacheRegion` | `database/ffldb/coldblock_test.go` | Region reads populate and hit the cache |
| `TestColdCacheEviction` | `database/ffldb/coldblock_test.go` | Cache evicts oldest entries when full |
| `TestColdWriteCursorResumesAfterReopen` | `database/ffldb/coldblock_test.go` | After close/reopen, cold write cursor resumes at latest cold file end; post-restart write does not append to file 0 or exceed `maxBlockFileSize` |
| `TestTxIndexColdCompactionRewrite` | `blockchain/indexers/txindex_cold_test.go` | txindex offsets are rewritten to stripped-relative on compaction; every tx in a cold block round-trips through `dbFetchTxIndexEntry` → `FetchBlockRegion` with the correct txid |
| `TestTxIndexColdDirectConnect` | `blockchain/indexers/txindex_cold_test.go` | `StoreBlockCold` + ConnectBlock indexes with stripped offsets; every tx round-trips without a later rewrite |
| `TestTxIndexColdCompactionWithoutRewrite` | `blockchain/indexers/txindex_cold_test.go` | Proves the bug exists without the rewrite: witness-relative offsets from `ConnectBlock` point past the stripped block boundary, so `FetchBlockRegion` fails for segwit txs |
| `FuzzColdCompactionOffsets` | `blockchain/indexers/coldcompaction_fuzz_test.go` | Coverage-guided fuzz of the offset-rewrite core: for any deserializable block, the stripped serialization's TxLocs point at byte ranges that deserialize to txs with the correct txid (pure-data, ~40k execs/s) |
| `FuzzColdCompactionTxIndex` | `blockchain/indexers/coldcompaction_fuzz_test.go` | Coverage-guided fuzz of the integrated path: compact → rewrite → txindex → `FetchBlockRegion` → txid match, including the idempotent reorg model (compact-already-cold + double rewrite) |
| `TestAddrIndexColdCompactionRewrite` | `blockchain/indexers/addrindex_cold_test.go` | addrindex offsets rewritten to stripped-relative; every address round-trips; **all** stored entries for the blockID use stripped TxStarts (completeness, not sampled) |
| `TestAddrIndexRewriteWithStrippedRefetchFails` | `blockchain/indexers/addrindex_cold_test.go` | Regression: rewrite after pending-cold FetchBlock (stripped) fails on unknown witness-relative offsets instead of silently leaving stale entries |
| `TestAddrIndexRewriteFullBlockSameTxAsCompact` | `blockchain/indexers/addrindex_cold_test.go` | Correct age-out path: CompactBlockToCold then rewrite with the original full block in the same tx |
| `TestAgeOutProcessBlockPassesFullBlockToAddrindexRewrite` | `blockchain/indexers/ageout_addrindex_e2e_test.go` | End-to-end: ProcessBlock + txindex/addrindex + segwit past `--witness-buffer`; rewrite sees full hot block (not stripped re-fetch); `TxRegionsForAddress` → `FetchBlockRegions` round-trips post-compaction |
| `TestAddrIndexColdCompactionWithoutRewrite` | `blockchain/indexers/addrindex_cold_test.go` | Proves the addrindex bug exists without the rewrite: witness-relative offsets point past the stripped block, so `FetchBlockRegions` returns wrong bytes / EOF for addresses in segwit blocks |
| `TestAddrIndexRewriteNilStxosRefused` | `blockchain/indexers/addrindex_cold_test.go` | Nil spend journal: rewrite refuses to leave stale input-address offsets; chain skips compaction when journal is missing |
| `TestAddrIndexRewriteUnknownOffsetFails` | `blockchain/indexers/addrindex_cold_test.go` | Matched blockID with stored txStart missing from oldToNew fails the rewrite (no silent stale offsets after cold commit) |
| `TestWitnessBufferAgeOut` | `blockchain/ageout_test.go` | Blockchain-layer age-out driver: blocks beyond buffer are compacted, cold files created on disk, all blocks remain readable |
| `TestColdDirectStoreWhenHeadersAhead` | `blockchain/cold_direct_test.go` | Headers-first IBD: bodies past buffer are `IsColdBlock` immediately (StoreBlockCold), tip window stays hot |
| `TestWitnessBufferDisabled` | `blockchain/ageout_test.go` | With `witnessBuffer=0`, no compaction occurs, no cold directory created |
| `TestWitnessBufferConfig` | `blockchain/ageout_test.go` | `Config.WitnessBuffer` wires through to `BlockChain.witnessBuffer` |
| `TestInvalidatePastWitnessBufferRefused` | `blockchain/witness_excision_ops_test.go` | Deep `InvalidateBlock` past the hot window is refused with `ErrWitnessExcised`; shallow invalidate still works |
| `TestReconsiderColdAttachPreservesInvalidStatus` | `blockchain/witness_excision_ops_test.go` | `ReconsiderBlock` of a cold attach path returns `ErrWitnessExcised` and restores `KnownInvalid` (does not leave cleared invalid flags in memory) |
| `TestStaleSideChainBodyDropped` | `blockchain/witness_excision_ops_test.go` | Alternate forks past the witness buffer drop bodies (headers retained); recent side tips stay hot |
| `TestDeleteBlock` | `database/ffldb/deleteblock_test.go` | `DeleteBlock` removes body from index, is idempotent, leaves sibling blocks intact |
| `TestColdCompactionSurvivesReopen` | `database/ffldb/cold_reopen_test.go` | After compact + close + reopen, block stays cold and FetchBlock matches stripped bytes (post-commit crash recovery) |
| `TestCreateTxRawResultWitnessExcised` | `witness_excised_rpc_test.go` | Verbose tx JSON sets `witness_excised` for cold loads and does not invent a historical wtxid; `getblock` verbosity 0 refuses cold hex (no flag carrier) |
| `TestFullBlocks` | `blockchain/fullblocks_test.go` | Consensus suite passes with two-tier storage enabled (`WitnessBuffer=8` so age-out runs during acceptance) |
| `TestRoundTrip` | `blockcompress/codec_test.go` | `decompress(compress(x)) == x` for representative inputs |
| `TestDeterminismAcrossInstances` | `blockcompress/codec_test.go` | Two codecs on same version produce identical compressed output |
| `TestDeterminismAcrossRuns` | `blockcompress/codec_test.go` | Repeated compression with same codec is identical |
| `TestCorruptionDetection` | `blockcompress/codec_test.go` | Corrupted compressed stream is rejected (not silently wrong bytes) |

## Real-Chain Measurement

The `dicttrain/` directory contains two `//go:build ignore` tools that run
against a real synced praxisd/btcd datadir:

- **`measure.go`**: Streams one block at a time from evenly-spaced `.fdb` files
  across the full chain. Reports per-file and blended witness fraction,
  compress-only, excise-only, and combined (excise+zstd) ratios. Optionally
  benchmarks with a trained dictionary.

- **`main.go`** (dicttrain): Streams blocks from evenly-spaced files, strips
  witness, trains a zstd dictionary, and benchmarks dict vs no-dict.

Results on a 1005 GB mainnet datadir (156K blocks sampled from 29 files):

| Approach | Reduction | Est. full-chain size |
|---|---|---|
| Compress whole block (zstd only) | 23.9% | 765 GB |
| Excise witness only (no zstd) | 35.8% | 645 GB |
| **Excise witness + zstd stripped** | **52.5%** | **477 GB** |

A ~200-block dictionary pilot gained <0.4 percentage points over plain zstd.
FormatV1 uses plain zstd (no trained dictionary); a future format version can
revisit this.

## Running the Tests

```bash
# Full unit suite (what GitHub Actions `unit` / `unit-race` run)
make unit
make unit-race

# M1-focused regressions (GitHub Actions `unit-m1` job): cold-tier, age-out,
# addrindex/txindex rewrite + completeness, and FuzzColdCompaction seed replay.
# Plain `go test ./...` does NOT execute FuzzXxx seed corpora — this target does.
make unit-m1

# Database-level tests (whitebox, ~23s with -race)
go test -race -timeout 120s ./database/ffldb/

# Blockchain-level tests (~65s with -race)
go test -race -timeout 300s ./blockchain/

# Consensus suite (~37s with -race)
go test -race -timeout 600s ./blockchain/ -run TestFullBlocks

# Codec tests (~7s with -race)
go test -race -timeout 120s ./blockcompress/

# Real-chain measurement (requires synced datadir, //go:build ignore)
go run blockcompress/dicttrain/measure.go -datadir /path/to/blocks_ffldb

# Cold-compaction fuzz targets (seed-corpus regression run, ~3s)
go test -race -timeout 120s -run='FuzzColdCompaction' ./blockchain/indexers/

# Cold-compaction fuzz exploration (optional local / longer CI budget):
go test -run='^$' -fuzz=FuzzColdCompactionOffsets -fuzztime=30s ./blockchain/indexers/
go test -run='^$' -fuzz=FuzzColdCompactionTxIndex -fuzztime=30s ./blockchain/indexers/
# Failing inputs are persisted under blockchain/indexers/testdata/fuzz/ and
# replayed by `make unit-m1` / `-run=FuzzColdCompaction` (not by plain ./...).
```

## Tests we ran

Run log for Bitcoin-Praxis M1 (witness-separated storage) and related soaks.
This is evidence, not a substitute for the matrix above.

| When (approx.) | What | Result |
|---|---|---|
| M1 development | Unit/integration: `database/ffldb`, `blockchain` age-out / excision ops, index cold rewrite; `go test -race` on touched packages; `TestFullBlocks` | Pass (race-clean) |
| M1 development | Mainnet measurement via `blockcompress/dicttrain/measure.go` on a 1005 GB datadir | **52.5%** blended reduction → ~477 GB |
| M1 development | Fuzz: `FuzzColdCompactionOffsets`, `FuzzColdCompactionTxIndex` (minutes of `-fuzztime`) | Pass |
| Post M1 | Testnet4 IBD with parallel block fetch to tip (~144k) | ~4.8 blk/s overall on soak host |
| Post M1 | Simnet LN cold soak vs excised witness (CSV ≪ `--witness-buffer`): warm/cold pay, coop close, force close, SCB restore | Pass |

**Still planned:** longer mainnet IBD soak; M2 IBD replay benchmark; broader LN
soak matrix; M3 Bitcoin-Praxis Wallet / GUI acceptance tests (see ROADMAP).

## What the Tests Prove

1. **Witness is actually excised**: `FetchBlock` on a cold block returns bytes
   that are shorter than the full block, no transaction has witness data, and
   the bytes match `SerializeNoWitness` output exactly.

2. **Block hash is unchanged**: The stripped block produces the same block hash
   as the full block (the hash is computed over the header, which is identical
   in both serializations).

3. **Cold blocks are served as legacy**: `Deserialize` (not just
   `DeserializeNoWitness`) works on cold block bytes, and no transaction has
   `HasWitness() == true`. This means `pushBlockMsg` in `server.go` serves cold
   blocks correctly as legacy (non-witness) blocks to peers.

4. **Consensus is not affected**: `TestFullBlocks` passes with `WitnessBuffer`
   enabled (two-tier age-out during acceptance), meaning all consensus checks
   operate correctly whether blocks are hot or cold.

5. **Disk savings are real**: The cold file on disk is smaller than the hot file.
   On real mainnet data, the blended reduction is 52.5% (1005 GB → 477 GB).

6. **Hot path is untouched**: Hot-tier blocks are stored and read exactly as
   before — no compression, no witness stripping, same code path.

7. **Rollback is safe**: A failed compaction transaction leaves the block in the
   hot tier with no orphaned cold data.

8. **Reclaim works**: Hot files are deleted after all their blocks are compacted,
   and blocks remain readable from cold. Files with remaining hot blocks are
   preserved.

9. **Offset-bearing indexes survive compaction**: The transaction index and
   address index store block-relative byte offsets that change when a block is
   compacted from the hot tier (witness-included) to the cold tier
   (witness-stripped). `RewriteTxOffsetsForColdCompaction` rewrites those
   offsets to the stripped serialization's offset space in the same database
   transaction as `CompactBlockToCold`. Without it, `getrawtransaction` and
   `searchrawtransactions` would return garbled bytes for any compacted block
   containing a segwit transaction, and wallet rescans would break. The rewrite
   is also applied when `CompactBlockToCold` is a no-op on an already-cold
   block (e.g. after a reorg reconnection), because `ConnectBlock` rebuilds
   those index entries with witness-relative offsets that must be corrected.

10. **`FetchBlockRegion` bounds check is correct for cold blocks**: The
    `blockLen` stored in the block index for a cold block is the on-disk
    compressed record length, but region offsets are relative to the
    uncompressed stripped block. The `FetchBlockRegion` bounds check skips
    cold blocks (the per-record `readBlockRegion` check against the
    decompressed length is authoritative for cold blocks).

11. **Offset rewrite is robust under fuzzing**: `FuzzColdCompactionOffsets`
    asserts the pure-data core of the rewrite — stripped TxLocs point at
    deserializable bytes hashing to the correct txid — for any deserializable
    block, at ~40k execs/s. `FuzzColdCompactionTxIndex` asserts the full
    integrated read path end to end, including the idempotent reorg model
    (compacting an already-cold block + re-running the rewrite). Both are
    seeded with the real mainnet fixtures plus a small synthetic
    witness-containing block so coverage-guided exploration is fast. This is
    the direct evidence that witness removal cannot silently corrupt a
    wallet's view of historical transactions.

12. **`searchrawtransactions` survives compaction**: The address-index
    rewrite is verified end to end (`TestAddrIndexColdCompactionRewrite`):
    after compaction + rewrite, every output address's tx regions resolve to
    the same tx hashes as before compaction. Without the rewrite the negative
    test shows `FetchBlockRegions` returning wrong bytes / EOF. This is the
    btcwallet address-history rescan path. Happy-path tests also assert
    **every** stored entry for the compacted blockID uses a stripped TxStart
    (`assertAllAddrEntriesUseStrippedOffsets`) — not merely that sampled
    addresses round-trip.

13. **Missing spend journal disables unsafe compaction**: when the spend journal
    for an aging-out block is unavailable, age-out skips compaction (block stays
    hot) rather than leaving stale addrindex input offsets.
    `TestAddrIndexRewriteNilStxosRefused` guards the direct-call refuse path.

14. **Index rewrite is fail-closed**: if an addrindex entry matches the block
    ID but its stored `txStart` is not in the old→new map, rewrite returns an
    error (`TestAddrIndexRewriteUnknownOffsetFails`,
    `TestAddrIndexRewriteWithStrippedRefetchFails`). Age-out then cancels the
    pending cold write and leaves the block hot. Do **not** treat “rewrite
    succeeds but reads are stale” as an acceptable negative test — that encodes
    the silent-`continue` bug. Negative cases must assert the rewrite **fails**
    (or cold is cancelled), never that corruption is merely observable later.
