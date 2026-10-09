package frost

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/internal"
	"github.com/btcsuite/btcd/chainhash/v2"
)

// MAX_PARTICIPANTS is the upper bound on the total number of participants n.
// See the footnote on this bound in the BIP text for the security rationale.
const MAX_PARTICIPANTS = 128

func hasDuplicates(ids []int) bool {
	mapIds := make(map[int]struct{})

	for i := range ids {
		mapIds[ids[i]] = struct{}{}
	}

	return len(mapIds) != len(ids)
}

// DerivePubShareAt implements the algorithm by the same name from the BIP. It
// allows deriving a public share for any 0-based ID x, given at least t known
// public shares and their IDs. To derive the threshold public key, pass -1 as
// the ID in x.
func DerivePubShareAt(ids []int, pubShares []*btcec.PublicKey, x int) (
	*btcec.PublicKey, error) {

	// Ensure each ID has a matching public share and vice versa.
	if len(ids) != len(pubShares) {
		return nil, ErrMismatchPubSharesIds
	}

	// Ensure there are no duplicate IDs.
	if hasDuplicates(ids) {
		return nil, ErrDuplicateElements
	}

	var (
		num     = new(btcec.ModNScalar)
		deno    = new(btcec.ModNScalar)
		numMul  = new(btcec.ModNScalar)
		denoMul = new(btcec.ModNScalar)
		idNeg   = new(btcec.ModNScalar)
		xScalar = new(btcec.ModNScalar)
		xi      = new(btcec.JacobianPoint)

		// 1. Q = inf_point
		q = new(btcec.JacobianPoint)
	)

	// Get ModNScalar representation of X.
	if x >= 0 {
		xScalar.SetInt(uint32(x))
	} else {
		xScalar.SetInt(uint32(0 - x)).Negate()
	}

	// 2. For i = 1..u
	for i := range ids {
		// a. Let num = Scalar(1)
		num.SetInt(uint32(1))

		// b. Let deno = Scalar(1)
		deno.SetInt(uint32(1))

		// Get Jacobian representation of Pi
		pubShares[i].AsJacobian(xi)

		// c. For k = 1..u
		for j := range ids {
			// i. If id_k != id_i
			if ids[i] == ids[j] {
				continue
			}

			// Get ModNScalar representation of id_k
			idNeg.SetInt(uint32(ids[j]))
			idNeg.Negate()

			// ii. Let num = num * Scalar(x - id_k) (mod ord)
			numMul.Set(xScalar).Add(idNeg)
			num.Mul(numMul)

			// iii. Let deno = deno * Scalar(id_i - id_k) (mod ord)
			denoMul.SetInt(uint32(ids[i])).Add(idNeg)
			deno.Mul(denoMul)
		}

		// d. Q = Q + (num * deno^(-1)) * Pi
		num.Mul(deno.InverseNonConst())
		btcec.ScalarMultNonConst(num, xi, xi)
		btcec.AddNonConst(q, xi, q)
	}

	// 3. Return Q, which may be the point at infinity
	q.ToAffine()

	return btcec.NewPublicKey(&q.X, &q.Y), nil
}

// DeriveThresholdPubKey implements the DeriveThreshPubkey algorithm from the
// BIP. It derives the threshold public key from at least t known public shares
// and their corresponding IDs.
func DeriveThresholdPubKey(ids []int, pubShares []*btcec.PublicKey) (
	*btcec.PublicKey, error) {

	// 1. Let Q = DerivePubShareAt(id_1..u, P_1..u, -1)
	q, err := DerivePubShareAt(ids, pubShares, -1)
	if err != nil {
		return nil, err
	}

	// 2. Fail if is_infinity(Q)
	if internal.IsIdentityPubKey(q) {
		return nil, errors.New("The threshold public key must not be " +
			"the point at infinity.")
	}

	// 3. Return cbytes(Q) (we return the PublicKey struct instead)
	return q, nil
}

// ThresholdInfo is a data structure holding the public key material that a
// key generation protocol produces. It consists of the following elements:
type ThresholdInfo struct {
	// T is the threshold number t of participants required to issue a
	// signature: an integer with 1 <= t <= n
	T int

	// ThresholdPubKey is the threshold public key thresh_pk: a 33-byte
	// array, compressed serialized point (we expect an already-parsed
	// public key)
	ThresholdPubKey *btcec.PublicKey

	// PubShares is the list of participant public shares pubshare_0..n-1:
	// n entries, each either a 33-byte array (a compressed serialized
	// point) or empty_bytestring, where 1 <= n <= 128 (we use already-
	// parsed PublicKey structs as known shares, and nil for unknown)
	PubShares []*btcec.PublicKey
}

// ValidateThresholdInfo implements the algorithm by the same name from the BIP.
func ValidateThresholdInfo(info *ThresholdInfo) error {
	n := len(info.PubShares)

	// 1. Fail if not 1 <= t <= n (<= 128)
	// TODO(aakselrod): comment on BIP since the reference code checks the
	// 128 limit here, but it's only mentioned in the ThresholdInfo
	// constraints and not the algorithm pseudocode.
	if info.T < 1 || info.T > n {
		return ErrInvalidThreshold
	}

	if n > MAX_PARTICIPANTS {
		return ErrInvalidNumParticipants
	}

	// 2. Fail if cpoint(thresh_pk) fails (skipped since we expect an
	// already-parsed threshold public key)
	//
	// 3. Let id_1..w be the identifiers i with pubshare_i !=
	// empty_bytestring (we use nil), in ascending order
	var (
		ids       []int
		pubShares []*btcec.PublicKey
	)

	for i, pubShare := range info.PubShares {
		if pubShare != nil {
			ids = append(ids, i)
			pubShares = append(pubShares, pubShare)
		}
	}

	// 4. For j = 1..w: Let P_j = cpoint(pubshare_p); fail if that fails
	// (skipped since we expect already-parsed public keys)
	//
	// 5. Fail if w < t
	if len(pubShares) < info.T {
		return errors.New("At least t pubshares must be present.")
	}

	// 6. For j = t+1..w:
	baseIds := ids[:info.T]
	basePoints := pubShares[:info.T]

	for i := range ids[info.T:] {
		// a. Fail if DerivePubShareAt(id_1..t, P_1..t, id) != P_j
		pubShare, err := DerivePubShareAt(baseIds, basePoints, ids[i])
		if err != nil {
			return err
		}
		if !pubShare.IsEqual(pubShares[i]) {
			return errors.New("The provided key material is " +
				"incorrect: the public shares do not lie on " +
				"a single polynomial.",
			)
		}
	}

	// 7. Fail if DeriveThreshPubKey(id_1..t, P_1..t) != thresh_pk
	threshPk, err := DeriveThresholdPubKey(baseIds, basePoints)
	if err != nil {
		return err
	}
	if !threshPk.IsEqual(info.ThresholdPubKey) {
		return ErrPubSharesThreshPKMismatch
	}

	return nil
}

// TweakContext aggregates tweaks with the threshold public key.
// TODO(aakselrod): maybe reuse MuSig2 KeyAgg implementation here instead?
// Seems like it would require some changes on the MuSig2 side, so maybe not.
type TweakContext struct {
	q    *btcec.JacobianPoint
	gAcc *btcec.ModNScalar
	tAcc *btcec.ModNScalar
}

// GetXOnlyPubKey returns the tweaked threshold public key, after applying all
// tweaks, such that the key can be expressed as an X-only public key.
// To serialize, use schnorr.SerializePubKey
func (t *TweakContext) GetXOnlyPubKey() *btcec.PublicKey {
	// 1. Return tweak_ctx = (Q, gacc, tacc)
	// 2. Return xbytes(Q)
	// NOTE: we return the actual public key
	return internal.GetXOnlyPubKey(t.q)
}

// GetPlayPubKey returns the tweaked threshold public key, after applying all
// tweaks, which may not be a valid X-only public key. To serialize,
// call SerializeCompressed() on the result.
func (t *TweakContext) GetPlainPubKey() *btcec.PublicKey {
	return btcec.NewPublicKey(&t.q.X, &t.q.Y)
}

// tweakCtxInit initializes a new TweakContext based on a threshold public key.
func tweakCtxInit(pubkey *btcec.PublicKey) *TweakContext {
	tCtx := &TweakContext{
		q:    new(btcec.JacobianPoint),
		gAcc: new(btcec.ModNScalar),
		tAcc: new(btcec.ModNScalar),
	}

	// 1. Let Q = cpoint(thresh_pk); fail if that fails
	pubkey.AsJacobian(tCtx.q)

	// 2. Let gacc = Scalar(1)
	tCtx.gAcc.SetInt(1)

	// 3. Let tacc = Scalar(0)
	tCtx.tAcc.SetInt(0)

	// 4. Return tweak_ctx = (Q, gacc, tacc)
	return tCtx
}

// applyTweak applies a single tweak to the TweakContext and returns a new one.
func applyTweak(tCtx *TweakContext, tweak *[32]byte, isXonly bool) (
	*TweakContext, error) {

	var (
		q     = new(btcec.JacobianPoint)
		twkPt = new(btcec.JacobianPoint)
		g     = new(btcec.ModNScalar)
		gAcc  = new(btcec.ModNScalar)
		tAcc  = new(btcec.ModNScalar)
		twk   = new(btcec.ModNScalar)
	)

	// 1. Let (Q, gacc, tacc) = tweak_ctx
	q.Set(tCtx.q)
	gAcc.Set(tCtx.gAcc)
	tAcc.Set(tCtx.tAcc)

	// 2. If is_xonly_t and not has_even_y(Q):
	//   Let g = Scalar(-1)
	// Else:
	//   Let g = Scalar(1)
	g.SetInt(1)
	if isXonly && q.Y.IsOdd() {
		g.Negate()
	}

	// 3. Let t = scalar_from_bytes_checked(tweak); fail if that fails
	overflow := twk.SetBytes(tweak)
	if overflow != 0 {
		return nil, errors.New("The tweak value is out of range.")
	}

	// 4. Let Q' = g · Q + t · G
	btcec.ScalarBaseMultNonConst(twk, twkPt)
	btcec.ScalarMultNonConst(g, q, q)
	btcec.AddNonConst(q, twkPt, q)
	q.ToAffine()

	// 5. Fail if is_infinity(Q')
	if internal.IsPointAtInfinity(q) {
		return nil, errors.New("The result of tweaking cannot be " +
			"infinity.")
	}

	// 6. Let gacc' = g · gacc  (mod ord)
	gAcc.Mul(g)

	// 7. Let tacc' = t + g · tacc  (mod ord)
	tAcc.Mul(g).Add(twk)

	// 8. Return tweak_ctx' = (Q', gacc', tacc')
	return &TweakContext{
		q:    q,
		gAcc: gAcc,
		tAcc: tAcc,
	}, nil
}

// ThresholdPubKeyAndTweak returns a TweakContext generated by applying tweaks
// to a threshold public key. It implements steps 2-3 of the GetSessionValues
// algorithm or steps 4-5 of the DeterministicSign algorithm.
func ThresholdPubKeyAndTweak(thresholdPubKey *btcec.PublicKey,
	tweaks []*[32]byte, isXonly []bool) (*TweakContext, error) {

	if len(tweaks) != len(isXonly) {
		return nil, errors.New("The tweaks and is_xonly arrays must " +
			"have the same length.")
	}

	var (
		// GetSessionValues: step 2.
		// DeterministicSign: step 4.
		// Let tweak_ctx0 = TweakCtxInit(thresh_pk); fail if that fails
		tCtx = tweakCtxInit(thresholdPubKey)
		err  error
	)

	// GetSessionValues: step 3.
	// DeterministicSign: step 5.
	// For i = 1 .. v:
	for i := range tweaks {
		// a. Let tweak_ctx_i = ApplyTweak(tweak_ctx_i-1, tweak_i,
		// is_xonly_t_i); fail if that fails
		tCtx, err = applyTweak(tCtx, tweaks[i], isXonly[i])
		if err != nil {
			return nil, err
		}
	}

	return tCtx, nil
}

// nonceHash hashes signing context to create a nonce. It performs the bulk of
// step 7 in the nonce generation algorithm.
func nonceHash(random, pubShare, thresholdPubKey []byte, i int,
	msgPrefixed []byte, extraIn []byte) *chainhash.Hash {

	var buf []byte

	// rand'
	buf = append(buf, random...)

	// bytes(1, len(pubshare))
	buf = append(buf, byte(len(pubShare)))

	// pubshare
	buf = append(buf, pubShare...)

	// bytes(1, len(thresh_pk_xonly))
	buf = append(buf, byte(len(thresholdPubKey)))

	// thresh_pk_xonly
	buf = append(buf, thresholdPubKey...)

	// m_prefixed
	buf = append(buf, msgPrefixed...)

	// bytes(4, len(extra_in))
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(extraIn)))

	// extra_in
	buf = append(buf, extraIn...)

	// bytes(1, i - 1)
	buf = append(buf, byte(i))

	// Return the tagged hash of the bytestring above.
	defer zeroSlice(buf)
	return chainhash.TaggedHash(chainhash.TagBIP0445Nonce, buf)
}

// SecretNonce holds a secret nonce in parsed form.
type SecretNonce struct {
	nonce1 *btcec.PrivateKey
	nonce2 *btcec.PrivateKey
}

// ParseSecretNonce parses a secret nonce's bytestring representation. At
// signing, the nonce must be cleared to avoid nonce reuse which would leak
// the secret share.
func ParseSecretNonce(b []byte) (*SecretNonce, error) {
	if len(b) != 64 {
		return nil, errors.New("Invalid secret nonce length")
	}

	var (
		n   = new(SecretNonce)
		err error
	)

	n.nonce1, err = internal.ParsePrivKeyNonZeroChecked(b[:32])
	if err != nil {
		return nil, errors.New("first secnonce value is out of range.")
	}

	n.nonce2, err = internal.ParsePrivKeyNonZeroChecked(b[32:])
	if err != nil {
		return nil, errors.New("second secnonce value is out of range.")
	}

	return n, nil
}

// Bytes serializes the secret nonce into a bytestring. The bytestring must
// be zeroed after signing with the nonce to avoid leaking the secret share.
func (n *SecretNonce) Bytes() []byte {
	return append(
		n.nonce1.Serialize(),
		n.nonce2.Serialize()...,
	)
}

// PubNonce returns the public nonce derived from the secret nonce. This is
// equivalent to step 10 of the NonceGen algorithm in the BIP.
func (n *SecretNonce) PubNonce() *PublicNonce {
	// Let pubnonce = cbytes(R*,1) || cbytes(R*,2)
	// NOTE: the nonces aren't serialized here, but in the Bytes() method
	// of the returned PublicNonce.
	return &PublicNonce{
		nonce1: n.nonce1.PubKey(),
		nonce2: n.nonce2.PubKey(),
	}
}

// PublicNonce is a public nonce used for nonce aggregation and partial signing.
type PublicNonce struct {
	nonce1 *btcec.PublicKey
	nonce2 *btcec.PublicKey
}

// ParsePublicNonce parses a public nonce from its bytestring representation.
func ParsePublicNonce(b []byte) (*PublicNonce, error) {
	if len(b) != 2*33 {
		return nil, errors.New("Invalid public nonce length")
	}

	var (
		n   = new(PublicNonce)
		err error
	)

	// We don't allow the point at infinity in public nonces.
	n.nonce1, err = btcec.ParsePubKey(b[:33])
	if err != nil {
		return nil, err
	}

	n.nonce2, err = btcec.ParsePubKey(b[33:])
	if err != nil {
		return nil, err
	}

	return n, nil
}

// ParseAggNonce parses an aggregate nonce from its bytestring representation.
// The difference from ParsePubNonce is that for aggregate nonces, the point at
// infinity is allowed as input.
func ParseAggNonce(b []byte) (*PublicNonce, error) {
	if len(b) != 2*33 {
		return nil, ErrInvalidContribution{-1, ContributionTypeAggNonce}
	}

	var (
		n   = new(PublicNonce)
		err error
	)

	// We DO allow the point at infinity in aggregate nonces.
	n.nonce1, err = internal.ParsePubKeyWithInfinity(b[:33])
	if err != nil {
		return nil, ErrInvalidContribution{-1, ContributionTypeAggNonce}
	}

	n.nonce2, err = internal.ParsePubKeyWithInfinity(b[33:])
	if err != nil {
		return nil, ErrInvalidContribution{-1, ContributionTypeAggNonce}
	}

	return n, nil
}

// Bytes serializes the public nonce as a bytestring.
func (n *PublicNonce) Bytes() []byte {
	return append(
		internal.SerializeCompressedWithInfinity(n.nonce1),
		internal.SerializeCompressedWithInfinity(n.nonce2)...,
	)
}

// nonceGen implements steps 2-12 of the NonceGen algorithm from the BIP,
// calling nonceHash for most of the step 7 implementation.
func nonceGen(random *[32]byte, secShare *btcec.PrivateKey,
	thresholdPubKey *btcec.PublicKey, msg *[]byte, extraIn []byte) (
	*SecretNonce, error) {

	var pubShareBytes []byte
	//TODO(aakselrod): separate handling of pubshare by allowing it to be
	// passed separately from secShare?
	if secShare != nil {
		// 2. If the optional argument secshare is present:
		//   Let rand' = xor_bytes(secshare, hash_BIP0445/aux(rand))
		// Else:
		//   Let rand' = rand
		secShareBytes := secShare.Serialize()
		randHash := chainhash.TaggedHash(
			chainhash.TagBIP0445Aux, random[:],
		)
		for i := range secShareBytes {
			random[i] = randHash[i] ^ secShareBytes[i]
		}
		defer zeroSlice(secShareBytes)

		// 3. If the optional argument pubshare is not present:
		//   Let pubshare = empty_bytestring
		pubShareBytes = secShare.PubKey().SerializeCompressed()
	}

	// 4. If the optional argument thresh_pk_xonly is not present:
	//   Let thresh_pk_xonly = empty_bytestring
	var thresholdPubKeyBytes []byte
	if thresholdPubKey != nil {
		thresholdPubKeyBytes = schnorr.SerializePubKey(thresholdPubKey)
	}

	// 5. If the optional argument m is not present:
	//   Let m_prefixed = bytes(1, 0)
	// Else:
	//   Let m_prefixed = bytes(1, 1) || bytes(8, len(m)) || m
	var msgPrefixed []byte
	if msg == nil {
		msgPrefixed = []byte{0}
	} else {
		msgPrefixed = []byte{1}
		msgPrefixed = binary.BigEndian.AppendUint64(
			msgPrefixed, uint64(len(*msg)),
		)
		msgPrefixed = append(msgPrefixed, *msg...)
	}

	// 6. If the optional argument extra_in is not present:
	//   Let extra_in = empty_bytestring
	//
	// NOTE: the empty bytestring in extraIn means that extraIn is not
	// present.
	// 7. Let ki = scalar_from_bytes_wrapping(hashBIP0445/nonce(
	//   rand' || bytes(1, len(pubshare)) || pubshare ||
	//   bytes(1, len(thresh_pk_xonly)) || thresh_pk_xonly || m_prefixed ||
	//   bytes(4, len(extra_in)) || extra_in || bytes(1, i - 1)
	// )) for i = 1,2
	// NOTE: we also do step 9 here because that's how the underlying
	// library works.
	//
	// 9. Let R*,1 = k1 · G, R*,2 = k2 · G
	k1, k1Pub := btcec.PrivKeyFromBytes(nonceHash(
		random[:], pubShareBytes, thresholdPubKeyBytes, 0, msgPrefixed,
		extraIn,
	)[:])

	k2, k2Pub := btcec.PrivKeyFromBytes(nonceHash(
		random[:], pubShareBytes, thresholdPubKeyBytes, 1, msgPrefixed,
		extraIn,
	)[:])

	// 8. Fail if k1 = Scalar(0) or k2 = Scalar(0)
	if k1.Key.IsZero() || k2.Key.IsZero() {
		return nil, errors.New("resulting secnonces cannot be zero")
	}

	if internal.IsIdentityPubKey(k1Pub) ||
		internal.IsIdentityPubKey(k2Pub) {

		return nil, errors.New("resulting pubnonces cannot be point " +
			"at infinity")
	}

	// 10. Let pubnonce = cbytes(R*,1) || cbytes(R*,2)
	// NOTE: this is done in the PublicNonce() method of SecretNonce as
	// well as the Bytes() method of PublicNonce.
	//
	// 11. Let secnonce = scalar_to_bytes(k1) || scalar_to_bytes(k2)
	// NOTE: This is done in the Bytes() method of SecretNonce.
	//
	// 12. Return (secnonce, pubnonce)
	// NOTE: We return only the secnonce here, and PublicNonce() can be
	// used to get the public nonce from there.
	// TODO(aakselrod): change this and return the pubnonce as well?
	return &SecretNonce{
		nonce1: k1,
		nonce2: k2,
	}, nil
}

// NonceGen implements the NonceGen algorithm from the BIP to generate a nonce
// for a signing session.
func NonceGen(secShare *btcec.PrivateKey, thresholdPubKey *btcec.PublicKey,
	msg *[]byte, extraIn []byte) (*SecretNonce, error) {

	// 1. Let rand = random_bytes(32)
	random, err := internal.Rand32Int()
	if err != nil {
		return nil, err
	}
	defer zeroSlice(random[:])

	// 2-12. Call nonceGen
	return nonceGen(random, secShare, thresholdPubKey, msg, extraIn)
}

// NonceAgg aggregates public nonces when there are at least t signers (or
// t-1 in the event of deterministic signing by the coordinator which is also
// a signer). It implements the NonceAgg algorithm from the BIP.
func NonceAgg(pubNonces []*PublicNonce) *PublicNonce {
	var (
		p1  = new(btcec.JacobianPoint)
		p2  = new(btcec.JacobianPoint)
		ap1 = new(btcec.JacobianPoint)
		ap2 = new(btcec.JacobianPoint)
	)

	// For j = 1 .. 2:
	// NOTE: we unroll the j loop here.
	//   For i = 1 .. u:
	//     Let R_i,j = cpoint(pubnonce_i[(j-1)*33:j*33]); fail if that
	//     fails and blame signer at index i for invalid pubnonce
	//     // NOTE: we use pre-parsed points as inputs.
	//   Let R_j = R_1,j + R_2,j + ... + R_u,j
	for _, nonce := range pubNonces {
		nonce.nonce1.AsJacobian(ap1)
		nonce.nonce2.AsJacobian(ap2)
		btcec.AddNonConst(p1, ap1, p1)
		btcec.AddNonConst(p2, ap2, p2)
	}

	p1.ToAffine()
	p2.ToAffine()

	// Return aggnonce = cbytes_ext(R_1) || cbytes_ext(R_2)
	// NOTE: we return a nonce with PublicKey structs, which can be
	// serialized on demand using the Bytes() method.
	return &PublicNonce{
		nonce1: btcec.NewPublicKey(&p1.X, &p1.Y),
		nonce2: btcec.NewPublicKey(&p2.X, &p2.Y),
	}
}

// SessionContext describes the signing session. It mirrors the identically-
// named data structure in the BIP.
type SessionContext struct {
	// N is the number of participants involved in generating the threshold
	// public key.
	N int

	// T is the threshold number of participants required to issue a
	// signature: 1 <= t <= n.
	T int

	// Ids is the list of participant identifiers id_1..u distinct integers,
	// each with 0 <= id <= n-1. The length of this list is u, the number
	// of signers for this session.
	Ids []int

	// PubShares is the list of participant public shares pubshare_1..u:
	// either u pubshares, where pubshare_i belongs to the participant with
	// identifier id_i, or the whole list is absent/0-length.
	PubShares []*btcec.PublicKey

	// ThresholdPubKey is the threshold public key resulting from key
	// generation.
	ThresholdPubKey *btcec.PublicKey

	// AggNonce is the aggregate public nonce, which is the output of
	// NonceAgg.
	AggNonce *PublicNonce

	// Tweaks is the list of tweaks tweak_1..v, 32-byte arrays, each a
	// serialized scalar.
	// TODO(aakselrod): should we pass actual ModNScalars here instead
	// of the serialized format?
	Tweaks []*[32]byte

	// IsXOnly is a list of tweak modes is_xonly_t_1..v, each a boolean.
	IsXOnly []bool

	// Msg is the message to be signed.
	Msg []byte
}

// ValidateSessionParams validates that the session parameters are consistent
// and sufficient to perform a signing ceremony.
func ValidateSessionParams(n, t int, ids []int, pubShares []*btcec.PublicKey,
	thresholdPubKey *btcec.PublicKey) error {

	// 1. Fail if not 1 <= t <= n
	if t < 1 || t > n {
		return ErrInvalidThreshold
	}

	// 1a. Also check that n <= MAX_PARTICIPANTS to ensure adaptive
	// adaptive security against LDVR. See the BIP and references to
	// Crites et al. for further details.
	if n > MAX_PARTICIPANTS {
		return ErrInvalidNumParticipants
	}

	// 2. Fail if not t <= u <= n
	if len(ids) < t || len(ids) > n {
		return errors.New("The number of signers must be between t " +
			"and n.")
	}

	if len(pubShares) != 0 && len(pubShares) != len(ids) {
		return ErrMismatchPubSharesIds
	}

	// 3. For i = 1 .. u:
	for idx, i := range ids {
		// a. Fail if not 0 <= id_i <= n - 1
		// b. If pubshare_1..u is present:
		//   Let P_i = cpoint(pubshare_i); fail if that fails
		// NOTE: we pass in pre-parsed pubshares here.
		if i < 0 || i >= n {
			return fmt.Errorf("Invalid id at index %d", idx)
		}
	}

	// 4. If pubshare1..u is present:
	if len(pubShares) != 0 {
		// Fail if DeriveThreshPubkey(id_1..u, P_1..u) != thresh_pk
		threshPk, err := DeriveThresholdPubKey(ids, pubShares)
		if err != nil {
			return err
		}
		if !threshPk.IsEqual(thresholdPubKey) {
			return ErrPubSharesThreshPKMismatch
		}
	}

	return nil
}

// serializeIds implements the internal algorithm SerializeIDs from the BIP.
func serializeIds(ids []int) []byte {
	// 1. Let sorted_id_1..u = sorted(id_1..u)
	idCopy := slices.Clone(ids)
	slices.Sort(idCopy)

	// 2. res = empty_bytestring
	b := make([]byte, 0, 4+len(ids)*4)

	// NOTE: We prepend the length of the IDs list here rather than in
	// the callsites to avoid duplication of the code.
	b = binary.BigEndian.AppendUint32(b, uint32(len(ids)))

	// 3. For i = 1..u:
	for _, id := range idCopy {
		// a. res = res || bytes(4, sorted_id_i)
		b = binary.BigEndian.AppendUint32(b, uint32(id))
	}

	// 4. Return res
	return b
}

// getSessionValues implements the GetSessionValues algorithm from the BIP.
func getSessionValues(sessionCtx *SessionContext) (*btcec.JacobianPoint,
	*btcec.ModNScalar, *btcec.ModNScalar, []int, []*btcec.PublicKey,
	*btcec.ModNScalar, *btcec.JacobianPoint, *btcec.ModNScalar, error) {

	// 1. Run ValidateSessionParams(n, t, u, id_1..u, pubshare_1..u,
	// thresh_pk); fail if that fails
	err := ValidateSessionParams(
		sessionCtx.N, sessionCtx.T, sessionCtx.Ids,
		sessionCtx.PubShares, sessionCtx.ThresholdPubKey,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	// 2-3. Handled by ThresholdPubKeyAndTweak. 4 is skipped and we use
	// the values directly from tweakCtx.
	tweakCtx, err := ThresholdPubKeyAndTweak(
		sessionCtx.ThresholdPubKey, sessionCtx.Tweaks,
		sessionCtx.IsXOnly,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	// 5. Let ser_ids = SerializeIds(id_1..u)
	// 6. Let b = scalar_from_bytes_wrapping(hashBIP0445/noncecoef(
	// bytes(4, u) || ser_ids || aggnonce || xbytes(Q) || m))
	hashData := append(
		serializeIds(sessionCtx.Ids),
		sessionCtx.AggNonce.Bytes()...,
	)
	hashData = append(
		hashData,
		schnorr.SerializePubKey(internal.GetXOnlyPubKey(tweakCtx.q))...,
	)
	hashData = append(hashData, sessionCtx.Msg...)

	bBytes := chainhash.TaggedHash(chainhash.TagBIP0445NonceCoef, hashData)
	b := new(btcec.ModNScalar)
	b.SetBytes((*[32]byte)(bBytes))

	// 7. Fail if b = Scalar(0)
	if b.IsZero() {
		return nil, nil, nil, nil, nil, nil, nil, nil,
			errors.New("resulting b cannot be 0")
	}

	// 8. Let R1 = cpoint_ext(aggnonce[0:33]),
	// R2 = cpoint_ext(aggnonce[33:66]); fail if that fails and blame the
	// coordinator for invalid aggnonce.
	// NOTE: We pass a pre-parsed aggnonce in the SessionContext.
	var (
		R1 = new(btcec.JacobianPoint)
		R2 = new(btcec.JacobianPoint)
		R  = new(btcec.JacobianPoint)
	)

	sessionCtx.AggNonce.nonce1.AsJacobian(R1)
	sessionCtx.AggNonce.nonce2.AsJacobian(R2)

	// 9. Let R' = R1 + b · R2
	btcec.ScalarMultNonConst(b, R2, R)
	btcec.AddNonConst(R, R1, R)

	// 10. If is_infinity(R'):
	if internal.IsPointAtInfinity(R) {
		// a. Let final nonce R = G
		one := new(btcec.ModNScalar)
		one.SetInt(uint32(1))
		btcec.ScalarBaseMultNonConst(one, R)
	}

	// b. Else: let final nonce R = R'
	R.ToAffine()

	// 11. Let e = scalar_from_bytes_wrapping(
	// hashBIP0340/challenge((xbytes(R) || xbytes(Q) || m)))
	hashData = append(
		schnorr.SerializePubKey(internal.GetXOnlyPubKey(R)),
		schnorr.SerializePubKey(internal.GetXOnlyPubKey(tweakCtx.q))...,
	)
	hashData = append(hashData, sessionCtx.Msg...)

	eBytes := chainhash.TaggedHash(chainhash.TagBIP0340Challenge, hashData)
	e := new(btcec.ModNScalar)
	e.SetBytes((*[32]byte)(eBytes))

	// 12. Fail if e = Scalar(0)
	if e.IsZero() {
		return nil, nil, nil, nil, nil, nil, nil, nil,
			errors.New("resulting e cannot be 0")
	}

	// 13. Return (Q, gacc, tacc, id1..u, pubshare1..u, b, R, e)
	return tweakCtx.q, tweakCtx.gAcc, tweakCtx.tAcc, sessionCtx.Ids,
		sessionCtx.PubShares, b, R, e, nil
}

// Sign creates a partial signature with a secret share and secret nonce whose
// public nonce must be part of the aggregated nonce. It implements the Sign
// algorithm from the BIP.
func Sign(secNonce *SecretNonce, secShare *btcec.PrivateKey, myId int,
	sessionCtx *SessionContext, clearNonce func() error) (
	*btcec.ModNScalar, error) {

	// 1. Let (Q, gacc, _, id1..u, pubshare1..u, b, R, e) =
	// GetSessionValues(session_ctx); fail if that fails
	Q, gAcc, _, ids, pubShares, b, R, e, err := getSessionValues(
		sessionCtx,
	)
	if err != nil {
		return nil, err
	}

	// 2. Let k1' = scalar_from_bytes_nonzero_checked(secnonce[0:32]); fail
	// if that fails
	// 3. Let k2' = scalar_from_bytes_nonzero_checked(secnonce[32:64]);
	// fail if that fails
	// NOTE: We get a pre-parsed nonce, so can skip these steps.
	// 4. Let k1 = k1', k2 = k2' if has_even_y(R), otherwise let
	// k1 = -k1', k2 = -k2'
	// NOTE: We zero k1/k2 after use to prevent nonce reuse.
	k1 := new(btcec.ModNScalar)
	k1.Set(&secNonce.nonce1.Key)
	if R.Y.IsOdd() {
		k1.Negate()
	}
	defer k1.Zero()

	k2 := new(btcec.ModNScalar)
	k2.Set(&secNonce.nonce2.Key)
	if R.Y.IsOdd() {
		k2.Negate()
	}
	defer k2.Zero()

	// We clear the secNonce before signing so it can't be reused.
	// NOTE: We perform step 14 here prior to clearing so the public nonce
	// is available later.
	pubNonce := secNonce.PubNonce()
	secNonce.nonce1.Zero()
	secNonce.nonce2.Zero()

	// We also clear secNonce in persistent storage using the callback to
	// prevent nonce reuse, also before signing.
	err = clearNonce()
	if err != nil {
		return nil, err
	}

	// 5. Let d' = scalar_from_bytes_nonzero_checked(secshare); fail if
	// that fails
	// NOTE: we get the secShare pre-parsed.
	// 6. Let pubshare = cbytes(d' · G)
	myPubShare := secShare.PubKey()

	// 7. Fail if my_id not in id_1..u
	if !slices.Contains(ids, myId) {
		return nil, errors.New("The signer's id is missing from the " +
			"ids list.")
	}

	// 8. If pubshare_1..u is present:
	if len(pubShares) > 0 {
		// a. Let j be the index with id_j = my_id
		myIdx := slices.Index(ids, myId)

		// b. Fail if pubshare ≠ pubshare_j
		if !myPubShare.IsEqual(pubShares[myIdx]) {
			return nil, errors.New("The signer's pubshare is " +
				"missing from the pubshares list.")
		}
	}

	// 9. Let λ = DeriveInterpolatingValue(id1..u, my_id); fail if that
	// fails (we use a for lambda here)
	a, err := internal.DeriveInterpolatingValue(ids, myId)
	if err != nil {
		return nil, err
	}

	// 10. Let g = Scalar(1) if has_even_y(Q), otherwise let g = Scalar(-1)
	g := new(btcec.ModNScalar)
	g.SetInt(uint32(1))
	if Q.Y.IsOdd() {
		g.Negate()
	}

	// 11. Let d = g · gacc · d' (mod ord)
	// 12. Let s = k1 + b · k2 + e · λ · d (mod ord)
	s := new(btcec.ModNScalar)

	// d
	s.Mul2(g, &secShare.Key).Mul(gAcc)

	// e * a (lambda) * d
	s.Mul(e).Mul(a)

	// + k1
	s.Add(k1)

	// + k2 * b
	s.Add(k2.Mul(b))

	// 13. Let psig = scalar_to_bytes(s)
	// NOTE: We don't convert to bytes here.
	// 14. Let pubnonce = cbytes(k1' · G) || cbytes(k2' · G)
	// NOTE: We performed this step above, prior to clearing the nonce,
	// so we would have the value available after.
	// 15. If PartialSigVerifyInternal(psig, my_id, pubnonce, pubshare,
	// session_ctx) (see below) returns failure, fail
	if !partialSigVerify(
		s, myId, pubNonce, secShare.PubKey(), sessionCtx,
	) {
		return nil, errors.New("signature doesn't match")
	}

	// 16. Return partial signature psig
	return s, nil
}

// partialSigVerify implements the PartialSigVerifyInternal algorithm from the
// BIP. It assumes basic sanity checks specified in the PartialSigVerify and
// Sign algorithms are done, and accepts a pre-constructed SessionContext.
func partialSigVerify(pSig *btcec.ModNScalar, myId int,
	pubNonce *PublicNonce, pubShare *btcec.PublicKey,
	sessionCtx *SessionContext) bool {

	// 1. Let (Q, gacc, _, id1..u, _, b, R, e) =
	// GetSessionValues(session_ctx); fail if that fails
	Q, gAcc, _, ids, _, b, R, e, err := getSessionValues(sessionCtx)
	if err != nil {
		return false
	}

	// 2. Let s = scalar_from_bytes_checked(psig); fail if that fails
	// 3. Let R*,1 = cpoint(pubnonce[0:33]),
	// R*,2 = cpoint(pubnonce[33:66]); fail if either fails
	// NOTE: We accept pre-parsed values here.
	// 4. Let Re*' = R*,1 + b · R*,2
	var (
		R1Partial = new(btcec.JacobianPoint)
		R2Partial = new(btcec.JacobianPoint)
		Res       = new(btcec.JacobianPoint)
		P         = new(btcec.JacobianPoint)
		sG        = new(btcec.JacobianPoint)
		g         = new(btcec.ModNScalar)
	)

	btcec.ScalarBaseMultNonConst(pSig, sG)
	sG.ToAffine()

	pubNonce.nonce1.AsJacobian(R1Partial)
	pubNonce.nonce2.AsJacobian(R2Partial)
	btcec.ScalarMultNonConst(b, R2Partial, Res)
	btcec.AddNonConst(R1Partial, Res, Res)

	// 5. Let effective nonce Re* = Re*' if has_even_y(R), otherwise let
	// Re* = -Re*'
	// NOTE: We temporarily use g here.
	g.SetInt(uint32(1))
	if R.Y.IsOdd() {
		g.Negate()
	}
	btcec.ScalarMultNonConst(g, Res, Res)

	// 6. Let P = cpoint(pubshare); fail if that fails
	// NOTE: We accept a pre-parsed value here, so this can't fail.
	pubShare.AsJacobian(P)

	// 7. Let λ = DeriveInterpolatingValue(id_1..u, my_id)
	// NOTE: We use a for lambda here.
	a, err := internal.DeriveInterpolatingValue(ids, myId)
	if err != nil {
		return false
	}

	// 8. Let g = Scalar(1) if has_even_y(Q), otherwise let g = Scalar(-1)
	g.SetInt(uint32(1))
	if Q.Y.IsOdd() {
		g.Negate()
	}

	// 9. Let g' = g · gacc (mod ord)
	// 10. Fail if s · G ≠ Re* + e · λ · g' · P
	btcec.ScalarMultNonConst(g, P, P)
	btcec.ScalarMultNonConst(gAcc, P, P)
	btcec.ScalarMultNonConst(e, P, P)
	btcec.ScalarMultNonConst(a, P, P)
	btcec.AddNonConst(Res, P, P)
	P.ToAffine()

	// 11. Return success iff no failure occurred before reaching this
	// point.
	return P.X.Equals(&sG.X) && P.Y.Equals(&sG.Y) && P.Z.Equals(&sG.Z)
}

// PartialSigVerify implements the PartialSigVerify algorithm from the BIP.
func PartialSigVerify(pSig *btcec.ModNScalar, pubNonces []*PublicNonce,
	n, t int, ids []int, pubShares []*btcec.PublicKey,
	thresholdPubKey *btcec.PublicKey, tweaks []*[32]byte, isXOnly []bool,
	msg []byte, idx int) bool {

	// Basic sanity checks.
	if len(pubNonces) != len(ids) || len(pubShares) != len(ids) {
		return false
	}

	if idx < 0 || idx >= len(ids) {
		return false
	}

	if len(tweaks) != len(isXOnly) {
		return false
	}

	// 1. Run ValidateSessionParams(n, t, u, id_1..u, pubshare_1..u,
	// thresh_pk); fail if that fails
	err := ValidateSessionParams(n, t, ids, pubShares, thresholdPubKey)
	if err != nil {
		return false
	}

	// 2. Let aggnonce = NonceAgg(pubnonce_1..u); fail if that fails
	// 3. Let session_ctx = (n, t, u, id_1..u, pubshare_1..u, thresh_pk,
	// aggnonce, v, tweak_1..v, is_xonly_t_1..v, m)
	sessionCtx := &SessionContext{
		N:               n,
		T:               t,
		Ids:             ids,
		PubShares:       pubShares,
		ThresholdPubKey: thresholdPubKey,
		AggNonce:        NonceAgg(pubNonces),
		Tweaks:          tweaks,
		IsXOnly:         isXOnly,
		Msg:             msg,
	}

	// 4. Run PartialSigVerifyInternal(psig, id_i, pubnonce_i, pubshare_i,
	// session_ctx)
	// 5. Return success iff no failure occurred before reaching this point.
	return partialSigVerify(
		pSig, ids[idx], pubNonces[idx], pubShares[idx], sessionCtx,
	)
}

// PartialSigAgg aggregates pre-verified partial signatures. It implements the
// PartialSigAgg algorithm from the BIP.
func PartialSigAgg(pSigs []*btcec.ModNScalar, sessionCtx *SessionContext) (
	*schnorr.Signature, error) {

	// 1. Let (Q, _, tacc, _, _, _, R, e) = GetSessionValues(session_ctx);
	// fail if that fails
	Q, _, tAcc, ids, _, _, R, e, err := getSessionValues(
		sessionCtx,
	)
	if err != nil {
		return nil, err
	}
	if len(pSigs) != len(ids) {
		return nil, errors.New("The psigs and ids lists must have " +
			"the same length.")
	}

	// 2. For i = 1 .. u:
	s := new(btcec.ModNScalar)
	for _, pSig := range pSigs {
		// a. Let s_i = scalar_from_bytes_checked(psig_i);
		// fail if that fails and blame signer at index i for invalid
		// partial signature.
		// NOTE: We accept pre-parsed partial signatures here.
		// NOTE: We perform the s_1 + ... + s_u addition from step 4
		// here.
		s.Add(pSig)
	}

	// 3. Let g = Scalar(1) if has_even_y(Q), otherwise let g = Scalar(-1)
	g := new(btcec.ModNScalar)
	g.SetInt(uint32(1))
	if Q.Y.IsOdd() {
		g.Negate()
	}

	// 4. Let s = s_1 + ... + s_u + e · g · tacc (mod ord)
	// NOTE: s_1 + ... + s_u is done above under step 2.
	s.Add(g.Mul(e).Mul(tAcc))

	// 5. Return sig = xbytes(R) || scalar_to_bytes(s)
	// NOTE: We return a schnorr.Signature here which can be serialized
	// when required, rather than returning the serialized signature.
	return schnorr.NewSignature(&R.X, s), nil
}

// detNonceHash implements the concatenation and hashing portion of each
// iteration of step 7 in the DeterministicSign algorithm.
func detNonceHash(secShareBytes []byte, myId int, ids []int,
	aggOtherNonce, tweakedThreshPkXOnly, msg []byte,
	i int) *chainhash.Hash {

	// secshare'
	// bytes(4, my_id)
	buf := binary.BigEndian.AppendUint32(secShareBytes, uint32(myId))

	// bytes(4, u)
	// SerializeIds(id1..u)
	// NOTE: serializeIds prepends u to avoid duplicating code.
	buf = append(buf, serializeIds(ids)...)

	// aggothernonce'
	buf = append(buf, aggOtherNonce...)

	// tweaked_thresh_pk_xonly
	buf = append(buf, tweakedThreshPkXOnly...)

	// bytes(8, len(m))
	buf = binary.BigEndian.AppendUint64(buf, uint64(len(msg)))

	// m
	buf = append(buf, msg...)

	// bytes(1, i - 1)))
	// NOTE: we pass a 0-based i.
	buf = append(buf, byte(i))

	// Zero the buffer when finished to prevent nonce reuse.
	defer zeroSlice(buf)

	// Return the tagged hash required by step 7.
	return chainhash.TaggedHash(chainhash.TagBIP0445DetNonce, buf)
}

// DeterministicSign allows the final cosigner/coordinator
func DeterministicSign(secShare *btcec.PrivateKey, myId int,
	aggOtherNonce *PublicNonce, n, t int, ids []int,
	pubShares []*btcec.PublicKey, thresholdPubKey *btcec.PublicKey,
	tweaks []*[32]byte, isXOnly []bool, msg []byte, auxRand []byte,
) (*PublicNonce, *btcec.ModNScalar, error) {

	// 1. Run ValidateSessionParams(n, t, u, id_1..u, pubshare_1..u,
	// thresh_pk); fail if that fails
	err := ValidateSessionParams(n, t, ids, pubShares, thresholdPubKey)
	if err != nil {
		return nil, nil, err
	}

	// 2. If the optional argument aux_rand is present:
	secShareBytes := secShare.Serialize()
	defer zeroSlice(secShareBytes)
	if len(auxRand) != 0 {
		// a. Let secshare' = xor_bytes(secshare,
		// hashBIP0445/aux(aux_rand))
		randHash := chainhash.TaggedHash(
			chainhash.TagBIP0445Aux, auxRand,
		)
		defer zeroSlice(randHash[:])

		for i, b := range randHash[:] {
			secShareBytes[i] = secShareBytes[i] ^ b
		}
	}
	// b. Else: Let secshare' = secshare

	// 3. If the optional argument aggothernonce is present:
	var aggOtherNonceBytes []byte
	if aggOtherNonce != nil {
		// a. Let aggothernonce' = aggothernonce
		aggOtherNonceBytes = aggOtherNonce.Bytes()
	}
	// b. Else: Let aggothernonce' = empty_bytestring

	// 4-5. Handled by ThresholdPubKeyAndTweak.
	tweakCtx, err := ThresholdPubKeyAndTweak(
		thresholdPubKey, tweaks, isXOnly,
	)
	if err != nil {
		return nil, nil, err
	}

	// 6. Let tweaked_thresh_pk_xonly = GetXonlyPubkey(tweak_ctx_v)
	tweakedThreshPkXOnly := schnorr.SerializePubKey(
		tweakCtx.GetXOnlyPubKey(),
	)

	// 7. Let k_i = scalar_from_bytes_wrapping(
	// hashBIP0445/deterministic/nonce(secshare' || bytes(4, my_id) ||
	// bytes(4, u) || SerializeIds(id1..u) || aggothernonce' ||
	// tweaked_thresh_pk_xonly || bytes(8, len(m)) || m || bytes(1, i - 1)))
	// for i = 1,2
	// NOTE: We do the concatenation and tagged hashing in the detNonceHash
	// and unroll the loop.
	// NOTE: We zero the nonces after use to prevent nonce reuse.
	k1 := detNonceHash(
		secShareBytes, myId, ids, aggOtherNonceBytes,
		tweakedThreshPkXOnly, msg, 0,
	)
	defer zeroSlice(k1[:])

	k2 := detNonceHash(
		secShareBytes, myId, ids, aggOtherNonceBytes,
		tweakedThreshPkXOnly, msg, 1,
	)
	defer zeroSlice(k2[:])

	secNonce := new(SecretNonce)
	secNonce.nonce1, _ = btcec.PrivKeyFromBytes(k1[:])
	secNonce.nonce2, _ = btcec.PrivKeyFromBytes(k2[:])

	// 8. Fail if k1 = Scalar(0) or k2 = Scalar(0)
	if secNonce.nonce1.Key.IsZero() || secNonce.nonce2.Key.IsZero() {
		return nil, nil, fmt.Errorf("resulting nonce must not be 0")
	}

	// 9. Let R*,1 = k1 · G, R*,2 = k2 · G
	// 10. Let pubnonce = cbytes(R*,1) || cbytes(R*,2)
	// NOTE: We don't serialize here, but allow it on demand later.
	pubNonce := secNonce.PubNonce()

	// 11. Let secnonce = scalar_to_bytes(k1) || scalar_to_bytes(k2)
	// NOTE: We don't serialize, but pass the nonce as a struct.
	// 12. If the optional argument aggothernonce is present:
	aggNonce := secNonce.PubNonce()
	if aggOtherNonce != nil {
		// a. Let aggnonce = NonceAgg((pubnonce, aggothernonce)); fail
		// if that fails and blame coordinator for invalid
		// aggothernonce.
		aggNonce = NonceAgg([]*PublicNonce{aggNonce, aggOtherNonce})
	}
	// b. Else: Let aggnonce = pubnonce

	// 13. Let session_ctx = (n, t, u, id_1..u, pubshare_1..u, thresh_pk,
	// aggnonce, v, tweak_1..v, is_xonly_t_1..v, m)
	sessionCtx := &SessionContext{
		N:               n,
		T:               t,
		Ids:             ids,
		PubShares:       pubShares,
		ThresholdPubKey: thresholdPubKey,
		AggNonce:        aggNonce,
		Tweaks:          tweaks,
		IsXOnly:         isXOnly,
		Msg:             msg,
	}

	// 14. Return (pubnonce, Sign(secnonce, secshare, my_id, session_ctx))
	pSig, err := Sign(
		secNonce, secShare, myId, sessionCtx,

		// The Sign() function clears the secret nonce in memory. The
		// callback doesn't need to do anything since the deterministic
		// nonce is never persisted.
		func() error {
			return nil
		},
	)
	if err != nil {
		return nil, nil, err
	}

	return pubNonce, pSig, nil
}

func zeroSlice(buf []byte) {
	for i := range buf {
		buf[i] = 0x00
	}
}
