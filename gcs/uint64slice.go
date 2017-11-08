// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2016-2017 The Lightning Network Developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

// uint64slice is a package-local utility class that allows us to use Go's sort
// package to sort a []uint64 by implementing sort.Interface.
type uint64Slice []uint64

// Len returns the length of the slice.
func (p uint64Slice) Len() int {
	return len(p)
}

// Less returns true when the ith element is smaller than the jth element of
// the slice, and returns false otherwise.
func (p uint64Slice) Less(i, j int) bool {
	return p[i] < p[j]
}

// Swap swaps two slice elements.
func (p uint64Slice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}
