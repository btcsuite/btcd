package btcec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
)

const (
	schnorrPublicKeyLen = 32
	schnorrMessageLen   = 32
	schnorrSignatureLen = 64
	schnorrAuxLen       = 32

	// BIP340Challenge is sha256("BIP0340/challenge")
	BIP340Challenge = "7bb52d7a9fef58323eb1bf7a407db382d2f3f2d81bb1224f49fe518f6d48d37ctag"

	// BIP340Aux is sha256("BIP0340/aux")
	BIP340Aux = "f1ef4e5ec063cada6d94cafa9d987ea069265839ecc11f972d77a52ed8c1cc90tag"

	// BIP340Nonce is sha256("BIP0340/nonce")
	BIP340Nonce = "07497734a79bcb355b9b8c7d034f121cf434d73ef72dda19870061fb52bfeb2ftag"
)

// SchnorrPublicKey is the x-coordinate of a public key that can be used with schnorr.
type SchnorrPublicKey struct{ x *big.Int }

// Serialize returns x(P) in a 32 byte slice.
func (p *SchnorrPublicKey) Serialize() []byte {
	return p.x.Bytes()
}

// ParseSchnorrPubKey parses a public key, verifies it is valid, and returns the schnorr key.
func ParseSchnorrPubKey(pubKeyStr []byte) (*SchnorrPublicKey, error) {
	if len(pubKeyStr) == 0 {
		return nil, errors.New("pubkey string is empty")
	}

	switch len(pubKeyStr) {
	// If key is 33 bytes, check if it using the compressed encoding.
	// If so, then it is safe to drop the first byte.
	case PubKeyBytesLenCompressed:
		format := pubKeyStr[0]
		format &= ^byte(0x1)

		if format != pubkeyCompressed {
			return nil, fmt.Errorf("invalid magic in compressed "+
				"pubkey string: %d", pubKeyStr[0])
		}

		// Drop first byte.
		pubKeyStr = pubKeyStr[1:]
	case schnorrPublicKeyLen:
	default:
		return nil, fmt.Errorf("pubkey length invalid : got %d want %d", len(pubKeyStr), schnorrPublicKeyLen)
	}

	x := new(big.Int)
	x.SetBytes(pubKeyStr)

	Px, Py, err := liftX(pubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey")
	}

	if !S256().IsOnCurve(Px, Py) {
		return nil, fmt.Errorf("pubkey is not on curve")
	}

	return &SchnorrPublicKey{x: x}, nil
}

// SchnorrSign signs a message using the schnorr signature algorithm scheme outlined in BIP340.
// Message must be 32 bytes.
// An optional auxiliary random data byte slice can be provided that must be 32 bytes.
func (p *PrivateKey) SchnorrSign(message, aux []byte) ([64]byte, error) {
	return schnorrSign(p.D.Bytes(), message, aux)
}

// SchnorrVerify verifies a schnorr signature.
// Message, public key, and signature must be 32 bytes.
func SchnorrVerify(msg, publicKey, signature []byte) (bool, error) {
	return schnorrVerify(msg, publicKey, signature)
}

// hasEvenY checks that the Point P's y-coordinate is even.
func hasEvenY(Py *big.Int) bool {
	// P cannot be infinite.
	if Py.Cmp(big.NewInt(0)) == 0 {
		return false
	}

	// y(P) mod 2 == 0
	return new(big.Int).Mod(Py, two).Cmp(zero) == 0
}

// schnorrSign implements BIP340's default signing algorithm.
func schnorrSign(privKey, msg []byte, a []byte) (sig [64]byte, err error) {
	// Message must be 32 bytes.
	if l := len(msg); l != schnorrMessageLen {
		return sig, fmt.Errorf("message is not 32 bytes : got %d, want %d", l, schnorrMessageLen)
	}

	// Auxiliary data must either be 32 bytes or 0.
	if l := len(a); l != schnorrAuxLen && l != 0 {
		return sig, fmt.Errorf("auxillary data is not 32 bytes : got %d, want %d", l, schnorrMessageLen)
	}

	curve := S256()

	// n is the curve order.
	n := curve.N

	e := new(big.Int)

	// Nonce k.
	k := new(big.Int)

	s := new(big.Int)

	// d is the private key integer.
	d := new(big.Int).SetBytes(privKey)

	// d cannot be zero or greater than the curve order.
	if d.Cmp(one) < 0 || d.Cmp(new(big.Int).Sub(n, one)) > 0 {
		return sig, errors.New("private key must be an integer in the range 1..n-1")
	}

	// P = d*G
	Px, Py := curve.ScalarBaseMult(d.Bytes())

	// If y(p) is not even, then negate d.
	if !hasEvenY(Py) {
		// d = n - d
		d = d.Sub(n, d)
	}

	// t is the byte-wise xor of d and the taggedHash(BIP0340/aux | a)
	t0 := new(big.Int).SetBytes(taggedHash(BIP340Aux, a))
	t := t0.Xor(d, t0)

	// Get a deterministic nonce k.
	{
		m := make([]byte, 96)
		copy(m[:32], t.Bytes())
		copy(m[32:64], Px.Bytes())
		copy(m[64:], msg)

		// rand = sha256(BIP0340/nonce || (t || P || m))
		k.SetBytes(taggedHash(BIP340Nonce, m))

		// k = rand mod n
		k.Mod(k, n)

		// k cannot be zero.
		if k.Cmp(zero) == 0 {
			return sig, errors.New("k is 0")
		}
	}

	// R = k*G
	Rx, Ry := curve.ScalarBaseMult(k.Bytes())

	// Negate k if y(R) is odd.
	if !hasEvenY(Ry) {
		k.Sub(n, k)
	}

	// e = int(hashBIP0340/challenge(R || P || m)) mod n
	{
		m := make([]byte, 96)
		copy(m[:32], Rx.Bytes())
		copy(m[32:64], Px.Bytes())
		copy(m[64:], msg)
		e.SetBytes(taggedHash(BIP340Challenge, m))
		e.Mod(e, n)
	}

	// (k + ed) mod n
	s.Mul(e, d)
	s.Add(k, s)
	s.Mod(s, n)

	// Signature is (x(R), s).
	copy(sig[:32], Rx.Bytes())
	copy(sig[32:], s.Bytes())

	// Verify signature before returning.
	if verify, err := schnorrVerify(msg, Px.Bytes(), sig[:]); !verify || err != nil {
		return sig, errors.New("cannot create signature")
	}

	return sig, nil
}

func liftX(key []byte) (*big.Int, *big.Int, error) {
	x := new(big.Int).SetBytes(key)

	// p is field size.
	p := S256().P

	if x.Cmp(p) >= 0 {
		return nil, nil, errors.New("inf")
	}

	// c = x^3 + 7 mod P.
	c := new(big.Int)
	c.Exp(x, three, p)
	c.Add(c, seven)
	c.Mod(c, p)

	// y = c^((p+1)/4) mod P.
	y := new(big.Int)
	y.Add(p, one)
	y.Div(y, four)
	y.Exp(c, y, p)

	if new(big.Int).And(y, one).Cmp(zero) != 0 {
		y.Sub(p, y)
	}

	return x, y, nil
}

func schnorrVerify(msg, publicKey, signature []byte) (bool, error) {
	if l := len(msg); l != schnorrMessageLen {
		return false, fmt.Errorf("message is not 32 bytes : got %d, want %d", l, schnorrMessageLen)
	}

	if l := len(publicKey); l != schnorrPublicKeyLen {
		return false, fmt.Errorf("public key is not 32 bytes : got %d, want %d", l, schnorrPublicKeyLen)
	}

	if l := len(signature); l != schnorrSignatureLen {
		return false, fmt.Errorf("signature is not 32 bytes : got %d, want %d", l, schnorrSignatureLen)
	}

	curve := S256()

	// n is the curve order.
	n := curve.N

	p := curve.P

	r := new(big.Int)
	s := new(big.Int)

	e := new(big.Int)

	// Get Point P from the x-coordinate.
	Px, Py, err := liftX(publicKey[:])
	if err != nil {
		return false, err
	}

	// Check that P is on the curve.
	if !curve.IsOnCurve(Px, Py) {
		return false, errors.New("public key is not on the curve")
	}

	r.SetBytes(signature[:32])
	s.SetBytes(signature[32:])

	// Fail if s >= n
	if s.Cmp(n) >= 0 {
		return false, nil
	}

	// Fail if r >= p
	if r.Cmp(p) >= 0 {
		return false, nil
	}

	// e = sha256(hashBIP0340/challenge || r || P || m) mod n.
	{
		m := make([]byte, 96)
		copy(m[:32], signature[:32])
		copy(m[32:64], publicKey)
		copy(m[64:], msg)
		e.SetBytes(taggedHash(BIP340Challenge, m))
		e.Mod(e, n)
	}

	// s*G
	sGx, sGy := curve.ScalarBaseMult(s.Bytes())

	// (n - e)*P
	ePx, ePy := curve.ScalarMult(Px, Py, new(big.Int).Sub(n, e).Bytes())

	// R = s*G + (N-e)*P
	Rx, Ry := curve.Add(sGx, sGy, ePx, ePy)

	// Fail if R is at infinity.
	if Rx.Cmp(zero) == 0 || Ry.Cmp(zero) == 0 {
		return false, nil
	}

	// Fail if y(R) is not even
	if !hasEvenY(Ry) {
		return false, nil
	}

	// Fail if x(R) != r
	if Rx.Cmp(r) != 0 {
		return false, nil
	}

	return true, nil
}

func taggedHash(tag string, msg []byte) []byte {
	tagHash, _ := hex.DecodeString(tag)

	tagLen := len(tagHash)
	msgLen := len(msg)

	m := make([]byte, tagLen*2+msgLen)
	copy(m[:tagLen], tagHash)
	copy(m[tagLen:tagLen*2], tagHash)
	copy(m[tagLen*2:], msg)
	h := sha256.Sum256(m)
	return h[:]
}
