// Copyright (c) 2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestIsSwiftSyncActive tests the isSwiftSyncActive function.
func TestIsSwiftSyncActive(t *testing.T) {
	testHash := chainhash.HashH([]byte("test"))
	swiftSyncData := &chaincfg.SwiftSyncData{
		Height: 100,
		Hash:   &testHash,
		Bitmap: []byte{0xff, 0xff},
	}

	tests := []struct {
		name            string
		swiftSyncData   *chaincfg.SwiftSyncData
		swiftSyncEnabled bool
		height          int32
		want            bool
	}{
		{
			name:            "disabled - no swift sync data",
			swiftSyncData:   nil,
			swiftSyncEnabled: false,
			height:          50,
			want:            false,
		},
		{
			name:            "disabled - swift sync not enabled",
			swiftSyncData:   swiftSyncData,
			swiftSyncEnabled: false,
			height:          50,
			want:            false,
		},
		{
			name:            "active - within range",
			swiftSyncData:   swiftSyncData,
			swiftSyncEnabled: true,
			height:          50,
			want:            true,
		},
		{
			name:            "active - at boundary",
			swiftSyncData:   swiftSyncData,
			swiftSyncEnabled: true,
			height:          100,
			want:            true,
		},
		{
			name:            "inactive - height 0",
			swiftSyncData:   swiftSyncData,
			swiftSyncEnabled: true,
			height:          0,
			want:            false,
		},
		{
			name:            "inactive - height above range",
			swiftSyncData:   swiftSyncData,
			swiftSyncEnabled: true,
			height:          101,
			want:            false,
		},
		{
			name:            "active - at height 1",
			swiftSyncData:   swiftSyncData,
			swiftSyncEnabled: true,
			height:          1,
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &BlockChain{
				swiftSync:        tt.swiftSyncData,
				swiftSyncEnabled: tt.swiftSyncEnabled,
			}

			got := b.isSwiftSyncActive(tt.height)
			if got != tt.want {
				t.Errorf("isSwiftSyncActive(%d) = %v, want %v",
					tt.height, got, tt.want)
			}
		})
	}
}

// TestGetSwiftSyncBitmapBit tests the getSwiftSyncBitmapBit function.
func TestGetSwiftSyncBitmapBit(t *testing.T) {
	// Test bitmap: 0b10101010, 0b11001100, 0b11110000
	// Bit indices (LSB first within each byte):
	// Byte 0: bits 0-7 = 0,1,0,1,0,1,0,1
	// Byte 1: bits 8-15 = 0,0,1,1,0,0,1,1
	// Byte 2: bits 16-23 = 0,0,0,0,1,1,1,1
	testBitmap := []byte{0xaa, 0xcc, 0xf0}
	testHash := chainhash.HashH([]byte("test"))
	swiftSyncData := &chaincfg.SwiftSyncData{
		Height: 100,
		Hash:   &testHash,
		Bitmap: testBitmap,
	}

	tests := []struct {
		name  string
		index uint64
		want  bool
	}{
		// Byte 0 (0xaa = 0b10101010)
		{"bit 0", 0, false},
		{"bit 1", 1, true},
		{"bit 2", 2, false},
		{"bit 3", 3, true},
		{"bit 4", 4, false},
		{"bit 5", 5, true},
		{"bit 6", 6, false},
		{"bit 7", 7, true},

		// Byte 1 (0xcc = 0b11001100)
		{"bit 8", 8, false},
		{"bit 9", 9, false},
		{"bit 10", 10, true},
		{"bit 11", 11, true},
		{"bit 12", 12, false},
		{"bit 13", 13, false},
		{"bit 14", 14, true},
		{"bit 15", 15, true},

		// Byte 2 (0xf0 = 0b11110000)
		{"bit 16", 16, false},
		{"bit 17", 17, false},
		{"bit 18", 18, false},
		{"bit 19", 19, false},
		{"bit 20", 20, true},
		{"bit 21", 21, true},
		{"bit 22", 22, true},
		{"bit 23", 23, true},

		// Out of bounds - should return false
		{"bit 24 (out of bounds)", 24, false},
		{"bit 100 (out of bounds)", 100, false},
	}

	b := &BlockChain{
		swiftSync: swiftSyncData,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.getSwiftSyncBitmapBit(tt.index)
			if got != tt.want {
				t.Errorf("getSwiftSyncBitmapBit(%d) = %v, want %v",
					tt.index, got, tt.want)
			}
		})
	}
}

// TestGetSwiftSyncBitmapBitEmptyBitmap tests edge case with empty bitmap.
func TestGetSwiftSyncBitmapBitEmptyBitmap(t *testing.T) {
	testHash := chainhash.HashH([]byte("test"))
	swiftSyncData := &chaincfg.SwiftSyncData{
		Height: 100,
		Hash:   &testHash,
		Bitmap: []byte{},
	}

	b := &BlockChain{
		swiftSync: swiftSyncData,
	}

	// Any bit index should return false for empty bitmap
	if b.getSwiftSyncBitmapBit(0) {
		t.Error("getSwiftSyncBitmapBit(0) = true for empty bitmap, want false")
	}
	if b.getSwiftSyncBitmapBit(100) {
		t.Error("getSwiftSyncBitmapBit(100) = true for empty bitmap, want false")
	}
}
