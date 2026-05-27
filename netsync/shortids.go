// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/aead/siphash"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

var (
	// ErrNilTx signals that the provided transaction is nil.
	ErrNilTx = errors.New("nil transaction")

	// ErrShortIDCountTooLow signals that the compact block short ID count is
	// lower than expected.
	ErrShortIDCountTooLow = errors.New("compact block short id count too low")

	// ErrShortIDCountTooHigh signals that the compact block short ID count is
	// higher than expected.
	ErrShortIDCountTooHigh = errors.New("compact block short id count too high")
)

// CompactBlockShortID returns the BIP 152 short ID for the given transaction.
func CompactBlockShortID(header *wire.BlockHeader, nonce uint64,
	tx *btcutil.Tx, witness bool) (wire.ShortID, error) {

	if tx == nil {
		return wire.ShortID{}, ErrNilTx
	}

	var txHash *chainhash.Hash
	if witness {
		txHash = tx.WitnessHash()
	} else {
		txHash = tx.Hash()
	}

	return CompactBlockShortIDFromHash(header, nonce, txHash)
}

// CompactBlockShortIDFromHash returns the BIP 152 short ID for the given
// transaction hash.
func CompactBlockShortIDFromHash(header *wire.BlockHeader, nonce uint64,
	txHash *chainhash.Hash) (wire.ShortID, error) {

	key, err := compactBlockSipHashKey(header, nonce)
	if err != nil {
		return wire.ShortID{}, err
	}

	hash := siphash.Sum64(txHash[:], &key)

	var shortID wire.ShortID
	var hashBytes [8]byte
	binary.LittleEndian.PutUint64(hashBytes[:], hash)
	copy(shortID[:], hashBytes[:wire.ShortIDSize])
	return shortID, nil
}

func compactBlockSipHashKey(header *wire.BlockHeader, nonce uint64) ([siphash.KeySize]byte, error) {

	var headerAndNonce bytes.Buffer
	if err := header.Serialize(&headerAndNonce); err != nil {
		return [siphash.KeySize]byte{}, err
	}

	var nonceBytes [8]byte
	binary.LittleEndian.PutUint64(nonceBytes[:], nonce)
	headerAndNonce.Write(nonceBytes[:])

	keyHash := chainhash.HashH(headerAndNonce.Bytes())

	var key [siphash.KeySize]byte
	copy(key[:], keyHash[:siphash.KeySize])
	return key, nil
}

func compactBlockShortIDIndexes(msg *wire.MsgCmpctBlock, totalTxns int,
	prefilled map[uint32]struct{}) (map[wire.ShortID]uint32, error) {

	indexes := make(map[wire.ShortID]uint32, len(msg.ShortIDs))
	shortIDOffset := 0
	for blockIndex := 0; blockIndex < totalTxns; blockIndex++ {
		if _, ok := prefilled[uint32(blockIndex)]; ok {
			continue
		}
		if shortIDOffset >= len(msg.ShortIDs) {
			return nil, ErrShortIDCountTooLow
		}

		shortID := msg.ShortIDs[shortIDOffset]
		if _, ok := indexes[shortID]; ok {
			return nil, fmt.Errorf("duplicate short id %x", shortID[:])
		}

		indexes[shortID] = uint32(blockIndex)
		shortIDOffset++
	}
	if shortIDOffset != len(msg.ShortIDs) {
		return nil, ErrShortIDCountTooHigh
	}

	return indexes, nil
}

func buildShortIDTxIndex(header *wire.BlockHeader, nonce uint64,
	txns []*btcutil.Tx, witness bool) (map[wire.ShortID]*btcutil.Tx, error) {

	index := make(map[wire.ShortID]*btcutil.Tx, len(txns))
	collisions := make(map[wire.ShortID]struct{})
	for _, tx := range txns {
		if tx == nil {
			continue
		}

		shortID, err := CompactBlockShortID(header, nonce, tx, witness)
		if err != nil {
			return nil, err
		}

		if _, collided := collisions[shortID]; collided {
			continue
		}
		if _, exists := index[shortID]; exists {
			delete(index, shortID)
			collisions[shortID] = struct{}{}
			continue
		}

		index[shortID] = tx
	}

	return index, nil
}
