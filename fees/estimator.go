// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fees

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/syndtr/goleveldb/leveldb"
	ldbutil "github.com/syndtr/goleveldb/leveldb/util"
)

const (
	// DefaultMaxBucketFeeMultiplier is the default multiplier used to find the
	// largest fee bucket, starting at the minimum fee.
	DefaultMaxBucketFeeMultiplier int = 100

	// DefaultMaxConfirmations is the default number of confirmation ranges to
	// track in the estimator.
	DefaultMaxConfirmations uint32 = 32

	// DefaultFeeRateStep is the default multiplier between two consecutive fee
	// rate buckets.
	DefaultFeeRateStep float64 = 1.1

	// defaultDecay is the default value used to decay old transactions from the
	// estimator.
	defaultDecay float64 = 0.998

	// maxAllowedBucketFees is an upper bound of how many bucket fees can be
	// used in the estimator. This is verified during estimator initialization
	// and database loading.
	maxAllowedBucketFees = 2000

	// maxAllowedConfirms is an upper bound of how many confirmation ranges can
	// be used in the estimator. This is verified during estimator
	// initialization and database loading.
	maxAllowedConfirms = 788
)

var (
	// ErrNoSuccessPctBucketFound is the error returned when no bucket has been
	// found with the minimum required percentage success.
	ErrNoSuccessPctBucketFound = errors.New("no bucket with the minimum " +
		"required success percentage found")

	// ErrNotEnoughTxsForEstimate is the error returned when not enough
	// transactions have been seen by the fee generator to give an estimate.
	ErrNotEnoughTxsForEstimate = errors.New("not enough transactions seen for " +
		"estimation")

	dbByteOrder = binary.BigEndian

	dbKeyVersion      = []byte("version")
	dbKeyBucketFees   = []byte("bucketFeeBounds")
	dbKeyMaxConfirms  = []byte("maxConfirms")
	dbKeyBestHeight   = []byte("bestHeight")
	dbKeyBucketPrefix = []byte{0x01, 0x70, 0x1d, 0x00}
)

// ErrTargetConfTooLarge is the type of error returned when an user of the
// estimator requested a confirmation range higher than tracked by the estimator.
type ErrTargetConfTooLarge struct {
	MaxConfirms int32
	ReqConfirms int32
}

func (e ErrTargetConfTooLarge) Error() string {
	return fmt.Sprintf("target confirmation requested (%d) higher than "+
		"maximum confirmation range tracked by estimator (%d)", e.ReqConfirms,
		e.MaxConfirms)
}

type feeRate float64

type txConfirmStatBucketCount struct {
	txCount float64
	feeSum  float64
}

type txConfirmStatBucket struct {
	confirmed    []txConfirmStatBucketCount
	confirmCount float64
	feeSum       float64
}

// EstimatorConfig stores the configuration parameters for a given fee
// estimator. It is used to initialize an empty fee estimator.
type EstimatorConfig struct {
	// MaxConfirms is the maximum number of confirmation ranges to check.
	MaxConfirms uint32

	// MinBucketFee is the value of the fee rate of the lowest bucket for which
	// estimation is tracked.
	MinBucketFee dcrutil.Amount

	// MaxBucketFee is the value of the fee for the highest bucket for which
	// estimation is tracked.
	//
	// It MUST be higher than MinBucketFee.
	MaxBucketFee dcrutil.Amount

	// ExtraBucketFee is an additional bucket fee rate to include in the
	// database for tracking transactions. Specifying this can be useful when
	// the default relay fee of the network is undergoing change (due to a new
	// release of the software for example), so that the older fee can be
	// tracked exactly.
	//
	// It MUST have a value between MinBucketFee and MaxBucketFee, otherwise
	// it's ignored.
	ExtraBucketFee dcrutil.Amount

	// FeeRateStep is the multiplier to generate the fee rate buckets (each
	// bucket is higher than the previous one by this factor).
	//
	// It MUST have a value > 1.0.
	FeeRateStep float64

	// DatabaseFile is the location of the estimator database file. If empty,
	// updates to the estimator state are not backed by the filesystem.
	DatabaseFile string

	// ReplaceBucketsOnLoad indicates whether to replace the buckets in the
	// current estimator by those stored in the feesdb file instead of
	// validating that they are both using the same set of fees.
	ReplaceBucketsOnLoad bool
}

// memPoolTxDesc is an aux structure used to track the local estimator mempool.
type memPoolTxDesc struct {
	addedHeight int64
	bucketIndex int32
	fees        feeRate
}

// Estimator tracks historical data for published and mined transactions in
// order to estimate fees to be used in new transactions for confirmation
// within a target block window.
type Estimator struct {
	// bucketFeeBounds are the upper bounds for each individual fee bucket.
	bucketFeeBounds []feeRate

	// buckets are the confirmed tx count and fee sum by bucket fee.
	buckets []txConfirmStatBucket

	// memPool are the mempool transaction count and fee sum by bucket fee.
	memPool []txConfirmStatBucket

	// memPoolTxs is the map of transaction hashes and data of known mempool txs.
	memPoolTxs map[chainhash.Hash]memPoolTxDesc

	maxConfirms int32
	decay       float64
	bestHeight  int64
	db          *leveldb.DB
	lock        sync.RWMutex
}

// NewEstimator returns an empty estimator given a config. This estimator
// then needs to be fed data for published and mined transactions before it can
// be used to estimate fees for new transactions.
func NewEstimator(cfg *EstimatorConfig) (*Estimator, error) {
	// Sanity check the config.
	if cfg.MaxBucketFee <= cfg.MinBucketFee {
		return nil, errors.New("maximum bucket fee should not be lower than " +
			"minimum bucket fee")
	}
	if cfg.FeeRateStep <= 1.0 {
		return nil, errors.New("fee rate step should not be <= 1.0")
	}
	if cfg.MinBucketFee <= 0 {
		return nil, errors.New("minimum bucket fee rate cannot be <= 0")
	}
	if cfg.MaxConfirms > maxAllowedConfirms {
		return nil, fmt.Errorf("confirmation count requested (%d) larger than "+
			"maximum allowed (%d)", cfg.MaxConfirms, maxAllowedConfirms)
	}

	decay := defaultDecay
	maxConfirms := cfg.MaxConfirms
	max := float64(cfg.MaxBucketFee)
	var bucketFees []feeRate
	prevF := 0.0
	extraBucketFee := float64(cfg.ExtraBucketFee)
	for f := float64(cfg.MinBucketFee); f < max; f *= cfg.FeeRateStep {
		if (f > extraBucketFee) && (prevF < extraBucketFee) {
			// Add the extra bucket fee for tracking.
			bucketFees = append(bucketFees, feeRate(extraBucketFee))
		}
		bucketFees = append(bucketFees, feeRate(f))
		prevF = f
	}

	// The last bucket catches everything else, so it uses an upper bound of
	// +inf which any rate must be lower than.
	bucketFees = append(bucketFees, feeRate(math.Inf(1)))

	nbBuckets := len(bucketFees)
	res := &Estimator{
		bucketFeeBounds: bucketFees,
		buckets:         make([]txConfirmStatBucket, nbBuckets),
		memPool:         make([]txConfirmStatBucket, nbBuckets),
		maxConfirms:     int32(maxConfirms),
		decay:           decay,
		memPoolTxs:      make(map[chainhash.Hash]memPoolTxDesc),
		bestHeight:      -1,
	}

	for i := range bucketFees {
		res.buckets[i] = txConfirmStatBucket{
			confirmed: make([]txConfirmStatBucketCount, maxConfirms),
		}
		res.memPool[i] = txConfirmStatBucket{
			confirmed: make([]txConfirmStatBucketCount, maxConfirms),
		}
	}

	if cfg.DatabaseFile != "" {
		db, err := leveldb.OpenFile(cfg.DatabaseFile, nil)
		if err != nil {
			return nil, fmt.Errorf("error opening estimator database: %v", err)
		}
		res.db = db

		err = res.loadFromDatabase(cfg.ReplaceBucketsOnLoad)
		if err != nil {
			return nil, fmt.Errorf("error loading estimator data from db: %v",
				err)
		}
	}

	return res, nil
}

// DumpBuckets returns the internal estimator state as a string.
func (stats *Estimator) DumpBuckets() string {
	res := "          |"
	for c := 0; c < int(stats.maxConfirms); c++ {
		if c == int(stats.maxConfirms)-1 {
			res += fmt.Sprintf("   %15s", "+Inf")
		} else {
			res += fmt.Sprintf("   %15d|", c+1)
		}
	}
	res += "\n"

	l := len(stats.bucketFeeBounds)
	for i := 0; i < l; i++ {
		res += fmt.Sprintf("%10.8f", stats.bucketFeeBounds[i]/1e8)
		for c := 0; c < int(stats.maxConfirms); c++ {
			avg := float64(0)
			count := stats.buckets[i].confirmed[c].txCount
			if stats.buckets[i].confirmed[c].txCount > 0 {
				avg = stats.buckets[i].confirmed[c].feeSum /
					stats.buckets[i].confirmed[c].txCount / 1e8
			}

			res += fmt.Sprintf("| %.8f %6.1f", avg, count)
		}
		res += "\n"
	}

	return res
}

// loadFromDatabase loads the estimator data from the currently opened database
// and performs any db upgrades if required. After loading, it updates the db
// with the current estimator configuration.
//
// Argument replaceBuckets indicates if the buckets in the current stats should
// be completely replaced by what is stored in the database or if the data
// should be validated against what is current in the estimator.
//
// The database should *not* be used while loading is taking place.
//
// The current code does not support loading from a database created with a
// different set of configuration parameters (fee rate buckets, max confirmation
// range, etc) than the current estimator is configured with. If an incompatible
// file is detected during loading, an error is returned and the user must
// either reconfigure the estimator to use the same parameters to allow the
// database to be loaded or they must ignore the database file (possibly by
// deleting it) so that the new parameters are used. In the future it might be
// possible to load from a different set of configuration parameters.
//
// The current code does not currently save mempool information, since saving
// information in the estimator without saving the corresponding data in the
// mempool itself could result in transactions lingering in the mempool
// estimator forever.
func (stats *Estimator) loadFromDatabase(replaceBuckets bool) error {
	if stats.db == nil {
		return errors.New("estimator database is not open")
	}

	// Database version is currently hardcoded here as this is the only
	// place that uses it.
	currentDbVersion := []byte{1}

	version, err := stats.db.Get(dbKeyVersion, nil)
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return fmt.Errorf("error reading version from db: %v", err)
	}
	if len(version) < 1 {
		// No data in the file. Fill with the current config.
		batch := new(leveldb.Batch)
		b := bytes.NewBuffer(nil)
		var maxConfirmsBytes [4]byte
		var bestHeightBytes [8]byte

		batch.Put(dbKeyVersion, currentDbVersion)

		dbByteOrder.PutUint32(maxConfirmsBytes[:], uint32(stats.maxConfirms))
		batch.Put(dbKeyMaxConfirms, maxConfirmsBytes[:])

		dbByteOrder.PutUint64(bestHeightBytes[:], uint64(stats.bestHeight))
		batch.Put(dbKeyBestHeight, bestHeightBytes[:])

		err := binary.Write(b, dbByteOrder, stats.bucketFeeBounds)
		if err != nil {
			return fmt.Errorf("error writing bucket fees to db: %v", err)
		}
		batch.Put(dbKeyBucketFees, b.Bytes())

		err = stats.db.Write(batch, nil)
		if err != nil {
			return fmt.Errorf("error writing initial estimator db file: %v",
				err)
		}

		err = stats.updateDatabase()
		if err != nil {
			return fmt.Errorf("error adding initial estimator data to db: %v",
				err)
		}

		log.Debug("Initialized fee estimator database")

		return nil
	}

	if !bytes.Equal(currentDbVersion, version) {
		return fmt.Errorf("incompatible database version: %d", version)
	}

	maxConfirmsBytes, err := stats.db.Get(dbKeyMaxConfirms, nil)
	if err != nil {
		return fmt.Errorf("error reading max confirmation range from db file: "+
			"%v", err)
	}
	if len(maxConfirmsBytes) != 4 {
		return errors.New("wrong number of bytes in stored maxConfirms")
	}
	fileMaxConfirms := int32(dbByteOrder.Uint32(maxConfirmsBytes))
	if fileMaxConfirms > maxAllowedConfirms {
		return fmt.Errorf("confirmation count stored in database (%d) larger "+
			"than maximum allowed (%d)", fileMaxConfirms, maxAllowedConfirms)
	}

	feesBytes, err := stats.db.Get(dbKeyBucketFees, nil)
	if err != nil {
		return fmt.Errorf("error reading fee bounds from db file: %v", err)
	}
	if feesBytes == nil {
		return errors.New("fee bounds not found in database file")
	}
	fileNbBucketFees := len(feesBytes) / 8
	if fileNbBucketFees > maxAllowedBucketFees {
		return fmt.Errorf("more fee buckets stored in file (%d) than allowed "+
			"(%d)", fileNbBucketFees, maxAllowedBucketFees)
	}
	fileBucketFees := make([]feeRate, fileNbBucketFees)
	err = binary.Read(bytes.NewReader(feesBytes), dbByteOrder,
		&fileBucketFees)
	if err != nil {
		return fmt.Errorf("error decoding file bucket fees: %v", err)
	}

	if !replaceBuckets {
		if stats.maxConfirms != fileMaxConfirms {
			return errors.New("max confirmation range in database file different " +
				"than currently configured max confirmation")
		}

		if len(stats.bucketFeeBounds) != len(fileBucketFees) {
			return errors.New("number of bucket fees stored in database file " +
				"different than currently configured bucket fees")
		}

		for i, f := range fileBucketFees {
			if stats.bucketFeeBounds[i] != f {
				return errors.New("bucket fee rates stored in database file " +
					"different than currently configured fees")
			}
		}
	}

	fileBuckets := make([]txConfirmStatBucket, fileNbBucketFees)

	iter := stats.db.NewIterator(ldbutil.BytesPrefix(dbKeyBucketPrefix), nil)
	err = nil
	var fbytes [8]byte
	for iter.Next() {
		key := iter.Key()
		if len(key) != 8 {
			err = fmt.Errorf("bucket key read from db has wrong length (%d)",
				len(key))
			break
		}
		idx := int(int32(dbByteOrder.Uint32(key[4:])))
		if (idx >= len(fileBuckets)) || (idx < 0) {
			err = fmt.Errorf("wrong bucket index read from db (%d vs %d)",
				idx, len(fileBuckets))
			break
		}
		value := iter.Value()
		if len(value) != 8+8+int(fileMaxConfirms)*16 {
			err = errors.New("wrong size of data in bucket read from db")
			break
		}

		b := bytes.NewBuffer(value)
		readf := func() float64 {
			// We ignore the error here because the only possible one is EOF and
			// we already previously checked the length of the source byte array
			// for consistency.
			b.Read(fbytes[:])
			return math.Float64frombits(dbByteOrder.Uint64(fbytes[:]))
		}

		fileBuckets[idx].confirmCount = readf()
		fileBuckets[idx].feeSum = readf()
		fileBuckets[idx].confirmed = make([]txConfirmStatBucketCount, fileMaxConfirms)
		for i := range fileBuckets[idx].confirmed {
			fileBuckets[idx].confirmed[i].txCount = readf()
			fileBuckets[idx].confirmed[i].feeSum = readf()
		}
	}
	iter.Release()
	if err != nil {
		return err
	}
	err = iter.Error()
	if err != nil {
		return fmt.Errorf("error on bucket iterator: %v", err)
	}

	stats.bucketFeeBounds = fileBucketFees
	stats.buckets = fileBuckets
	stats.maxConfirms = fileMaxConfirms
	log.Debug("Loaded fee estimator database")

	return nil
}

// updateDatabase updates the current database file with the current bucket
// data. This is called during normal operation after processing mined
// transactions, so it only updates data that might have changed.
func (stats *Estimator) updateDatabase() error {
	if stats.db == nil {
		return errors.New("estimator database is closed")
	}

	batch := new(leveldb.Batch)
	buf := bytes.NewBuffer(nil)

	var key [8]byte
	copy(key[:], dbKeyBucketPrefix)
	var fbytes [8]byte
	writef := func(f float64) {
		dbByteOrder.PutUint64(fbytes[:], math.Float64bits(f))
		_, err := buf.Write(fbytes[:])
		if err != nil {
			panic(err) // only possible error is ErrTooLarge
		}
	}

	for i, b := range stats.buckets {
		dbByteOrder.PutUint32(key[4:], uint32(i))
		buf.Reset()
		writef(b.confirmCount)
		writef(b.feeSum)
		for _, c := range b.confirmed {
			writef(c.txCount)
			writef(c.feeSum)
		}
		batch.Put(key[:], buf.Bytes())
	}

	var bestHeightBytes [8]byte

	dbByteOrder.PutUint64(bestHeightBytes[:], uint64(stats.bestHeight))
	batch.Put(dbKeyBestHeight, bestHeightBytes[:])

	err := stats.db.Write(batch, nil)
	if err != nil {
		return fmt.Errorf("error writing update to estimator db file: %v",
			err)
	}

	return nil
}

// lowerBucket returns the bucket that has the highest upperBound such that it
// is still lower than rate.
func (stats *Estimator) lowerBucket(rate feeRate) int32 {
	res := sort.Search(len(stats.bucketFeeBounds), func(i int) bool {
		return stats.bucketFeeBounds[i] >= rate
	})
	return int32(res)
}

// confirmRange returns the confirmation range index to be used for the given
// number of blocks to confirm. The last confirmation range has an upper bound
// of +inf to mean that it represents all confirmations higher than the second
// to last bucket.
func (stats *Estimator) confirmRange(blocksToConfirm int32) int32 {
	idx := blocksToConfirm - 1
	if idx >= stats.maxConfirms {
		return stats.maxConfirms - 1
	}
	return idx
}

// updateMovingAverages updates the moving averages for the existing confirmed
// statistics and increases the confirmation ranges for mempool txs. This is
// meant to be called when a new block is mined, so that we discount older
// information.
func (stats *Estimator) updateMovingAverages(newHeight int64) {
	log.Debugf("Updated moving averages into block %d", newHeight)

	// decay the existing stats so that, over time, we rely on more up to date
	// information regarding fees.
	for b := 0; b < len(stats.buckets); b++ {
		bucket := &stats.buckets[b]
		bucket.feeSum *= stats.decay
		bucket.confirmCount *= stats.decay
		for c := 0; c < len(bucket.confirmed); c++ {
			conf := &bucket.confirmed[c]
			conf.feeSum *= stats.decay
			conf.txCount *= stats.decay
		}
	}

	// For unconfirmed (mempool) transactions, every transaction will now take
	// at least one additional block to confirm. So for every fee bucket, we
	// move the stats up one confirmation range.
	for b := 0; b < len(stats.memPool); b++ {
		bucket := &stats.memPool[b]

		// The last confirmation range represents all txs confirmed at >= than
		// the initial maxConfirms, so we *add* the second to last range into
		// the last range.
		c := len(bucket.confirmed) - 1
		bucket.confirmed[c].txCount += bucket.confirmed[c-1].txCount
		bucket.confirmed[c].feeSum += bucket.confirmed[c-1].feeSum

		// For the other ranges, just move up the stats.
		for c--; c > 0; c-- {
			bucket.confirmed[c] = bucket.confirmed[c-1]
		}

		// and finally, the very first confirmation range (ie, what will enter
		// the mempool now that a new block has been mined) is zeroed so we can
		// start tracking brand new txs.
		bucket.confirmed[0].txCount = 0
		bucket.confirmed[0].feeSum = 0
	}

	stats.bestHeight = newHeight
}

// newMemPoolTx records a new memPool transaction into the stats. A brand new
// mempool transaction has a minimum confirmation range of 1, so it is inserted
// into the very first confirmation range bucket of the appropriate fee rate
// bucket.
func (stats *Estimator) newMemPoolTx(bucketIdx int32, fees feeRate) {
	conf := &stats.memPool[bucketIdx].confirmed[0]
	conf.feeSum += float64(fees)
	conf.txCount++
}

// newMinedTx moves a mined tx from the mempool into the confirmed statistics.
// Note that this should only be called if the transaction had been seen and
// previously tracked by calling newMemPoolTx for it. Failing to observe that
// will result in undefined statistical results.
func (stats *Estimator) newMinedTx(blocksToConfirm int32, rate feeRate) {
	bucketIdx := stats.lowerBucket(rate)
	confirmIdx := stats.confirmRange(blocksToConfirm)
	bucket := &stats.buckets[bucketIdx]

	// increase the counts for all confirmation ranges starting at the first
	// confirmIdx because it took at least `blocksToConfirm` for this tx to be
	// mined. This is used to simplify the bucket selection during estimation,
	// so that we only need to check a single confirmation range (instead of
	// iterating to sum all confirmations with <= `minConfs`).
	for c := int(confirmIdx); c < len(bucket.confirmed); c++ {
		conf := &bucket.confirmed[c]
		conf.feeSum += float64(rate)
		conf.txCount++
	}
	bucket.confirmCount++
	bucket.feeSum += float64(rate)
}

func (stats *Estimator) removeFromMemPool(blocksInMemPool int32, rate feeRate) {
	bucketIdx := stats.lowerBucket(rate)
	confirmIdx := stats.confirmRange(blocksInMemPool + 1)
	bucket := &stats.memPool[bucketIdx]
	conf := &bucket.confirmed[confirmIdx]
	conf.feeSum -= float64(rate)
	conf.txCount--
	if conf.txCount < 0 {
		// If this happens, it means a transaction has been called on this
		// function but not on a previous newMemPoolTx. This leaves the fee db
		// in an undefined state and should never happen in regular use. If this
		// happens, then there is a logic or coding error somewhere, either in
		// the estimator itself or on its hooking to the mempool/network sync
		// manager.  Either way, the easiest way to fix this is to completely
		// delete the database and start again.  During development, you can use
		// a panic() here and we might return it after being confident that the
		// estimator is completely bug free.
		log.Errorf("Transaction count in bucket index %d and confirmation "+
			"index %d became < 0", bucketIdx, confirmIdx)
	}
}

// estimateMedianFee estimates the median fee rate for the current recorded
// statistics such that at least successPct transactions have been mined on all
// tracked fee rate buckets with fee >= to the median.
// In other words, this is the median fee of the lowest bucket such that it and
// all higher fee buckets have >= successPct transactions confirmed in at most
// `targetConfs` confirmations.
// Note that sometimes the requested combination of targetConfs and successPct is
// not achievable (hypothetical example: 99% of txs confirmed within 1 block)
// or there are not enough recorded statistics to derive a successful estimate
// (eg: confirmation tracking has only started or there was a period of very few
// transactions). In those situations, the appropriate error is returned.
func (stats *Estimator) estimateMedianFee(targetConfs int32, successPct float64) (feeRate, error) {
	if targetConfs <= 0 {
		return 0, errors.New("target confirmation range cannot be <= 0")
	}

	const minTxCount float64 = 1

	if (targetConfs - 1) >= stats.maxConfirms {
		// We might want to add support to use a targetConf at +infinity to
		// allow us to make estimates at confirmation interval higher than what
		// we currently track.
		return 0, ErrTargetConfTooLarge{MaxConfirms: stats.maxConfirms,
			ReqConfirms: targetConfs}
	}

	startIdx := len(stats.buckets) - 1
	confirmRangeIdx := stats.confirmRange(targetConfs)

	var totalTxs, confirmedTxs float64
	bestBucketsStt := startIdx
	bestBucketsEnd := startIdx
	curBucketsEnd := startIdx

	for b := startIdx; b >= 0; b-- {
		totalTxs += stats.buckets[b].confirmCount
		confirmedTxs += stats.buckets[b].confirmed[confirmRangeIdx].txCount

		// Add the mempool (unconfirmed) transactions to the total tx count
		// since a very large mempool for the given bucket might mean that
		// miners are reluctant to include these in their mined blocks.
		totalTxs += stats.memPool[b].confirmed[confirmRangeIdx].txCount

		if totalTxs > minTxCount {
			if confirmedTxs/totalTxs < successPct {
				if curBucketsEnd == startIdx {
					return 0, ErrNoSuccessPctBucketFound
				}
				break
			}

			bestBucketsStt = b
			bestBucketsEnd = curBucketsEnd
			curBucketsEnd = b - 1
			totalTxs = 0
			confirmedTxs = 0
		}
	}

	txCount := float64(0)
	for b := bestBucketsStt; b <= bestBucketsEnd; b++ {
		txCount += stats.buckets[b].confirmCount
	}
	if txCount <= 0 {
		return 0, ErrNotEnoughTxsForEstimate
	}
	txCount /= 2
	for b := bestBucketsStt; b <= bestBucketsEnd; b++ {
		if stats.buckets[b].confirmCount < txCount {
			txCount -= stats.buckets[b].confirmCount
		} else {
			median := stats.buckets[b].feeSum / stats.buckets[b].confirmCount
			return feeRate(median), nil
		}
	}

	return 0, errors.New("this isn't supposed to be reached")
}

// EstimateFee is the public version of estimateMedianFee. It calculates the
// suggested fee for a transaction to be confirmed in at most `targetConf`
// blocks after publishing with a high degree of certainty.
//
// This function is safe to be called from multiple goroutines but might block
// until concurrent modifications to the internal database state are complete.
func (stats *Estimator) EstimateFee(targetConfs int32) (dcrutil.Amount, error) {
	stats.lock.RLock()
	rate, err := stats.estimateMedianFee(targetConfs, 0.95)
	stats.lock.RUnlock()

	if err != nil {
		return 0, err
	}

	rate = feeRate(math.Round(float64(rate)))
	if rate < stats.bucketFeeBounds[0] {
		// Prevent our public facing api to ever return something lower than the
		// minimum fee
		rate = stats.bucketFeeBounds[0]
	}

	return dcrutil.Amount(rate), nil
}

// Enable establishes the current best height of the blockchain after
// initializing the chain. All new mempool transactions will be added at this
// block height.
func (stats *Estimator) Enable(bestHeight int64) {
	log.Debugf("Setting best height as %d", bestHeight)
	stats.lock.Lock()
	stats.bestHeight = bestHeight
	stats.lock.Unlock()
}

// IsEnabled returns whether the fee estimator is ready to accept new mined and
// mempool transactions.
func (stats *Estimator) IsEnabled() bool {
	stats.lock.RLock()
	enabled := stats.bestHeight > -1
	stats.lock.RUnlock()
	return enabled
}

// AddMemPoolTransaction adds a mempool transaction to the estimator in order to
// account for it in the estimations. It assumes that this transaction is
// entering the mempool at the currently recorded best chain hash, using the
// total fee amount (in atoms) and with the provided size (in bytes).
//
// This is safe to be called from multiple goroutines.
func (stats *Estimator) AddMemPoolTransaction(txHash *chainhash.Hash, fee, size int64, txType stake.TxType) {
	stats.lock.Lock()
	defer stats.lock.Unlock()

	if stats.bestHeight < 0 {
		return
	}

	if _, exists := stats.memPoolTxs[*txHash]; exists {
		// we should not double count transactions
		return
	}

	// Ignore tspends for the purposes of fee estimation, since they remain
	// in the mempool for a long time and have special rules about when
	// they can be included in blocks.
	if txType == stake.TxTypeTSpend {
		return
	}

	// Note that we use this less exact version instead of fee * 1000 / size
	// (using ints) because it naturally "downsamples" the fee rates towards the
	// minimum at values less than 0.001 DCR/KB. This is needed because due to
	// how the wallet estimates the final fee given an input rate and the final
	// tx size, there's usually a small discrepancy towards a higher effective
	// rate in the published tx.
	rate := feeRate(fee / size * 1000)

	if rate < stats.bucketFeeBounds[0] {
		// Transactions paying less than the current relaying fee can only
		// possibly be included in the high priority/zero fee area of blocks,
		// which are usually of limited size, so we explicitly don't track
		// those.
		// This also naturally handles votes (SSGen transactions) which don't
		// carry a tx fee and are required for inclusion in blocks. Note that
		// the test is explicitly < instead of <= so that we *can* track
		// transactions that pay *exactly* the minimum fee.
		return
	}

	log.Debugf("Adding mempool tx %s using fee rate %.8f", txHash, rate/1e8)

	tx := memPoolTxDesc{
		addedHeight: stats.bestHeight,
		bucketIndex: stats.lowerBucket(rate),
		fees:        rate,
	}
	stats.memPoolTxs[*txHash] = tx
	stats.newMemPoolTx(tx.bucketIndex, rate)
}

// RemoveMemPoolTransaction removes a mempool transaction from statistics
// tracking.
//
// This is safe to be called from multiple goroutines.
func (stats *Estimator) RemoveMemPoolTransaction(txHash *chainhash.Hash) {
	stats.lock.Lock()
	defer stats.lock.Unlock()

	desc, exists := stats.memPoolTxs[*txHash]
	if !exists {
		return
	}

	log.Debugf("Removing tx %s from mempool", txHash)

	stats.removeFromMemPool(int32(stats.bestHeight-desc.addedHeight), desc.fees)
	delete(stats.memPoolTxs, *txHash)
}

// processMinedTransaction moves the transaction that exist in the currently
// tracked mempool into a mined state.
//
// This function is *not* safe to be called from multiple goroutines.
func (stats *Estimator) processMinedTransaction(blockHeight int64, txh *chainhash.Hash) {
	desc, exists := stats.memPoolTxs[*txh]
	if !exists {
		// We cannot use transactions that we didn't know about to estimate
		// because that opens up the possibility of miners introducing dummy,
		// high fee transactions which would tend to then increase the average
		// fee estimate.
		// Tracking only previously known transactions forces miners trying to
		// pull off this attack to broadcast their transactions and possibly
		// forfeit their coins by having the transaction mined by a competitor.
		log.Tracef("Processing previously unknown mined tx %s", txh)
		return
	}

	stats.removeFromMemPool(int32(blockHeight-desc.addedHeight), desc.fees)
	delete(stats.memPoolTxs, *txh)

	if blockHeight <= desc.addedHeight {
		// This shouldn't usually happen but we need to explicitly test for
		// because we can't account for non positive confirmation ranges in
		// mined transactions.
		log.Errorf("Mined transaction %s (%d) that was known from "+
			"mempool at a higher block height (%d)", txh, blockHeight,
			desc.addedHeight)
		return
	}

	mineDelay := int32(blockHeight - desc.addedHeight)
	log.Debugf("Processing mined tx %s (rate %.8f, delay %d)", txh,
		desc.fees/1e8, mineDelay)
	stats.newMinedTx(mineDelay, desc.fees)
}

// ProcessBlock processes all mined transactions in the provided block.
//
// This function is safe to be called from multiple goroutines.
func (stats *Estimator) ProcessBlock(block *dcrutil.Block) error {
	stats.lock.Lock()
	defer stats.lock.Unlock()

	if stats.bestHeight < 0 {
		return nil
	}

	blockHeight := block.Height()
	if blockHeight <= stats.bestHeight {
		// we don't explicitly track reorgs right now
		log.Warnf("Trying to process mined transactions at block %d when "+
			"previous best block was at height %d", blockHeight,
			stats.bestHeight)
		return nil
	}

	stats.updateMovingAverages(blockHeight)

	for _, tx := range block.Transactions() {
		stats.processMinedTransaction(blockHeight, tx.Hash())
	}

	for _, tx := range block.STransactions() {
		stats.processMinedTransaction(blockHeight, tx.Hash())
	}

	if stats.db != nil {
		return stats.updateDatabase()
	}

	return nil
}

// Close closes the database (if it is currently opened).
func (stats *Estimator) Close() {
	stats.lock.Lock()

	if stats.db != nil {
		log.Trace("Closing fee estimator database")
		stats.db.Close()
		stats.db = nil
	}

	stats.lock.Unlock()
}
