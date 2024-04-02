package testhelper

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
)

// SpendableOut represents a transaction output that is spendable along with
// additional metadata such as the block its in and how much it pays.
type SpendableOut struct {
	PrevOut wire.OutPoint
	Amount  btcutil.Amount
}

// MakeSpendableOutForTx returns a spendable output for the given transaction
// and transaction output index within the transaction.
func MakeSpendableOutForTx(tx *wire.MsgTx, txOutIndex uint32) SpendableOut {
	return SpendableOut{
		PrevOut: wire.OutPoint{
			Hash:  tx.TxHash(),
			Index: txOutIndex,
		},
		Amount: btcutil.Amount(tx.TxOut[txOutIndex].Value),
	}
}

// MakeSpendableOut returns a spendable output for the given block, transaction
// index within the block, and transaction output index within the transaction.
func MakeSpendableOut(block *wire.MsgBlock, txIndex, txOutIndex uint32) SpendableOut {
	return MakeSpendableOutForTx(block.Transactions[txIndex], txOutIndex)
}
