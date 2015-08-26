// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"math/big"

	"github.com/decred/ed25519/edwards25519"
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
	fieldIntSize          = 32
	fieldElementSize      = 10
	fieldElementBytesSize = 40
)

// feZero is the field element representation of zero.
var feZero = edwards25519.FieldElement{
	0, 0, 0, 0, 0,
	0, 0, 0, 0, 0,
}

// feOne is the field element representation of one. This is
// also the neutral (null) element.
var feOne = edwards25519.FieldElement{
	1, 0, 0, 0, 0,
	0, 0, 0, 0, 0,
}

// feTwo is the field element representation of one.
var feTwo = edwards25519.FieldElement{
	1, 0, 0, 0, 0,
	0, 0, 0, 0, 0,
}

// feThree is the field element representation of one.
var feThree = edwards25519.FieldElement{
	3, 0, 0, 0, 0,
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

// feSqrtM1 is the field element representation of M^(1/2).
var feSqrtM1 = edwards25519.FieldElement{
	-32595792, -7943725, 9377950, 3500415, 12389472,
	-272473, -25146209, -2005654, 326686, 11406482,
}

// feI is the field element representation of I.
var feI = edwards25519.FieldElement{
	-32595792, -7943725, 9377950, 3500415, 12389472,
	-272473, -25146209, -2005654, 326686, 11406482,
}

// feExtBasePoint is the base point of the curve represented
// in projective extended format with field elements.
var feExtBasePoint = edwards25519.ExtendedGroupElement{
	X: edwards25519.FieldElement{
		25485296, 5318399, 8791791, -8299916, -14349720,
		6939349, -3324311, -7717049, 7287234, -6577708,
	},
	Y: edwards25519.FieldElement{
		-758052, -1832720, 13046421, -4857925, 6576754,
		14371947, -13139572, 6845540, -2198883, -4003719,
	},
	Z: edwards25519.FieldElement{
		-947565, 6097708, -469190, 10704810, -8556274,
		-15589498, -16424464, -16608899, 14028613, -5004649,
	},
	T: edwards25519.FieldElement{
		6966464, -2456167, 7033433, 6781840, 28785542,
		12262365, -2659449, 13959020, -21013759, -5262166,
	},
}
