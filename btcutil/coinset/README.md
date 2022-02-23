coinset
=======

[![Build Status](http://img.shields.io/travis/btcsuite/btcutil.svg)](https://travis-ci.org/btcsuite/btcutil)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](http://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/btcsuite/btcd/btcutil/coinset)

Package coinset provides bitcoin-specific convenience functions for selecting
from and managing sets of unspent transaction outpoints (UTXOs).

A comprehensive suite of tests is provided to ensure proper functionality.  See
`test_coverage.txt` for the gocov coverage report.  Alternatively, if you are
running a POSIX OS, you can run the `cov_report.sh` script for a real-time
report.

## Installation and Updating

```bash
$ go get -u github.com/btcsuite/btcd/btcutil/coinset
```

## Usage

Each unspent transaction outpoint is represented by the Coin interface.  An
example of a concrete type that implements Coin is coinset.SimpleCoin.

The typical use case for this library is for creating raw bitcoin transactions
given a set of Coins that may be spent by the user, for example as below:

```Go
var unspentCoins = []coinset.Coin{ ... }
```

When the user needs to spend a certain amount, they will need to select a
subset of these coins which contain at least that value.  CoinSelector is
an interface that represents types that implement coin selection algos,
subject to various criteria.  There are a few examples of CoinSelector's:

- MinIndexCoinSelector

- MinNumberCoinSelector

- MaxValueAgeCoinSelector

- MinPriorityCoinSelector

For example, if the user wishes to maximize the probability that their
transaction is mined quickly, they could use the MaxValueAgeCoinSelector to
select high priority coins, then also attach a relatively high fee.

```Go
selector := &coinset.MaxValueAgeCoinSelector{
    MaxInputs: 10,
    MinAmountChange: 10000,
}
selectedCoins, err := selector.CoinSelect(targetAmount + bigFee, unspentCoins)
if err != nil {
	return err
}
msgTx := coinset.NewMsgTxWithInputCoins(selectedCoins)
...

```

The user can then create the msgTx.TxOut's as required, then sign the
transaction and transmit it to the network.

## License

Package coinset is licensed under the [copyfree](http://copyfree.org) ISC
License.
