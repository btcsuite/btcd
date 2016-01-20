// Copyright (c) 2014-2015 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package coinset_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/btcsuite/fastsha256"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrutil"
	"github.com/decred/dcrutil/coinset"
)

type TestCoin struct {
	TxHash     *chainhash.Hash
	TxIndex    uint32
	TxValue    dcrutil.Amount
	TxNumConfs int64
}

func (c *TestCoin) Hash() *chainhash.Hash { return c.TxHash }
func (c *TestCoin) Index() uint32         { return c.TxIndex }
func (c *TestCoin) Value() dcrutil.Amount { return c.TxValue }
func (c *TestCoin) PkScript() []byte      { return nil }
func (c *TestCoin) NumConfs() int64       { return c.TxNumConfs }
func (c *TestCoin) ValueAge() int64       { return int64(c.TxValue) * c.TxNumConfs }

func NewCoin(index int64, value dcrutil.Amount, numConfs int64) coinset.Coin {
	h := fastsha256.New()
	h.Write([]byte(fmt.Sprintf("%d", index)))
	hash, _ := chainhash.NewHash(h.Sum(nil))
	c := &TestCoin{
		TxHash:     hash,
		TxIndex:    0,
		TxValue:    value,
		TxNumConfs: numConfs,
	}
	return coinset.Coin(c)
}

type coinSelectTest struct {
	selector      coinset.CoinSelector
	inputCoins    []coinset.Coin
	targetValue   dcrutil.Amount
	expectedCoins []coinset.Coin
	expectedError error
}

func testCoinSelector(tests []coinSelectTest, t *testing.T) {
	for testIndex, test := range tests {
		cs, err := test.selector.CoinSelect(test.targetValue, test.inputCoins)
		if err != test.expectedError {
			t.Errorf("[%d] expected a different error: got=%v, expected=%v", testIndex, err, test.expectedError)
			continue
		}
		if test.expectedCoins != nil {
			if cs == nil {
				t.Errorf("[%d] expected non-nil coinset", testIndex)
				continue
			}
			coins := cs.Coins()
			if len(coins) != len(test.expectedCoins) {
				t.Errorf("[%d] expected different number of coins: got=%d, expected=%d", testIndex, len(coins), len(test.expectedCoins))
				continue
			}
			for n := 0; n < len(test.expectedCoins); n++ {
				if coins[n] != test.expectedCoins[n] {
					t.Errorf("[%d] expected different coins at coin index %d: got=%#v, expected=%#v", testIndex, n, coins[n], test.expectedCoins[n])
					continue
				}
			}
			coinSet := coinset.NewCoinSet(coins)
			if coinSet.TotalValue() < test.targetValue {
				t.Errorf("[%d] targetValue not satistifed", testIndex)
				continue
			}
		}
	}
}

var coins = []coinset.Coin{
	NewCoin(1, 100000000, 1),
	NewCoin(2, 10000000, 20),
	NewCoin(3, 50000000, 0),
	NewCoin(4, 25000000, 6),
}

func TestCoinSet(t *testing.T) {
	cs := coinset.NewCoinSet(nil)
	if cs.PopCoin() != nil {
		t.Error("Expected popCoin of empty to be nil")
	}
	if cs.ShiftCoin() != nil {
		t.Error("Expected shiftCoin of empty to be nil")
	}

	cs.PushCoin(coins[0])
	cs.PushCoin(coins[1])
	cs.PushCoin(coins[2])
	if cs.PopCoin() != coins[2] {
		t.Error("Expected third coin")
	}
	if cs.ShiftCoin() != coins[0] {
		t.Error("Expected first coin")
	}

	mtx := coinset.NewMsgTxWithInputCoins(cs)
	if len(mtx.TxIn) != 1 {
		t.Errorf("Expected only 1 TxIn, got %d", len(mtx.TxIn))
	}
	op := mtx.TxIn[0].PreviousOutPoint
	if !op.Hash.IsEqual(coins[1].Hash()) || op.Index != coins[1].Index() {
		t.Errorf("Expected the second coin to be added as input to mtx")
	}
}

var minIndexSelectors = []coinset.MinIndexCoinSelector{
	{MaxInputs: 10, MinChangeAmount: 10000},
	{MaxInputs: 2, MinChangeAmount: 10000},
}

var minIndexTests = []coinSelectTest{
	{minIndexSelectors[0], coins, coins[0].Value() - minIndexSelectors[0].MinChangeAmount, []coinset.Coin{coins[0]}, nil},
	{minIndexSelectors[0], coins, coins[0].Value() - minIndexSelectors[0].MinChangeAmount + 1, []coinset.Coin{coins[0], coins[1]}, nil},
	{minIndexSelectors[0], coins, 100000000, []coinset.Coin{coins[0]}, nil},
	{minIndexSelectors[0], coins, 110000000, []coinset.Coin{coins[0], coins[1]}, nil},
	{minIndexSelectors[0], coins, 140000000, []coinset.Coin{coins[0], coins[1], coins[2]}, nil},
	{minIndexSelectors[0], coins, 200000000, nil, coinset.ErrCoinsNoSelectionAvailable},
	{minIndexSelectors[1], coins, 10000000, []coinset.Coin{coins[0]}, nil},
	{minIndexSelectors[1], coins, 110000000, []coinset.Coin{coins[0], coins[1]}, nil},
	{minIndexSelectors[1], coins, 140000000, nil, coinset.ErrCoinsNoSelectionAvailable},
}

func TestMinIndexSelector(t *testing.T) {
	testCoinSelector(minIndexTests, t)
}

var minNumberSelectors = []coinset.MinNumberCoinSelector{
	{MaxInputs: 10, MinChangeAmount: 10000},
	{MaxInputs: 2, MinChangeAmount: 10000},
}

var minNumberTests = []coinSelectTest{
	{minNumberSelectors[0], coins, coins[0].Value() - minNumberSelectors[0].MinChangeAmount, []coinset.Coin{coins[0]}, nil},
	{minNumberSelectors[0], coins, coins[0].Value() - minNumberSelectors[0].MinChangeAmount + 1, []coinset.Coin{coins[0], coins[2]}, nil},
	{minNumberSelectors[0], coins, 100000000, []coinset.Coin{coins[0]}, nil},
	{minNumberSelectors[0], coins, 110000000, []coinset.Coin{coins[0], coins[2]}, nil},
	{minNumberSelectors[0], coins, 160000000, []coinset.Coin{coins[0], coins[2], coins[3]}, nil},
	{minNumberSelectors[0], coins, 184990000, []coinset.Coin{coins[0], coins[2], coins[3], coins[1]}, nil},
	{minNumberSelectors[0], coins, 184990001, nil, coinset.ErrCoinsNoSelectionAvailable},
	{minNumberSelectors[0], coins, 200000000, nil, coinset.ErrCoinsNoSelectionAvailable},
	{minNumberSelectors[1], coins, 10000000, []coinset.Coin{coins[0]}, nil},
	{minNumberSelectors[1], coins, 110000000, []coinset.Coin{coins[0], coins[2]}, nil},
	{minNumberSelectors[1], coins, 140000000, []coinset.Coin{coins[0], coins[2]}, nil},
}

func TestMinNumberSelector(t *testing.T) {
	testCoinSelector(minNumberTests, t)
}

var maxValueAgeSelectors = []coinset.MaxValueAgeCoinSelector{
	{MaxInputs: 10, MinChangeAmount: 10000},
	{MaxInputs: 2, MinChangeAmount: 10000},
}

var maxValueAgeTests = []coinSelectTest{
	{maxValueAgeSelectors[0], coins, 100000, []coinset.Coin{coins[1]}, nil},
	{maxValueAgeSelectors[0], coins, 10000000, []coinset.Coin{coins[1]}, nil},
	{maxValueAgeSelectors[0], coins, 10000001, []coinset.Coin{coins[1], coins[3]}, nil},
	{maxValueAgeSelectors[0], coins, 35000000, []coinset.Coin{coins[1], coins[3]}, nil},
	{maxValueAgeSelectors[0], coins, 135000000, []coinset.Coin{coins[1], coins[3], coins[0]}, nil},
	{maxValueAgeSelectors[0], coins, 185000000, []coinset.Coin{coins[1], coins[3], coins[0], coins[2]}, nil},
	{maxValueAgeSelectors[0], coins, 200000000, nil, coinset.ErrCoinsNoSelectionAvailable},
	{maxValueAgeSelectors[1], coins, 40000000, nil, coinset.ErrCoinsNoSelectionAvailable},
	{maxValueAgeSelectors[1], coins, 35000000, []coinset.Coin{coins[1], coins[3]}, nil},
	{maxValueAgeSelectors[1], coins, 34990001, nil, coinset.ErrCoinsNoSelectionAvailable},
}

func TestMaxValueAgeSelector(t *testing.T) {
	testCoinSelector(maxValueAgeTests, t)
}

var minPrioritySelectors = []coinset.MinPriorityCoinSelector{
	{MaxInputs: 10, MinChangeAmount: 10000, MinAvgValueAgePerInput: 100000000},
	{MaxInputs: 02, MinChangeAmount: 10000, MinAvgValueAgePerInput: 200000000},
	{MaxInputs: 02, MinChangeAmount: 10000, MinAvgValueAgePerInput: 150000000},
	{MaxInputs: 03, MinChangeAmount: 10000, MinAvgValueAgePerInput: 150000000},
	{MaxInputs: 10, MinChangeAmount: 10000, MinAvgValueAgePerInput: 1000000000},
	{MaxInputs: 10, MinChangeAmount: 10000, MinAvgValueAgePerInput: 175000000},
	{MaxInputs: 02, MinChangeAmount: 10000, MinAvgValueAgePerInput: 125000000},
}

var connectedCoins = []coinset.Coin{coins[0], coins[1], coins[3]}

var minPriorityTests = []coinSelectTest{
	{minPrioritySelectors[0], connectedCoins, 100000000, []coinset.Coin{coins[0]}, nil},
	{minPrioritySelectors[0], connectedCoins, 125000000, []coinset.Coin{coins[0], coins[3]}, nil},
	{minPrioritySelectors[0], connectedCoins, 135000000, []coinset.Coin{coins[0], coins[3], coins[1]}, nil},
	{minPrioritySelectors[0], connectedCoins, 140000000, nil, coinset.ErrCoinsNoSelectionAvailable},
	{minPrioritySelectors[1], connectedCoins, 100000000, nil, coinset.ErrCoinsNoSelectionAvailable},
	{minPrioritySelectors[1], connectedCoins, 10000000, []coinset.Coin{coins[1]}, nil},
	{minPrioritySelectors[1], connectedCoins, 100000000, nil, coinset.ErrCoinsNoSelectionAvailable},
	{minPrioritySelectors[2], connectedCoins, 11000000, []coinset.Coin{coins[3]}, nil},
	{minPrioritySelectors[2], connectedCoins, 25000001, []coinset.Coin{coins[3], coins[1]}, nil},
	{minPrioritySelectors[3], connectedCoins, 25000001, []coinset.Coin{coins[3], coins[1], coins[0]}, nil},
	{minPrioritySelectors[3], connectedCoins, 100000000, []coinset.Coin{coins[3], coins[1], coins[0]}, nil},
	{minPrioritySelectors[3], []coinset.Coin{coins[1], coins[2]}, 10000000, []coinset.Coin{coins[1]}, nil},
	{minPrioritySelectors[4], connectedCoins, 1, nil, coinset.ErrCoinsNoSelectionAvailable},
	{minPrioritySelectors[5], connectedCoins, 20000000, []coinset.Coin{coins[1], coins[3]}, nil},
	{minPrioritySelectors[6], connectedCoins, 25000000, []coinset.Coin{coins[3], coins[0]}, nil},
}

func TestMinPrioritySelector(t *testing.T) {
	testCoinSelector(minPriorityTests, t)
}

var (
	// should be two outpoints, with 1st one having 1.29994545DCR value.
	testSimpleCoinNumConfs            = int64(1)
	testSimpleCoinTxHash              = "fdc5aa15e3c9fdef4e6436f79ad334842b1596edae13e8b2450ab576dc5494f5"
	testSimpleCoinTxHex               = "010000000101e4d1fdb04871f69d198701e8c8c410da20507a74c3ffc4dea00b4d7444491c0600000001ffffffff02318fbf070000000000001976a9148485ee5dba5ac084f12450f8ebac97e2114fc90088ace06735000000000000001976a914784ebee20805af80a526f8d7603bffd6355d6d1988ac000000000000000001f9faf40700000000c92a0000090000006b483045022100ae3188239dc0983de2cfd2ed47ce5996888ef3512ee0b88c6cd6e1996781277f022021d849fc851df171bd9b4b1304bd24aee4050c7bda111fddbd9ebf83faa8640c0121026eea31de604e54e9027e1913d82e3d7f072b9553fde5792d2ac2317b9babda31"
	testSimpleCoinTxValue0            = dcrutil.Amount(129994545)
	testSimpleCoinTxValueAge0         = int64(testSimpleCoinTxValue0) * testSimpleCoinNumConfs
	testSimpleCoinTxPkScript0Hex      = "76a9148485ee5dba5ac084f12450f8ebac97e2114fc90088ac"
	testSimpleCoinTxPkScript0Bytes, _ = hex.DecodeString(testSimpleCoinTxPkScript0Hex)
	testSimpleCoinTxBytes, _          = hex.DecodeString(testSimpleCoinTxHex)
	testSimpleCoinTx, _               = dcrutil.NewTxFromBytes(testSimpleCoinTxBytes)
	testSimpleCoin                    = &coinset.SimpleCoin{
		Tx:         testSimpleCoinTx,
		TxIndex:    0,
		TxNumConfs: testSimpleCoinNumConfs,
	}
)

func TestSimpleCoin(t *testing.T) {
	if testSimpleCoin.Hash().String() != testSimpleCoinTxHash {
		t.Error("Different value for tx hash than expected")
	}
	if testSimpleCoin.Index() != 0 {
		t.Error("Different value for index of outpoint than expected")
	}
	if testSimpleCoin.Value() != testSimpleCoinTxValue0 {
		t.Error("Different value of coin value than expected")
	}
	if !bytes.Equal(testSimpleCoin.PkScript(), testSimpleCoinTxPkScript0Bytes) {
		t.Error("Different value of coin pkScript than expected", testSimpleCoin.PkScript())
	}
	if testSimpleCoin.NumConfs() != 1 {
		t.Error("Differet value of num confs than expected")
	}
	if testSimpleCoin.ValueAge() != testSimpleCoinTxValueAge0 {
		t.Error("Different value of coin value * age than expected")
	}
}
