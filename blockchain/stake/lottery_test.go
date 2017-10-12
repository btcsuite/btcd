// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/decred/dcrd/blockchain/stake/internal/tickettreap"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

func TestBasicPRNG(t *testing.T) {
	seed := chainhash.HashB([]byte{0x01})
	prng := NewHash256PRNG(seed)
	for i := 0; i < 100000; i++ {
		prng.Hash256Rand()
	}

	lastHashExp, _ := chainhash.NewHashFromStr("24f1cd72aefbfc85a9d3e21e2eb" +
		"732615688d3634bf94499af5a81e0eb45c4e4")
	lastHash := prng.StateHash()
	if *lastHashExp != lastHash {
		t.Errorf("expected final state of %v, got %v", lastHashExp, lastHash)
	}
}

type TicketData struct {
	Prefix      uint8 // Bucket prefix
	SStxHash    chainhash.Hash
	SpendHash   chainhash.Hash
	BlockHeight int64  // Block for where the original sstx was located
	TxIndex     uint32 // Position within a block, in stake tree
	Missed      bool   // Whether or not the ticket was spent
	Expired     bool   // Whether or not the ticket expired
}

// SStxMemMap is a memory map of SStx keyed to the txHash.
type SStxMemMap map[chainhash.Hash]*TicketData

// TicketDataSlice is a sortable data structure of pointers to TicketData.
type TicketDataSlice []*TicketData

func NewTicketDataSlice(size int) TicketDataSlice {
	slice := make([]*TicketData, size)
	return TicketDataSlice(slice)
}

// Less determines which of two *TicketData values is smaller; used for sort.
func (tds TicketDataSlice) Less(i, j int) bool {
	cmp := bytes.Compare(tds[i].SStxHash[:], tds[j].SStxHash[:])
	isISmaller := (cmp == -1)
	return isISmaller
}

// Swap swaps two *TicketData values.
func (tds TicketDataSlice) Swap(i, j int) { tds[i], tds[j] = tds[j], tds[i] }

// Len returns the length of the slice.
func (tds TicketDataSlice) Len() int { return len(tds) }

func TestLotteryNumSelection(t *testing.T) {
	// Test finding ticket indexes.
	seed := chainhash.HashB([]byte{0x01})
	prng := NewHash256PRNG(seed)
	ticketsInPool := 56789
	tooFewTickets := 4
	justEnoughTickets := 5
	ticketsPerBlock := uint16(5)

	_, err := FindTicketIdxs(tooFewTickets, ticketsPerBlock, prng)
	if err == nil {
		t.Errorf("got unexpected no error for FindTicketIdxs too few tickets " +
			"test")
	}

	tickets, err := FindTicketIdxs(ticketsInPool, ticketsPerBlock, prng)
	if err != nil {
		t.Errorf("got unexpected error for FindTicketIdxs 1 test")
	}
	ticketsExp := []int{34850, 8346, 27636, 54482, 25482}
	if !reflect.DeepEqual(ticketsExp, tickets) {
		t.Errorf("Unexpected tickets selected; got %v, want %v", tickets,
			ticketsExp)
	}

	// Ensure that it can find all suitable ticket numbers in a small
	// bucket of tickets.
	tickets, err = FindTicketIdxs(justEnoughTickets, ticketsPerBlock, prng)
	if err != nil {
		t.Errorf("got unexpected error for FindTicketIdxs 2 test")
	}
	ticketsExp = []int{3, 0, 4, 2, 1}
	if !reflect.DeepEqual(ticketsExp, tickets) {
		t.Errorf("Unexpected tickets selected; got %v, want %v", tickets,
			ticketsExp)
	}

	lastHashExp, _ := chainhash.NewHashFromStr("e97ce54aea63a883a82871e752c" +
		"6ec3c5731fffc63dafc3767c06861b0b2fa65")
	lastHash := prng.StateHash()
	if *lastHashExp != lastHash {
		t.Errorf("expected final state of %v, got %v", lastHashExp, lastHash)
	}
}

func TestLotteryNumErrors(t *testing.T) {
	seed := chainhash.HashB([]byte{0x01})
	prng := NewHash256PRNG(seed)

	// Too big pool.
	_, err := FindTicketIdxs(1000000000000, 5, prng)
	if err == nil {
		t.Errorf("Expected pool size too big error")
	}
}

func TestFetchWinnersErrors(t *testing.T) {
	treap := new(tickettreap.Immutable)
	for i := 0; i < 0xff; i++ {
		h := chainhash.HashH([]byte{byte(i)})
		v := &tickettreap.Value{
			Height:  uint32(i),
			Missed:  i%2 == 0,
			Revoked: i%2 != 0,
			Spent:   i%2 == 0,
			Expired: i%2 != 0,
		}
		treap = treap.Put(tickettreap.Key(h), v)
	}

	// No indexes.
	_, err := fetchWinners(nil, treap)
	if err == nil {
		t.Errorf("Expected nil slice error")
	}

	// No treap.
	_, err = fetchWinners([]int{1, 2, 3, 4, -1}, nil)
	if err == nil {
		t.Errorf("Expected nil treap error")
	}

	// Bad index too small.
	_, err = fetchWinners([]int{1, 2, 3, 4, -1}, treap)
	if err == nil {
		t.Errorf("Expected index too small error")
	}

	// Bad index too big.
	_, err = fetchWinners([]int{1, 2, 3, 4, 256}, treap)
	if err == nil {
		t.Errorf("Expected index too big error")
	}
}

func TestTicketSorting(t *testing.T) {
	ticketsPerBlock := 5
	ticketPoolSize := uint16(8192)
	totalTickets := uint32(ticketPoolSize) * uint32(5)
	bucketsSize := 256

	randomGen := rand.New(rand.NewSource(12345))
	ticketMap := make([]SStxMemMap, int(bucketsSize))

	for i := 0; i < bucketsSize; i++ {
		ticketMap[i] = make(SStxMemMap)
	}

	toMake := int(ticketPoolSize) * ticketsPerBlock
	for i := 0; i < toMake; i++ {
		td := new(TicketData)

		rint64 := randomGen.Int63n(1 << 62)
		randBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(randBytes, uint64(rint64))
		h := chainhash.HashH(randBytes)
		td.SStxHash = h

		prefix := byte(h[0])

		ticketMap[prefix][h] = td
	}

	// Pre-sort with buckets (faster).
	sortedSlice := make([]*TicketData, 0, totalTickets)
	for i := 0; i < bucketsSize; i++ {
		tempTdSlice := NewTicketDataSlice(len(ticketMap[i]))
		itr := 0 // Iterator
		for _, td := range ticketMap[i] {
			tempTdSlice[itr] = td
			itr++
		}
		sort.Sort(tempTdSlice)
		sortedSlice = append(sortedSlice, tempTdSlice...)
	}
	sortedSlice1 := sortedSlice

	// However, it should be the same as a sort without the buckets.
	toSortSlice := make([]*TicketData, 0, totalTickets)
	for i := 0; i < bucketsSize; i++ {
		tempTdSlice := make([]*TicketData, len(ticketMap[i]))
		itr := 0 // Iterator
		for _, td := range ticketMap[i] {
			tempTdSlice[itr] = td
			itr++
		}
		toSortSlice = append(toSortSlice, tempTdSlice...)
	}
	sortedSlice = NewTicketDataSlice(int(totalTickets))
	copy(sortedSlice, toSortSlice)
	sort.Sort(TicketDataSlice(sortedSlice))
	sortedSlice2 := sortedSlice

	if !reflect.DeepEqual(sortedSlice1, sortedSlice2) {
		t.Errorf("bucket sort failed to sort to the same slice as global sort")
	}
}

func BenchmarkHashPRNG(b *testing.B) {
	seed := chainhash.HashB([]byte{0x01})
	prng := NewHash256PRNG(seed)

	for n := 0; n < b.N; n++ {
		prng.Hash256Rand()
	}
}
