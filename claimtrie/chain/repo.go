package chain

import "github.com/lbryio/lbcd/claimtrie/change"

type Repo interface {
	Save(height int32, changes []change.Change) error
	Load(height int32) ([]change.Change, error)
	Close() error
	Flush() error
}
