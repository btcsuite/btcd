// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/blockchain/internal/testhelper"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// rewriteSpy wraps AddrIndex and records whether the age-out rewrite for a
// target block was given a full (hot) block or a stripped one. A future
// chain.go regression that re-fetches after CompactBlockToCold would pass
// stripped bytes here.
type rewriteSpy struct {
	*AddrIndex
	target               chainhash.Hash
	sawFullSegwitRewrite bool
	sawStrippedOnly      bool
}

func (s *rewriteSpy) RewriteTxOffsetsForColdCompaction(dbTx database.Tx,
	block *btcutil.Block, stxos []blockchain.SpentTxOut) error {

	if block.Hash().IsEqual(&s.target) {
		full, err := block.Bytes()
		if err != nil {
			return err
		}
		stripped, err := block.BytesNoWitness()
		if err != nil {
			return err
		}
		if len(full) > len(stripped) {
			s.sawFullSegwitRewrite = true
		} else {
			// Target was connected with witness; rewrite seeing equal lengths
			// means the block was already stripped (re-fetch-after-compact).
			s.sawStrippedOnly = true
		}
	}
	return s.AddrIndex.RewriteTxOffsetsForColdCompaction(dbTx, block, stxos)
}

// TestAgeOutProcessBlockPassesFullBlockToAddrindexRewrite drives the real
// blockchain.ProcessBlock → connectBlock age-out path with txindex+addrindex
// enabled and a segwit-bearing block. It asserts:
//  1. The rewrite sees the full hot block (not a stripped re-fetch).
//  2. After compaction, TxRegionsForAddress → FetchBlockRegions still
//     round-trips the expected txid (searchrawtransactions path).
func TestAgeOutProcessBlockPassesFullBlockToAddrindexRewrite(t *testing.T) {
	params := chaincfg.RegressionNetParams
	params.CoinbaseMaturity = 1
	dbPath := t.TempDir()
	db, err := database.Create("ffldb", dbPath, params.Net)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer db.Close()

	txIdx := NewTxIndex(db)
	addrIdx := NewAddrIndex(db, &params)
	spy := &rewriteSpy{AddrIndex: addrIdx}
	mgr := NewManager(db, []Indexer{txIdx, spy})

	const buffer int32 = 3
	chain, err := blockchain.New(&blockchain.Config{
		DB:            db,
		ChainParams:   &params,
		TimeSource:    blockchain.NewMedianTime(),
		SigCache:      txscript.NewSigCache(1000),
		IndexManager:  mgr,
		WitnessBuffer: buffer,
	})
	if err != nil {
		t.Fatalf("New chain: %v", err)
	}

	genesis := btcutil.NewBlock(params.GenesisBlock)
	genesis.SetHeight(0)

	// Height 1: coinbase pays to P2WSH(OP_TRUE) so the next block can spend
	// with a witness stack.
	b1, spend1, err := processWitnessCoinbaseBlock(t, chain, genesis)
	if err != nil {
		t.Fatalf("block 1: %v", err)
	}

	// Height 2: spend with witness — this is the segwit block that must age
	// out with a full-block rewrite once tip advances past buffer.
	b2, _, err := processWitnessSpendBlock(t, chain, b1, spend1)
	if err != nil {
		t.Fatalf("block 2: %v", err)
	}
	if !blockHasWitness(b2) {
		t.Fatal("block 2 has no witness; test cannot detect stripped re-fetch")
	}
	spy.target = *b2.Hash()

	// Heights 3..2+buffer: push tip so height 2 ages out (ageOut = tip-buffer).
	prev := b2
	for h := int32(3); h <= 2+buffer; h++ {
		next, _, err := processAnyoneSpendBlock(t, chain, prev)
		if err != nil {
			t.Fatalf("block %d: %v", h, err)
		}
		prev = next
	}

	if tip := chain.BestSnapshot().Height; tip != 2+buffer {
		t.Fatalf("tip height=%d, want %d", tip, 2+buffer)
	}

	var cold bool
	err = db.View(func(tx database.Tx) error {
		var err error
		cold, err = tx.(database.ColdCompactor).IsColdBlock(b2.Hash())
		return err
	})
	if err != nil {
		t.Fatalf("IsColdBlock: %v", err)
	}
	if !cold {
		t.Fatal("expected segwit block 2 to be cold after age-out")
	}

	if spy.sawStrippedOnly {
		t.Fatal("age-out rewrite received a stripped-only block; " +
			"chain.go likely re-fetched after CompactBlockToCold")
	}
	if !spy.sawFullSegwitRewrite {
		t.Fatal("expected rewrite of full segwit block 2; " +
			"age-out may not have run or block had no witness delta")
	}

	assertAddrIndexRoundTrip(t, db, spy.AddrIndex, &params, []*btcutil.Block{b1, b2})
	assertAllAddrEntriesUseStrippedOffsets(t, db, b2)
}

func assertAddrIndexRoundTrip(t *testing.T, db database.DB, idx *AddrIndex,
	params *chaincfg.Params, blocks []*btcutil.Block) {

	t.Helper()
	txByBlock := make(map[chainhash.Hash]map[chainhash.Hash]struct{})
	var addrs []address.Address
	seenAddr := make(map[string]address.Address)
	for _, block := range blocks {
		txs := make(map[chainhash.Hash]struct{})
		for _, tx := range block.Transactions() {
			txs[*tx.Hash()] = struct{}{}
		}
		txByBlock[*block.Hash()] = txs
		for _, a := range outputAddresses(t, block, params) {
			seenAddr[a.String()] = a
		}
	}
	for _, a := range seenAddr {
		addrs = append(addrs, a)
	}
	if len(addrs) == 0 {
		t.Fatal("no parseable output addresses for round-trip")
	}

	found := false
	err := db.View(func(dbTx database.Tx) error {
		for _, addr := range addrs {
			regions, _, err := idx.TxRegionsForAddress(dbTx, addr, 0, 1<<20, false)
			if err != nil {
				return err
			}
			if len(regions) == 0 {
				continue
			}
			results, err := dbTx.FetchBlockRegions(regions)
			if err != nil {
				return fmt.Errorf("FetchBlockRegions %s: %v", addr, err)
			}
			for i, raw := range results {
				msgTx := wire.MsgTx{}
				if err := msgTx.DeserializeNoWitness(bytes.NewReader(raw)); err != nil {
					return fmt.Errorf("deserialize region %d for %s: %v",
						i, addr, err)
				}
				hash := msgTx.TxHash()
				want, ok := txByBlock[*regions[i].Hash]
				if !ok {
					return fmt.Errorf("addr %s region in unknown block %s",
						addr, regions[i].Hash)
				}
				if _, ok := want[hash]; !ok {
					return fmt.Errorf("addr %s region resolved to unexpected "+
						"txid %s in block %s (stale/wrong offset after age-out)",
						addr, hash, regions[i].Hash)
				}
				found = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("addrindex cold round-trip: %v", err)
	}
	if !found {
		t.Fatal("no addrindex regions resolved for chain outputs")
	}
}

func outputAddresses(t *testing.T, block *btcutil.Block,
	params *chaincfg.Params) []address.Address {

	t.Helper()
	seen := make(map[string]address.Address)
	for _, tx := range block.Transactions() {
		for _, out := range tx.MsgTx().TxOut {
			_, addrs, _, err := txscript.ExtractPkScriptAddrs(out.PkScript, params)
			if err != nil {
				continue
			}
			for _, a := range addrs {
				seen[a.String()] = a
			}
		}
	}
	out := make([]address.Address, 0, len(seen))
	for _, a := range seen {
		out = append(out, a)
	}
	return out
}

func blockHasWitness(block *btcutil.Block) bool {
	for _, tx := range block.MsgBlock().Transactions {
		for _, in := range tx.TxIn {
			if len(in.Witness) > 0 {
				return true
			}
		}
	}
	return false
}

func p2wshOpTrueScript() []byte {
	witnessScript := []byte{txscript.OP_TRUE}
	sum := sha256.Sum256(witnessScript)
	script, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).AddData(sum[:]).Script()
	if err != nil {
		panic(err)
	}
	return script
}

func addWitnessCommitment(coinbase *wire.MsgTx, blockTxns []*btcutil.Tx) {
	var witnessNonce [blockchain.CoinbaseWitnessDataLen]byte
	coinbase.TxIn[0].Witness = wire.TxWitness{witnessNonce[:]}
	witnessMerkleRoot := blockchain.CalcMerkleRoot(blockTxns, true)
	var preimage [64]byte
	copy(preimage[:32], witnessMerkleRoot[:])
	copy(preimage[32:], witnessNonce[:])
	commitment := chainhash.DoubleHashB(preimage[:])
	pkScript := append(append([]byte{}, blockchain.WitnessMagicBytes...), commitment...)
	coinbase.AddTxOut(wire.NewTxOut(0, pkScript))
}

func processWitnessCoinbaseBlock(t *testing.T, chain *blockchain.BlockChain,
	prev *btcutil.Block) (*btcutil.Block, *testhelper.SpendableOut, error) {

	t.Helper()
	height := prev.Height() + 1
	cb := testhelper.CreateCoinbaseTx(height,
		blockchain.CalcBlockSubsidy(height, chain.ChainParams()))
	cb.TxOut[0].PkScript = p2wshOpTrueScript()

	utilTxns := []*btcutil.Tx{btcutil.NewTx(cb)}
	addWitnessCommitment(cb, utilTxns)
	utilTxns = []*btcutil.Tx{btcutil.NewTx(cb)}

	ts := time.Unix(time.Now().Unix(), 0)
	if height > 1 {
		ts = prev.MsgBlock().Header.Timestamp.Add(time.Second)
	}

	block := btcutil.NewBlock(&wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    4,
			PrevBlock:  *prev.Hash(),
			MerkleRoot: blockchain.CalcMerkleRoot(utilTxns, false),
			Bits:       chain.ChainParams().PowLimitBits,
			Timestamp:  ts,
			Nonce:      0,
		},
		Transactions: []*wire.MsgTx{cb},
	})
	block.SetHeight(height)
	if !testhelper.SolveBlock(&block.MsgBlock().Header) {
		return nil, nil, fmt.Errorf("solve block %d", height)
	}
	if _, _, err := chain.ProcessBlock(block, blockchain.BFNone); err != nil {
		return nil, nil, err
	}
	spend := testhelper.MakeSpendableOutForTx(cb, 0)
	return block, &spend, nil
}

func processWitnessSpendBlock(t *testing.T, chain *blockchain.BlockChain,
	prev *btcutil.Block, spend *testhelper.SpendableOut) (*btcutil.Block, *testhelper.SpendableOut, error) {

	t.Helper()
	height := prev.Height() + 1
	cb := testhelper.CreateCoinbaseTx(height,
		blockchain.CalcBlockSubsidy(height, chain.ChainParams()))

	spendTx := wire.NewMsgTx(2)
	spendTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.PrevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		Witness:          wire.TxWitness{[]byte{txscript.OP_TRUE}},
	})
	// Pay to P2WSH(OP_TRUE) again so ExtractPkScriptAddrs yields a stable
	// address for the post-compaction searchrawtransactions round-trip.
	spendTx.AddTxOut(wire.NewTxOut(int64(spend.Amount-testhelper.LowFee),
		p2wshOpTrueScript()))
	opRet, err := testhelper.UniqueOpReturnScript()
	if err != nil {
		return nil, nil, err
	}
	spendTx.AddTxOut(wire.NewTxOut(0, opRet))
	cb.TxOut[0].Value += int64(testhelper.LowFee)

	txns := []*wire.MsgTx{cb, spendTx}
	utilTxns := []*btcutil.Tx{btcutil.NewTx(cb), btcutil.NewTx(spendTx)}
	addWitnessCommitment(cb, utilTxns)
	utilTxns = []*btcutil.Tx{btcutil.NewTx(cb), btcutil.NewTx(spendTx)}

	block := btcutil.NewBlock(&wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    4,
			PrevBlock:  *prev.Hash(),
			MerkleRoot: blockchain.CalcMerkleRoot(utilTxns, false),
			Bits:       chain.ChainParams().PowLimitBits,
			Timestamp:  prev.MsgBlock().Header.Timestamp.Add(time.Second),
			Nonce:      0,
		},
		Transactions: txns,
	})
	block.SetHeight(height)
	if !testhelper.SolveBlock(&block.MsgBlock().Header) {
		return nil, nil, fmt.Errorf("solve block %d", height)
	}
	if _, _, err := chain.ProcessBlock(block, blockchain.BFNone); err != nil {
		return nil, nil, err
	}
	out := testhelper.MakeSpendableOutForTx(spendTx, 0)
	return block, &out, nil
}

func processAnyoneSpendBlock(t *testing.T, chain *blockchain.BlockChain,
	prev *btcutil.Block) (*btcutil.Block, *testhelper.SpendableOut, error) {

	t.Helper()
	height := prev.Height() + 1
	cb := testhelper.CreateCoinbaseTx(height,
		blockchain.CalcBlockSubsidy(height, chain.ChainParams()))
	utilTxns := []*btcutil.Tx{btcutil.NewTx(cb)}
	block := btcutil.NewBlock(&wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    4,
			PrevBlock:  *prev.Hash(),
			MerkleRoot: blockchain.CalcMerkleRoot(utilTxns, false),
			Bits:       chain.ChainParams().PowLimitBits,
			Timestamp:  prev.MsgBlock().Header.Timestamp.Add(time.Second),
			Nonce:      0,
		},
		Transactions: []*wire.MsgTx{cb},
	})
	block.SetHeight(height)
	if !testhelper.SolveBlock(&block.MsgBlock().Header) {
		return nil, nil, fmt.Errorf("solve block %d", height)
	}
	if _, _, err := chain.ProcessBlock(block, blockchain.BFNone); err != nil {
		return nil, nil, err
	}
	out := testhelper.MakeSpendableOutForTx(cb, 0)
	return block, &out, nil
}
