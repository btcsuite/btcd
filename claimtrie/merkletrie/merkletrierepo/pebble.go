package merkletrierepo

import (
	"io"

	"github.com/cockroachdb/pebble"
	"github.com/pkg/errors"
)

type Pebble struct {
	db *pebble.DB
}

func NewPebble(path string) (*Pebble, error) {

	cache := pebble.NewCache(512 << 20)
	//defer cache.Unref()
	//
	//go func() {
	//	tick := time.NewTicker(60 * time.Second)
	//	for range tick.C {
	//
	//		m := cache.Metrics()
	//		fmt.Printf("cnt: %s, objs: %s, hits: %s, miss: %s, hitrate: %.2f\n",
	//			humanize.Bytes(uint64(m.Size)),
	//			humanize.Comma(m.Count),
	//			humanize.Comma(m.Hits),
	//			humanize.Comma(m.Misses),
	//			float64(m.Hits)/float64(m.Hits+m.Misses))
	//
	//	}
	//}()

	db, err := pebble.Open(path, &pebble.Options{Cache: cache, BytesPerSync: 32 << 20, MaxOpenFiles: 2000})
	repo := &Pebble{db: db}

	return repo, errors.Wrapf(err, "unable to open %s", path)
}

func (repo *Pebble) Get(key []byte) ([]byte, io.Closer, error) {
	d, c, e := repo.db.Get(key)
	if e == pebble.ErrNotFound {
		return nil, c, nil
	}
	return d, c, e
}

func (repo *Pebble) Set(key, value []byte) error {
	return repo.db.Set(key, value, pebble.NoSync)
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
