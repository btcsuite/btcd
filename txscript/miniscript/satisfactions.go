package miniscript

import (
	"fmt"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// SignFunc is a function type that returns a signature for a pubkey or false if
// no signer is available.
type SignFunc func(pubKey []byte) (signature []byte, available bool)

// PreimageFunc is a function type that returns the preimage of a hash value.
type PreimageFunc func(hashFunc string, hash []byte) (preimage []byte,
	available bool)

// Satisfier is provided to the satisfier to generate signatures for pubkeys and
// preimages to hash values that occur in the miniscript.
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

	// hasSig indicates this satisfaction requires a signature, which means
	// a third party cannot malleate this satisfaction even if `malleable`
	// is true. If `malleable` and `hasSig` is true, only we (the
	// key-holders) can malleate this satisfaction.
	hasSig bool
}

func (s *satisfaction) setAvailable(available bool) *satisfaction {
	s.available = available
	return s
}

func (s *satisfaction) withSig() *satisfaction {
	s.hasSig = true
	return s
}

func (s *satisfaction) setMalleable(malleable bool) *satisfaction {
	s.malleable = malleable
	return s
}

func (s *satisfaction) and(b *satisfaction) *satisfaction {
	witness := append(wire.TxWitness{}, s.witness...)
	return &satisfaction{
		witness:   append(witness, b.witness...),
		available: s.available && b.available,
		malleable: s.malleable || b.malleable,
		hasSig:    s.hasSig || b.hasSig,
	}
}

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

	// Both available, pick smaller one.
	if s.available && b.available {
		if s.witness.SerializeSize() <= b.witness.SerializeSize() {
			return s
		}
		return b
	}
	// If only one available, return that one. If both unavailable, the
	// result is unavailable.
	if s.available {
		return s
	}
	return b
}

type satisfactions struct {
	dsat, sat *satisfaction
}

// subsets returns all subsets of the set {0, ..., n-1} of length k.
// - Written by ChatGPT.
func subsets(n int, k int) [][]int {
	type stackItem struct {
		subset []int
		start  int
	}

	var subsets [][]int
	stack := []stackItem{{
		subset: []int{},
		start:  0,
	}}

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if len(current.subset) == k {
			subsets = append(subsets, current.subset)
			continue
		}

		for i := current.start; i < n; i++ {
			newSubset := append([]int{}, current.subset...)
			newSubset = append(newSubset, i)
			stack = append(stack, stackItem{
				subset: newSubset,
				start:  i + 1,
			})
		}
	}

	return subsets
}

func containsInt(ints []int, i int) bool {
	for _, el := range ints {
		if el == i {
			return true
		}
	}
	return false
}

func verifyLockTime(txLockTime uint32, threshold uint32, lockTime uint32) bool {
	if !((txLockTime < threshold && lockTime < threshold) ||
		(txLockTime >= threshold && lockTime >= threshold)) {

		// Can't mix time lock types (blocks vs time).
		return false
	}
	return lockTime <= txLockTime
}

// CheckOlder checks if the OP_CHECKSEQUENCEVERIFY (BIP112, BIP68) call is
// satisfied given the lock time value.
//
// txVersion is the version of the transaction being signed.
// OP_CHECKSEQUENCEVERIFY requires this to be at least 2, otherwise the script
// fails.
//
// txInputSequence should be set to the sequence field of the input that is
// being signed. It is compared to the lock time value.
func CheckOlder(lockTime uint32, txVersion uint32,
	txInputSequence uint32) bool {

	// See BIP68. Mask off non-consensus bits before doing comparisons.
	lockTimeMask := uint32(
		wire.SequenceLockTimeIsSeconds | wire.SequenceLockTimeMask,
	)
	return txInputSequence&wire.SequenceLockTimeDisabled == 0 &&
		txVersion >= 2 && verifyLockTime(
		txInputSequence&lockTimeMask,
		wire.SequenceLockTimeIsSeconds,
		lockTime&lockTimeMask,
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
			sat: witness(sig).setAvailable(available).and(
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
		k := node.args[0].num
		n := len(node.args) - 1
		subSats := make([]*satisfactions, n)
		for i, arg := range node.args[1:] {
			sat, err := satisfy(arg, satisfier)
			if err != nil {
				return nil, err
			}
			subSats[i] = sat
		}

		dsat := empty().setAvailable(false)
		sat := empty().setAvailable(false)

		// TODO: make more efficient, this is a naive implementation
		// that has 2^n loop iterations.
		for ks := 0; ks < n; ks++ {
			// Iterate over all subsets of length ks.
			for _, subset := range subsets(n, ks) {
				// The witness is the concatenation of all
				// subexpressions, ks of which are satisfied and
				// n-ks which are dissatisfied.
				candidateSat := empty()
				for i := 0; i < n; i++ {
					subSat := subSats[i]
					if containsInt(subset, i) {
						candidateSat = subSat.sat.and(
							candidateSat,
						)
					} else {
						candidateSat = subSat.dsat.and(
							candidateSat,
						)
					}
				}
				if ks == int(k) {
					// If exactly k subs are satisfied, it's
					// a valid satisfaction.
					sat = sat.or(candidateSat)
				} else {
					// Any other number of satisfied subs
					// results in an overall
					// dissatisfaction.
					dsat = dsat.or(candidateSat)
				}
			}
		}
		return &satisfactions{
			dsat: dsat,
			sat:  sat,
		}, nil

	case f_multi:
		k := node.args[0].num
		n := len(node.args) - 1
		dsat := zero()
		for i := uint64(0); i < k; i++ {
			dsat = dsat.and(zero())
		}

		// All actual signatures. If a sig is unavailable, it is left
		// empty.
		sigs := make([][]byte, n)
		for i, arg := range node.args[1:] {
			key := arg.value
			if arg.value == nil {
				return nil, fmt.Errorf("empty key for %s (%s)",
					node.identifier, arg.identifier)
			}
			sig, available := satisfier.Sign(key)
			if available {
				sigs[i] = sig
			}
		}

		sigsSat := empty().setAvailable(false)

		// TODO: make more efficient, this is a naive implementation
		// that has (n choose k) loop iterations.
		// Iterate over all k-subsets.
		for _, subset := range subsets(n, int(k)) {
			// Candidate satisfaction for one subset of keys:
			// `sig sig sig ...`.
			candidateSat := empty()
			for _, i := range subset {
				sigsetAvailable := len(sigs[i]) > 0
				candidateSat = candidateSat.and(
					witness(sigs[i]).withSig().setAvailable(
						sigsetAvailable,
					),
				)
			}
			sigsSat = sigsSat.or(candidateSat)
		}
		return &satisfactions{
			dsat: dsat,
			sat:  zero().and(sigsSat), // 0 sig sig sig ...
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
