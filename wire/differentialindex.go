// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
	"math"
)

func readDifferentialIndex(r io.Reader, pver uint32, buf []byte,
	lastIndex uint64, hasLastIndex bool) (uint64, error) {

	differential, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return 0, err
	}

	index := differential
	if hasLastIndex {
		if differential > math.MaxUint64-lastIndex-1 {
			str := fmt.Sprintf("differential value overflows uint64 "+
				"[last %v, differential %v]", lastIndex, differential)
			return 0, messageError("readDifferentialIndex", str)
		}
		index = lastIndex + 1 + differential
	}

	if index > math.MaxUint32 {
		str := fmt.Sprintf("differential index overflows uint32 "+
			"[index %v]", index)
		return 0, messageError("readDifferentialIndex", str)
	}

	return index, nil
}

func writeDifferentialIndex(w io.Writer, pver uint32, buf []byte, index,
	lastIndex uint64, hasLastIndex bool) error {

	var differential uint64
	if hasLastIndex {
		if index <= lastIndex {
			str := fmt.Sprintf("indexes must be strictly increasing "+
				"[index %v, last %v]", index, lastIndex)
			return messageError("writeDifferentialIndex", str)
		}
		differential = index - lastIndex - 1
	} else {
		differential = index
	}

	return WriteVarIntBuf(w, pver, differential, buf)
}
