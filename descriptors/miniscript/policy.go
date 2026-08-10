package miniscript

import "fmt"

// PolicyType identifies the type of a semantic policy node.
type PolicyType int

const (
	// PolicyUnsatisfiable is a policy that can never be satisfied.
	PolicyUnsatisfiable PolicyType = iota

	// PolicyTrivial is a policy that is always satisfied.
	PolicyTrivial

	// PolicyKey requires a signature for a public key.
	PolicyKey

	// PolicyAfter is an absolute locktime constraint.
	PolicyAfter

	// PolicyOlder is a relative locktime constraint.
	PolicyOlder

	// PolicySha256 requires a SHA256 preimage.
	PolicySha256

	// PolicyHash256 requires a double-SHA256 preimage.
	PolicyHash256

	// PolicyRipemd160 requires a RIPEMD160 preimage.
	PolicyRipemd160

	// PolicyHash160 requires a HASH160 preimage.
	PolicyHash160

	// PolicyThresh is a threshold combination of sub policies.
	PolicyThresh
)

// Policy is the abstract semantic policy of a miniscript: it captures what is
// required to satisfy the script while discarding the concrete script
// structure, enabling analysis such as normalization. It mirrors
// rust-miniscript's semantic Policy.
type Policy struct {
	// Type is the kind of this policy node.
	Type PolicyType

	// Key is the public key identifier, for PolicyKey.
	Key string

	// Locktime is the consensus locktime value, for PolicyAfter and
	// PolicyOlder.
	Locktime uint32

	// Hash is the raw hash bytes, for the hash policy types.
	Hash []byte

	// K is the required threshold, for PolicyThresh.
	K int

	// Subs are the sub policies, for PolicyThresh.
	Subs []*Policy
}

// Lift converts this miniscript into its normalized abstract semantic policy.
func (a *AST) Lift() (*Policy, error) {
	policy, err := a.liftRaw()
	if err != nil {
		return nil, err
	}
	return policy.normalized(), nil
}

// liftRaw converts a miniscript node into a semantic policy without
// normalization, following rust-miniscript's per-fragment lifting: AND-like
// fragments become a k=n threshold, OR-like fragments a k=1 threshold, and the
// wrappers pass through their child.
func (a *AST) liftRaw() (*Policy, error) {
	switch a.identifier {
	case f_0:
		return &Policy{Type: PolicyUnsatisfiable}, nil

	case f_1:
		return &Policy{Type: PolicyTrivial}, nil

	case f_pk_k, f_pk_h:
		return &Policy{Type: PolicyKey, Key: a.args[0].identifier}, nil

	case f_after:
		return &Policy{
			Type: PolicyAfter, Locktime: uint32(a.args[0].num),
		}, nil

	case f_older:
		return &Policy{
			Type: PolicyOlder, Locktime: uint32(a.args[0].num),
		}, nil

	case f_sha256:
		return &Policy{Type: PolicySha256, Hash: a.args[0].value}, nil

	case f_hash256:
		return &Policy{Type: PolicyHash256, Hash: a.args[0].value}, nil

	case f_ripemd160:
		return &Policy{
			Type: PolicyRipemd160, Hash: a.args[0].value,
		}, nil

	case f_hash160:
		return &Policy{Type: PolicyHash160, Hash: a.args[0].value}, nil

	case f_and_v, f_and_b:
		// AND is a k=n threshold over both sub expressions.
		return a.liftThresh(2, a.args[0], a.args[1])

	case f_or_b, f_or_c, f_or_d, f_or_i:
		// OR is a k=1 threshold over both sub expressions.
		return a.liftThresh(1, a.args[0], a.args[1])

	case f_andor:
		// andor(X, Y, Z) is or(and(X, Y), Z).
		andPart, err := a.liftThresh(2, a.args[0], a.args[1])
		if err != nil {
			return nil, err
		}
		z, err := a.args[2].liftRaw()
		if err != nil {
			return nil, err
		}
		return &Policy{
			Type: PolicyThresh, K: 1,
			Subs: []*Policy{andPart, z},
		}, nil

	case f_thresh:
		k := int(a.args[0].num)
		subs := make([]*Policy, 0, len(a.args)-1)
		for _, arg := range a.args[1:] {
			sub, err := arg.liftRaw()
			if err != nil {
				return nil, err
			}
			subs = append(subs, sub)
		}
		return &Policy{Type: PolicyThresh, K: k, Subs: subs}, nil

	case f_multi, f_multi_a:
		k := int(a.args[0].num)
		subs := make([]*Policy, 0, len(a.args)-1)
		for _, arg := range a.args[1:] {
			subs = append(subs, &Policy{
				Type: PolicyKey, Key: arg.identifier,
			})
		}
		return &Policy{Type: PolicyThresh, K: k, Subs: subs}, nil

	case f_wrap_a, f_wrap_s, f_wrap_c, f_wrap_d, f_wrap_v, f_wrap_j,
		f_wrap_n:

		// Wrappers do not change the semantics.
		return a.args[0].liftRaw()

	default:
		return nil, fmt.Errorf("cannot lift unknown identifier: %s",
			a.identifier)
	}
}

// liftThresh lifts two sub expressions into a threshold policy with the given
// threshold.
func (a *AST) liftThresh(k int, left, right *AST) (*Policy, error) {
	l, err := left.liftRaw()
	if err != nil {
		return nil, err
	}
	r, err := right.liftRaw()
	if err != nil {
		return nil, err
	}
	return &Policy{Type: PolicyThresh, K: k, Subs: []*Policy{l, r}}, nil
}

// Normalize flattens nested and/or thresholds and eliminates trivial and
// unsatisfiable branches, without reordering. It mirrors rust-miniscript's
// Policy::normalized.
func (p *Policy) Normalize() *Policy {
	return p.normalized()
}

// normalized flattens nested and/or thresholds and eliminates trivial and
// unsatisfiable branches, without reordering. It mirrors rust-miniscript's
// Policy::normalized.
func (p *Policy) normalized() *Policy {
	if p.Type != PolicyThresh {
		return p
	}

	// First normalize every sub policy.
	subs := make([]*Policy, len(p.Subs))
	trivialCount, unsatCount := 0, 0
	for i, sub := range p.Subs {
		subs[i] = sub.normalized()
		switch subs[i].Type {
		case PolicyTrivial:
			trivialCount++

		case PolicyUnsatisfiable:
			unsatCount++
		}
	}

	// n is the number of remaining (non-constant) sub policies; m is the
	// number that still need to be satisfied after removing all the trivial
	// ones.
	n := len(subs) - unsatCount - trivialCount
	m := p.K - trivialCount
	if m < 0 {
		m = 0
	}

	isAnd := m == n
	isOr := m == 1

	var retSubs []*Policy
	for _, sub := range subs {
		switch {
		case sub.Type == PolicyTrivial,
			sub.Type == PolicyUnsatisfiable:

			// Drop constants.

		case sub.Type == PolicyThresh:
			switch {
			case isAnd && isOr:
				// m == n == 1, a thresh(1, X) wrapper; keep the
				// sub threshold as a single element.
				retSubs = append(retSubs, sub)

			case isAnd && sub.K == len(sub.Subs):
				// Flatten a nested AND into an AND.
				retSubs = append(retSubs, sub.Subs...)

			case isOr && sub.K == 1:
				// Flatten a nested OR into an OR.
				retSubs = append(retSubs, sub.Subs...)

			default:
				retSubs = append(retSubs, sub)
			}

		default:
			retSubs = append(retSubs, sub)
		}
	}

	switch {
	case m == 0:
		return &Policy{Type: PolicyTrivial}

	case m > len(retSubs):
		return &Policy{Type: PolicyUnsatisfiable}

	case len(retSubs) == 1:
		return retSubs[0]

	case isAnd:
		return &Policy{
			Type: PolicyThresh, K: len(retSubs), Subs: retSubs,
		}

	case isOr:
		return &Policy{Type: PolicyThresh, K: 1, Subs: retSubs}

	default:
		return &Policy{Type: PolicyThresh, K: m, Subs: retSubs}
	}
}
