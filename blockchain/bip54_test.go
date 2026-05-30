// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// The BIP-54 test vectors are copied verbatim from
// https://github.com/bitcoin/bips/tree/master/bip-0054/test_vectors.
// See testdata/bip54/README.md for the upstream description of each file.
const bip54TestDataDir = "testdata/bip54"

// ----------------------------------------------------------------------------
// JSON shapes
// ----------------------------------------------------------------------------

type bip54TxSizeCase struct {
	Tx      string `json:"tx"`
	Valid   bool   `json:"valid"`
	Comment string `json:"comment"`
}

type bip54SigopsCase struct {
	SpentOutputs []string `json:"spent_outputs"`
	Tx           string   `json:"tx"`
	Valid        bool     `json:"valid"`
	Comment      string   `json:"comment"`
}

type bip54CoinbaseCase struct {
	BlockChain []string `json:"block_chain"`
	Valid      bool     `json:"valid"`
	Comment    string   `json:"comment"`
}

// bip54TimestampsNode is one node of the prefix tree in timestamps.json.
// A node is a leaf when Valid is non-nil; otherwise it carries Extensions
// that share the same prefix. The full header chain for a leaf is the
// concatenation of every BlockHeaders array on the root-to-leaf path.
type bip54TimestampsNode struct {
	BlockHeaders []string              `json:"block_headers"`
	Extensions   []bip54TimestampsNode `json:"extensions"`
	Valid        *bool                 `json:"valid"`
	Comment      string                `json:"comment"`
}

type bip54TimestampsLeaf struct {
	Headers []*wire.BlockHeader
	Valid   bool
	Comment string
}

// ----------------------------------------------------------------------------
// Loaders / decoders
// ----------------------------------------------------------------------------

func loadBIP54JSON(t *testing.T, name string, dst interface{}) {
	t.Helper()
	path := filepath.Join(bip54TestDataDir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}

func decodeBIP54Tx(s string) (*btcutil.Tx, error) {
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("hex decode tx: %w", err)
	}
	msgTx := wire.NewMsgTx(wire.TxVersion)
	if err := msgTx.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("deserialize tx: %w", err)
	}
	return btcutil.NewTx(msgTx), nil
}

func decodeBIP54Header(s string) (*wire.BlockHeader, error) {
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("hex decode header: %w", err)
	}
	var hdr wire.BlockHeader
	if err := hdr.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("deserialize header: %w", err)
	}
	return &hdr, nil
}

func decodeBIP54Block(s string) (*btcutil.Block, error) {
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("hex decode block: %w", err)
	}
	block, err := btcutil.NewBlockFromBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("deserialize block: %w", err)
	}
	return block, nil
}

func decodeBIP54TxOut(s string) (*wire.TxOut, error) {
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("hex decode TxOut: %w", err)
	}
	r := bytes.NewReader(raw)
	var out wire.TxOut
	if err := wire.ReadTxOut(r, 0, wire.TxVersion, &out); err != nil {
		return nil, fmt.Errorf("read TxOut: %w", err)
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("TxOut hex has %d trailing bytes", r.Len())
	}
	return &out, nil
}

func decodeBIP54Headers(in []string) ([]*wire.BlockHeader, error) {
	out := make([]*wire.BlockHeader, len(in))
	for i, s := range in {
		hdr, err := decodeBIP54Header(s)
		if err != nil {
			return nil, fmt.Errorf("header[%d]: %w", i, err)
		}
		out[i] = hdr
	}
	return out, nil
}

// flattenBIP54Timestamps walks the timestamps.json tree depth-first and
// returns one entry per leaf with the full root-to-leaf header chain
// materialized.
func flattenBIP54Timestamps(t *testing.T, root bip54TimestampsNode) []bip54TimestampsLeaf {
	t.Helper()
	rootHdrs, err := decodeBIP54Headers(root.BlockHeaders)
	if err != nil {
		t.Fatalf("root headers: %v", err)
	}

	var leaves []bip54TimestampsLeaf
	var walk func(parent []*wire.BlockHeader, n bip54TimestampsNode)
	walk = func(parent []*wire.BlockHeader, n bip54TimestampsNode) {
		hdrs, err := decodeBIP54Headers(n.BlockHeaders)
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		chain := make([]*wire.BlockHeader, 0, len(parent)+len(hdrs))
		chain = append(chain, parent...)
		chain = append(chain, hdrs...)
		if n.Valid != nil {
			leaves = append(leaves, bip54TimestampsLeaf{
				Headers: chain,
				Valid:   *n.Valid,
				Comment: n.Comment,
			})
			return
		}
		for _, ext := range n.Extensions {
			walk(chain, ext)
		}
	}
	for _, ext := range root.Extensions {
		walk(rootHdrs, ext)
	}
	return leaves
}

// ----------------------------------------------------------------------------
// Test: txsize.json
// ----------------------------------------------------------------------------

// TestBIP54TxSize exercises BIP-54's restriction on non-witness transaction
// size: a transaction whose stripped (no-witness) serialization is exactly
// 64 bytes must be rejected, to prevent Merkle-tree malleability.
func TestBIP54TxSize(t *testing.T) {
	var cases []bip54TxSizeCase
	loadBIP54JSON(t, "txsize.json", &cases)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Comment, func(t *testing.T) {
			tx, err := decodeBIP54Tx(tc.Tx)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			err = CheckBIP54TxSize(tx)
			switch {
			case tc.Valid && err != nil:
				t.Fatalf("expected valid; got error: %v", err)
			case !tc.Valid && err == nil:
				t.Fatalf("expected invalid; got nil")
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Test: sigops.json
// ----------------------------------------------------------------------------

// TestBIP54Sigops exercises BIP-54's cap on potentially-executed legacy
// signature operations per transaction.
func TestBIP54Sigops(t *testing.T) {
	var cases []bip54SigopsCase
	loadBIP54JSON(t, "sigops.json", &cases)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Comment, func(t *testing.T) {
			tx, err := decodeBIP54Tx(tc.Tx)
			if err != nil {
				t.Fatalf("decode tx: %v", err)
			}
			prevOuts := make([]*wire.TxOut, len(tc.SpentOutputs))
			for i, s := range tc.SpentOutputs {
				po, err := decodeBIP54TxOut(s)
				if err != nil {
					t.Fatalf("decode spent_outputs[%d]: %v", i, err)
				}
				prevOuts[i] = po
			}
			if len(prevOuts) != len(tx.MsgTx().TxIn) {
				t.Fatalf("vector has %d spent_outputs but tx has %d inputs",
					len(prevOuts), len(tx.MsgTx().TxIn))
			}
			t.Skip("BIP-54 sigop counter not implemented yet")
		})
	}
}

// ----------------------------------------------------------------------------
// Unit test: CheckBIP54Coinbase
// ----------------------------------------------------------------------------

// TestCheckBIP54Coinbase exercises the CheckBIP54Coinbase helper in
// isolation, without going through the full block-processing pipeline.
// The rule under test is:
//
//   - the coinbase's nLockTime must equal blockHeight - 1, and
//   - the coinbase input's nSequence must not equal 0xffffffff.
func TestCheckBIP54Coinbase(t *testing.T) {
	const (
		finalSeq    = wire.MaxTxInSequenceNum
		nonFinalSeq = wire.MaxTxInSequenceNum - 1
	)

	makeCoinbase := func(lockTime, sequence uint32) *btcutil.Tx {
		msgTx := wire.NewMsgTx(wire.TxVersion)
		msgTx.LockTime = lockTime
		msgTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
			SignatureScript:  []byte{0x51},
			Sequence:         sequence,
		})
		msgTx.AddTxOut(&wire.TxOut{Value: 5000000000, PkScript: []byte{0x51}})
		return btcutil.NewTx(msgTx)
	}

	tests := []struct {
		name        string
		lockTime    uint32
		sequence    uint32
		blockHeight int32
		wantErr     ErrorCode // 0 means expect nil
	}{{
		name:        "valid: lockTime=height-1 with non-final sequence",
		lockTime:    3,
		sequence:    nonFinalSeq,
		blockHeight: 4,
	}, {
		name:        "valid: lockTime=0 at height 1",
		lockTime:    0,
		sequence:    nonFinalSeq,
		blockHeight: 1,
	}, {
		name:        "invalid: lockTime greater than height-1",
		lockTime:    5,
		sequence:    nonFinalSeq,
		blockHeight: 4,
		wantErr:     ErrBadCoinbaseLockTime,
	}, {
		name:        "invalid: lockTime equal to height",
		lockTime:    4,
		sequence:    nonFinalSeq,
		blockHeight: 4,
		wantErr:     ErrBadCoinbaseLockTime,
	}, {
		name:        "invalid: lockTime less than height-1",
		lockTime:    2,
		sequence:    nonFinalSeq,
		blockHeight: 4,
		wantErr:     ErrBadCoinbaseLockTime,
	}, {
		name:        "invalid: final nSequence",
		lockTime:    3,
		sequence:    finalSeq,
		blockHeight: 4,
		wantErr:     ErrBadCoinbaseLockTime,
	}, {
		// nLockTime values >= 500_000_000 are interpreted as unix
		// timestamps, not block heights; BIP-54 requires a height-
		// based lockTime equal to blockHeight-1, so this is rejected.
		name:        "invalid: time-based lockTime",
		lockTime:    1700000000,
		sequence:    nonFinalSeq,
		blockHeight: 4,
		wantErr:     ErrBadCoinbaseLockTime,
	}}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tx := makeCoinbase(tt.lockTime, tt.sequence)
			err := CheckBIP54Coinbase(tx, tt.blockHeight)

			if tt.wantErr == 0 {
				if err != nil {
					t.Fatalf("expected nil; got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected ErrorCode %v; got nil", tt.wantErr)
			}
			re, ok := err.(RuleError)
			if !ok {
				t.Fatalf("expected RuleError, got %T: %v", err, err)
			}
			if re.ErrorCode != tt.wantErr {
				t.Fatalf("expected ErrorCode %v; got %v (%v)",
					tt.wantErr, re.ErrorCode, re)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Test: coinbases.json
// ----------------------------------------------------------------------------

// bip54MainNetParams returns a value copy of chaincfg.MainNetParams with
// DeploymentBIP54 forced active from height 1. It exists so the BIP-54
// test vectors can drive the validation pipeline before BIP-54 has any
// scheduled activation on real mainnet.
func bip54MainNetParams() *chaincfg.Params {
	params := chaincfg.MainNetParams
	deploy := params.Deployments[chaincfg.DeploymentBIP54]
	deploy.AlwaysActiveHeight = 1
	params.Deployments[chaincfg.DeploymentBIP54] = deploy
	return &params
}

// TestBIP54Coinbases exercises BIP-54's restrictions on coinbase
// transactions, intended to make duplicate coinbases impossible without a
// BIP-30 lookup. Each test case is a fresh chain of mainnet blocks
// starting from the real genesis; the chain is valid iff every non-genesis
// block is accepted as the new tip via ProcessBlock.
func TestBIP54Coinbases(t *testing.T) {
	var cases []bip54CoinbaseCase
	loadBIP54JSON(t, "coinbases.json", &cases)

	for i, tc := range cases {
		i, tc := i, tc
		t.Run(tc.Comment, func(t *testing.T) {
			dbName := fmt.Sprintf("bip54_coinbases_%d", i)
			chain, teardown, err := chainSetup(dbName, bip54MainNetParams())
			if err != nil {
				t.Fatalf("chainSetup: %v", err)
			}
			defer teardown()

			// Genesis at index 0 is already present in the fresh chain.
			rejected := false
			var rejectErr error
			for h, hexBlock := range tc.BlockChain[1:] {
				block, err := decodeBIP54Block(hexBlock)
				if err != nil {
					t.Fatalf("decode block %d: %v", h+1, err)
				}
				isMain, isOrphan, perr := chain.ProcessBlock(block, BFNone)
				if perr != nil {
					rejected = true
					rejectErr = fmt.Errorf("block %d: %w", h+1, perr)
					break
				}
				if isOrphan {
					rejected = true
					rejectErr = fmt.Errorf("block %d accepted as orphan", h+1)
					break
				}
				if !isMain {
					rejected = true
					rejectErr = fmt.Errorf("block %d did not extend main chain", h+1)
					break
				}
			}

			switch {
			case tc.Valid && rejected:
				t.Fatalf("expected valid; %v", rejectErr)
			case !tc.Valid && !rejected:
				t.Fatalf("expected invalid; chain accepted")
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Test: timestamps.json
// ----------------------------------------------------------------------------

// TestBIP54Timestamps exercises BIP-54's two new constraints on block-header
// timestamps: the Timewarp fix (the first block of a new difficulty period
// must be ≥ the last block of the previous period minus 7200 s) and the
// Murch-Zawy fix (the last block of a difficulty period must be ≥ the
// first block of the same period). Each test case is a complete mainnet
// header chain starting from the real genesis.
func TestBIP54Timestamps(t *testing.T) {
	var root bip54TimestampsNode
	loadBIP54JSON(t, "timestamps.json", &root)
	leaves := flattenBIP54Timestamps(t, root)

	for _, leaf := range leaves {
		leaf := leaf
		t.Run(leaf.Comment, func(t *testing.T) {
			t.Skip("BIP-54 timestamp rules not implemented yet")
		})
	}
}

