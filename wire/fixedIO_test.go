// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

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

// Write writes the contents of p to w.  When the contents of p would cause
// the writer to exceed the maximum allowed size of the fixed writer,
// io.ErrShortWrite is returned and the writer is left unchanged.
//
// This satisfies the io.Writer interface.
func (w *fixedWriter) Write(p []byte) (n int, err error) {
	lenp := len(p)
	if w.pos+lenp > cap(w.b) {
		return 0, io.ErrShortWrite
	}
	n = lenp
	w.pos += copy(w.b[w.pos:], p)
	return
}

// Bytes returns the bytes already written to the fixed writer.
func (w *fixedWriter) Bytes() []byte {
	return w.b
}

// newFixedWriter returns a new io.Writer that will error once more bytes than
// the specified max have been written.
func newFixedWriter(max int) io.Writer {
	b := make([]byte, max)
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

// Read reads the next len(p) bytes from the fixed reader.  When the number of
// bytes read would exceed the maximum number of allowed bytes to be read from
// the fixed writer, an error is returned.
//
// This satisfies the io.Reader interface.
func (fr *fixedReader) Read(p []byte) (n int, err error) {
	n, err = fr.iobuf.Read(p)
	fr.pos += n
	return
}

// newFixedReader returns a new io.Reader that will error once more bytes than
// the specified max have been read.
func newFixedReader(max int, buf []byte) io.Reader {
	b := make([]byte, max)
	if buf != nil {
		copy(b, buf)
	}

	iobuf := bytes.NewBuffer(b)
	fr := fixedReader{b, 0, iobuf}
	return &fr
}
