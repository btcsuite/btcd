package coinset

import (
	"container/list"
	"errors"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"sort"
)

// Coin represents a spendable transaction outpoint
type Coin interface {
	Hash() *btcwire.ShaHash
	Index() uint32
	Value() int64
	PkScript() []byte
	NumConfs() int64
	ValueAge() int64
}

// Coins represents a set of Coins
type Coins interface {
	Coins() []Coin
}

// CoinSet is a utility struct for the modifications of a set of
// Coins that implements the Coins interface.  To create a CoinSet,
// you must call NewCoinSet with nil for an empty set or a slice of
// coins as the initial contents.
//
// It is important to note that the all the Coins being added or removed
// from a CoinSet must have a constant ValueAge() during the use of
// the CoinSet, otherwise the cached values will be incorrect.
type CoinSet struct {
	coinList      *list.List
	totalValue    int64
	totalValueAge int64
}

// Ensure that CoinSet is a Coins
var _ Coins = NewCoinSet(nil)

// NewCoinSet creates a CoinSet containing the coins provided.
// To create an empty CoinSet, you may pass null as the coins input parameter.
func NewCoinSet(coins []Coin) *CoinSet {
	newCoinSet := &CoinSet{
		coinList:      list.New(),
		totalValue:    0,
		totalValueAge: 0,
	}
	for _, coin := range coins {
		newCoinSet.PushCoin(coin)
	}
	return newCoinSet
}

// Coins returns a new slice of the coins contained in the set.
func (cs *CoinSet) Coins() []Coin {
	coins := make([]Coin, cs.coinList.Len())
	for i, e := 0, cs.coinList.Front(); e != nil; i, e = i+1, e.Next() {
		coins[i] = e.Value.(Coin)
	}
	return coins
}

// TotalValue returns the total value of the coins in the set.
func (cs *CoinSet) TotalValue() (value int64) {
	return cs.totalValue
}

// TotalValueAge returns the total value * number of confirmations
//  of the coins in the set.
func (cs *CoinSet) TotalValueAge() (valueAge int64) {
	return cs.totalValueAge
}

// Num returns the number of coins in the set
func (cs *CoinSet) Num() int {
	return cs.coinList.Len()
}

// PushCoin adds a coin to the end of the list and updates
// the cached value amounts.
func (cs *CoinSet) PushCoin(c Coin) {
	cs.coinList.PushBack(c)
	cs.totalValue += c.Value()
	cs.totalValueAge += c.ValueAge()
}

// PopCoin removes the last coin on the list and returns it.
func (cs *CoinSet) PopCoin() Coin {
	back := cs.coinList.Back()
	if back == nil {
		return nil
	}
	return cs.removeElement(back)
}

// ShiftCoin removes the first coin on the list and returns it.
func (cs *CoinSet) ShiftCoin() Coin {
	front := cs.coinList.Front()
	if front == nil {
		return nil
	}
	return cs.removeElement(front)
}

// removeElement updates the cached value amounts in the CoinSet,
// removes the element from the list, then returns the Coin that
// was removed to the caller.
func (cs *CoinSet) removeElement(e *list.Element) Coin {
	c := e.Value.(Coin)
	cs.coinList.Remove(e)
	cs.totalValue -= c.Value()
	cs.totalValueAge -= c.ValueAge()
	return c
}

// NewMsgTxWithInputCoins takes the coins in the CoinSet and makes them
// the inputs to a new btcwire.MsgTx which is returned.
func NewMsgTxWithInputCoins(inputCoins Coins) *btcwire.MsgTx {
	msgTx := btcwire.NewMsgTx()
	coins := inputCoins.Coins()
	msgTx.TxIn = make([]*btcwire.TxIn, len(coins))
	for i, coin := range coins {
		msgTx.TxIn[i] = &btcwire.TxIn{
			PreviousOutpoint: btcwire.OutPoint{
				Hash:  *coin.Hash(),
				Index: coin.Index(),
			},
			SignatureScript: nil,
			Sequence:        btcwire.MaxTxInSequenceNum,
		}
	}
	return msgTx
}

var (
	// ErrCoinsNoSelectionAvailable is returned when a CoinSelector believes there is no
	// possible combination of coins which can meet the requirements provided to the selector.
	ErrCoinsNoSelectionAvailable = errors.New("no coin selection possible")
)

// satisfiesTargetValue checks that the totalValue is either exactly the targetValue
// or is greater than the targetValue by at least the minChange amount.
func satisfiesTargetValue(targetValue, minChange, totalValue int64) bool {
	return (totalValue == targetValue || totalValue >= targetValue+minChange)
}

// CoinSelector is an interface that wraps the CoinSelect method.
//
// CoinSelect will attempt to select a subset of the coins which has at
// least the targetValue amount.  CoinSelect is not guaranteed to return a
// selection of coins even if the total value of coins given is greater
// than the target value.
//
// The exact choice of coins in the subset will be implementation specific.
//
// It is important to note that the Coins being used as inputs need to have
// a constant ValueAge() during the execution of CoinSelect.
type CoinSelector interface {
	CoinSelect(targetValue int64, coins []Coin) (Coins, error)
}

// MinIndexCoinSelector is a CoinSelector that attempts to construct a
// selection of coins whose total value is at least targetValue and prefers
// any number of lower indexes (as in the ordered array) over higher ones.
type MinIndexCoinSelector struct {
	MaxInputs       int
	MinChangeAmount int64
}

// CoinSelect will attempt to select coins using the algorithm described
// in the MinIndexCoinSelector struct.
func (s MinIndexCoinSelector) CoinSelect(targetValue int64, coins []Coin) (Coins, error) {
	cs := NewCoinSet(nil)
	for n := 0; n < len(coins) && n < s.MaxInputs; n++ {
		cs.PushCoin(coins[n])
		if satisfiesTargetValue(targetValue, s.MinChangeAmount, cs.TotalValue()) {
			return cs, nil
		}
	}
	return nil, ErrCoinsNoSelectionAvailable
}

// MinNumberCoinSelector is a CoinSelector that attempts to construct
// a selection of coins whose total value is at least targetValue
// that uses as few of the inputs as possible.
type MinNumberCoinSelector struct {
	MaxInputs       int
	MinChangeAmount int64
}

// CoinSelect will attempt to select coins using the algorithm described
// in the MinNumberCoinSelector struct.
func (s MinNumberCoinSelector) CoinSelect(targetValue int64, coins []Coin) (Coins, error) {
	sortedCoins := make([]Coin, 0, len(coins))
	sortedCoins = append(sortedCoins, coins...)
	sort.Sort(sort.Reverse(byAmount(sortedCoins)))
	return (&MinIndexCoinSelector{
		MaxInputs:       s.MaxInputs,
		MinChangeAmount: s.MinChangeAmount,
	}).CoinSelect(targetValue, sortedCoins)
}

// MaxValueAgeCoinSelector is a CoinSelector that attempts to construct
// a selection of coins whose total value is at least targetValue
// that has as much input value-age as possible.
//
// This would be useful in the case where you want to maximize
// likelihood of the inclusion of your transaction in the next mined
// block.
type MaxValueAgeCoinSelector struct {
	MaxInputs       int
	MinChangeAmount int64
}

// CoinSelect will attempt to select coins using the algorithm described
// in the MaxValueAgeSelector struct.
func (s MaxValueAgeCoinSelector) CoinSelect(targetValue int64, coins []Coin) (Coins, error) {
	sortedCoins := make([]Coin, 0, len(coins))
	sortedCoins = append(sortedCoins, coins...)
	sort.Sort(sort.Reverse(byValueAge(sortedCoins)))
	return (&MinIndexCoinSelector{
		MaxInputs:       s.MaxInputs,
		MinChangeAmount: s.MinChangeAmount,
	}).CoinSelect(targetValue, sortedCoins)
}

// MinPriorityCoinSelector is a CoinSelector that attempts to construct
// a selection of coins whose total value is at least targetValue and
// whose average value-age per input is greater than MinAvgValueAgePerInput.
// If there is change, it must exceed MinChangeAmount to be a valid selection.
//
// When possible, MinPriorityCoinSelector will attempt to reduce the average
// input priority over the threshold, but no guarantees will be made as to
// minimality of the selection.  The selection below is almost certainly
// suboptimal.
//
type MinPriorityCoinSelector struct {
	MaxInputs              int
	MinChangeAmount        int64
	MinAvgValueAgePerInput int64
}

// CoinSelect will attempt to select coins using the algorithm described
// in the MinPriorityCoinSelector struct.
func (s MinPriorityCoinSelector) CoinSelect(targetValue int64, coins []Coin) (Coins, error) {
	possibleCoins := make([]Coin, 0, len(coins))
	possibleCoins = append(possibleCoins, coins...)

	sort.Sort(byValueAge(possibleCoins))

	// find the first coin with sufficient valueAge
	cutoffIndex := -1
	for i := 0; i < len(possibleCoins); i++ {
		if possibleCoins[i].ValueAge() >= s.MinAvgValueAgePerInput {
			cutoffIndex = i
			break
		}
	}
	if cutoffIndex < 0 {
		return nil, ErrCoinsNoSelectionAvailable
	}

	// create sets of input coins that will obey minimum average valueAge
	for i := cutoffIndex; i < len(possibleCoins); i++ {
		possibleHighCoins := possibleCoins[cutoffIndex : i+1]

		// choose a set of high-enough valueAge coins
		highSelect, err := (&MinNumberCoinSelector{
			MaxInputs:       s.MaxInputs,
			MinChangeAmount: s.MinChangeAmount,
		}).CoinSelect(targetValue, possibleHighCoins)

		if err != nil {
			// attempt to add available low priority to make a solution

			for numLow := 1; numLow <= cutoffIndex && numLow+(i-cutoffIndex) <= s.MaxInputs; numLow++ {
				allHigh := NewCoinSet(possibleCoins[cutoffIndex : i+1])
				newTargetValue := targetValue - allHigh.TotalValue()
				newMaxInputs := allHigh.Num() + numLow
				if newMaxInputs > numLow {
					newMaxInputs = numLow
				}
				newMinAvgValueAge := ((s.MinAvgValueAgePerInput * int64(allHigh.Num()+numLow)) - allHigh.TotalValueAge()) / int64(numLow)

				// find the minimum priority that can be added to set
				lowSelect, err := (&MinPriorityCoinSelector{
					MaxInputs:              newMaxInputs,
					MinChangeAmount:        s.MinChangeAmount,
					MinAvgValueAgePerInput: newMinAvgValueAge,
				}).CoinSelect(newTargetValue, possibleCoins[0:cutoffIndex])

				if err != nil {
					continue
				}

				for _, coin := range lowSelect.Coins() {
					allHigh.PushCoin(coin)
				}

				return allHigh, nil
			}
			// oh well, couldn't fix, try to add more high priority to the set.
		} else {
			extendedCoins := NewCoinSet(highSelect.Coins())

			// attempt to lower priority towards target with lowest ones first
			for n := 0; n < cutoffIndex; n++ {
				if extendedCoins.Num() >= s.MaxInputs {
					break
				}
				if possibleCoins[n].ValueAge() == 0 {
					continue
				}

				extendedCoins.PushCoin(possibleCoins[n])
				if extendedCoins.TotalValueAge()/int64(extendedCoins.Num()) < s.MinAvgValueAgePerInput {
					extendedCoins.PopCoin()
					continue
				}
			}
			return extendedCoins, nil
		}
	}

	return nil, ErrCoinsNoSelectionAvailable
}

type byValueAge []Coin

func (a byValueAge) Len() int           { return len(a) }
func (a byValueAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byValueAge) Less(i, j int) bool { return a[i].ValueAge() < a[j].ValueAge() }

type byAmount []Coin

func (a byAmount) Len() int           { return len(a) }
func (a byAmount) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byAmount) Less(i, j int) bool { return a[i].Value() < a[j].Value() }

// SimpleCoin defines a concrete instance of Coin that is backed by a
// btcutil.Tx, a specific outpoint index, and the number of confirmations
// that transaction has had.
type SimpleCoin struct {
	Tx         *btcutil.Tx
	TxIndex    uint32
	TxNumConfs int64
}

// Ensure that SimpleCoin is a Coin
var _ Coin = &SimpleCoin{}

// Hash returns the hash value of the transaction on which the Coin is an output
func (c *SimpleCoin) Hash() *btcwire.ShaHash {
	return c.Tx.Sha()
}

// Index returns the index of the output on the transaction which the Coin represents
func (c *SimpleCoin) Index() uint32 {
	return c.TxIndex
}

// txOut returns the TxOut of the transaction the Coin represents
func (c *SimpleCoin) txOut() *btcwire.TxOut {
	return c.Tx.MsgTx().TxOut[c.TxIndex]
}

// Value returns the value of the Coin
func (c *SimpleCoin) Value() int64 {
	return c.txOut().Value
}

// PkScript returns the outpoint script of the Coin.
//
// This can be used to determine what type of script the Coin uses
// and extract standard addresses if possible using
// btcscript.ExtractPkScriptAddrs for example.
func (c *SimpleCoin) PkScript() []byte {
	return c.txOut().PkScript
}

// NumConfs returns the number of confirmations that the transaction the Coin references
// has had.
func (c *SimpleCoin) NumConfs() int64 {
	return c.TxNumConfs
}

// ValueAge returns the product of the value and the number of confirmations.  This is
// used as an input to calculate the priority of the transaction.
func (c *SimpleCoin) ValueAge() int64 {
	return c.TxNumConfs * c.Value()
}
