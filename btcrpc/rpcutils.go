package btcrpc

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// NewBlockLocatorHash creates a new BlockLocator using the given block hash.
func NewBlockLocatorHash(hash *chainhash.Hash) *BlockLocator {
	return &BlockLocator{
		Locator: &BlockLocator_Hash{
			Hash: hash[:],
		},
	}
}

// NewBlockLocatorHeight creates a new BlockLocator using the given block height.
func NewBlockLocatorHeight(height int32) *BlockLocator {
	return &BlockLocator{
		Locator: &BlockLocator_Height{
			Height: height,
		},
	}
}

// NewTransactionFilter creates a new transaction filter with the given addresses
// and outpoints.
func NewTransactionFilter(addresses []btcutil.Address, outpoints []wire.OutPoint) *TransactionFilter {
	f := TransactionFilter{
		Addresses: make([]string, len(addresses)),
		Outpoints: make([]*Transaction_Input_Outpoint, len(outpoints)),
	}

	for i, addr := range addresses {
		f.Addresses[i] = addr.EncodeAddress()
	}
	for i, op := range outpoints {
		f.Outpoints[i] = &Transaction_Input_Outpoint{
			Hash:  op.Hash[:],
			Index: op.Index,
		}
	}

	return &f
}
