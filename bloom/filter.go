// Copyright (c) 2014, 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloom

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// ln2Squared is simply the square of the natural log of 2.
const ln2Squared = math.Ln2 * math.Ln2

// minUint32 is a convenience function to return the minimum value of the two
// passed uint32 values.
func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// Filter defines a bitcoin bloom filter that provides easy manipulation of raw
// filter data.
type Filter struct {
	mtx           sync.Mutex
	msgFilterLoad *wire.MsgFilterLoad
}

// NewFilter creates a new bloom filter instance, mainly to be used by SPV
// clients.  The tweak parameter is a random value added to the seed value.
// The false positive rate is the probability of a false positive where 1.0 is
// "match everything" and zero is unachievable.  Thus, providing any false
// positive rates less than 0 or greater than 1 will be adjusted to the valid
// range.
//
// For more information on what values to use for both elements and fprate,
// see https://en.wikipedia.org/wiki/Bloom_filter.
func NewFilter(elements, tweak uint32, fprate float64, flags wire.BloomUpdateType) *Filter {
	// Massage the false positive rate to sane values.
	if fprate > 1.0 {
		fprate = 1.0
	}
	if fprate < 1e-9 {
		fprate = 1e-9
	}

	// Calculate the size of the filter in bytes for the given number of
	// elements and false positive rate.
	//
	// Equivalent to m = -(n*ln(p) / ln(2)^2), where m is in bits.
	// Then clamp it to the maximum filter size and convert to bytes.
	dataLen := uint32(-1 * float64(elements) * math.Log(fprate) / ln2Squared)
	dataLen = minUint32(dataLen, wire.MaxFilterLoadFilterSize*8) / 8

	// Calculate the number of hash functions based on the size of the
	// filter calculated above and the number of elements.
	//
	// Equivalent to k = (m/n) * ln(2)
	// Then clamp it to the maximum allowed hash funcs.
	hashFuncs := uint32(float64(dataLen*8) / float64(elements) * math.Ln2)
	hashFuncs = minUint32(hashFuncs, wire.MaxFilterLoadHashFuncs)

	data := make([]byte, dataLen)
	msg := wire.NewMsgFilterLoad(data, hashFuncs, tweak, flags)

	return &Filter{
		msgFilterLoad: msg,
	}
}

// LoadFilter creates a new Filter instance with the given underlying
// wire.MsgFilterLoad.
func LoadFilter(filter *wire.MsgFilterLoad) *Filter {
	return &Filter{
		msgFilterLoad: filter,
	}
}

// IsLoaded returns true if a filter is loaded, otherwise false.
//
// This function is safe for concurrent access.
func (bf *Filter) IsLoaded() bool {
	bf.mtx.Lock()
	loaded := bf.msgFilterLoad != nil
	bf.mtx.Unlock()
	return loaded
}

// Reload loads a new filter replacing any existing filter.
//
// This function is safe for concurrent access.
func (bf *Filter) Reload(filter *wire.MsgFilterLoad) {
	bf.mtx.Lock()
	bf.msgFilterLoad = filter
	bf.mtx.Unlock()
}

// Unload unloads the bloom filter.
//
// This function is safe for concurrent access.
func (bf *Filter) Unload() {
	bf.mtx.Lock()
	bf.msgFilterLoad = nil
	bf.mtx.Unlock()
}

// hash returns the bit offset in the bloom filter which corresponds to the
// passed data for the given indepedent hash function number.
func (bf *Filter) hash(hashNum uint32, data []byte) uint32 {
	// bitcoind: 0xfba4c795 chosen as it guarantees a reasonable bit
	// difference between hashNum values.
	//
	// Note that << 3 is equivalent to multiplying by 8, but is faster.
	// Thus the returned hash is brought into range of the number of bits
	// the filter has and returned.
	mm := MurmurHash3(hashNum*0xfba4c795+bf.msgFilterLoad.Tweak, data)
	return mm % (uint32(len(bf.msgFilterLoad.Filter)) << 3)
}

// matches returns true if the bloom filter might contain the passed data and
// false if it definitely does not.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) matches(data []byte) bool {
	if bf.msgFilterLoad == nil {
		return false
	}

	// The bloom filter does not contain the data if any of the bit offsets
	// which result from hashing the data using each independent hash
	// function are not set.  The shifts and masks below are a faster
	// equivalent of:
	//   arrayIndex := idx / 8     (idx >> 3)
	//   bitOffset := idx % 8      (idx & 7)
	///  if filter[arrayIndex] & 1<<bitOffset == 0 { ... }
	for i := uint32(0); i < bf.msgFilterLoad.HashFuncs; i++ {
		idx := bf.hash(i, data)
		if bf.msgFilterLoad.Filter[idx>>3]&(1<<(idx&7)) == 0 {
			return false
		}
	}
	return true
}

// Matches returns true if the bloom filter might contain the passed data and
// false if it definitely does not.
//
// This function is safe for concurrent access.
func (bf *Filter) Matches(data []byte) bool {
	bf.mtx.Lock()
	match := bf.matches(data)
	bf.mtx.Unlock()
	return match
}

// matchesOutPoint returns true if the bloom filter might contain the passed
// outpoint and false if it definitely does not.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) matchesOutPoint(outpoint *wire.OutPoint) bool {
	// Serialize
	var buf [wire.HashSize + 4]byte
	copy(buf[:], outpoint.Hash.Bytes())
	binary.LittleEndian.PutUint32(buf[wire.HashSize:], outpoint.Index)

	return bf.matches(buf[:])
}

// MatchesOutPoint returns true if the bloom filter might contain the passed
// outpoint and false if it definitely does not.
//
// This function is safe for concurrent access.
func (bf *Filter) MatchesOutPoint(outpoint *wire.OutPoint) bool {
	bf.mtx.Lock()
	match := bf.matchesOutPoint(outpoint)
	bf.mtx.Unlock()
	return match
}

// add adds the passed byte slice to the bloom filter.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) add(data []byte) {
	if bf.msgFilterLoad == nil {
		return
	}

	// Adding data to a bloom filter consists of setting all of the bit
	// offsets which result from hashing the data using each independent
	// hash function.  The shifts and masks below are a faster equivalent
	// of:
	//   arrayIndex := idx / 8    (idx >> 3)
	//   bitOffset := idx % 8     (idx & 7)
	///  filter[arrayIndex] |= 1<<bitOffset
	for i := uint32(0); i < bf.msgFilterLoad.HashFuncs; i++ {
		idx := bf.hash(i, data)
		bf.msgFilterLoad.Filter[idx>>3] |= (1 << (7 & idx))
	}
}

// Add adds the passed byte slice to the bloom filter.
//
// This function is safe for concurrent access.
func (bf *Filter) Add(data []byte) {
	bf.mtx.Lock()
	bf.add(data)
	bf.mtx.Unlock()
}

// AddShaHash adds the passed wire.ShaHash to the Filter.
//
// This function is safe for concurrent access.
func (bf *Filter) AddShaHash(sha *wire.ShaHash) {
	bf.mtx.Lock()
	bf.add(sha.Bytes())
	bf.mtx.Unlock()
}

// addOutPoint adds the passed transaction outpoint to the bloom filter.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) addOutPoint(outpoint *wire.OutPoint) {
	// Serialize
	var buf [wire.HashSize + 4]byte
	copy(buf[:], outpoint.Hash.Bytes())
	binary.LittleEndian.PutUint32(buf[wire.HashSize:], outpoint.Index)

	bf.add(buf[:])
}

// AddOutPoint adds the passed transaction outpoint to the bloom filter.
//
// This function is safe for concurrent access.
func (bf *Filter) AddOutPoint(outpoint *wire.OutPoint) {
	bf.mtx.Lock()
	bf.addOutPoint(outpoint)
	bf.mtx.Unlock()
}

// maybeAddOutpoint potentially adds the passed outpoint to the bloom filter
// depending on the bloom update flags and the type of the passed public key
// script.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) maybeAddOutpoint(pkScript []byte, outHash *wire.ShaHash, outIdx uint32) {
	switch bf.msgFilterLoad.Flags {
	case wire.BloomUpdateAll:
		outpoint := wire.NewOutPoint(outHash, outIdx)
		bf.addOutPoint(outpoint)
	case wire.BloomUpdateP2PubkeyOnly:
		class := txscript.GetScriptClass(pkScript)
		if class == txscript.PubKeyTy || class == txscript.MultiSigTy {
			outpoint := wire.NewOutPoint(outHash, outIdx)
			bf.addOutPoint(outpoint)
		}
	}
}

// matchTxAndUpdate returns true if the bloom filter matches data within the
// passed transaction, otherwise false is returned.  If the filter does match
// the passed transaction, it will also update the filter depending on the bloom
// update flags set via the loaded filter if needed.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) matchTxAndUpdate(tx *btcutil.Tx) bool {
	// Check if the filter matches the hash of the transaction.
	// This is useful for finding transactions when they appear in a block.
	matched := bf.matches(tx.Sha().Bytes())

	// Check if the filter matches any data elements in the public key
	// scripts of any of the outputs.  When it does, add the outpoint that
	// matched so transactions which spend from the matched transaction are
	// also included in the filter.  This removes the burden of updating the
	// filter for this scenario from the client.  It is also more efficient
	// on the network since it avoids the need for another filteradd message
	// from the client and avoids some potential races that could otherwise
	// occur.
	for i, txOut := range tx.MsgTx().TxOut {
		pushedData, err := txscript.PushedData(txOut.PkScript)
		if err != nil {
			continue
		}

		for _, data := range pushedData {
			if !bf.matches(data) {
				continue
			}

			matched = true
			bf.maybeAddOutpoint(txOut.PkScript, tx.Sha(), uint32(i))
			break
		}
	}

	// Nothing more to do if a match has already been made.
	if matched {
		return true
	}

	// At this point, the transaction and none of the data elements in the
	// public key scripts of its outputs matched.

	// Check if the filter matches any outpoints this transaction spends or
	// any any data elements in the signature scripts of any of the inputs.
	for _, txin := range tx.MsgTx().TxIn {
		if bf.matchesOutPoint(&txin.PreviousOutPoint) {
			return true
		}

		pushedData, err := txscript.PushedData(txin.SignatureScript)
		if err != nil {
			continue
		}
		for _, data := range pushedData {
			if bf.matches(data) {
				return true
			}
		}
	}

	return false
}

// MatchTxAndUpdate returns true if the bloom filter matches data within the
// passed transaction, otherwise false is returned.  If the filter does match
// the passed transaction, it will also update the filter depending on the bloom
// update flags set via the loaded filter if needed.
//
// This function is safe for concurrent access.
func (bf *Filter) MatchTxAndUpdate(tx *btcutil.Tx) bool {
	bf.mtx.Lock()
	match := bf.matchTxAndUpdate(tx)
	bf.mtx.Unlock()
	return match
}

// MsgFilterLoad returns the underlying wire.MsgFilterLoad for the bloom
// filter.
//
// This function is safe for concurrent access.
func (bf *Filter) MsgFilterLoad() *wire.MsgFilterLoad {
	bf.mtx.Lock()
	msg := bf.msgFilterLoad
	bf.mtx.Unlock()
	return msg
}
