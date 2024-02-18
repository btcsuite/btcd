//go:build !js && !wasm

package addrmgr

import (
	"io"
	"os"
)

type Store struct {
	path string
}

func (s *Store) Reader() (io.ReadCloser, error) {
	// Open the file.
	r, err := os.Open(s.path)

	// Convert into a generic error.
	if os.IsNotExist(err) {
		return nil, ErrNotExist
	}

	return r, err
}

func (s *Store) Writer() (io.WriteCloser, error) {
	// Create or open the file.
	return os.Create(s.path)
}

func (s *Store) Remove() error {
	return os.Remove(s.path)
}

func (s *Store) String() string {
	return s.path
}

func NewStore(path string) *Store {
	return &Store{
		path: path,
	}
}
