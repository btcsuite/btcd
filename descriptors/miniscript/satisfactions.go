package miniscript

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// SignFunc is a function type that returns a signature for a pubkey or false if
// no signer is available.
type SignFunc func(pubKey []byte) (signature []byte, available bool)

// PreimageFunc is a function type that returns the preimage of a hash value.
type PreimageFunc func(hashFunc string, hash []byte) (preimage []byte,
	available bool)

// Satisfier supplies transaction-specific signatures, preimages and timelock
// checks for the keys and hash locks in a Miniscript expression.
//
// A function only has to be set if the expression contains a fragment that
// needs it, i.e. Sign for pk_k/pk_h/multi/multi_a, CheckOlder for older,
// CheckAfter for after and Preimage for the hash fragments. Satisfy reports a
// missing function as an error naming it.
type Satisfier struct {
	// CheckOlder checks if the OP_CHECKSEQUENCEVERIFY call is satisfied in
	// the context of a transaction. Use the `CheckOlder` utility function.
	CheckOlder func(locktime uint32) (bool, error)

	// CheckAfter checks if the OP_CHECKLOCKTIMEVERIFY call is satisfied in
	// the context of a transaction. Use the `CheckAfter` utility function.
	CheckAfter func(locktime uint32) (bool, error)

	// Sign returns a signature for the pubkey or false if a signer is not
	// available.
	Sign SignFunc

	// Preimage returns the preimage of the hash value. hashFunc is one of
	// "sha256", "ripemd160", "hash256", "hash160".
	Preimage PreimageFunc
}

// check returns an error if the satisfier is missing a function that satisfying
// the given expression needs. Without it, satisfy() would call a nil function
// value and panic with a nil-pointer dereference, which is a particularly
// unpleasant way for a library to report a partially populated struct.
func (s *Satisfier) check(node *AST) error {
	if s == nil {
		return errors.New("no satisfier provided")
	}

	missing := func(name, fragment string) error {
		return fmt.Errorf("satisfier has no %s function, which is "+
			"required to satisfy the %s fragment", name, fragment)
	}

	// The transformation is the identity, so this only walks the tree.
	_, err := node.apply(func(node *AST) (*AST, error) {
		switch node.identifier {
		case f_pk_k, f_pk_h, f_multi, f_multi_a:
			if s.Sign == nil {
				return nil, missing("Sign", node.identifier)
			}

		case f_older:
			if s.CheckOlder == nil {
				return nil, missing(
					"CheckOlder", node.identifier,
				)
			}

		case f_after:
			if s.CheckAfter == nil {
				return nil, missing(
					"CheckAfter", node.identifier,
				)
			}

		case f_sha256, f_hash256, f_ripemd160, f_hash160:
			if s.Preimage == nil {
				return nil, missing("Preimage", node.identifier)
			}
		}

		return node, nil
	})

	return err
}

// satisfaction is a struct based on `InputStack` of the Bitcoin Core
// implementation at
// https://github.com/bitcoin/bitcoin/blob/a13f374/src/script/miniscript.cpp
type satisfaction struct {
	// witness is a list of data elements that will be pushed onto the
	// witness stack.
	witness wire.TxWitness

	// available, if false, indicates there is no valid satisfaction (i.e.
	// private key or hash preimage not available, time lock not yet valid,
	// generally not satisfiable, etc.).
	available bool

	// malleable, if true, indicates the satisfaction is malleable by a
	// third party.
	malleable bool

	// hasSig marks a candidate containing a signature. It distinguishes
	// alternatives that an outsider can construct without signing; it does
	// not by itself imply the candidate is non-malleable.
	hasSig bool
}

// setAvailable updates whether the candidate can be constructed and returns s.
func (s *satisfaction) setAvailable(available bool) *satisfaction {
	s.available = available
	return s
}

// withSig marks the candidate as requiring a signature and returns s.
func (s *satisfaction) withSig() *satisfaction {
	s.hasSig = true
	return s
}

// setMalleable updates the candidate's malleability marker and returns s.
func (s *satisfaction) setMalleable(malleable bool) *satisfaction {
	s.malleable = malleable
	return s
}

// and concatenates two required candidates, placing b above s on the stack.
// The outer witness slice is new, but the element bytes remain shared.
func (s *satisfaction) and(b *satisfaction) *satisfaction {
	witness := append(wire.TxWitness{}, s.witness...)
	return &satisfaction{
		witness:   append(witness, b.witness...),
		available: s.available && b.available,
		malleable: s.malleable || b.malleable,
		hasSig:    s.hasSig || b.hasSig,
	}
}

// or selects between alternatives using BIP379's signature/malleability
// preferences and then serialized size. Ties favor s. It returns an input
// candidate and may mark both inputs malleable when neither requires signing.
func (s *satisfaction) or(b *satisfaction) *satisfaction {
	// If only one (or neither) is valid, pick the other one.
	if !s.available {
		return b
	}
	if !b.available {
		return s
	}

	// If only one of the solutions has a signature, we must pick the other
	// one.
	if !s.hasSig && b.hasSig {
		return s
	}
	if s.hasSig && !b.hasSig {
		return b
	}
	if !s.hasSig && !b.hasSig {
		// If neither solution requires a signature, the result is
		// inevitably malleable.
		s.malleable = true
		b.malleable = true
	} else {
		// If both options require a signature, prefer the non-malleable
		// one.
		if b.malleable && !s.malleable {
			return s
		}
		if s.malleable && !b.malleable {
			return b
		}
	}

	// Both candidates are available after the early returns. Keep the
	// smaller witness, preserving left preference when sizes are equal.
	if s.witness.SerializeSize() <= b.witness.SerializeSize() {
		return s
	}

	return b
}

// satisfactions keeps the best available satisfaction and dissatisfaction of
// one fragment so its parent can combine either outcome with sibling results.
type satisfactions struct {
	dsat, sat *satisfaction
}

// verifyLockTime compares values only when they use the same units: heights
// below threshold, or times at/above it. Equality satisfies the lower bound.
func verifyLockTime(txLockTime uint32, threshold uint32, lockTime uint32) bool {
	// Numeric ordering is meaningful only if both values measure blocks or
	// both measure time. The threshold separates those two unit systems.
	txUsesHeight := txLockTime < threshold
	lockUsesHeight := lockTime < threshold
	if txUsesHeight != lockUsesHeight {
		return false
	}

	return lockTime <= txLockTime
}

// CheckOlder checks if the OP_CHECKSEQUENCEVERIFY (BIP112, BIP68) call is
// satisfied given the lock time value.
//
// txVersion is the version of the transaction being signed, which is a signed
// field in the transaction serialization. OP_CHECKSEQUENCEVERIFY requires it to
// be at least 2, otherwise the script fails.
//
// txInputSequence should be set to the sequence field of the input that is
// being signed. It is compared to the lock time value.
func CheckOlder(lockTime uint32, txVersion int32, txInputSequence uint32) bool {
	// See BIP68. Mask off non-consensus bits before doing comparisons.
	lockTimeMask := uint32(
		wire.SequenceLockTimeIsSeconds | wire.SequenceLockTimeMask,
	)
	return txInputSequence&wire.SequenceLockTimeDisabled == 0 &&
		txVersion >= 2 &&
		verifyLockTime(
			txInputSequence&lockTimeMask,
			wire.SequenceLockTimeIsSeconds, lockTime&lockTimeMask,
		)
}

// CheckAfter checks if the OP_CHECKLOCKTIMEVERIFY (BIP65) call is satisfied
// given the lock time value.
//
// TxLockTime is the nLockTime of the transaction that is being signed. It is
// compared to the lock time value.
//
// txInputSequence should be set to the sequence field of the input that is
// being signed. According to BIP65, it must be smaller than 0xFFFFFFFF (maximum
// value) for this OP-code to not abort.
func CheckAfter(value uint32, txLockTime uint32, txInputSequence uint32) bool {
	return txInputSequence != wire.MaxTxInSequenceNum &&
		verifyLockTime(txLockTime, txscript.LockTimeThreshold, value)
}

// satisfy is based on `ProduceInput()` of the Bitcoin Core implementation at:
// https://github.com/bitcoin/bitcoin/blob/a13f374/src/script/miniscript.h#L850
func satisfy(node *AST, satisfier *Satisfier) (*satisfactions, error) {
	zero := func() *satisfaction {
		// Empty data translates to OP_0/OP_FALSE (push zero bytes)
		return &satisfaction{
			witness:   wire.TxWitness{{}},
			available: true,
		}
	}
	one := func() *satisfaction {
		return &satisfaction{
			witness:   wire.TxWitness{{1}},
			available: true,
		}
	}
	empty := func() *satisfaction {
		return &satisfaction{
			witness:   wire.TxWitness{},
			available: true,
		}
	}
	unavailable := func() *satisfaction {
		return &satisfaction{available: false}
	}
	witness := func(w []byte) *satisfaction {
		return &satisfaction{
			witness:   wire.TxWitness{w},
			available: true,
		}
	}

	switch node.identifier {
	case f_0:
		return &satisfactions{
			dsat: empty(),
			sat:  unavailable(),
		}, nil

	case f_1:
		return &satisfactions{
			dsat: unavailable(),
			sat:  empty(),
		}, nil

	case f_pk_k:
		arg := node.args[0]
		key := arg.value
		if arg.value == nil {
			return nil, fmt.Errorf("empty key for %s (%s)",
				node.identifier, arg.identifier)
		}
		sig, available := satisfier.Sign(key)
		return &satisfactions{
			dsat: zero(),
			sat:  witness(sig).withSig().setAvailable(available),
		}, nil

	case f_pk_h:
		arg := node.args[0]
		key := arg.value
		if arg.value == nil {
			return nil, fmt.Errorf("empty key for %s (%s)",
				node.identifier, arg.identifier)
		}
		sig, available := satisfier.Sign(key)
		return &satisfactions{
			dsat: zero().and(witness(key)),
			// The satisfaction requires a signature, just like
			// pk_k, so it must be marked with withSig(). Without
			// this, a pk_h in a dissatisfied thresh position is
			// wrongly treated as a no-signature malleability
			// vector.
			sat: witness(sig).withSig().setAvailable(available).and(
				witness(key),
			),
		}, nil

	case f_older:
		// BIP112 - OP_CHECKSEQUENCEVERIFY
		value := node.args[0].num
		satisfied, err := satisfier.CheckOlder(uint32(value))
		if err != nil {
			return nil, err
		}
		if satisfied {
			return &satisfactions{
				dsat: unavailable(),
				sat:  empty(),
			}, nil
		}
		return &satisfactions{
			dsat: unavailable(),
			sat:  unavailable(),
		}, nil

	case f_after:
		// BIP65 - OP_CHECKLOCKTIMEVERIFY
		value := node.args[0].num
		satisfied, err := satisfier.CheckAfter(uint32(value))
		if err != nil {
			return nil, err
		}
		if satisfied {
			return &satisfactions{
				dsat: unavailable(),
				sat:  empty(),
			}, nil
		}
		return &satisfactions{
			dsat: unavailable(),
			sat:  unavailable(),
		}, nil

	case f_sha256, f_ripemd160, f_hash256, f_hash160:
		hashValue := node.args[0].value
		if hashValue == nil {
			return nil, fmt.Errorf("hash value empty for %s (%s)",
				node.identifier, node.args[0].identifier)
		}
		preimage, available := satisfier.Preimage(
			node.identifier, hashValue,
		)
		if available && len(preimage) != 32 {
			return nil, fmt.Errorf("length of %s preimage of %x "+
				"of expected to be 32, got %d",
				node.identifier, hashValue, len(preimage))
		}
		sat := witness(preimage).setAvailable(available)
		return &satisfactions{
			// Preimage 0x0000... is assumed invalid.
			dsat: witness(make([]byte, 32)).setMalleable(true),
			sat:  sat,
		}, nil

	case f_andor:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		y, err := satisfy(node.args[1], satisfier)
		if err != nil {
			return nil, err
		}
		z, err := satisfy(node.args[2], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: z.dsat.and(x.dsat).or(y.dsat.and(x.sat)),
			sat:  y.sat.and(x.sat).or(z.sat.and(x.dsat)),
		}, nil

	case f_and_v:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		y, err := satisfy(node.args[1], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: y.dsat.and(x.sat),
			sat:  y.sat.and(x.sat),
		}, nil

	case f_and_b:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		y, err := satisfy(node.args[1], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: y.dsat.and(x.dsat).or(
				y.sat.and(x.dsat).setMalleable(true),
			).or(
				y.dsat.and(x.sat).setMalleable(true),
			),
			sat: y.sat.and(x.sat),
		}, nil

	case f_or_b:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		z, err := satisfy(node.args[1], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: z.dsat.and(x.dsat),
			sat: z.dsat.and(x.sat).or(
				z.sat.and(x.dsat),
			).or(
				z.sat.and(x.sat).setMalleable(true),
			),
		}, nil

	case f_or_c:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		z, err := satisfy(node.args[1], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: unavailable(),
			sat:  x.sat.or(z.sat.and(x.dsat)),
		}, nil

	case f_or_d:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		z, err := satisfy(node.args[1], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: z.dsat.and(x.dsat),
			sat:  x.sat.or(z.sat.and(x.dsat)),
		}, nil

	case f_or_i:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		z, err := satisfy(node.args[1], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: x.dsat.and(one()).or(z.dsat.and(zero())),
			sat:  x.sat.and(one()).or(z.sat.and(zero())),
		}, nil

	case f_thresh:
		k := int(node.args[0].num)
		n := len(node.args) - 1
		subSats := make([]*satisfactions, n)
		for i, arg := range node.args[1:] {
			sat, err := satisfy(arg, satisfier)
			if err != nil {
				return nil, err
			}
			subSats[i] = sat
		}

		// Dissatisfaction of a threshold dissatisfies every sub
		// expression. The sub expressions are concatenated so that the
		// first one's witness ends up on top of the stack.
		dsat := empty()
		for _, sub := range subSats {
			dsat = sub.dsat.and(dsat)
		}

		// Satisfaction satisfies exactly k of the n sub expressions. We
		// start by dissatisfying everything and then satisfy the best
		// k. This mirrors rust-miniscript's non-malleable `thresh`
		// algorithm and runs in O(n log n) instead of enumerating all
		// subsets.
		chosen := make([]*satisfaction, n)
		for i, sub := range subSats {
			chosen[i] = sub.dsat
		}

		// Order the sub expressions so that the best candidates to
		// satisfy come first. We prefer, in order: sub expressions we
		// can satisfy at all; satisfactions without a signature (a
		// third party could satisfy an unpicked one, making the result
		// malleable, so we must pick those); and finally the smallest
		// witness.
		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		weight := func(i int) int {
			sat, dsat := subSats[i].sat, subSats[i].dsat
			switch {
			case !dsat.available:
				// Cannot be dissatisfied, so it must be
				// satisfied: sort first.
				return math.MinInt

			case !sat.available:
				// Cannot be satisfied: sort last.
				return math.MaxInt

			default:
				return sat.witness.SerializeSize() -
					dsat.witness.SerializeSize()
			}
		}
		slices.SortStableFunc(order, func(i, j int) int {
			if subSats[i].sat.available !=
				subSats[j].sat.available {

				if subSats[i].sat.available {
					return -1
				}
				return 1
			}
			if subSats[i].sat.hasSig != subSats[j].sat.hasSig {
				if subSats[i].sat.hasSig {
					return 1
				}
				return -1
			}
			return cmp.Compare(weight(i), weight(j))
		})

		// Satisfy the best k sub expressions.
		for i := range k {
			chosen[order[i]] = subSats[order[i]].sat
		}

		var sat *satisfaction
		switch {
		case !subSats[order[k-1]].sat.available:
			// Fewer than k sub expressions can be satisfied.
			sat = unavailable()

		case k < n && subSats[order[k]].sat.available &&
			!subSats[order[k]].sat.hasSig:

			// A satisfiable sub expression that requires no
			// signature was left unpicked; a third party could add
			// it, so there is no non-malleable satisfaction.
			sat = unavailable()

		default:
			// Concatenate the chosen (dis)satisfactions, first sub
			// expression on top of the stack.
			sat = empty()
			for _, candidate := range chosen {
				sat = candidate.and(sat)
			}
		}

		return &satisfactions{
			dsat: dsat,
			sat:  sat,
		}, nil

	case f_multi:
		k := int(node.args[0].num)

		// Dissatisfaction is a dummy `0` for the OP_CHECKMULTISIG
		// off-by-one bug, followed by k zeros, one per required
		// signature.
		dsat := zero()
		for range k {
			dsat = dsat.and(zero())
		}

		// Collect the available signatures in key order, which is the
		// order OP_CHECKMULTISIG requires them in.
		var sigs [][]byte
		for _, arg := range node.args[1:] {
			key := arg.value
			if key == nil {
				return nil, fmt.Errorf("empty key for %s (%s)",
					node.identifier, arg.identifier)
			}
			sig, available := satisfier.Sign(key)
			if available {
				sigs = append(sigs, sig)
			}
		}

		// If fewer than k signatures are available, there is no
		// satisfaction.
		if len(sigs) < k {
			return &satisfactions{
				dsat: dsat,
				sat:  unavailable(),
			}, nil
		}

		// We only need k signatures. Drop the largest ones, keeping the
		// remaining signatures in key order, so the witness is as small
		// as possible.
		for len(sigs) > k {
			maxIdx := 0
			for i := range sigs {
				if len(sigs[i]) > len(sigs[maxIdx]) {
					maxIdx = i
				}
			}
			sigs = slices.Delete(sigs, maxIdx, maxIdx+1)
		}

		// The satisfaction is a dummy `0` for the OP_CHECKMULTISIG
		// off-by-one bug, followed by the k signatures in key order.
		sat := zero()
		for _, sig := range sigs {
			sat = sat.and(witness(sig).withSig())
		}
		return &satisfactions{
			dsat: dsat,
			sat:  sat,
		}, nil

	case f_multi_a:
		k := int(node.args[0].num)
		n := len(node.args) - 1

		// Dissatisfaction pushes an empty element for every key, so
		// that every OP_CHECKSIGADD adds zero and the final count is
		// 0 != k.
		dsat := empty()
		for range n {
			dsat = dsat.and(zero())
		}

		// Collect a signature for each key, if available.
		sigs := make([][]byte, n)
		available := 0
		for i, arg := range node.args[1:] {
			key := arg.value
			if key == nil {
				return nil, fmt.Errorf("empty key for %s (%s)",
					node.identifier, arg.identifier)
			}
			if sig, ok := satisfier.Sign(key); ok {
				sigs[i] = sig
				available++
			}
		}

		// Fewer than k available signatures means no satisfaction.
		if available < k {
			return &satisfactions{
				dsat: dsat,
				sat:  unavailable(),
			}, nil
		}

		// The first key is checked against the top witness element, so
		// the witness is built in reverse key order. We push a
		// signature for exactly k of the signable keys (preferring the
		// last ones, like rust-miniscript) and an empty element for all
		// others.
		sat := empty()
		placed := 0
		for i := n - 1; i >= 0; i-- {
			if sigs[i] != nil && placed < k {
				sat = sat.and(witness(sigs[i]).withSig())
				placed++
			} else {
				sat = sat.and(zero())
			}
		}
		return &satisfactions{
			dsat: dsat,
			sat:  sat,
		}, nil

	case f_wrap_a, f_wrap_s, f_wrap_c, f_wrap_n:
		return satisfy(node.args[0], satisfier)

	case f_wrap_d:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: zero(),
			sat:  x.sat.and(one()),
		}, nil

	case f_wrap_v:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: unavailable(),
			sat:  x.sat,
		}, nil

	case f_wrap_j:
		x, err := satisfy(node.args[0], satisfier)
		if err != nil {
			return nil, err
		}
		return &satisfactions{
			dsat: zero().setMalleable(
				x.dsat.available && !x.dsat.hasSig,
			),
			sat: x.sat,
		}, nil

	default:
		return nil, fmt.Errorf("unrecognized identifier: %s",
			node.identifier)
	}
}
