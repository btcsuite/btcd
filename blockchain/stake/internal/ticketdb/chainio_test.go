// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ticketdb

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/internal/dbnamespace"
	"github.com/decred/dcrd/blockchain/stake/internal/tickettreap"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// testDbRoot is the root directory used to create all test databases.
	testDbRoot = "testdbs"
)

// hexToBytes converts a hex string to bytes, without returning any errors.
func hexToBytes(s string) []byte {
	b, _ := hex.DecodeString(s)

	return b
}

// newHashFromStr converts a 64 character hex string to a chainhash.Hash.
func newHashFromStr(s string) *chainhash.Hash {
	h, _ := chainhash.NewHashFromStr(s)

	return h
}

// TestDatabaseInfoSerialization ensures serializing and deserializing the
// database version information works as expected.
func TestDatabaseInfoSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		info       DatabaseInfo
		serialized []byte
	}{
		{
			name: "not upgrade",
			info: DatabaseInfo{
				Version:        currentDatabaseVersion,
				Date:           time.Unix(int64(0x57acca95), 0),
				UpgradeStarted: false,
			},
			serialized: hexToBytes("0100000095caac57"),
		},
		{
			name: "upgrade",
			info: DatabaseInfo{
				Version:        currentDatabaseVersion,
				Date:           time.Unix(int64(0x57acca95), 0),
				UpgradeStarted: true,
			},
			serialized: hexToBytes("0100008095caac57"),
		},
	}

	for i, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeDatabaseInfo(&test.info)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeDatabaseInfo #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		info, err := deserializeDatabaseInfo(test.serialized)
		if err != nil {
			t.Errorf("deserializeDatabaseInfo #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(info, &test.info) {
			t.Errorf("deserializeDatabaseInfo #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, info, test.info)
			continue
		}
	}
}

// TestDbInfoDeserializeErrors performs negative tests against
// deserializing the database information to ensure error paths
// work as expected.
func TestDbInfoDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errCode    ErrorCode
	}{
		{
			name:       "short read",
			serialized: hexToBytes("0000"),
			errCode:    ErrDatabaseInfoShortRead,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		_, err := deserializeDatabaseInfo(test.serialized)
		ticketDBErr, ok := err.(DBError)
		if !ok {
			t.Errorf("couldn't convert deserializeDatabaseInfo error "+
				"to ticket db error (err: %v)", err)
			continue
		}
		if ticketDBErr.GetCode() != test.errCode {
			t.Errorf("deserializeDatabaseInfo (%s): expected error type "+
				"does not match - got %v, want %v", test.name,
				ticketDBErr.ErrorCode, test.errCode)
			continue
		}
	}
}

// TestBestChainStateSerialization ensures serializing and deserializing the
// best chain state works as expected.
func TestBestChainStateSerialization(t *testing.T) {
	t.Parallel()

	hash1 := chainhash.HashH([]byte{0x00})
	hash2 := chainhash.HashH([]byte{0x01})
	hash3 := chainhash.HashH([]byte{0x02})
	hash4 := chainhash.HashH([]byte{0x03})
	hash5 := chainhash.HashH([]byte{0x04})

	tests := []struct {
		name       string
		state      BestChainState
		serialized []byte
	}{
		{
			name: "generic block",
			state: BestChainState{
				Hash:        *newHashFromStr("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"),
				Height:      12323,
				Live:        29399,
				Missed:      293929392,
				Revoked:     349839493,
				PerBlock:    5,
				NextWinners: []chainhash.Hash{hash1, hash2, hash3, hash4, hash5},
			},
			serialized: hexToBytes("6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d619000000000023300000d7720000b0018511000000008520da140000000005000ce8d4ef4dd7cd8d62dfded9d4edb0a774ae6a41929a74da23109e8f11139c874a6c419a1e25c85327115c4ace586decddfe2990ed8f3d4d801871158338501d49af37ab5270015fe25276ea5a3bb159d852943df23919522a202205fb7d175cb706d561742ad3671703c247eb927ee8a386369c79644131cdeb2c5c26bf6c5d4c6eb9e38415034f4c93d3304d10bef38bf0ad420eefd0f72f940f11c5857786"),
		},
	}

	for i, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeBestChainState(test.state)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeBestChainState #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		state, err := deserializeBestChainState(test.serialized)
		if err != nil {
			t.Errorf("deserializeBestChainState #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(state, test.state) {
			t.Errorf("deserializeBestChainState #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, state, test.state)
			continue

		}
	}
}

// TestBestChainStateDeserializeErrors performs negative tests against
// deserializing the chain state to ensure error paths work as expected.
func TestBestChainStateDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errCode    ErrorCode
	}{
		{
			name:       "short read",
			serialized: hexToBytes("0000"),
			errCode:    ErrChainStateShortRead,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		_, err := deserializeBestChainState(test.serialized)
		ticketDBErr, ok := err.(DBError)
		if !ok {
			t.Errorf("couldn't convert deserializeBestChainState error "+
				"to ticket db error (err: %v)", err)
			continue
		}
		if ticketDBErr.GetCode() != test.errCode {
			t.Errorf("deserializeBestChainState (%s): expected error type "+
				"does not match - got %v, want %v", test.name,
				ticketDBErr.ErrorCode, test.errCode)
			continue
		}
	}
}

// TestBlockUndoDataSerializing ensures serializing and deserializing the
// block undo data works as expected.
func TestBlockUndoDataSerializing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		utds       []UndoTicketData
		serialized []byte
	}{
		{
			name: "two ticket datas",
			utds: []UndoTicketData{{
				TicketHash:   chainhash.HashH([]byte{0x00}),
				TicketHeight: 123456,
				Missed:       true,
				Revoked:      false,
				Spent:        false,
				Expired:      true,
			}, {
				TicketHash:   chainhash.HashH([]byte{0x01}),
				TicketHeight: 122222,
				Missed:       false,
				Revoked:      true,
				Spent:        true,
				Expired:      false,
			}},
			serialized: hexToBytes("0ce8d4ef4dd7cd8d62dfded9d4edb0a774ae6a41929a74da23109e8f11139c8740e20100094a6c419a1e25c85327115c4ace586decddfe2990ed8f3d4d801871158338501d6edd010006"),
		},
	}

	for i, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeBlockUndoData(test.utds)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeBlockUndoData #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		utds, err := deserializeBlockUndoData(test.serialized)
		if err != nil {
			t.Errorf("deserializeBlockUndoData #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(utds, test.utds) {
			t.Errorf("deserializeBlockUndoData #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, utds, test.utds)
			continue

		}
	}
}

// TestBlockUndoDataDeserializing performs negative tests against decoding block
// undo data to ensure error paths work as expected.
func TestBlockUndoDataDeserializingErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errCode    ErrorCode
	}{
		{
			name:       "short read",
			serialized: hexToBytes("00"),
			errCode:    ErrUndoDataShortRead,
		},
		{
			name:       "bad size",
			serialized: hexToBytes("00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
			errCode:    ErrUndoDataCorrupt,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		_, err := deserializeBlockUndoData(test.serialized)
		ticketDBErr, ok := err.(DBError)
		if !ok {
			t.Errorf("couldn't convert deserializeBlockUndoData error "+
				"to ticket db error (err: %v)", err)
			continue
		}
		if ticketDBErr.GetCode() != test.errCode {
			t.Errorf("deserializeBlockUndoData (%s): expected error type "+
				"does not match - got %v, want %v", test.name,
				ticketDBErr.ErrorCode, test.errCode)
			continue
		}
	}
}

// TestTicketHashesSerializing ensures serializing and deserializing the
// ticket hashes works as expected.
func TestTicketHashesSerializing(t *testing.T) {
	t.Parallel()
	hash1 := chainhash.HashH([]byte{0x00})
	hash2 := chainhash.HashH([]byte{0x01})

	tests := []struct {
		name       string
		ths        TicketHashes
		serialized []byte
	}{
		{
			name: "two ticket hashes",
			ths: TicketHashes{
				hash1,
				hash2,
			},
			serialized: hexToBytes("0ce8d4ef4dd7cd8d62dfded9d4edb0a774ae6a41929a74da23109e8f11139c874a6c419a1e25c85327115c4ace586decddfe2990ed8f3d4d801871158338501d"),
		},
	}

	for i, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeTicketHashes(test.ths)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeBlockUndoData #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		ths, err := deserializeTicketHashes(test.serialized)
		if err != nil {
			t.Errorf("deserializeBlockUndoData #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(ths, test.ths) {
			t.Errorf("deserializeBlockUndoData #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, ths, test.ths)
			continue

		}
	}
}

// TestTicketHashesDeserializingErrors performs negative tests against decoding block
// undo data to ensure error paths work as expected.
func TestTicketHashesDeserializingErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errCode    ErrorCode
	}{
		{
			name:       "short read",
			serialized: hexToBytes("00"),
			errCode:    ErrTicketHashesShortRead,
		},
		{
			name:       "bad size",
			serialized: hexToBytes("00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
			errCode:    ErrTicketHashesCorrupt,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		_, err := deserializeTicketHashes(test.serialized)
		ticketDBErr, ok := err.(DBError)
		if !ok {
			t.Errorf("couldn't convert deserializeTicketHashes error "+
				"to ticket db error (err: %v)", err)
			continue
		}
		if ticketDBErr.GetCode() != test.errCode {
			t.Errorf("deserializeTicketHashes (%s): expected error type "+
				"does not match - got %v, want %v", test.name,
				ticketDBErr.ErrorCode, test.errCode)
			continue
		}
	}
}

// TestLiveDatabase tests various functions that require a live database.
func TestLiveDatabase(t *testing.T) {
	// Create a new database to store the accepted stake node data into.
	dbName := "ffldb_ticketdb_test"
	dbPath := filepath.Join(testDbRoot, dbName)
	_ = os.RemoveAll(dbPath)
	testDb, err := database.Create(testDbType, dbPath, chaincfg.SimNetParams.Net)
	if err != nil {
		t.Fatalf("error creating db: %v", err)
	}

	// Setup a teardown.
	defer os.RemoveAll(dbPath)
	defer os.RemoveAll(testDbRoot)
	defer testDb.Close()

	// Initialize the database, then try to read the version.
	err = testDb.Update(func(dbTx database.Tx) error {
		return DbCreate(dbTx)
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	var dbi *DatabaseInfo
	err = testDb.View(func(dbTx database.Tx) error {
		dbi, err = DbFetchDatabaseInfo(dbTx)
		return err
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	if dbi.Version != currentDatabaseVersion {
		t.Fatalf("bad version after reading from DB; want %v, got %v",
			currentDatabaseVersion, dbi.Version)
	}

	// Test storing arbitrary ticket treaps.
	ticketMap := make(map[tickettreap.Key]*tickettreap.Value)
	tickets := make([]chainhash.Hash, 5)
	for i := 0; i < 4; i++ {
		h := chainhash.HashH(bytes.Repeat([]byte{0x01}, i))
		ticketMap[tickettreap.Key(h)] = &tickettreap.Value{
			Height:  12345 + uint32(i),
			Missed:  i%2 == 0,
			Revoked: i%2 != 0,
			Spent:   i%2 == 0,
			Expired: i%2 != 0,
		}
		tickets[i] = h
	}

	err = testDb.Update(func(dbTx database.Tx) error {
		for k, v := range ticketMap {
			h := chainhash.Hash(k)
			err = DbPutTicket(dbTx, dbnamespace.LiveTicketsBucketName, &h,
				v.Height, v.Missed, v.Revoked, v.Spent, v.Expired)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	var treap *tickettreap.Immutable
	ticketMap2 := make(map[tickettreap.Key]*tickettreap.Value)
	err = testDb.View(func(dbTx database.Tx) error {
		treap, err = DbLoadAllTickets(dbTx, dbnamespace.LiveTicketsBucketName)
		return err
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	treap.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		ticketMap2[k] = v

		return true
	})

	if !reflect.DeepEqual(ticketMap, ticketMap2) {
		t.Fatalf("not same ticket maps")
	}
}
