// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"io"
)

// fixedWriter implements the io.Writer interface and intentially allows
// testing of error paths by forcing short writes.
type fixedWriter struct {
	b   []byte
	pos int
}

// Write ...
func (w *fixedWriter) Write(p []byte) (n int, err error) {
	lenp := len(p)
	if w.pos+lenp > cap(w.b) {
		return 0, io.ErrShortWrite
	}
	n = lenp
	w.pos += copy(w.b[w.pos:], p)
	return
}

// Bytes ...
func (w *fixedWriter) Bytes() []byte {
	return w.b
}

// newFixedWriter...
func newFixedWriter(max int) *fixedWriter {
	b := make([]byte, max, max)
	fw := fixedWriter{b, 0}
	return &fw
}

// fixedReader implements the io.Reader interface and intentially allows
// testing of error paths by forcing short reads.
type fixedReader struct {
	buf   []byte
	pos   int
	iobuf *bytes.Buffer
}

// Read ....
func (fr *fixedReader) Read(p []byte) (n int, err error) {
	n, err = fr.iobuf.Read(p)
	fr.pos += n
	return
}

// newFixedReader ...
func newFixedReader(max int, buf []byte) *fixedReader {
	b := make([]byte, max, max)
	if buf != nil {
		copy(b[:], buf)
	}

	iobuf := bytes.NewBuffer(b)
	fr := fixedReader{b, 0, iobuf}
	return &fr
}
