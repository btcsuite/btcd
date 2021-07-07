package chainrepo

import (
	"encoding/binary"

	"github.com/pkg/errors"

	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/cockroachdb/pebble"
)

type Pebble struct {
	db *pebble.DB
}

func NewPebble(path string) (*Pebble, error) {

	db, err := pebble.Open(path, &pebble.Options{BytesPerSync: 64 << 20, MaxOpenFiles: 2000})
	repo := &Pebble{db: db}

	return repo, errors.Wrapf(err, "open %s", path)
}

func (repo *Pebble) Save(height int32, changes []change.Change) error {

	if len(changes) == 0 {
		return nil
	}

	var key [4]byte
	binary.BigEndian.PutUint32(key[:], uint32(height))

	value, err := msgpack.Marshal(changes)
	if err != nil {
		return errors.Wrap(err, "in marshaller")
	}

	err = repo.db.Set(key[:], value, pebble.NoSync)
	return errors.Wrap(err, "in set")
}

func (repo *Pebble) Load(height int32) ([]change.Change, error) {

	var key [4]byte
	binary.BigEndian.PutUint32(key[:], uint32(height))

	b, closer, err := repo.db.Get(key[:])
	if closer != nil {
		defer closer.Close()
	}
	if err != nil {
		return nil, errors.Wrap(err, "in get")
	}

	var changes []change.Change
	err = msgpack.Unmarshal(b, &changes)
	return changes, errors.Wrap(err, "in unmarshaller")
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
