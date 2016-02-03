// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is part of the ffldb package rather than the ffldb_test package as
// it is part of the whitebox testing.

package ffldb

import (
	"errors"
	"io"
	"sync"
)

// Errors used for the mock file.
var (
	// errMockFileClosed is used to indicate a mock file is closed.
	errMockFileClosed = errors.New("file closed")

	// errInvalidOffset is used to indicate an offset that is out of range
	// for the file was provided.
	errInvalidOffset = errors.New("invalid offset")

	// errSyncFail is used to indicate simulated sync failure.
	errSyncFail = errors.New("simulated sync failure")
)

// mockFile implements the filer interface and used in order to force failures
// the database code related to reading and writing from the flat block files.
// A maxSize of -1 is unlimited.
type mockFile struct {
	sync.RWMutex
	maxSize      int64
	data         []byte
	forceSyncErr bool
	closed       bool
}

// Close closes the mock file without releasing any data associated with it.
// This allows it to be "reopened" without losing the data.
//
// This is part of the filer implementation.
func (f *mockFile) Close() error {
	f.Lock()
	defer f.Unlock()

	if f.closed {
		return errMockFileClosed
	}
	f.closed = true
	return nil
}

// ReadAt reads len(b) bytes from the mock file starting at byte offset off. It
// returns the number of bytes read and the error, if any.  ReadAt always
// returns a non-nil error when n < len(b). At end of file, that error is
// io.EOF.
//
// This is part of the filer implementation.
func (f *mockFile) ReadAt(b []byte, off int64) (int, error) {
	f.RLock()
	defer f.RUnlock()

	if f.closed {
		return 0, errMockFileClosed
	}
	maxSize := int64(len(f.data))
	if f.maxSize > -1 && maxSize > f.maxSize {
		maxSize = f.maxSize
	}
	if off < 0 || off > maxSize {
		return 0, errInvalidOffset
	}

	// Limit to the max size field, if set.
	numToRead := int64(len(b))
	endOffset := off + numToRead
	if endOffset > maxSize {
		numToRead = maxSize - off
	}

	copy(b, f.data[off:off+numToRead])
	if numToRead < int64(len(b)) {
		return int(numToRead), io.EOF
	}
	return int(numToRead), nil
}

// Truncate changes the size of the mock file.
//
// This is part of the filer implementation.
func (f *mockFile) Truncate(size int64) error {
	f.Lock()
	defer f.Unlock()

	if f.closed {
		return errMockFileClosed
	}
	maxSize := int64(len(f.data))
	if f.maxSize > -1 && maxSize > f.maxSize {
		maxSize = f.maxSize
	}
	if size > maxSize {
		return errInvalidOffset
	}

	f.data = f.data[:size]
	return nil
}

// Write writes len(b) bytes to the mock file. It returns the number of bytes
// written and an error, if any.  Write returns a non-nil error any time
// n != len(b).
//
// This is part of the filer implementation.
func (f *mockFile) WriteAt(b []byte, off int64) (int, error) {
	f.Lock()
	defer f.Unlock()

	if f.closed {
		return 0, errMockFileClosed
	}
	maxSize := f.maxSize
	if maxSize < 0 {
		maxSize = 100 * 1024 // 100KiB
	}
	if off < 0 || off > maxSize {
		return 0, errInvalidOffset
	}

	// Limit to the max size field, if set, and grow the slice if needed.
	numToWrite := int64(len(b))
	if off+numToWrite > maxSize {
		numToWrite = maxSize - off
	}
	if off+numToWrite > int64(len(f.data)) {
		newData := make([]byte, off+numToWrite)
		copy(newData, f.data)
		f.data = newData
	}

	copy(f.data[off:], b[:numToWrite])
	if numToWrite < int64(len(b)) {
		return int(numToWrite), io.EOF
	}
	return int(numToWrite), nil
}

// Sync doesn't do anything for mock files.  However, it will return an error if
// the mock file's forceSyncErr flag is set.
//
// This is part of the filer implementation.
func (f *mockFile) Sync() error {
	if f.forceSyncErr {
		return errSyncFail
	}

	return nil
}

// Ensure the mockFile type implements the filer interface.
var _ filer = (*mockFile)(nil)
