package blockrepo

import (
	"encoding/binary"

	"github.com/pkg/errors"

	"github.com/lbryio/lbcd/chaincfg/chainhash"

	"github.com/cockroachdb/pebble"
)

type Pebble struct {
	db *pebble.DB
}

func NewPebble(path string) (*Pebble, error) {

	db, err := pebble.Open(path, &pebble.Options{MaxOpenFiles: 2000})
	repo := &Pebble{db: db}

	return repo, errors.Wrapf(err, "unable to open %s", path)
}

func (repo *Pebble) Load() (int32, error) {

	iter := repo.db.NewIter(nil)
	if !iter.Last() {
		err := iter.Close()
		return 0, errors.Wrap(err, "closing iterator with no last")
	}

	height := int32(binary.BigEndian.Uint32(iter.Key()))
	err := iter.Close()
	return height, errors.Wrap(err, "closing iterator")
}

func (repo *Pebble) Get(height int32) (*chainhash.Hash, error) {

	key := make([]byte, 4)
	binary.BigEndian.PutUint32(key, uint32(height))

	b, closer, err := repo.db.Get(key)
	if closer != nil {
		defer closer.Close()
	}
	if err != nil {
		return nil, errors.Wrap(err, "in get")
	}
	hash, err := chainhash.NewHash(b)
	return hash, errors.Wrap(err, "creating hash")
}

func (repo *Pebble) Set(height int32, hash *chainhash.Hash) error {

	key := make([]byte, 4)
	binary.BigEndian.PutUint32(key, uint32(height))

	return errors.WithStack(repo.db.Set(key, hash[:], pebble.NoSync))
}

func (repo *Pebble) Delete(heightMin, heightMax int32) error {
	lower := make([]byte, 4)
	binary.BigEndian.PutUint32(lower, uint32(heightMin))

	upper := make([]byte, 4)
	binary.BigEndian.PutUint32(upper, uint32(heightMax)+1)

	return errors.Wrap(repo.db.DeleteRange(lower, upper, pebble.NoSync), "on range delete")
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
