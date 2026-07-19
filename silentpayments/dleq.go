package silentpayments

import (
	"bytes"
	crand "crypto/rand"
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

var (
	// TagBIP0374Aux is the BIP-0374 tag for auxiliary randomness.
	TagBIP0374Aux = []byte("BIP0374/aux")

	// TagBIP0374Nonce is the BIP-0374 tag for a nonce.
	TagBIP0374Nonce = []byte("BIP0374/nonce")

	// TagBIP0374Challenge is the BIP-0374 tag for a challenge.
	TagBIP0374Challenge = []byte("BIP0374/challenge")

	// ErrPrivateKeyZero is returned if the private key is zero.
	ErrPrivateKeyZero = errors.New("private key is zero")
)

// DLEQProof generates a DLEQ proof according to BIP-0374 for the given private
// key a, public key B, generator point G, auxiliary randomness r, and optional
// message m.
func DLEQProof(a *btcec.PrivateKey, B, G *btcec.PublicKey, r [32]byte,
	m *[32]byte) ([64]byte, error) {

	// Fail if a = 0 or a ≥ n.
	if a.Key.IsZero() {
		return [64]byte{}, ErrPrivateKeyZero
	}

	// Fail if is_infinite(B).
	if !B.IsOnCurve() {
		return [64]byte{}, secp.ErrPubKeyNotOnCurve
	}

	// Let A = a⋅G.
	A := ScalarMult(a.Key, G)

	// Let C = a⋅B.
	C := ScalarMult(a.Key, B)

	// Let t be the byte-wise xor of bytes(32, a) and hash_BIP0374/aux(r).
	aBytes := a.Key.Bytes()
	t := chainhash.TaggedHash(TagBIP0374Aux, r[:])
	for i := 0; i < len(t); i++ {
		t[i] ^= aBytes[i]
	}

	// Let m' = m if m is provided, otherwise an empty byte array.
	var mPrime []byte
	if m != nil {
		mPrime = m[:]
	}

	// Let rand = hash_BIP0374/nonce(t || cbytes(A) || cbytes(C) || m').
	rand := chainhash.TaggedHash(
		TagBIP0374Nonce, t[:], A.SerializeCompressed(),
		C.SerializeCompressed(), mPrime,
	)

	// Let k = int(rand) mod n.
	var k secp.ModNScalar
	_ = k.SetByteSlice(rand[:])

	// Fail if k = 0.
	if k.IsZero() {
		return [64]byte{}, ErrPrivateKeyZero
	}

	// Let R1 = k⋅G.
	R1 := ScalarMult(k, G)

	// Let R2 = k⋅B.
	R2 := ScalarMult(k, B)

	// Let m' = m if m is provided, otherwise an empty byte array.
	// Let e = int(hash_BIP0374/challenge(
	//    cbytes(A) || cbytes(B) || cbytes(C) || cbytes(G) || cbytes(R1) ||
	//    cbytes(R2) || m')
	// ).
	//
	// Spec: e is the raw 256-bit hash, NOT reduced mod n. It is serialized
	// verbatim as the first half of the proof, so we keep the raw hash
	// bytes here and only reduce a copy for the scalar math below.
	eHash := challengeHash(A, B, C, G, R1, R2, m)
	e := new(secp.ModNScalar)
	_ = e.SetByteSlice(eHash[:])

	// Let s = (k + e⋅a) mod n.
	s := new(btcec.ModNScalar)
	s.Mul2(e, &a.Key).Add(&k)
	sBytes := s.Bytes()
	k.Zero()

	// Let proof = bytes(32, e) || bytes(32, s).
	var proof [64]byte
	copy(proof[:32], eHash[:])
	copy(proof[32:], sBytes[:])

	// If VerifyProof(A, B, C, proof) (see below) returns failure, abort.
	if !DLEQVerify(A, B, C, G, proof, m) {
		return [64]byte{}, errors.New("proof verification failed")
	}

	// Return the proof.
	return proof, nil
}

// DLEQVerify verifies a DLEQ proof according to BIP-0374 for the given public
// keys A, B, C, G, proof, and optional message m.
func DLEQVerify(A, B, C, G *btcec.PublicKey, proof [64]byte, m *[32]byte) bool {
	// Fail if any of is_infinite(A), is_infinite(B), is_infinite(C),
	// is_infinite(G).
	if !A.IsOnCurve() || !B.IsOnCurve() || !C.IsOnCurve() ||
		!G.IsOnCurve() {

		return false
	}

	// Let e = int(proof[0:32]).
	e := new(secp.ModNScalar)
	_ = e.SetByteSlice(proof[:32])

	// Let s = int(proof[32:64]); fail if s ≥ n.
	s := new(secp.ModNScalar)
	overflow := s.SetByteSlice(proof[32:])
	if overflow {
		return false
	}

	// Let R1 = s⋅G - e⋅A.
	eA := ScalarMult(*e, A)
	negEA := Negate(eA)
	sG := ScalarMult(*s, G)
	R1 := Add(sG, negEA)

	// Fail if is_infinite(R1).
	if !R1.IsOnCurve() {
		return false
	}

	// Let R2 = s⋅B - e⋅C.
	eC := ScalarMult(*e, C)
	negEC := Negate(eC)
	sB := ScalarMult(*s, B)
	R2 := Add(sB, negEC)

	// Fail if is_infinite(R2).
	if !R2.IsOnCurve() {
		return false
	}

	// Let m' = m if m is provided, otherwise an empty byte array.
	// Fail if e ≠ int(hash_BIP0374/challenge(
	//    cbytes(A) || cbytes(B) || cbytes(C) || cbytes(G) || cbytes(R1) ||
	//    cbytes(R2) || m')
	// ).
	//
	// Spec: e is compared as the raw 256-bit value, so we compare the raw
	// challenge-hash bytes against the proof, not the reduced scalar.
	expectedHash := challengeHash(A, B, C, G, R1, R2, m)
	if !bytes.Equal(proof[:32], expectedHash[:]) {
		return false
	}

	// Return success iff no failure occurred before reaching this point.
	return true
}

// challengeHash computes the raw BIP-0374 challenge hash for the given public
// keys A, B, C, G, R1, R2, and optional message m. Per the spec the challenge
// e is the unreduced 256-bit value int(hash(...)); it is serialized into the
// proof and compared verbatim, so callers that need the wire-format e must use
// the raw bytes rather than a scalar reduced mod n.
func challengeHash(A, B, C, G, R1, R2 *btcec.PublicKey,
	m *[32]byte) *chainhash.Hash {

	// Let m' = m if m is provided, otherwise an empty byte array.
	var mPrime []byte
	if m != nil {
		mPrime = m[:]
	}

	// Let e = int(hash_BIP0374/challenge(
	//    cbytes(A) || cbytes(B) || cbytes(C) || cbytes(G) || cbytes(R1) ||
	//    cbytes(R2) || m')
	// ).
	return chainhash.TaggedHash(
		TagBIP0374Challenge, A.SerializeCompressed(),
		B.SerializeCompressed(), C.SerializeCompressed(),
		G.SerializeCompressed(), R1.SerializeCompressed(),
		R2.SerializeCompressed(), mPrime,
	)
}

// DLEQChallenge generates a DLEQ challenge scalar according to BIP-0374 for the
// given public keys A, B, C, G, R1, R2, and optional message m. The returned
// scalar is reduced mod n for use in scalar arithmetic; note that the proof
// serializes the raw challenge hash (see challengeHash), not this value.
func DLEQChallenge(A, B, C, G, R1, R2 *btcec.PublicKey,
	m *[32]byte) *secp.ModNScalar {

	eHash := challengeHash(A, B, C, G, R1, R2, m)
	e := new(secp.ModNScalar)
	_ = e.SetByteSlice(eHash[:])

	return e
}

// CreateShare creates a silent payment ECDH share according to BIP-0374,
// including the corresponding DLEQ proof for it. The returned share is the raw
// ECDH point C = privateKey·scanKey; the BIP-0352 input_hash tweak is NOT
// applied here and must be added by the caller when aggregating shares.
func CreateShare(privateKey *btcec.PrivateKey,
	scanKey *btcec.PublicKey) (*btcec.PublicKey, [64]byte, error) {

	// Generate a random 32-byte value.
	var r [32]byte
	if _, err := crand.Read(r[:]); err != nil {
		return nil, [64]byte{}, err
	}

	// The share is the ECDH point C = a·B_scan.
	share := ScalarMult(privateKey.Key, scanKey)
	G := Generator()

	proof, err := DLEQProof(privateKey, scanKey, G, r, nil)
	if err != nil {
		return nil, [64]byte{}, err
	}

	return share, proof, nil
}
