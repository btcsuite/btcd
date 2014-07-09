// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloom

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
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
	sync.Mutex
	msgFilterLoad *btcwire.MsgFilterLoad
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
func NewFilter(elements, tweak uint32, fprate float64, flags btcwire.BloomUpdateType) *Filter {
	// Massage the false positive rate to sane values.
	if fprate > 1.0 {
		fprate = 1.0
	}
	if fprate < 0 {
		fprate = 1e-9
	}

	// Calculate the size of the filter in bytes for the given number of
	// elements and false positive rate.
	//
	// Equivalent to m = -(n*ln(p) / ln(2)^2), where m is in bits.
	// Then clamp it to the maximum filter size and convert to bytes.
	dataLen := uint32(-1 * float64(elements) * math.Log(fprate) / ln2Squared)
	dataLen = minUint32(dataLen, btcwire.MaxFilterLoadFilterSize*8) / 8

	// Calculate the number of hash functions based on the size of the
	// filter calculated above and the number of elements.
	//
	// Equivalent to k = (m/n) * ln(2)
	// Then clamp it to the maximum allowed hash funcs.
	hashFuncs := uint32(float64(dataLen*8) / float64(elements) * math.Ln2)
	hashFuncs = minUint32(hashFuncs, btcwire.MaxFilterLoadHashFuncs)

	data := make([]byte, dataLen)
	msg := btcwire.NewMsgFilterLoad(data, hashFuncs, tweak, flags)

	return &Filter{
		msgFilterLoad: msg,
	}
}

// LoadFilter creates a new Filter instance with the given underlying
// btcwire.MsgFilterLoad.
func LoadFilter(filter *btcwire.MsgFilterLoad) *Filter {
	return &Filter{
		msgFilterLoad: filter,
	}
}

// IsLoaded returns true if a filter is loaded, otherwise false.
//
// This function is safe for concurrent access.
func (bf *Filter) IsLoaded() bool {
	bf.Lock()
	defer bf.Unlock()

	return bf.msgFilterLoad != nil
}

// Unload clears the bloom filter.
//
// This function is safe for concurrent access.
func (bf *Filter) Unload() {
	bf.Lock()
	defer bf.Unlock()

	bf.msgFilterLoad = nil
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
	bf.Lock()
	defer bf.Unlock()

	return bf.matches(data)
}

// matchesOutPoint returns true if the bloom filter might contain the passed
// outpoint and false if it definitely does not.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) matchesOutPoint(outpoint *btcwire.OutPoint) bool {
	// Serialize
	var buf [btcwire.HashSize + 4]byte
	copy(buf[:], outpoint.Hash.Bytes())
	binary.LittleEndian.PutUint32(buf[btcwire.HashSize:], outpoint.Index)

	return bf.matches(buf[:])
}

// MatchesOutPoint returns true if the bloom filter might contain the passed
// outpoint and false if it definitely does not.
//
// This function is safe for concurrent access.
func (bf *Filter) MatchesOutPoint(outpoint *btcwire.OutPoint) bool {
	bf.Lock()
	defer bf.Unlock()

	return bf.matchesOutPoint(outpoint)
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
	bf.Lock()
	defer bf.Unlock()

	bf.add(data)
}

// AddShaHash adds the passed btcwire.ShaHash to the Filter.
//
// This function is safe for concurrent access.
func (bf *Filter) AddShaHash(sha *btcwire.ShaHash) {
	bf.Lock()
	defer bf.Unlock()

	bf.add(sha.Bytes())
}

// addOutPoint adds the passed transaction outpoint to the bloom filter.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) addOutPoint(outpoint *btcwire.OutPoint) {
	// Serialize
	var buf [btcwire.HashSize + 4]byte
	copy(buf[:], outpoint.Hash.Bytes())
	binary.LittleEndian.PutUint32(buf[btcwire.HashSize:], outpoint.Index)

	bf.add(buf[:])
}

// AddOutPoint adds the passed transaction outpoint to the bloom filter.
//
// This function is safe for concurrent access.
func (bf *Filter) AddOutPoint(outpoint *btcwire.OutPoint) {
	bf.Lock()
	defer bf.Unlock()

	bf.addOutPoint(outpoint)
}

// maybeAddOutpoint potentially adds the passed outpoint to the bloom filter
// depending on the bloom update flags and the type of the passed public key
// script.
//
// This function MUST be called with the filter lock held.
func (bf *Filter) maybeAddOutpoint(pkScript []byte, outHash *btcwire.ShaHash, outIdx uint32) {
	switch bf.msgFilterLoad.Flags {
	case btcwire.BloomUpdateAll:
		outpoint := btcwire.NewOutPoint(outHash, outIdx)
		bf.addOutPoint(outpoint)
	case btcwire.BloomUpdateP2PubkeyOnly:
		class := btcscript.GetScriptClass(pkScript)
		if class == btcscript.PubKeyTy || class == btcscript.MultiSigTy {
			outpoint := btcwire.NewOutPoint(outHash, outIdx)
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
		pushedData, err := btcscript.PushedData(txOut.PkScript)
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
		if bf.matchesOutPoint(&txin.PreviousOutpoint) {
			return true
		}

		pushedData, err := btcscript.PushedData(txin.SignatureScript)
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
	bf.Lock()
	defer bf.Unlock()

	return bf.matchTxAndUpdate(tx)
}

// MsgFilterLoad returns the underlying btcwire.MsgFilterLoad for the bloom
// filter.
//
// This function is safe for concurrent access.
func (bf *Filter) MsgFilterLoad() *btcwire.MsgFilterLoad {
	bf.Lock()
	defer bf.Unlock()

	return bf.msgFilterLoad
}
