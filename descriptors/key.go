package descriptors

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
)

// Serialized public key lengths.
const (
	// xOnlyKeyLen is the length of an x-only public key.
	xOnlyKeyLen = 32

	// compressedKeyLen is the length of a compressed public key.
	compressedKeyLen = 33

	// uncompressedKeyLen is the length of an uncompressed public key.
	uncompressedKeyLen = 65

	// keyOriginFingerprintLen is the number of hex characters of the
	// fingerprint a key origin starts with (BIP380).
	keyOriginFingerprintLen = 8
)

// stepKind is the kind of a single derivation-path step after an extended key.
type stepKind uint8

const (
	// stepNum is a fixed derivation index.
	stepNum stepKind = iota

	// stepMultipath is a "<a;b;...>" multipath element, which selects a
	// different index per multipath sub-descriptor.
	stepMultipath

	// stepWildcard is a "*" element, which is replaced by the derivation
	// index at address-derivation time.
	stepWildcard
)

// pathIndex is one BIP32 derivation index of a path step, i.e. the number that
// "h" or "'" may have been applied to.
type pathIndex struct {
	// num is the index below the hardened range.
	num uint32

	// hardened is set if the index was written with a hardened indicator.
	hardened bool
}

// childIndex returns the BIP32 child index this index derives, i.e. the number
// with the hardened bit applied.
func (i pathIndex) childIndex() uint32 {
	if i.hardened {
		return i.num + hdkeychain.HardenedKeyStart
	}

	return i.num
}

// String renders the index the way it appears in a descriptor.
func (i pathIndex) String() string {
	s := strconv.FormatUint(uint64(i.num), 10)
	if i.hardened {
		s += "'"
	}

	return s
}

// pathStep is one element of the derivation path of an extended key.
type pathStep struct {
	kind stepKind

	// index is the index of a stepNum. For a stepWildcard only its hardened
	// flag is used, which says whether the wildcard was written as "*h" or
	// "*'" rather than "*".
	index pathIndex

	// multipath holds the indices of a stepMultipath, in order.
	multipath []pathIndex
}

// keyForm is the public key serialization a descriptor position requires. Which
// forms are valid where is defined by the descriptor BIPs, and the choice is
// consensus-relevant: the form decides the bytes that end up in the script, and
// therefore the address.
type keyForm uint8

const (
	// keyFormLegacy is a pre-segwit position: the key of a pk(), pkh(),
	// multi() or sortedmulti() at the top level or inside an sh(). A raw
	// key may be 33-byte compressed or 65-byte uncompressed there, and is
	// used in the script exactly as written (BIP380, BIP381, BIP383). A key
	// derived from an extended key is always compressed, since BIP380
	// forbids serializing a derived key uncompressed.
	keyFormLegacy keyForm = iota

	// keyFormCompressed is a segwit v0 position: wpkh(), and everything
	// inside a wsh(). Only 33-byte compressed keys are valid there
	// (BIP382). It is also the form used inside miniscript expressions,
	// which BIP379 only defines for the wsh() and tr() contexts.
	keyFormCompressed

	// keyFormXOnly is a taproot position: the internal key and the leaf
	// scripts of a tr(). Keys are serialized as 32-byte x-only keys; a
	// 33-byte compressed key is implicitly converted, and uncompressed keys
	// are invalid (BIP386).
	keyFormXOnly
)

// descKey is a parsed descriptor public key: either a raw public key or an
// extended (xpub/xprv) key with an optional key-origin prefix and a derivation
// path that may contain a multipath element and a wildcard.
type descKey struct {
	// raw is the original string of the key as it appears in the
	// descriptor.
	raw string

	// form is the serialization the position the key appears in requires.
	form keyForm

	// xpub is the parsed extended key, or nil if this is a raw public key.
	xpub *hdkeychain.ExtendedKey

	// rawKey holds the bytes of a raw public key (33-byte compressed,
	// 65-byte uncompressed or 32-byte x-only), or nil for an extended key.
	// For a WIF encoded private key it holds the public key of that private
	// key, serialized the way the WIF asks for.
	rawKey []byte

	// steps is the derivation path following the extended key.
	steps []pathStep
}

// parseDescKey parses a single descriptor public key that appears in a position
// requiring the given key form.
func parseDescKey(s string, form keyForm) (*descKey, error) {
	k := &descKey{raw: s, form: form}

	body := s

	// The origin records an already-taken path. Validate it, but do not
	// apply it again when deriving children of the supplied key.
	if strings.HasPrefix(body, "[") {
		end := strings.Index(body, "]")
		if end < 0 {
			return nil, fmt.Errorf("unterminated key "+
				"origin in %q", s)
		}
		if err := checkKeyOrigin(body[1:end]); err != nil {
			return nil, fmt.Errorf("invalid key origin in %q: %w",
				s, err)
		}

		body = body[end+1:]
	}

	// The key is the part before the first path separator. We use
	// strings.Cut rather than strings.Split so that a key with no
	// derivation path (the common case) does not allocate a slice.
	keyStr, pathStr, hasPath := strings.Cut(body, "/")

	// A raw public key is pure hex; an extended key starts with a base58
	// string that fails to hex-decode.
	if rawKey, err := hex.DecodeString(keyStr); err == nil &&
		isPubKeyLen(len(rawKey)) {

		if hasPath {
			return nil, fmt.Errorf("raw key %q must not have a "+
				"derivation path", keyStr)
		}
		if err := form.checkRawKey(rawKey); err != nil {
			return nil, err
		}

		k.rawKey = rawKey
		return k, nil
	}

	// A WIF encoded private key is a valid key expression (BIP380); the key
	// it contributes to the script is its public key, serialized compressed
	// or uncompressed as the WIF says. The network the WIF names is
	// ignored, as it is for an extended key: the network is a parameter of
	// address derivation here, not of the descriptor.
	if wif, err := btcutil.DecodeWIF(keyStr); err == nil {
		if hasPath {
			return nil, fmt.Errorf("private key %q must not have "+
				"a derivation path", keyStr)
		}

		rawKey := wif.SerializePubKey()
		if err := form.checkRawKey(rawKey); err != nil {
			return nil, err
		}

		k.rawKey = rawKey
		return k, nil
	}

	xpub, err := hdkeychain.NewKeyFromString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid key %q: %w", keyStr, err)
	}

	// An extended private key computes its public key lazily and caches it
	// in the hdkeychain.ExtendedKey, i.e. the first derivation from it
	// writes to a key that the descriptor keeps for its whole lifetime.
	// Filling that cache here, before the key is retained, keeps every
	// later derivation a read of immutable state, so a Descriptor holding
	// an xprv can be shared across goroutines like any other. The child
	// keys a derivation computes are local to it and need no such
	// treatment.
	if xpub.IsPrivate() {
		if _, err := xpub.ECPubKey(); err != nil {
			return nil, fmt.Errorf("invalid private extended "+
				"key %q: %w", keyStr, err)
		}
	}

	k.xpub = xpub

	if !hasPath {
		return k, nil
	}

	pathParts := strings.Split(pathStr, "/")
	k.steps = make([]pathStep, 0, len(pathParts))

	multipathSeen := false
	for i, p := range pathParts {
		step, err := parsePathStep(p)
		if err != nil {
			return nil, err
		}

		switch step.kind {
		case stepMultipath:
			if multipathSeen {
				return nil, fmt.Errorf("multiple multipath "+
					"elements in %q", s)
			}
			multipathSeen = true

		case stepWildcard:
			if i != len(pathParts)-1 {
				return nil, fmt.Errorf("wildcard must be the "+
					"last path element in %q", s)
			}
		}

		k.steps = append(k.steps, step)
	}

	return k, nil
}

// isPubKeyLen returns whether the given byte length is a valid public key
// length: 32 (x-only), 33 (compressed) or 65 (uncompressed).
func isPubKeyLen(n int) bool {
	return n == xOnlyKeyLen || n == compressedKeyLen ||
		n == uncompressedKeyLen
}

// checkRawKey returns an error if a raw public key of the given serialization
// must not appear in a position that requires this key form.
func (f keyForm) checkRawKey(rawKey []byte) error {
	switch f {
	// Legacy positions take compressed and uncompressed keys, but not
	// x-only ones, which BIP386 only defines inside tr().
	case keyFormLegacy:
		if len(rawKey) == xOnlyKeyLen {
			return fmt.Errorf("x-only public key %x is only valid "+
				"inside a tr() descriptor", rawKey)
		}

	// Segwit v0 only takes compressed keys (BIP382), and so does
	// miniscript (BIP379).
	case keyFormCompressed:
		if len(rawKey) == uncompressedKeyLen {
			return fmt.Errorf("uncompressed public key %x is not "+
				"valid in a segwit v0 or miniscript context",
				rawKey)
		}
		if len(rawKey) == xOnlyKeyLen {
			return fmt.Errorf("x-only public key %x is only valid "+
				"inside a tr() descriptor", rawKey)
		}

	// Taproot takes x-only keys and compressed keys, which it converts to
	// x-only, but no uncompressed keys (BIP386).
	case keyFormXOnly:
		if len(rawKey) == uncompressedKeyLen {
			return fmt.Errorf("uncompressed public key %x is not "+
				"valid inside a tr() descriptor", rawKey)
		}
	}

	return nil
}

// checkKeyOrigin validates the content of a key-origin prefix, i.e. what stands
// between the brackets of "[fingerprint/path]": exactly eight hex characters
// for the fingerprint of the key the derivation started from, followed by zero
// or more hardened or unhardened derivation steps (BIP380).
//
// The origin says how the key was derived, which is not needed to derive from
// the key itself, but a malformed one still makes the descriptor invalid: other
// implementations reject it, so it is neither portable nor a faithful record of
// what its author wrote.
func checkKeyOrigin(origin string) error {
	fingerprint, path, hasPath := strings.Cut(origin, "/")

	if len(fingerprint) != keyOriginFingerprintLen {
		return fmt.Errorf("fingerprint %q must be exactly %d hex "+
			"characters", fingerprint, keyOriginFingerprintLen)
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return fmt.Errorf("fingerprint %q is not hex", fingerprint)
	}

	if !hasPath {
		return nil
	}

	// Only fixed derivation steps are allowed: a key origin describes a
	// path that was already taken, so it cannot hold a wildcard or a
	// multipath element. A trailing slash leaves an empty element, which is
	// rejected as well.
	for element := range strings.SplitSeq(path, "/") {
		step, err := parsePathStep(element)
		if err != nil {
			return err
		}
		if step.kind != stepNum {
			return fmt.Errorf("element %q is not a derivation "+
				"index", element)
		}
	}

	return nil
}

// parsePathStep parses a single derivation-path element.
func parsePathStep(p string) (pathStep, error) {
	switch {
	// A wildcard stands for every direct child, hardened if it carries a
	// hardened indicator (BIP380). Only an extended private key can derive
	// a hardened child, which is checked at derivation time rather than
	// here, since the expression itself is valid either way.
	case p == "*", p == "*'", p == "*h":
		return pathStep{
			kind:  stepWildcard,
			index: pathIndex{hardened: p != "*"},
		}, nil

	case strings.HasPrefix(p, "<") && strings.HasSuffix(p, ">"):
		inner := p[1 : len(p)-1]
		elems := strings.Split(inner, ";")
		if len(elems) < 2 {
			return pathStep{}, fmt.Errorf("multipath %q must have "+
				"at least two elements", p)
		}

		values := make([]pathIndex, len(elems))
		seen := make(map[uint32]struct{}, len(elems))
		for i, e := range elems {
			index, err := parsePathIndex(e)
			if err != nil {
				return pathStep{}, err
			}

			// BIP389 does not allow a multipath element to repeat
			// an index, which would derive the same sub-descriptor
			// twice.
			if _, ok := seen[index.childIndex()]; ok {
				return pathStep{}, fmt.Errorf("multipath %q "+
					"holds the index %v more than once", p,
					index)
			}
			seen[index.childIndex()] = struct{}{}

			values[i] = index
		}

		return pathStep{kind: stepMultipath, multipath: values}, nil

	default:
		index, err := parsePathIndex(p)
		if err != nil {
			return pathStep{}, err
		}

		return pathStep{kind: stepNum, index: index}, nil
	}
}

// parsePathIndex parses a derivation index with an optional hardened indicator,
// which is "h" or "'" (BIP380).
func parsePathIndex(s string) (pathIndex, error) {
	hardened := strings.HasSuffix(s, "'") || strings.HasSuffix(s, "h")
	numStr := s
	if hardened {
		numStr = s[:len(s)-1]
	}

	num, err := parseIndex(numStr)
	if err != nil {
		return pathIndex{}, err
	}

	return pathIndex{num: num, hardened: hardened}, nil
}

// parseIndex parses a non-negative derivation index below the hardened range.
func parseIndex(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid derivation index %q: %w", s, err)
	}
	if n >= hdkeychain.HardenedKeyStart {
		return 0, fmt.Errorf("derivation index %q out of range", s)
	}
	return uint32(n), nil
}

// multipathLen returns the number of multipath sub-descriptors this key
// contributes: the length of its multipath element, or 1 if it has none.
func (k *descKey) multipathLen() int {
	for _, step := range k.steps {
		if step.kind == stepMultipath {
			return len(step.multipath)
		}
	}
	return 1
}

// definiteString returns the canonical string of the key with its path fully
// resolved at the given multipath and derivation index: the multipath element
// is replaced by its multipath-index value and the wildcard by the derivation
// index. This matches rust-miniscript's DefiniteDescriptorKey display and is
// the identifier passed to the plan asset and satisfier lookups.
func (k *descKey) definiteString(multipathIndex,
	derivationIndex uint32) string {

	// Keep any optional "[origin]" prefix verbatim, since it contains its
	// own path separators.
	origin, rest := "", k.raw
	if strings.HasPrefix(rest, "[") {
		if end := strings.IndexByte(rest, ']'); end >= 0 {
			origin, rest = rest[:end+1], rest[end+1:]
		}
	}

	// The remainder is the key followed by its path steps; the key is the
	// part before the first separator.
	base := rest
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		base = rest[:slash]
	}

	var b strings.Builder
	b.WriteString(origin)
	b.WriteString(base)
	for _, step := range k.steps {
		b.WriteByte('/')
		switch step.kind {
		case stepMultipath:
			b.WriteString(step.multipath[multipathIndex].String())

		case stepWildcard:
			// The wildcard resolves to the derivation index, which
			// keeps the hardened indicator of the wildcard itself.
			resolved := pathIndex{
				num:      derivationIndex,
				hardened: step.index.hardened,
			}
			b.WriteString(resolved.String())

		default:
			b.WriteString(step.index.String())
		}
	}

	return b.String()
}

// isWildcard returns whether the key has a wildcard element and is therefore
// ranged.
func (k *descKey) isWildcard() bool {
	for _, step := range k.steps {
		if step.kind == stepWildcard {
			return true
		}
	}
	return false
}

// derivePub derives the concrete public key at the given multipath and
// derivation index.
func (k *descKey) derivePub(
	multipathIndex, derivationIndex uint32) (*btcec.PublicKey, error) {

	if k.rawKey != nil {
		if len(k.rawKey) == 32 {
			return schnorr.ParsePubKey(k.rawKey)
		}
		return btcec.ParsePubKey(k.rawKey)
	}

	cur := k.xpub
	for _, step := range k.steps {
		var index pathIndex
		switch step.kind {
		case stepNum:
			index = step.index

		case stepMultipath:
			if uint64(multipathIndex) >=
				uint64(len(step.multipath)) {

				return nil, fmt.Errorf("multipath index out " +
					"of bounds")
			}
			index = step.multipath[multipathIndex]

		case stepWildcard:
			// The derivation index has to stay below the hardened
			// range, otherwise it would silently turn the wildcard
			// into a hardened derivation.
			if derivationIndex >= hdkeychain.HardenedKeyStart {
				return nil, fmt.Errorf("derivation index %d is "+
					"in the hardened range",
					derivationIndex)
			}
			index = pathIndex{
				num:      derivationIndex,
				hardened: step.index.hardened,
			}
		}

		// A hardened child can only be derived from a private extended
		// key, so a descriptor that names one is valid but not
		// derivable from an xpub.
		if index.hardened && !cur.IsPrivate() {
			return nil, fmt.Errorf("cannot derive the hardened "+
				"child %v of %q from an extended public key",
				index, k.raw)
		}

		var err error
		cur, err = cur.Derive(index.childIndex())
		if err != nil {
			return nil, err
		}
	}

	return cur.ECPubKey()
}

// derive derives the concrete public key at the given indices and serializes it
// in the form the key's position requires: 32-byte x-only in a taproot
// position, 33-byte compressed in a segwit v0 or miniscript position, and
// verbatim in a legacy position, where an uncompressed key stays uncompressed.
func (k *descKey) derive(multipathIndex, derivationIndex uint32) ([]byte,
	error) {

	pub, err := k.derivePub(multipathIndex, derivationIndex)
	if err != nil {
		return nil, err
	}

	switch {
	case k.form == keyFormXOnly:
		return schnorr.SerializePubKey(pub), nil

	// An uncompressed raw key in a legacy position is used as written,
	// since that is the key material the descriptor commits to: compressing
	// it would produce a different script, and therefore a different
	// address, than every other implementation derives (BIP381, BIP383).
	case k.form == keyFormLegacy && len(k.rawKey) == uncompressedKeyLen:
		return pub.SerializeUncompressed(), nil

	default:
		return pub.SerializeCompressed(), nil
	}
}

// pushLen returns the number of bytes the key takes up in a script: its
// serialized length plus the one-byte push opcode.
func (k *descKey) pushLen() int {
	switch {
	case k.form == keyFormXOnly:
		return 1 + xOnlyKeyLen

	case k.form == keyFormLegacy && len(k.rawKey) == uncompressedKeyLen:
		return 1 + uncompressedKeyLen

	default:
		return 1 + compressedKeyLen
	}
}
