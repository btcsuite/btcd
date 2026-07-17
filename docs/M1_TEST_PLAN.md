# M1 Test Plan: Witness-Separated Storage

## Overview

The witness-separated storage design (M1) is validated at three levels:
database-level whitebox tests, blockchain-level integration tests, and
real-chain measurement tools. Together they cover correctness, consensus safety,
and the disk-reduction headline.

## Test Matrix

| Test | Location | What it verifies |
|---|---|---|
| `TestWitnessPruningEndToEnd` | `database/ffldb/witness_prune_test.go` | Full lifecycle: store hot → compact to cold → FetchBlock returns stripped → no witness on any tx → block hash unchanged → cold file smaller → reclaim hot space → block still readable |
| `TestWitnessPruningMultipleBlocks` | `database/ffldb/witness_prune_test.go` | Multiple blocks compacted together, hot window preserved, all blocks readable after reclaim |
| `TestColdBlockRoundTrip` | `database/ffldb/coldblock_test.go` | Cold block reads back as stripped serialization, byte-identical, deserializes via `DeserializeNoWitness` |
| `TestColdBlockRegion` | `database/ffldb/coldblock_test.go` | `readBlockRegion` on cold blocks (header reads from decompressed stripped block) |
| `TestHotPathUnchanged` | `database/ffldb/coldblock_test.go` | Hot-tier write/read path untouched by cold-tier code |
| `TestMixedHotAndCold` | `database/ffldb/coldblock_test.go` | Coexistence of hot and cold blocks in the same store |
| `TestColdCorruptionDetection` | `database/ffldb/coldblock_test.go` | CRC catches corrupted cold blocks |
| `TestColdSavingsOnDisk` | `database/ffldb/coldblock_test.go` | Cold file is smaller than hot file on disk |
| `TestCompactBlockToCold` | `database/ffldb/coldblock_test.go` | `CompactBlockToCold` through full `database.Tx`: store hot → compact → FetchBlock returns stripped, block index has cold flag |
| `TestCompactBlockToColdRollback` | `database/ffldb/coldblock_test.go` | Rolled-back compaction leaves block in hot tier, no orphaned cold data |
| `TestCompactAlreadyCold` | `database/ffldb/coldblock_test.go` | Compacting an already-cold block is a no-op |
| `TestReclaimHotSpace` | `database/ffldb/coldblock_test.go` | Hot file deleted after reclaim, block still readable from cold |
| `TestReclaimHotSpacePartial` | `database/ffldb/coldblock_test.go` | Hot file with remaining hot blocks is NOT deleted |
| `TestColdCacheHit` | `database/ffldb/coldblock_test.go` | LRU cache returns cached decompressed block without file I/O |
| `TestColdCacheRegion` | `database/ffldb/coldblock_test.go` | Region reads populate and hit the cache |
| `TestColdCacheEviction` | `database/ffldb/coldblock_test.go` | Cache evicts oldest entries when full |
| `TestWitnessBufferAgeOut` | `blockchain/ageout_test.go` | Blockchain-layer age-out driver: blocks beyond buffer are compacted, cold files created on disk, all blocks remain readable |
| `TestWitnessBufferDisabled` | `blockchain/ageout_test.go` | With `witnessBuffer=0`, no compaction occurs, no cold directory created |
| `TestWitnessBufferConfig` | `blockchain/ageout_test.go` | `Config.WitnessBuffer` wires through to `BlockChain.witnessBuffer` |
| `TestFullBlocks` | `blockchain/fullblocktests` | Consensus suite passes with two-tier storage enabled (22s, race-clean) |
| `TestRoundTrip` | `blockcompress/codec_test.go` | `decompress(compress(x)) == x` for representative inputs |
| `TestDeterminismAcrossInstances` | `blockcompress/codec_test.go` | Two codecs on same version produce identical compressed output |
| `TestDeterminismAcrossRuns` | `blockcompress/codec_test.go` | Repeated compression with same codec is identical |
| `TestCorruptionDetection` | `blockcompress/codec_test.go` | Corrupted compressed stream is rejected (not silently wrong bytes) |

## Real-Chain Measurement

The `dicttrain/` directory contains two `//go:build ignore` tools that run
against a real synced btcd datadir:

- **`measure.go`**: Streams one block at a time from evenly-spaced `.fdb` files
  across the full chain. Reports per-file and blended witness fraction,
  compress-only, prune-only, and combined (prune+zstd) ratios. Optionally
  benchmarks with a trained dictionary.

- **`main.go`** (dicttrain): Streams blocks from evenly-spaced files, strips
  witness, trains a zstd dictionary, and benchmarks dict vs no-dict.

Results on a 1005 GB mainnet datadir (156K blocks sampled from 29 files):

| Approach | Reduction | Est. full-chain size |
|---|---|---|
| Compress whole block (zstd only) | 23.9% | 765 GB |
| Prune witness only (no zstd) | 35.8% | 645 GB |
| **Prune witness + zstd stripped** | **52.5%** | **477 GB** |

Dictionary training added <0.4 percentage points. FormatV1 ships dict-free.

## Running the Tests

```bash
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
```

## What the Tests Prove

1. **Witness is actually pruned**: `FetchBlock` on a cold block returns bytes
   that are shorter than the full block, no transaction has witness data, and
   the bytes match `SerializeNoWitness` output exactly.

2. **Block hash is unchanged**: The stripped block produces the same block hash
   as the full block (the hash is computed over the header, which is identical
   in both serializations).

3. **Cold blocks are served as legacy**: `Deserialize` (not just
   `DeserializeNoWitness`) works on cold block bytes, and no transaction has
   `HasWitness() == true`. This means `pushBlockMsg` in `server.go` serves cold
   blocks correctly as legacy (non-witness) blocks to peers.

4. **Consensus is not affected**: `TestFullBlocks` passes, meaning all consensus
   checks operate on the decompressed block and produce correct results.

5. **Disk savings are real**: The cold file on disk is smaller than the hot file.
   On real mainnet data, the blended reduction is 52.5% (1005 GB → 477 GB).

6. **Hot path is untouched**: Hot-tier blocks are stored and read exactly as
   before — no compression, no witness stripping, same code path.

7. **Rollback is safe**: A failed compaction transaction leaves the block in the
   hot tier with no orphaned cold data.

8. **Reclaim works**: Hot files are deleted after all their blocks are compacted,
   and blocks remain readable from cold. Files with remaining hot blocks are
   preserved.
