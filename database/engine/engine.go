package engine

type Engine interface {
	Transaction() (Transaction, error)
	Snapshot() (Snapshot, error)
	Close() error
}

type Transaction interface {
	Put(key, value []byte) error
	Delete(key []byte) error
	Commit() error
	Discard()
}

type Snapshot interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	NewIterator(*Range) Iterator
	Releaser
}

type Releaser interface {
	Release()
}
