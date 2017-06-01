// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"math/big"

	"github.com/agl/ed25519/edwards25519"
)

var (
	// zero through eight are big.Int numbers useful in
	// elliptical curve math.
	zero  = new(big.Int).SetInt64(0)
	one   = new(big.Int).SetInt64(1)
	two   = new(big.Int).SetInt64(2)
	three = new(big.Int).SetInt64(3)
	four  = new(big.Int).SetInt64(4)
	eight = new(big.Int).SetInt64(8)

	// fieldIntSize is the size of a field element encoded
	// as bytes.
	fieldIntSize = 32
)

// feOne is the field element representation of one. This is
// also the neutral (null) element.
var feOne = edwards25519.FieldElement{
	1, 0, 0, 0, 0,
	0, 0, 0, 0, 0,
}

// feA is the field element representation of one.
var feA = edwards25519.FieldElement{
	486662, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

// fed is the field element representation of D.
var fed = edwards25519.FieldElement{
	-10913610, 13857413, -15372611, 6949391, 114729,
	-8787816, -6275908, -3247719, -18696448, -12055116,
}

// fed2 is the field element representation of D^2.
var fed2 = edwards25519.FieldElement{
	-21827239, -5839606, -30745221, 13898782, 229458,
	15978800, -12551817, -6495438, 29715968, 9444199,
}

// feI is the field element representation of I.
var feI = edwards25519.FieldElement{
	-32595792, -7943725, 9377950, 3500415, 12389472,
	-272473, -25146209, -2005654, 326686, 11406482,
}
