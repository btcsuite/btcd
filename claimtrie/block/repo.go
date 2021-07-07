package block

import (
	"github.com/lbryio/lbcd/chaincfg/chainhash"
)

// Repo defines APIs for Block to access persistence layer.
type Repo interface {
	Load() (int32, error)
	Set(height int32, hash *chainhash.Hash) error
	Get(height int32) (*chainhash.Hash, error)
	Close() error
	Flush() error
	Delete(heightMin, heightMax int32) error
}
