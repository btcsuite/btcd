package index

import (
	"fmt"

	"github.com/btcsuite/btcd/blockchain"
)

// indexes holds all of the registered indexes.
var (
	indexes = make(map[string]blockchain.Index)
)

// RegisterIndex adds an index to the available indexes. An error will be
// returned if an index with the same name is already registered.
func RegisterIndex(index blockchain.Index) error {
	if _, exists := indexes[index.Name()]; exists {
		return fmt.Errorf("index %q is already registered", index.Name())
	}

	indexes[index.Name()] = index
	return nil
}

// GetIndexes returns a list of indexes by name. Duplicate index names will be
// ignored (the index will be returned only once). Will return an error if
// at least one of the requested indexes does not exist.
func GetIndexes(names []string) ([]blockchain.Index, error) {
	var res []blockchain.Index
	found := make(map[string]struct{})

	for _, name := range names {
		_, exists := found[name]
		if exists {
			continue
		}
		found[name] = struct{}{}
		index, exists := indexes[name]
		if !exists {
			return nil, fmt.Errorf("index %q does not exist", name)
		}
		res = append(res, index)
	}

	return res, nil
}
