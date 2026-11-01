// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
)

// -----------------------------------------------------------------------------
// A fast build writes the address index for the whole main chain at once
// instead of connecting the blocks one at a time.  Manager.Init runs it when
// the address index is uninitialized, after it catches up the indexes before
// the address index, since the write pass reads block IDs from the transaction
// index.  Manager.Init runs inside blockchain.New, so the main chain does not
// change while a build runs.
//
// The scan reads the blocks from the genesis block through the best chain tip,
// which the manifest records as the target, with a pool of workers.  It stages
// one record per address index entry in the addrindexbuild directory under the
// data directory.  A record is the address key followed by the entry in the
// address index value format, except that its block ID field holds the block
// height.  After every addrBuildScanChunkSize blocks the scan syncs the staging
// files and replaces the manifest, which records the completed height and the
// size of each staging file.  A resumed scan truncates a longer staging file to
// its recorded size, and a shorter one is an error.
//
// The write pass reads one shard at a time, sorts its records into chain order,
// replaces each height with the block ID of the block at that height, and
// writes all levels of an address in one database transaction.  The layout
// depends only on the number of entries (see addrLevelEntryCounts), so it
// matches what connecting the blocks one at a time leaves.  Writing an address
// puts every level of its layout, so repeating the write pass after a crash
// gives the same result.
//
// Until the build sets the index tip, the index can hold entries from an
// unfinished write pass, and only the staging shows that the build is
// unfinished.  So staging is removed only after the database is flushed, both
// after a build sets the tip and after an index drop.  When the target is no
// longer in the main chain, Manager.Init drops the index and its staging and
// the build starts over.
// -----------------------------------------------------------------------------

const addrIndexBuildDirName = "addrindexbuild"

// addrBuildScanChunkSize is the number of blocks the scan stages between
// checkpoints.
const addrBuildScanChunkSize = 50000

var _ FastBuilder = (*AddrIndex)(nil)
var _ staleBuildDropper = (*AddrIndex)(nil)

// createAddrBuildStager creates the staging directory for a new build.  It
// creates the staging files and the initial manifest in staging.tmp and then
// renames it to staging, so a staging directory always holds a manifest.
func createAddrBuildStager(dataDir string,
	manifest *addrBuildManifest) (*addrStager, error) {

	buildDir := filepath.Join(dataDir, addrIndexBuildDirName)
	if err := os.MkdirAll(buildDir, 0700); err != nil {
		return nil, err
	}
	if err := syncAddrBuildDir(filepath.Dir(buildDir)); err != nil {
		return nil, err
	}

	stagingDir := filepath.Join(buildDir, "staging")
	tmpDir := stagingDir + ".tmp"
	if err := os.Mkdir(tmpDir, 0700); err != nil {
		return nil, err
	}
	stager, err := newAddrStager(tmpDir)
	if err != nil {
		return nil, err
	}
	if err := stager.checkpoint(manifest, nil); err != nil {
		stager.close()
		return nil, err
	}
	stager.close()

	if err := os.Rename(tmpDir, stagingDir); err != nil {
		return nil, err
	}
	if err := syncAddrBuildDir(buildDir); err != nil {
		return nil, err
	}
	stager, err = openAddrStager(stagingDir, manifest, nil)
	if err != nil {
		return nil, err
	}
	return stager, nil
}

// removeAddrIndexBuildDeleteDir removes addrindexbuild.delete, which an
// interrupted removeAddrIndexBuildDir can leave behind, and syncs the data
// directory.  It does nothing when addrindexbuild.delete does not exist.
func removeAddrIndexBuildDeleteDir(dataDir string) error {
	deleteDir := filepath.Join(dataDir, addrIndexBuildDirName) + ".delete"
	if _, err := os.Stat(deleteDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.RemoveAll(deleteDir); err != nil {
		return err
	}
	return syncAddrBuildDir(dataDir)
}

// removeAddrIndexBuildDir removes the build directory.  It first renames the
// directory to addrindexbuild.delete, which the next start removes, so a crash
// during the removal cannot leave a staging directory with some of its files
// gone.
func removeAddrIndexBuildDir(dataDir string) error {
	buildDir := filepath.Join(dataDir, addrIndexBuildDirName)
	deleteDir := buildDir + ".delete"
	if err := removeAddrIndexBuildDeleteDir(dataDir); err != nil {
		return err
	}
	if err := os.Rename(buildDir, deleteDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else if err := syncAddrBuildDir(dataDir); err != nil {
		return err
	}
	if err := os.RemoveAll(deleteDir); err != nil {
		return err
	}
	return syncAddrBuildDir(dataDir)
}

// addrBuildOnMainChain reports whether the build target the manifest records
// is still in the main chain.
func addrBuildOnMainChain(chain *blockchain.BlockChain,
	manifest *addrBuildManifest) bool {

	targetHash, err := chain.BlockHashByHeight(manifest.TargetHeight)
	return err == nil && manifest.TargetHash.IsEqual(targetHash)
}

// addrBuildDBFlusher forces pending writes to the database's backing store.
// The ffldb driver implements it.
type addrBuildDBFlusher interface {
	Flush() error
}

// addrBuildFlusher returns db as an addrBuildDBFlusher, or an error when db
// cannot flush pending writes.
func addrBuildFlusher(db database.DB) (addrBuildDBFlusher, error) {
	flusher, ok := db.(addrBuildDBFlusher)
	if !ok {
		return nil, fmt.Errorf("database %T cannot flush pending "+
			"writes", db)
	}
	return flusher, nil
}

// flushAddrIndexDB makes the database durable before staging is removed.
func flushAddrIndexDB(db database.DB) error {
	flusher, err := addrBuildFlusher(db)
	if err != nil {
		return err
	}
	return flusher.Flush()
}

// addrBuildState holds the manifest in the staging directory and the index
// tip.
type addrBuildState struct {
	manifest  addrBuildManifest
	tipHash   *chainhash.Hash
	tipHeight int32
}

// loadAddrBuildState returns the manifest in the staging directory and the
// index tip, or nil when there is no data directory or no staging.  It first
// finishes an interrupted removal of the build directory, and it removes a
// build directory that has no staging directory.  An invalid manifest is an
// error.
func (idx *AddrIndex) loadAddrBuildState() (*addrBuildState, error) {
	if idx.dataDir == "" {
		return nil, nil
	}
	if err := removeAddrIndexBuildDeleteDir(idx.dataDir); err != nil {
		return nil, err
	}
	buildDir, exists, err := addrIndexBuildDirExists(idx.dataDir)
	if err != nil || !exists {
		return nil, err
	}

	stagingDir := filepath.Join(buildDir, "staging")
	if _, err := os.Stat(stagingDir); err != nil {
		if os.IsNotExist(err) {
			return nil, removeAddrIndexBuildDir(idx.dataDir)
		}
		return nil, err
	}
	manifest, ok := readAddrBuildManifest(stagingDir)
	if !ok {
		return nil, fmt.Errorf("address index build checkpoint is " +
			"invalid, drop the address index before restarting " +
			"the build")
	}

	state := &addrBuildState{manifest: manifest}
	err = idx.db.View(func(dbTx database.Tx) error {
		var err error
		state.tipHash, state.tipHeight, err = dbFetchIndexerTip(
			dbTx, addrIndexKey,
		)
		return err
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

// Init removes staging after a completed build is durable.
//
// This is part of the Indexer interface.
func (idx *AddrIndex) Init() error {
	state, err := idx.loadAddrBuildState()
	if err != nil || state == nil {
		return err
	}
	completed := state.tipHeight == state.manifest.TargetHeight &&
		state.tipHash.IsEqual(&state.manifest.TargetHash)
	if !completed {
		return nil
	}

	if err := flushAddrIndexDB(idx.db); err != nil {
		return err
	}
	log.Infof("Removing stale address index build staging")
	return removeAddrIndexBuildDir(idx.dataDir)
}

// dropStaleBuild drops the address index and its staging when the build target
// is no longer in the main chain, and reports whether it dropped them.
// Manager.Init calls Init first, which removes the staging of a build that set
// the index tip, so staging for an initialized index is an error.
func (idx *AddrIndex) dropStaleBuild(chain *blockchain.BlockChain,
	interrupt <-chan struct{}) (bool, error) {

	state, err := idx.loadAddrBuildState()
	if err != nil || state == nil {
		return false, err
	}
	if state.tipHeight != -1 {
		return false, fmt.Errorf("address index build staging exists " +
			"for an initialized address index, drop the address " +
			"index before restarting the build")
	}
	if addrBuildOnMainChain(chain, &state.manifest) {
		return false, nil
	}

	log.Infof("Rebuilding address index after a chain reorganization")
	if err := DropAddrIndexWithDataDir(
		idx.db, idx.dataDir, interrupt,
	); err != nil {

		return false, err
	}
	return true, nil
}

// addrBuildScanRange returns the first and last heights of the scan chunk of
// chunkSize blocks that follows the completed height, with the last height
// capped at the target.  The first height is past the target once the scan is
// complete.  The heights are int64 so the height after a target at the largest
// int32 height does not overflow.
func addrBuildScanRange(completed, target int32,
	chunkSize int64) (int64, int64) {

	start := int64(completed) + 1
	end := start + chunkSize - 1
	if end > int64(target) {
		end = int64(target)
	}
	return start, end
}

// buildAddrIndexRecords resumes or starts a scan from the genesis block to a
// snapshot of the current chain tip.  The scan stages chunkSize blocks between
// checkpoints.
func (idx *AddrIndex) buildAddrIndexRecords(
	chain *blockchain.BlockChain, chunkSize int64,
	interrupt <-chan struct{}) (*addrStager, addrBuildManifest, error) {

	best := chain.BestSnapshot()
	targetHeight := best.Height
	targetHash := best.Hash
	stagingDir := filepath.Join(
		idx.dataDir, addrIndexBuildDirName, "staging",
	)

	var (
		stager   *addrStager
		manifest addrBuildManifest
	)
	_, err := os.Stat(stagingDir)
	switch {
	case os.IsNotExist(err):
		manifest = addrBuildManifest{
			Completed:    -1,
			TargetHeight: targetHeight,
			TargetHash:   targetHash,
		}
		stager, err = createAddrBuildStager(idx.dataDir, &manifest)
		if err != nil {
			return nil, addrBuildManifest{}, err
		}

	case err != nil:
		return nil, addrBuildManifest{}, err

	default:
		var ok bool
		manifest, ok = readAddrBuildManifest(stagingDir)
		if !ok {
			return nil, addrBuildManifest{}, fmt.Errorf(
				"address index build checkpoint is invalid")
		}
		if !addrBuildOnMainChain(chain, &manifest) {
			return nil, addrBuildManifest{}, fmt.Errorf(
				"address index build range is not on the " +
					"main chain")
		}
		stager, err = openAddrStager(stagingDir, &manifest, interrupt)
		if err != nil {
			return nil, addrBuildManifest{}, err
		}
	}

	startHeight, _ := addrBuildScanRange(
		manifest.Completed, targetHeight, chunkSize,
	)
	if startHeight > int64(targetHeight) {
		return stager, manifest, nil
	}

	numWorkers := min(runtime.GOMAXPROCS(0), addrBuildDecodedScanMaxWorkers)
	log.Infof("Scanning blocks %d to %d for address index entries "+
		"using %d workers", startHeight, targetHeight, numWorkers)
	totalBlocks := uint64(int64(targetHeight) - startHeight + 1)
	progress := newAddrScanProgress(totalBlocks)

	for manifest.Completed < targetHeight {
		chunkStart, chunkEnd := addrBuildScanRange(
			manifest.Completed, targetHeight, chunkSize,
		)
		err := idx.scanAddrHeightRange(chain, stager, chunkStart,
			chunkEnd, numWorkers, progress, interrupt)
		if err != nil {
			stager.close()
			return nil, addrBuildManifest{}, err
		}

		manifest.Completed = int32(chunkEnd)
		manifest.TargetHeight = targetHeight
		manifest.TargetHash = targetHash
		if err := stager.checkpoint(&manifest, interrupt); err != nil {
			stager.close()
			return nil, addrBuildManifest{}, err
		}
	}
	return stager, manifest, nil
}

// buildAddrIndexFromChain writes the address index for every block in the
// main chain.  The scan stages chunkSize blocks between checkpoints.
func (idx *AddrIndex) buildAddrIndexFromChain(
	chain *blockchain.BlockChain, chunkSize int64,
	interrupt <-chan struct{}) (chainhash.Hash, int32, error) {

	stager, manifest, err := idx.buildAddrIndexRecords(
		chain, chunkSize, interrupt,
	)
	if err != nil {
		return chainhash.Hash{}, 0, err
	}
	defer stager.close()

	blockIDs, err := idx.fetchAddrBlockIDs(
		chain, manifest.TargetHeight, interrupt,
	)
	if err != nil {
		return chainhash.Hash{}, 0, err
	}

	log.Infof("Writing address index into the database")
	err = idx.writeAddrIndexToDB(
		stager, blockIDs, addrBuildWriteBatchBytes, interrupt,
	)
	if err != nil {
		return chainhash.Hash{}, 0, err
	}

	log.Infof("Built address index into the database at height %d (%s)",
		manifest.TargetHeight, manifest.TargetHash)
	return manifest.TargetHash, manifest.TargetHeight, nil
}

// fetchAddrBlockIDs loads the block IDs of the blocks through the target,
// indexed by height.
func (idx *AddrIndex) fetchAddrBlockIDs(chain *blockchain.BlockChain,
	targetHeight int32, interrupt <-chan struct{}) ([]uint32, error) {

	blockIDs := make([]uint32, int64(targetHeight)+1)
	err := idx.db.View(func(dbTx database.Tx) error {
		for i := range blockIDs {
			if interruptRequested(interrupt) {
				return errInterruptRequested
			}
			hash, err := chain.BlockHashByHeight(int32(i))
			if err != nil {
				return err
			}
			blockIDs[i], err = dbFetchBlockIDByHash(dbTx, hash)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return blockIDs, err
}

// NeedsFastBuild reports whether the address index has a data directory to
// stage a fast build in and is uninitialized, which holds for a new index and
// for an index with an unfinished fast build.
func (idx *AddrIndex) NeedsFastBuild(
	chain *blockchain.BlockChain) (bool, error) {

	if idx.dataDir == "" {
		return false, nil
	}

	var tipHeight int32
	err := idx.db.View(func(dbTx database.Tx) error {
		var err error
		_, tipHeight, err = dbFetchIndexerTip(dbTx, addrIndexKey)
		return err
	})
	return err == nil && tipHeight == -1, err
}

// FastBuild builds an uninitialized address index from the main chain, as
// described at the top of this file, and sets the index tip.  The transaction
// index must be caught up before this method is called.
func (idx *AddrIndex) FastBuild(chain *blockchain.BlockChain,
	interrupt <-chan struct{}) (err error) {

	defer func() {
		if err == nil || errors.Is(err, errInterruptRequested) {
			return
		}

		log.Errorf("Address index fast build failed: %v", err)
		log.Errorf("Run btcd with --dropaddrindex to remove the " +
			"failed address index, then restart with --addrindex " +
			"to try again")
	}()

	needed, err := idx.NeedsFastBuild(chain)
	if err != nil || !needed {
		return err
	}

	// Fail before staging exists when the database cannot flush, since
	// staging is only removed after the tip is flushed.
	if _, err := addrBuildFlusher(idx.db); err != nil {
		return err
	}

	log.Warnf("Older btcd versions cannot detect a partially built " +
		"address index and will silently corrupt it. If this build " +
		"is interrupted, drop the index with --dropaddrindex before " +
		"downgrading btcd")

	builtHash, builtHeight, err := idx.buildAddrIndexFromChain(
		chain, addrBuildScanChunkSize, interrupt,
	)
	if err != nil {
		return err
	}

	err = idx.db.Update(func(dbTx database.Tx) error {
		return dbPutIndexerTip(
			dbTx, addrIndexKey, &builtHash, builtHeight,
		)
	})
	if err != nil {
		return err
	}

	// Staging shows the build is unfinished until the tip is durable.
	if err := flushAddrIndexDB(idx.db); err != nil {
		return err
	}
	return removeAddrIndexBuildDir(idx.dataDir)
}

// addrIndexBuildDirExists returns the path of the build directory under
// dataDir and whether it exists.
func addrIndexBuildDirExists(dataDir string) (string, bool, error) {
	dir := filepath.Join(dataDir, addrIndexBuildDirName)
	_, err := os.Stat(dir)
	if err == nil {
		return dir, true, nil
	}
	if os.IsNotExist(err) {
		return dir, false, nil
	}
	return dir, false, err
}

// AddrIndexFastBuildDir returns the build directory and whether it exists.
func AddrIndexFastBuildDir(dataDir string) (string, bool) {
	dir, exists, _ := addrIndexBuildDirExists(dataDir)
	return dir, exists
}

// DropAddrIndexWithDataDir drops the address index and removes fast-build
// staging from dataDir.
func DropAddrIndexWithDataDir(db database.DB, dataDir string,
	interrupt <-chan struct{}) error {

	_, buildExists, err := addrIndexBuildDirExists(dataDir)
	if err != nil {
		return err
	}
	// Fail before changing the index when the database cannot flush, since
	// staging is only removed after the drop is flushed.
	if buildExists {
		if _, err := addrBuildFlusher(db); err != nil {
			return err
		}
	}
	if err := DropAddrIndex(db, interrupt); err != nil {
		return err
	}
	if !buildExists {
		return removeAddrIndexBuildDeleteDir(dataDir)
	}
	return removeAddrIndexBuildStaging(db, dataDir)
}

// removeAddrIndexBuildStaging flushes the database and then removes the build
// directory, so staging is removed only after an index drop is durable.
func removeAddrIndexBuildStaging(db database.DB, dataDir string) error {
	if err := flushAddrIndexDB(db); err != nil {
		return err
	}
	return removeAddrIndexBuildDir(dataDir)
}
