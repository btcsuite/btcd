//go:build js && wasm

package addrmgr

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/linden/localstorage"
)

type Store struct {
	path string
}

func (s *Store) Reader() (io.ReadCloser, error) {
	// Get the value from localStorage.
	val := localstorage.Get(s.path)

	// Convert into a generic error.
	if val == "" {
		return nil, ErrNotExist
	}

	// Create a new buffer storing our value.
	buf := bytes.NewBufferString(val)

	// Create a NOP closer, we have nothing to do upon close.
	return io.NopCloser(buf), nil
}

func (s *Store) Writer() (io.WriteCloser, error) {
	// Create a new writer.
	return newWriter(s.path), nil
}

func (s *Store) Remove() error {
	// Remove the key/value from localStorage.
	localstorage.Remove(s.path)

	return nil
}

func (s *Store) String() string {
	return s.path
}

func NewStore(path string) *Store {
	return &Store{
		path: path,
	}
}

// writer updates the localStorage on write.
type writer struct {
	path    string
	closed  bool
	builder strings.Builder
}

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("writer already closed")
	}

	// Write the bytes to our string builder.
	n, _ := w.builder.Write(p)

	// Update the localStorage value.
	localstorage.Set(w.path, w.builder.String())

	// Return the length written,
	return n, nil
}

func (w *writer) Close() error {
	w.closed = true
	return nil
}

func newWriter(path string) *writer {
	return &writer{
		path: path,
	}
}
