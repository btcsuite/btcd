package noderepo

import (
	"bytes"
	"sort"

	"github.com/cockroachdb/pebble"
	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/pkg/errors"
)

type Pebble struct {
	db *pebble.DB
}

func NewPebble(path string) (*Pebble, error) {

	db, err := pebble.Open(path, &pebble.Options{Cache: pebble.NewCache(64 << 20), BytesPerSync: 8 << 20, MaxOpenFiles: 2000})
	repo := &Pebble{db: db}

	return repo, errors.Wrapf(err, "unable to open %s", path)
}

func (repo *Pebble) AppendChanges(changes []change.Change) error {

	batch := repo.db.NewBatch()
	defer batch.Close()

	buffer := bytes.NewBuffer(nil)

	for _, chg := range changes {
		buffer.Reset()
		err := chg.Marshal(buffer)
		if err != nil {
			return errors.Wrap(err, "in marshaller")
		}

		err = batch.Merge(chg.Name, buffer.Bytes(), pebble.NoSync)
		if err != nil {
			return errors.Wrap(err, "in merge")
		}
	}
	return errors.Wrap(batch.Commit(pebble.NoSync), "in commit")
}

func (repo *Pebble) LoadChanges(name []byte) ([]change.Change, error) {

	data, closer, err := repo.db.Get(name)
	if err != nil && err != pebble.ErrNotFound {
		return nil, errors.Wrapf(err, "in get %s", name) // does returning a name in an error expose too much?
	}
	if closer != nil {
		defer closer.Close()
	}

	return unmarshalChanges(name, data)
}

func unmarshalChanges(name, data []byte) ([]change.Change, error) {
	// data is 84bytes+ per change
	changes := make([]change.Change, 0, len(data)/84+1) // average is 5.1 changes

	buffer := bytes.NewBuffer(data)
	sortNeeded := false
	for buffer.Len() > 0 {
		var chg change.Change
		err := chg.Unmarshal(buffer)
		if err != nil {
			return nil, errors.Wrap(err, "in decode")
		}
		chg.Name = name
		if len(changes) > 0 && chg.Height < changes[len(changes)-1].Height {
			sortNeeded = true // alternately: sortNeeded || chg.Height != chg.VisibleHeight
		}
		changes = append(changes, chg)
	}

	if sortNeeded {
		// this was required for the normalization stuff:
		sort.SliceStable(changes, func(i, j int) bool {
			return changes[i].Height < changes[j].Height
		})
	}
	return changes, nil
}

func (repo *Pebble) DropChanges(name []byte, finalHeight int32) error {
	changes, err := repo.LoadChanges(name)
	if err != nil {
		return errors.Wrapf(err, "in load changes for %s", name)
	}
	buffer := bytes.NewBuffer(nil)
	for i := 0; i < len(changes); i++ { // assuming changes are ordered by height
		if changes[i].Height > finalHeight {
			break
		}
		if changes[i].VisibleHeight > finalHeight { // created after this height has to be skipped
			continue
		}
		// having to sort the changes really messes up performance here. It would be better to not remarshal
		err := changes[i].Marshal(buffer)
		if err != nil {
			return errors.Wrap(err, "in marshaller")
		}
	}

	// making a performance assumption that DropChanges won't happen often:
	err = repo.db.Set(name, buffer.Bytes(), pebble.NoSync)
	return errors.Wrapf(err, "in set at %s", name)
}

func (repo *Pebble) IterateChildren(name []byte, f func(changes []change.Change) bool) error {
	start := make([]byte, len(name)+1) // zeros that last byte; need a constant len for stack alloc?
	copy(start, name)

	end := make([]byte, len(name)) // max name length is 255
	copy(end, name)
	validEnd := false
	for i := len(name) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			validEnd = true
			break
		}
	}
	if !validEnd {
		end = nil // uh, we think this means run to the end of the table
	}

	prefixIterOptions := &pebble.IterOptions{
		LowerBound: start,
		UpperBound: end,
	}

	iter := repo.db.NewIter(prefixIterOptions)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// NOTE! iter.Key() is ephemeral!
		changes, err := unmarshalChanges(iter.Key(), iter.Value())
		if err != nil {
			return errors.Wrapf(err, "from unmarshaller at %s", iter.Key())
		}
		if !f(changes) {
			break
		}
	}
	return nil
}

func (repo *Pebble) IterateAll(predicate func(name []byte) bool) {
	iter := repo.db.NewIter(nil)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		if !predicate(iter.Key()) {
			break
		}
	}
}

func (repo *Pebble) Close() error {

	err := repo.db.Flush()
	if err != nil {
		// if we fail to close are we going to try again later?
		return errors.Wrap(err, "on flush")
	}

	err = repo.db.Close()
	return errors.Wrap(err, "on close")
}

func (repo *Pebble) Flush() error {
	_, err := repo.db.AsyncFlush()
	return err
}
