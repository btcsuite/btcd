// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloomfilter

import (
	"encoding/binary"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"math"
	"sync"
)

// BloomFilter defines a bitcoin bloomfilter the provides easy manipulation of raw
// filter data.
type BloomFilter struct {
	sync.Mutex
	msgFilterLoad *btcwire.MsgFilterLoad
}

// New creates a new Filter instance, mainly to be used by SPV clients.  The tweak parameter is
// a random value added to the seed value.  For more information on what values to use for both
// elements and fprate, please see https://en.wikipedia.org/wiki/Bloom_filter.
func New(elements, tweak uint32, fprate float64, flags btcwire.BloomUpdateType) *BloomFilter {
	dataLen := uint32(math.Abs(math.Log(fprate)) * float64(elements) / (math.Ln2 * math.Ln2))
	dataLen = min(dataLen, btcwire.MaxFilterLoadFilterSize*8) / 8

	hashFuncs := min(uint32(float64(dataLen)*8.0/float64(elements)*math.Ln2), btcwire.MaxFilterLoadHashFuncs)

	data := make([]byte, dataLen)
	msg := btcwire.NewMsgFilterLoad(data, hashFuncs, tweak, flags)

	return &BloomFilter{
		msgFilterLoad: msg,
	}
}

// Load creates a new BloomFilter instance with the given btcwire.MsgFilterLoad.
func Load(filter *btcwire.MsgFilterLoad) *BloomFilter {
	return &BloomFilter{
		msgFilterLoad: filter,
	}
}

// Loaded returns true if a filter is loaded, otherwise false.
func (bf *BloomFilter) IsLoaded() bool {
	bf.Lock()
	defer bf.Unlock()

	return bf.msgFilterLoad != nil
}

// Unload clears the Filter.
func (bf *BloomFilter) Unload() {
	bf.Lock()
	defer bf.Unlock()

	bf.msgFilterLoad = nil
}

func (bf *BloomFilter) contains(data []byte) bool {
	if bf.msgFilterLoad == nil {
		return false
	}

	for i := uint32(0); i < bf.msgFilterLoad.HashFuncs; i++ {
		idx := bf.hash(i, data)
		if bf.msgFilterLoad.Filter[idx>>3]&(1<<(7&idx)) != 1<<(7&idx) {
			return false
		}
	}
	return true
}

// Contains returns true if the BloomFilter contains the passed byte slice. Otherwise,
// it returns false.
func (bf *BloomFilter) Contains(data []byte) bool {
	bf.Lock()
	defer bf.Unlock()

	return bf.contains(data)
}

func (bf *BloomFilter) containsOutPoint(outpoint *btcwire.OutPoint) bool {
	// Serialize
	var buf [btcwire.HashSize + 4]byte
	copy(buf[:], outpoint.Hash.Bytes())
	binary.LittleEndian.PutUint32(buf[btcwire.HashSize:], outpoint.Index)

	return bf.contains(buf[:])
}

// ContainsOutPoint returns true if the BloomFilter contains the given
// btcwire.OutPoint.  Otherwise, it returns false.
func (bf *BloomFilter) ContainsOutPoint(outpoint *btcwire.OutPoint) bool {
	bf.Lock()
	defer bf.Unlock()

	return bf.containsOutPoint(outpoint)
}

func (bf *BloomFilter) add(data []byte) {
	if bf.msgFilterLoad == nil {
		return
	}

	for i := uint32(0); i < bf.msgFilterLoad.HashFuncs; i++ {
		idx := bf.hash(i, data)
		bf.msgFilterLoad.Filter[idx>>3] |= (1 << (7 & idx))
	}
}

// Add adds the passed byte slice to the BloomFilter.
func (bf *BloomFilter) Add(data []byte) {
	bf.Lock()
	defer bf.Unlock()

	bf.add(data)
}

// AddShaHash adds the passed btcwire.ShaHash to the BloomFilter.
func (bf *BloomFilter) AddShaHash(sha *btcwire.ShaHash) {
	bf.Lock()
	defer bf.Unlock()

	bf.add(sha.Bytes())
}

// AddOutPoint adds the passed btcwire.OutPoint to the BloomFilter.
func (bf *BloomFilter) AddOutPoint(outpoint *btcwire.OutPoint) {
	bf.Lock()
	defer bf.Unlock()

	bf.addOutPoint(outpoint)
}

func (bf *BloomFilter) addOutPoint(outpoint *btcwire.OutPoint) {
	// Serialize
	var buf [btcwire.HashSize + 4]byte
	copy(buf[:], outpoint.Hash.Bytes())
	binary.LittleEndian.PutUint32(buf[btcwire.HashSize:], outpoint.Index)

	bf.add(buf[:])
}

func (bf *BloomFilter) matches(tx *btcutil.Tx) bool {
	hash := tx.Sha().Bytes()
	matched := bf.contains(hash)

	for i, txout := range tx.MsgTx().TxOut {
		pushedData, err := btcscript.PushedData(txout.PkScript)
		if err != nil {
			break
		}
		for _, p := range pushedData {
			if bf.contains(p) {
				switch bf.msgFilterLoad.Flags {
				case btcwire.BloomUpdateAll:
					outpoint := btcwire.NewOutPoint(tx.Sha(), uint32(i))
					bf.addOutPoint(outpoint)
				case btcwire.BloomUpdateP2PubkeyOnly:
					class := btcscript.GetScriptClass(txout.PkScript)
					if class == btcscript.PubKeyTy || class == btcscript.MultiSigTy {
						outpoint := btcwire.NewOutPoint(tx.Sha(), uint32(i))
						bf.addOutPoint(outpoint)
					}
				}
				return true
			}
		}
	}

	if matched {
		return true
	}

	for _, txin := range tx.MsgTx().TxIn {
		if bf.containsOutPoint(&txin.PreviousOutpoint) {
			return true
		}
		pushedData, err := btcscript.PushedData(txin.SignatureScript)
		if err != nil {
			break
		}
		for _, p := range pushedData {
			if bf.contains(p) {
				return true
			}
		}
	}
	return false
}

// MatchesTx returns true if the BloomFilter matches data within the passed transaction,
// otherwise false is returned.  If the BloomFilter does match the passed transaction,
// it will also update the BloomFilter if required.
func (bf *BloomFilter) MatchesTx(tx *btcutil.Tx) bool {
	bf.Lock()
	defer bf.Unlock()

	return bf.matches(tx)
}

// MsgFilterLoad returns the underlying btcwire.MsgFilterLoad for the BloomFilter.
func (bf *BloomFilter) MsgFilterLoad() *btcwire.MsgFilterLoad {
	return bf.msgFilterLoad
}

func (bf *BloomFilter) hash(hashNum uint32, data []byte) uint32 {
	// bitcoind: 0xFBA4C795 chosen as it guarantees a reasonable bit
	// difference between nHashNum values.
	mm := MurmurHash3(hashNum*0xFBA4C795+bf.msgFilterLoad.Tweak, data)
	return mm % (uint32(len(bf.msgFilterLoad.Filter)) * 8)
}

// min is a convenience function to return the minimum value of the two
// passed uint32 values.
func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
