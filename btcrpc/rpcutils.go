package btcrpc

import "github.com/btcsuite/btcd/chaincfg/chainhash"

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
