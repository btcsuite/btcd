package btcec

import (
	"crypto/rand"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

var (
	// c is sqrt(-3) (mod p)
	c FieldVal

	cBytes = [32]byte{
		0x0a, 0x2d, 0x2b, 0xa9, 0x35, 0x07, 0xf1, 0xdf,
		0x23, 0x37, 0x70, 0xc2, 0xa7, 0x97, 0x96, 0x2c,
		0xc6, 0x1f, 0x6d, 0x15, 0xda, 0x14, 0xec, 0xd4,
		0x7d, 0x8d, 0x27, 0xae, 0x1c, 0xd5, 0xf8, 0x52,
	}

	ellswiftTag = []byte("bip324_ellswift_xonly_ecdh")

	// ErrPointNotOnCurve is returned when we're unable to find a point on the
	// curve.
	ErrPointNotOnCurve = fmt.Errorf("point does not exist on secp256k1 curve")
)

func init() {
	c.SetByteSlice(cBytes[:])
}

// XSwiftEC() takes two field elements (u, t) and gives us an x-coordinate that
// is on the secp256k1 curve. This is used to take an ElligatorSwift-encoded
// public key (u, t) and return the point on the curve it maps to.
// TODO: Rewrite these so to avoid new(FieldVal).Add(...) usage?
// NOTE: u, t MUST be normalized. The result x is normalized.
func XSwiftEC(u, t *FieldVal) *FieldVal {
	// 1. Let u' = u if u != 0, else = 1
	if u.IsZero() {
		u.SetInt(1)
	}

	// 2. Let t' = t if t != 0, else 1
	if t.IsZero() {
		t.SetInt(1)
	}

	// 3. Let t'' = t' if g(u') != -(t'^2); t'' = 2t' otherwise
	// g(x) = x^3 + ax + b, a = 0, b = 7

	// Calculate g(u').
	gu := new(FieldVal).SquareVal(u).Mul(u).AddInt(7).Normalize()

	// Calculate the right-hand side of the equation (-t'^2)
	rhs := new(FieldVal).SquareVal(t).Negate(1).Normalize()

	if gu.Equals(rhs) {
		// t'' = 2t'
		t = t.Add(t)
	}

	// 4. X = (u'^3 + b - t''^2) / (2t'')
	tSquared := new(FieldVal).SquareVal(t).Negate(1)
	xNum := new(FieldVal).SquareVal(u).Mul(u).AddInt(7).Add(tSquared)
	xDenom := new(FieldVal).Add2(t, t).Inverse()
	x := xNum.Mul(xDenom)

	// 5. Y = (X+t'') / (u' * c)
	yNum := new(FieldVal).Add2(x, t)
	yDenom := new(FieldVal).Mul2(u, &c).Inverse()
	y := yNum.Mul(yDenom)

	// 6. Return the first x in (u'+4Y^2, -X/2Y - u'/2, X/2Y - u'/2) for which
	//    x^3 + b is square.

	//    6a. Calculate u' +4Y^2 and determine if x^3+7 is square.
	ySqr := new(FieldVal).Add(y).Mul(y)
	quadYSqr := new(FieldVal).Add(ySqr).MulInt(4)
	firstX := new(FieldVal).Add(u).Add(quadYSqr)

	firstXCurve := new(FieldVal).Add(firstX).Square().Mul(firstX).AddInt(7)

	// Now determine if firstXCurve is square (on the curve).
	if new(FieldVal).SquareRootVal(firstXCurve) {
		return firstX.Normalize()
	}

	//    6b. Calculate -X/2Y - u'/2 and determine if x^3 + 7 is square
	doubleYInv := new(FieldVal).Add(y).Add(y).Inverse()
	xDivDoubleYInv := new(FieldVal).Add(x).Mul(doubleYInv)
	negXDivDoubleYInv := new(FieldVal).Add(xDivDoubleYInv).Negate(1)
	invTwo := new(FieldVal).AddInt(2).Inverse()
	negUDivTwo := new(FieldVal).Add(u).Mul(invTwo).Negate(1)
	secondX := new(FieldVal).Add(negXDivDoubleYInv).Add(negUDivTwo)

	secondXCurve := new(FieldVal).Add(secondX).Square().Mul(secondX).AddInt(7)

	// Now determine if secondXCurve is square.
	if new(FieldVal).SquareRootVal(secondXCurve) {
		return secondX.Normalize()
	}

	//    6c. Calculate X/2Y -u'/2 and determine if x^3 + 7 is square
	thirdX := new(FieldVal).Add(xDivDoubleYInv).Add(negUDivTwo)

	thirdXCurve := new(FieldVal).Add(thirdX).Square().Mul(thirdX).AddInt(7)

	// Now determine if thirdXCurve is square.
	if new(FieldVal).SquareRootVal(thirdXCurve) {
		return thirdX.Normalize()
	}

	// Should have found a square above.
	panic("unreachable - no calculated x-values were square")
}

// XSwiftECInv takes two field elements (u, x) (where x is on the curve) and
// returns a field element t. This is used to take a random field element u and
// a point on the curve and return a field element t where (u, t) forms the
// ElligatorSwift encoding.
// TODO: Rewrite these so to avoid new(FieldVal).Add(...) usage?
// NOTE: u, x MUST be normalized. The result `t` is normalized.
func XSwiftECInv(u, x *FieldVal, caseNum int) *FieldVal {
	v := new(FieldVal)
	s := new(FieldVal)
	twoInv := new(FieldVal).AddInt(2).Inverse()

	if caseNum&2 == 0 {
		// If lift_x(-x-u) succeeds, return None
		if _, found := liftX(new(FieldVal).Add(x).Add(u).Negate(2)); found {
			return nil
		}

		// Let v = x
		v.Add(x)

		// Let s = -(u^3+7)/(u^2 + uv + v^2)
		uSqr := new(FieldVal).Add(u).Square()
		vSqr := new(FieldVal).Add(v).Square()
		sDenom := new(FieldVal).Add(u).Mul(v).Add(uSqr).Add(vSqr)
		sNum := new(FieldVal).Add(uSqr).Mul(u).AddInt(7)

		s = sDenom.Inverse().Mul(sNum).Negate(1)
	} else {
		// Let s = x - u
		negU := new(FieldVal).Add(u).Negate(1)
		s.Add(x).Add(negU).Normalize()

		// If s = 0, return None
		if s.IsZero() {
			return nil
		}

		// Let r be the square root of -s(4(u^3 + 7) + 3u^2s)
		uSqr := new(FieldVal).Add(u).Square()
		lhs := new(FieldVal).Add(uSqr).Mul(u).AddInt(7).MulInt(4)
		rhs := new(FieldVal).Add(uSqr).MulInt(3).Mul(s)

		// Add the two terms together and multiply by -s.
		lhs.Add(rhs).Normalize().Mul(s).Negate(1)

		r := new(FieldVal)
		if !r.SquareRootVal(lhs) {
			// If no square root was found, return None.
			return nil
		}

		if caseNum&1 == 1 && r.Normalize().IsZero() {
			// If case & 1 = 1 and r = 0, return None.
			return nil
		}

		// Let v = (r/s - u)/2
		sInv := new(FieldVal).Add(s).Inverse()
		uNeg := new(FieldVal).Add(u).Negate(1)

		v.Add(r).Mul(sInv).Add(uNeg).Mul(twoInv)
	}

	w := new(FieldVal)

	if !w.SquareRootVal(s) {
		// If no square root was found, return None.
		return nil
	}

	switch caseNum & 5 {
	case 0:
		// If case & 5 = 0, return -w(u(1-c)/2 + v)
		oneMinusC := new(FieldVal).Add(&c).Negate(1).AddInt(1)
		t := new(FieldVal).Add(u).Mul(oneMinusC).Mul(twoInv).Add(v).Mul(w).
			Negate(1).Normalize()

		return t

	case 1:
		// If case & 5 = 1, return w(u(1+c)/2 + v)
		onePlusC := new(FieldVal).Add(&c).AddInt(1)
		t := new(FieldVal).Add(u).Mul(onePlusC).Mul(twoInv).Add(v).Mul(w).
			Normalize()

		return t

	case 4:
		// If case & 5 = 4, return w(u(1-c)/2 + v)
		oneMinusC := new(FieldVal).Add(&c).Negate(1).AddInt(1)
		t := new(FieldVal).Add(u).Mul(oneMinusC).Mul(twoInv).Add(v).Mul(w).
			Normalize()

		return t

	case 5:
		// If case & 5 = 5, return -w(u(1+c)/2 + v)
		onePlusC := new(FieldVal).Add(&c).AddInt(1)
		t := new(FieldVal).Add(u).Mul(onePlusC).Mul(twoInv).Add(v).Mul(w).
			Negate(1).Normalize()

		return t
	}

	panic("should not reach here")
}

// XElligatorSwift takes the x-coordinate of a point on secp256k1 and generates
// ElligatorSwift encoding of that point composed of two field elements (u, t).
// NOTE: x MUST be normalized. The return values u, t are normalized.
func XElligatorSwift(x *FieldVal) (*FieldVal, *FieldVal, error) {
	// We'll choose a random `u` value and a random case so that we can
	// generate a `t` value.
	for {
		// Choose random u value.
		var randUBytes [32]byte
		_, err := rand.Read(randUBytes[:])
		if err != nil {
			return nil, nil, err
		}

		u := new(FieldVal)
		overflow := u.SetBytes(&randUBytes)
		if overflow == 1 {
			u.Normalize()
		}

		// Choose a random case in the interval [0, 7]
		var randCaseByte [1]byte
		_, err = rand.Read(randCaseByte[:])
		if err != nil {
			return nil, nil, err
		}

		caseNum := randCaseByte[0] & 7

		// Find t, if none is found, continue with the loop.
		t := XSwiftECInv(u, x, int(caseNum))
		if t != nil {
			return u, t, nil
		}
	}
}

// EllswiftCreate generates a random private key and returns that along with
// the ElligatorSwift encoding of its corresponding public key.
func EllswiftCreate() (*PrivateKey, [64]byte, error) {
	var randPrivKeyBytes [64]byte

	// Generate a random private key
	_, err := rand.Read(randPrivKeyBytes[:])
	if err != nil {
		return nil, [64]byte{}, err
	}

	privKey, _ := PrivKeyFromBytes(randPrivKeyBytes[:])

	// Fetch the x-coordinate of the public key.
	x := getXCoord(privKey)

	// Get the ElligatorSwift encoding of the public key.
	u, t, err := XElligatorSwift(x)
	if err != nil {
		return nil, [64]byte{}, err
	}

	uBytes := u.Bytes()
	tBytes := t.Bytes()

	// ellswift_pub = bytes(u) || bytes(t), its encoding as 64 bytes
	var ellswiftPub [64]byte
	copy(ellswiftPub[0:32], (*uBytes)[:])
	copy(ellswiftPub[32:64], (*tBytes)[:])

	// Return (priv, ellswift_pub)
	return privKey, ellswiftPub, nil
}

// EllswiftECDHXOnly takes the ElligatorSwift-encoded public key of a
// counter-party and performs ECDH with our private key.
func EllswiftECDHXOnly(ellswiftTheirs [64]byte, privKey *PrivateKey) ([32]byte,
	error) {

	// Let u = int(ellswift_theirs[:32]) mod p.
	// Let t = int(ellswift_theirs[32:]) mod p.
	uBytesTheirs := ellswiftTheirs[0:32]
	tBytesTheirs := ellswiftTheirs[32:64]

	var uTheirs FieldVal
	overflow := uTheirs.SetByteSlice(uBytesTheirs[:])
	if overflow {
		uTheirs.Normalize()
	}

	var tTheirs FieldVal
	overflow = tTheirs.SetByteSlice(tBytesTheirs[:])
	if overflow {
		tTheirs.Normalize()
	}

	// Calculate bytes(x(priv⋅lift_x(XSwiftEC(u, t))))
	xTheirs := XSwiftEC(&uTheirs, &tTheirs)
	pubKey, found := liftX(xTheirs)
	if !found {
		return [32]byte{}, ErrPointNotOnCurve
	}

	var pubJacobian JacobianPoint
	pubKey.AsJacobian(&pubJacobian)

	var sharedPoint JacobianPoint
	ScalarMultNonConst(&privKey.Key, &pubJacobian, &sharedPoint)
	sharedPoint.ToAffine()

	return *sharedPoint.X.Bytes(), nil
}

// getXCoord fetches the corresponding public key's x-coordinate given a
// private key.
func getXCoord(privKey *PrivateKey) *FieldVal {
	var result JacobianPoint
	ScalarBaseMultNonConst(&privKey.Key, &result)
	result.ToAffine()
	return &result.X
}

// liftX returns the point P with x-coordinate `x` and even y-coordinate. If a
// point exists on the curve, it returns true and false otherwise.
// TODO: Use quadratic residue formula instead (see: BIP340)?
func liftX(x *FieldVal) (*PublicKey, bool) {
	ySqr := new(FieldVal).Add(x).Square().Mul(x).AddInt(7)

	y := new(FieldVal)
	if !y.SquareRootVal(ySqr) {
		// If we've reached here, the point does not exist on the curve.
		return nil, false
	}

	if !y.Normalize().IsOdd() {
		return NewPublicKey(x, y), true
	}

	// Negate y if it's odd.
	if !y.Negate(1).Normalize().IsOdd() {
		return NewPublicKey(x, y), true
	}

	return nil, false
}

// V2Ecdh performs x-only ecdh and returns a shared secret composed of a tagged
// hash which itself is composed of two ElligatorSwift-encoded public keys and
// the x-only ecdh point.
func V2Ecdh(priv *PrivateKey, ellswiftTheirs, ellswiftOurs [64]byte,
	initiating bool) (*chainhash.Hash, error) {

	ecdhPoint, err := EllswiftECDHXOnly(ellswiftTheirs, priv)
	if err != nil {
		return nil, err
	}

	if initiating {
		// Initiating, place our public key encoding first.
		var msg []byte
		msg = append(msg, ellswiftOurs[:]...)
		msg = append(msg, ellswiftTheirs[:]...)
		msg = append(msg, ecdhPoint[:]...)
		return chainhash.TaggedHash(ellswiftTag, msg), nil
	}

	var msg []byte
	msg = append(msg, ellswiftTheirs[:]...)
	msg = append(msg, ellswiftOurs[:]...)
	msg = append(msg, ecdhPoint[:]...)
	return chainhash.TaggedHash(ellswiftTag, msg), nil
}
