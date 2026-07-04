package descriptors

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
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

// pathStep is one element of the derivation path of an extended key.
type pathStep struct {
	kind stepKind

	// num is the index for a stepNum, or the value that "'" was applied to
	// for a hardened step.
	num uint32

	// hardened is set for a hardened stepNum. Hardened steps cannot be
	// derived from a public extended key.
	hardened bool

	// multipath holds the values of a stepMultipath, in order.
	multipath []uint32
}

// descKey is a parsed descriptor public key: either a raw public key or an
// extended (xpub) key with an optional key-origin prefix and a derivation path
// that may contain a multipath element and a wildcard.
type descKey struct {
	// raw is the original, canonical string of the key as it appears in the
	// descriptor.
	raw string

	// xpub is the parsed extended key, or nil if this is a raw public key.
	xpub *hdkeychain.ExtendedKey

	// rawKey holds the bytes of a raw public key (33-byte compressed,
	// 65-byte uncompressed or 32-byte x-only), or nil for an extended key.
	rawKey []byte

	// steps is the derivation path following the extended key.
	steps []pathStep
}

// parseDescKey parses a single descriptor public key.
func parseDescKey(s string) (*descKey, error) {
	k := &descKey{raw: s}

	body := s

	// Strip an optional key-origin prefix "[fingerprint/...]". Its content
	// is informational for derivation, so we only validate its shape.
	if strings.HasPrefix(body, "[") {
		end := strings.Index(body, "]")
		if end < 0 {
			return nil, fmt.Errorf("unterminated key "+
				"origin in %q", s)
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
		k.rawKey = rawKey
		return k, nil
	}

	xpub, err := hdkeychain.NewKeyFromString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid key %q: %w", keyStr, err)
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
	return n == 32 || n == 33 || n == 65
}

// parsePathStep parses a single derivation-path element.
func parsePathStep(p string) (pathStep, error) {
	switch {
	case p == "*":
		return pathStep{kind: stepWildcard}, nil

	case p == "*'" || p == "*h":
		return pathStep{}, fmt.Errorf("hardened wildcard %q cannot be "+
			"derived from an extended public key", p)

	case strings.HasPrefix(p, "<") && strings.HasSuffix(p, ">"):
		inner := p[1 : len(p)-1]
		elems := strings.Split(inner, ";")
		if len(elems) < 2 {
			return pathStep{}, fmt.Errorf("multipath %q must have "+
				"at least two elements", p)
		}
		values := make([]uint32, len(elems))
		for i, e := range elems {
			n, err := parseIndex(e)
			if err != nil {
				return pathStep{}, err
			}
			values[i] = n
		}
		return pathStep{kind: stepMultipath, multipath: values}, nil

	default:
		hardened := strings.HasSuffix(p, "'") ||
			strings.HasSuffix(p, "h")
		numStr := p
		if hardened {
			numStr = p[:len(p)-1]
		}
		n, err := parseIndex(numStr)
		if err != nil {
			return pathStep{}, err
		}
		return pathStep{kind: stepNum, num: n, hardened: hardened}, nil
	}
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
func (k *descKey) derivePub(multipathIndex, derivationIndex uint32) (
	*btcec.PublicKey, error) {

	if k.rawKey != nil {
		if len(k.rawKey) == 32 {
			return schnorr.ParsePubKey(k.rawKey)
		}
		return btcec.ParsePubKey(k.rawKey)
	}

	cur := k.xpub
	for _, step := range k.steps {
		var childIndex uint32
		switch step.kind {
		case stepNum:
			if step.hardened {
				return nil, fmt.Errorf("cannot derive "+
					"hardened step from extended public "+
					"key in %q", k.raw)
			}
			childIndex = step.num

		case stepMultipath:
			if uint64(multipathIndex) >=
				uint64(len(step.multipath)) {

				return nil, fmt.Errorf("multipath index out " +
					"of bounds")
			}
			childIndex = step.multipath[multipathIndex]

		case stepWildcard:
			childIndex = derivationIndex
		}

		var err error
		cur, err = cur.Derive(childIndex)
		if err != nil {
			return nil, err
		}
	}

	return cur.ECPubKey()
}

// derive derives the concrete public key at the given indices and serializes it
// for the given context: 32-byte x-only for Taproot, 33-byte compressed
// otherwise.
func (k *descKey) derive(multipathIndex, derivationIndex uint32,
	xOnly bool) ([]byte, error) {

	pub, err := k.derivePub(multipathIndex, derivationIndex)
	if err != nil {
		return nil, err
	}

	if xOnly {
		return schnorr.SerializePubKey(pub), nil
	}
	return pub.SerializeCompressed(), nil
}
