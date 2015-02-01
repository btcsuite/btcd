// Copyright 2015 Conformal Systems LLC. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package btcec

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"io/ioutil"
)

//go:generate go run -tags gensecp256k1 genprecomps.go

// loadS256BytePoints decompresses and deserializes the pre-computed byte points
// used to accelerate scalar base multiplication for the secp256k1 curve.  This
// approach is used since it allows the compile to use significantly less ram
// and be performed much faster than it is with hard-coding the final in-memory
// data structure.  At the same time, it is quite fast to generate the in-memory
// data structure at init time with this approach versus computing the table.
func loadS256BytePoints() error {
	// There will be no byte points to load when generating them.
	bp := secp256k1BytePoints
	if len(secp256k1BytePoints) == 0 {
		return nil
	}

	// Decompress the pre-computed table used to accelerate scalar base
	// multiplication.
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(bp)))
	if _, err := base64.StdEncoding.Decode(decoded, bp); err != nil {
		return err
	}
	r, err := zlib.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return err
	}
	serialized, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	// Deserialize the precomputed byte points and set the curve to them.
	offset := 0
	var bytePoints [32][256][3]fieldVal
	for byteNum := 0; byteNum < 32; byteNum++ {
		// All points in this window.
		for i := 0; i < 256; i++ {
			px, py, pz := new(fieldVal), new(fieldVal), new(fieldVal)
			for i := 0; i < 10; i++ {
				px.n[i] = binary.LittleEndian.Uint32(serialized[offset:])
				offset += 4
			}
			for i := 0; i < 10; i++ {
				py.n[i] = binary.LittleEndian.Uint32(serialized[offset:])
				offset += 4
			}
			for i := 0; i < 10; i++ {
				pz.n[i] = binary.LittleEndian.Uint32(serialized[offset:])
				offset += 4
			}
			bytePoints[byteNum][i][0] = *px
			bytePoints[byteNum][i][1] = *py
			bytePoints[byteNum][i][2] = *pz
		}
	}
	secp256k1.bytePoints = &bytePoints
	return nil
}
