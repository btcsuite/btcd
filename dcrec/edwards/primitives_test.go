// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type ConversionVector struct {
	bIn *[32]byte
}

func testConversionVectors() []ConversionVector {
	r := rand.New(rand.NewSource(12345))

	numCvs := 50
	cvs := make([]ConversionVector, numCvs, numCvs)
	for i := 0; i < numCvs; i++ {
		bIn := new([32]byte)
		for j := 0; j < fieldIntSize; j++ {
			randByte := r.Intn(255)
			bIn[j] = uint8(randByte)
		}

		// Zero out the LSB as these aren't points.
		bIn[31] = bIn[31] &^ (1 << 7)
		cvs[i] = ConversionVector{bIn}
		r.Seed(int64(i) + 12345)
	}

	return cvs
}

// Tested functions:
//   EncodedBytesToBigInt
//   BigIntToFieldElement
//   FieldElementToEncodedBytes
//   BigIntToEncodedBytes
//   FieldElementToBigInt
//   EncodedBytesToFieldElement
func TestConversion(t *testing.T) {
	for _, vector := range testConversionVectors() {
		// Test encoding to FE --> bytes.
		feFB := EncodedBytesToFieldElement(vector.bIn)
		feTB := FieldElementToEncodedBytes(feFB)
		assert.Equal(t, vector.bIn, feTB)

		// Test encoding to big int --> FE --> bytes.
		big := EncodedBytesToBigInt(vector.bIn)
		fe := BigIntToFieldElement(big)
		b := FieldElementToEncodedBytes(fe)
		assert.Equal(t, vector.bIn, b)

		// Test encoding to big int --> bytes.
		b = BigIntToEncodedBytes(big)
		assert.Equal(t, vector.bIn, b)

		// Test encoding FE --> big int --> bytes.
		feBig := FieldElementToBigInt(fe)
		b = BigIntToEncodedBytes(feBig)
		assert.Equal(t, vector.bIn, b)

		// Test python lib bytes --> int vs our results.
		args := []string{"testdata/decodeint.py", hex.EncodeToString(vector.bIn[:])}
		pyNumStr, _ := exec.Command("python", args...).Output()
		stripped := strings.TrimSpace(string(pyNumStr))
		assert.Equal(t, stripped, big.String())

		// Test python lib int --> bytes versus our results.
		args = []string{"testdata/encodeint.py", big.String()}
		pyHexStr, _ := exec.Command("python", args...).Output()
		stripped = strings.TrimSpace(string(pyHexStr))
		assert.Equal(t, hex.EncodeToString(vector.bIn[:]), string(stripped))
	}
}

func testPointConversionVectors() []ConversionVector {
	r := rand.New(rand.NewSource(54321))

	numCvs := 50
	cvs := make([]ConversionVector, numCvs, numCvs)
	for i := 0; i < numCvs; i++ {
		bIn := new([32]byte)
		for j := 0; j < fieldIntSize; j++ {
			randByte := r.Intn(255)
			bIn[j] = uint8(randByte)
		}

		cvs[i] = ConversionVector{bIn}
		r.Seed(int64(i) + 54321)
	}

	return cvs
}

// Tested functions:
//   BigIntPointToEncodedBytes
//   extendedToBigAffine
//   EncodedBytesToBigIntPoint
func TestPointConversion(t *testing.T) {
	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()

	for _, vector := range testPointConversionVectors() {
		x, y, err := curve.EncodedBytesToBigIntPoint(vector.bIn)
		// The random point wasn't on the curve.
		if err != nil {
			continue
		}

		yB := BigIntPointToEncodedBytes(x, y)
		assert.Equal(t, vector.bIn, yB)

		// Test python lib bytes --> point vs our results.
		args := []string{"testdata/decodepoint.py", hex.EncodeToString(vector.bIn[:])}
		pyNumStr, _ := exec.Command("python", args...).Output()
		stripped := strings.TrimSpace(string(pyNumStr))
		var buffer bytes.Buffer
		buffer.WriteString(x.String())
		buffer.WriteString(",")
		buffer.WriteString(y.String())
		localStr := buffer.String()
		assert.Equal(t, localStr, stripped)

		// Test python lib point --> bytes versus our results.
		args = []string{"testdata/encodepoint.py", x.String(), y.String()}
		pyHexStr, _ := exec.Command("python", args...).Output()
		stripped = strings.TrimSpace(string(pyHexStr))
		assert.Equal(t, hex.EncodeToString(vector.bIn[:]), string(stripped))
	}
}
