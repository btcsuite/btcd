// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

const (
	// addrRecordSize is an address key followed by one serialized address
	// index entry.
	addrRecordSize = addrKeySize + txEntrySize

	// numAddrStagingShards is the number of files used to stage records.
	// The first byte of the address hash selects the shard.
	numAddrStagingShards = 256

	// addrStagingIOBufferSize is the buffer used by each staging shard.
	addrStagingIOBufferSize = 64 * 1024

	// addrStagingInterruptCheckRecords controls interrupt checks in record
	// processing loops.
	addrStagingInterruptCheckRecords = 1 << 16

	// addrBuildManifestVersion is the manifest format version.  A manifest
	// with another version is not valid.
	addrBuildManifestVersion = 1

	// addrBuildManifestName is the name of the manifest file in the staging
	// directory.
	addrBuildManifestName = "manifest"
)

// addrRecord is the fixed size staging representation of an address index
// entry. Its trailing bytes use the address index value encoding. On disk, the
// block ID field holds the block height, which the write pass replaces with the
// block ID before it writes the records to the database.
type addrRecord [addrRecordSize]byte

// makeAddrRecord returns the staging record for the provided address key,
// block ID and transaction location.  The bytes after the address key are the
// entry serializeAddrIndexEntry returns for the same block ID and transaction
// location.
func makeAddrRecord(addrKey *[addrKeySize]byte, blockID uint32,
	txLoc wire.TxLoc) addrRecord {

	var record addrRecord
	copy(record[:addrKeySize], addrKey[:])
	byteOrder.PutUint32(record[addrKeySize:], blockID)
	byteOrder.PutUint32(record[addrKeySize+4:], uint32(txLoc.TxStart))
	byteOrder.PutUint32(record[addrKeySize+8:], uint32(txLoc.TxLen))
	return record
}

// key returns the address key of the record.
func (r *addrRecord) key() [addrKeySize]byte {
	var key [addrKeySize]byte
	copy(key[:], r[:addrKeySize])
	return key
}

// blockID returns the block ID field of the record, which holds the block
// height until the write pass replaces it with the block ID.
func (r *addrRecord) blockID() uint32 {
	return byteOrder.Uint32(r[addrKeySize:])
}

// less orders records by address key, block height and position in the block.
// This is the order ConnectBlock adds the entries of an address in, since
// indexBlock visits the transactions of a block in order.  Records that compare
// equal are identical, so the unstable sort.Sort gives one result.
func (r *addrRecord) less(other *addrRecord) bool {
	cmp := bytes.Compare(r[:addrKeySize], other[:addrKeySize])
	if cmp != 0 {
		return cmp < 0
	}
	if r.blockID() != other.blockID() {
		return r.blockID() < other.blockID()
	}
	return byteOrder.Uint32(r[addrKeySize+4:]) <
		byteOrder.Uint32(other[addrKeySize+4:])
}

// addrRecords implements sort.Interface to allow a slice of staging records to
// be sorted in the order defined by less.
type addrRecords []addrRecord

// Len returns the number of records in the slice.  It is part of the
// sort.Interface implementation.
func (r addrRecords) Len() int {
	return len(r)
}

// Less returns whether the record with index i should sort before the record
// with index j.  It is part of the sort.Interface implementation.
func (r addrRecords) Less(i, j int) bool {
	return r[i].less(&r[j])
}

// Swap swaps the records at the passed indices.  It is part of the
// sort.Interface implementation.
func (r addrRecords) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

// addrStagingShard serializes concurrent writes to one staging file.
type addrStagingShard struct {
	mu  sync.Mutex
	f   *os.File
	buf *bufio.Writer
}

// addrStager owns the staging files for one address index build.
type addrStager struct {
	dir    string
	shards [numAddrStagingShards]addrStagingShard
}

// addrStagingShardPath returns the path of the staging file for the provided
// shard in dir.
func addrStagingShardPath(dir string, shard int) string {
	return filepath.Join(dir, fmt.Sprintf("shard-%03d", shard))
}

// newAddrStager creates an empty staging file for each shard in dir and
// returns a stager that writes to them.
func newAddrStager(dir string) (*addrStager, error) {
	stager := &addrStager{dir: dir}
	for i := range stager.shards {
		f, err := os.Create(addrStagingShardPath(dir, i))
		if err != nil {
			stager.close()
			return nil, err
		}
		stager.shards[i].f = f
		stager.shards[i].buf = bufio.NewWriterSize(
			f, addrStagingIOBufferSize,
		)
	}
	return stager, nil
}

// openAddrStager opens the staging files in dir at the sizes the manifest
// records.  A shard longer than its recorded size is truncated, which drops
// the records written after the last checkpoint, and a shorter shard is an
// error.  New records are appended after the recorded size.
func openAddrStager(dir string, manifest *addrBuildManifest,
	interrupt <-chan struct{}) (*addrStager, error) {

	stager := &addrStager{dir: dir}
	for i, expectedSize := range manifest.ShardSizes {
		if interruptRequested(interrupt) {
			stager.close()
			return nil, errInterruptRequested
		}

		// Windows does not allow truncating a handle opened with
		// O_APPEND, so writes resume from an explicit offset.
		path := addrStagingShardPath(dir, i)
		f, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			stager.close()
			return nil, err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			stager.close()
			return nil, err
		}
		if info.Size() < expectedSize {
			f.Close()
			stager.close()
			return nil, fmt.Errorf("address staging shard %d is "+
				"shorter than its checkpoint", i)
		}
		if info.Size() != expectedSize {
			if err := f.Truncate(expectedSize); err != nil {
				f.Close()
				stager.close()
				return nil, err
			}
		}
		if _, err := f.Seek(expectedSize, io.SeekStart); err != nil {
			f.Close()
			stager.close()
			return nil, err
		}

		stager.shards[i].f = f
		stager.shards[i].buf = bufio.NewWriterSize(
			f, addrStagingIOBufferSize,
		)
	}
	return stager, nil
}

// add stages the record for the provided address key, block ID and transaction
// location in the shard selected by the first byte of the address hash.  The
// next checkpoint makes the record durable, and a scan resumed before then
// drops it.  It is safe for concurrent use.
func (s *addrStager) add(addrKey *[addrKeySize]byte, blockID uint32,
	txLoc wire.TxLoc) error {

	shard := &s.shards[addrKey[1]]
	shard.mu.Lock()
	defer shard.mu.Unlock()

	record := makeAddrRecord(addrKey, blockID, txLoc)
	_, err := shard.buf.Write(record[:])
	return err
}

// checkpoint makes all staged records durable, then records their exact file
// sizes with the completed height.
func (s *addrStager) checkpoint(manifest *addrBuildManifest,
	interrupt <-chan struct{}) error {

	if len(manifest.ShardSizes) != numAddrStagingShards {
		manifest.ShardSizes = make([]int64, numAddrStagingShards)
	}
	for i := range s.shards {
		if interruptRequested(interrupt) {
			return errInterruptRequested
		}
		shard := &s.shards[i]
		shard.mu.Lock()
		if err := shard.buf.Flush(); err != nil {
			shard.mu.Unlock()
			return err
		}
		if err := shard.f.Sync(); err != nil {
			shard.mu.Unlock()
			return err
		}
		info, err := shard.f.Stat()
		shard.mu.Unlock()
		if err != nil {
			return err
		}
		manifest.ShardSizes[i] = info.Size()
	}
	return writeAddrBuildManifest(s.dir, manifest)
}

// close closes the staging files.  It does not flush buffered records, so
// records added after the last checkpoint can be missing from the files.  It
// must not run concurrently with add or checkpoint.  Calling close again does
// nothing.
func (s *addrStager) close() {
	for i := range s.shards {
		if s.shards[i].f == nil {
			continue
		}
		s.shards[i].f.Close()
		s.shards[i].f = nil
		s.shards[i].buf = nil
	}
}

// readAddrStagingShard reads every record in the staging file at path.  It
// returns an error when the file ends with a partial record.
func readAddrStagingShard(path string,
	interrupt <-chan struct{}) ([]addrRecord, error) {

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size()%addrRecordSize != 0 {
		return nil, fmt.Errorf("address staging shard has a partial " +
			"record")
	}
	numRecords := info.Size() / addrRecordSize
	if int64(int(numRecords)) != numRecords {
		return nil, fmt.Errorf("address staging shard is too large")
	}

	records := make([]addrRecord, int(numRecords))
	reader := bufio.NewReaderSize(f, addrStagingIOBufferSize)
	for i := range records {
		if i%addrStagingInterruptCheckRecords == 0 &&
			interruptRequested(interrupt) {

			return nil, errInterruptRequested
		}
		if _, err := io.ReadFull(reader, records[i][:]); err != nil {
			return nil, err
		}
	}
	return records, nil
}

// addrBuildManifest identifies the chain range and durable shard prefixes of
// a resumable scan.
type addrBuildManifest struct {
	// Version is the manifest format version.  writeAddrBuildManifest
	// sets it to addrBuildManifestVersion.
	Version uint32 `json:"version"`

	// Completed is the completed height, or -1 until the checkpoint after
	// the first scan chunk.  The shard prefixes whose sizes ShardSizes
	// holds contain the records of every block from the genesis block
	// through Completed.
	Completed int32 `json:"completed"`

	// TargetHeight and TargetHash identify the build target, the last
	// block the scan stages.  A resumed scan extends the target to the
	// best chain tip at its next checkpoint, and the build sets the index
	// tip to the target after the write pass.
	TargetHeight int32          `json:"target_height"`
	TargetHash   chainhash.Hash `json:"target_hash"`

	// ShardSizes holds the size in bytes of each staging file at the last
	// checkpoint, indexed by shard.  A resumed scan truncates a longer
	// staging file to its recorded size, and a shorter one is an error.
	ShardSizes []int64 `json:"shard_sizes"`
}

// writeAddrBuildManifest sets the manifest version and writes the manifest to
// stagingDir.  It writes and syncs manifest.tmp, renames it over any earlier
// manifest and syncs stagingDir, so a crash leaves the earlier manifest, if
// any, or the new one.
func writeAddrBuildManifest(stagingDir string,
	manifest *addrBuildManifest) error {

	manifest.Version = addrBuildManifestVersion
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmpPath := filepath.Join(stagingDir, addrBuildManifestName+".tmp")
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		f.Close()
		if remove {
			os.Remove(tmpPath)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	manifestPath := filepath.Join(stagingDir, addrBuildManifestName)
	if err := os.Rename(tmpPath, manifestPath); err != nil {
		return err
	}
	remove = false
	return syncAddrBuildDir(stagingDir)
}

// readAddrBuildManifest reads the manifest in stagingDir and reports whether it
// is valid.  A valid manifest has version addrBuildManifestVersion, a target
// height of at least 0, a completed height from -1 through the target height,
// and one size per shard, each a whole number of records.  A manifest that
// cannot be read or decoded is not valid.
func readAddrBuildManifest(stagingDir string) (addrBuildManifest, bool) {
	var manifest addrBuildManifest
	path := filepath.Join(stagingDir, addrBuildManifestName)
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &manifest) != nil ||
		manifest.Version != addrBuildManifestVersion ||
		manifest.TargetHeight < 0 || manifest.Completed < -1 ||
		manifest.Completed > manifest.TargetHeight ||
		len(manifest.ShardSizes) != numAddrStagingShards {

		return addrBuildManifest{}, false
	}
	for _, size := range manifest.ShardSizes {
		if size < 0 || size%addrRecordSize != 0 {
			return addrBuildManifest{}, false
		}
	}
	return manifest, true
}

// syncAddrBuildDir makes a published staging entry durable.  Directory handles
// cannot be synced on Windows through os.File.
func syncAddrBuildDir(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		dir.Close()
		return err
	}
	return dir.Close()
}
