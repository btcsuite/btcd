package miniscript

import (
	"errors"
	"fmt"
	"sort"
)

// errImpossibleSatisfaction is returned by the satisfaction-size accessors when
// the expression can never be satisfied (e.g. the "0" combinator).
var errImpossibleSatisfaction = errors.New("miniscript cannot be satisfied")

// satBound holds one worst-case satisfaction (or dissatisfaction) of a
// (sub)expression: its total witness byte size and element count, or an
// invalid marker when the case is impossible.
//
// A signature is assumed to be 73 bytes in the P2WSH (ECDSA) context and 66
// bytes in the P2TR (Schnorr) context, in both cases including the length
// prefix and sighash byte. Sizes always include each element's own length
// prefix but never the prefix that encodes the number of stack elements.
type satBound struct {
	// valid is false when this (dis)satisfaction is impossible.
	valid bool

	// size is the total size in bytes of the witness stack elements.
	size int

	// count is the number of witness stack elements.
	count int
}

// concat combines two bounds that are applied one after the other (both must be
// possible), summing their sizes and counts. This mirrors rust-miniscript's
// `zip` of two SatData values.
func (a satBound) concat(b satBound) satBound {
	if !a.valid || !b.valid {
		return satBound{}
	}
	return satBound{
		valid: true,
		size:  a.size + b.size,
		count: a.count + b.count,
	}
}

// fieldMax takes the field-wise maximum of two alternative bounds, where either
// alternative may be impossible. This mirrors rust-miniscript's
// `fieldwise_max_opt`: an impossible alternative is ignored, and if both are
// possible each field is maximised independently.
func (a satBound) fieldMax(b satBound) satBound {
	if !a.valid {
		return b
	}
	if !b.valid {
		return a
	}

	out := satBound{valid: true, size: a.size, count: a.count}
	if b.size > out.size {
		out.size = b.size
	}
	if b.count > out.count {
		out.count = b.count
	}
	return out
}

// witSize holds the worst-case satisfaction and dissatisfaction bounds of a
// node.
type witSize struct {
	// dsat is the worst-case dissatisfaction bound.
	dsat satBound

	// sat is the worst-case satisfaction bound.
	sat satBound
}

// computeSatSize computes the worst-case satisfaction and dissatisfaction
// witness byte sizes (and element counts) of a node. It follows
// rust-miniscript's ExtData satisfaction sizing exactly, including the
// threshold's per-field greedy selection, so the values match the reference for
// differential weight estimation.
func computeSatSize(node *AST) (*AST, error) {
	invalid := satBound{}
	zero := satBound{valid: true, size: 0, count: 0}

	// Signature and key element sizes depend on the signature type of the
	// context: ECDSA (P2WSH) or Schnorr (P2TR).
	sigSize, keyBytes := 73, 34
	if node.ctx == P2TR {
		sigSize, keyBytes = 66, 33
	}

	switch node.identifier {
	case f_0:
		// Dissatisfied by a single empty element; cannot be satisfied.
		node.satSize = witSize{dsat: zero, sat: invalid}

	case f_1:
		// Satisfied by an empty witness; cannot be dissatisfied.
		node.satSize = witSize{dsat: invalid, sat: zero}

	case f_pk_k:
		// sat: <sig>, dsat: <empty>.
		node.satSize = witSize{
			dsat: satBound{valid: true, size: 1, count: 1},
			sat:  satBound{valid: true, size: sigSize, count: 1},
		}

	case f_pk_h:
		// sat: <sig> <key>, dsat: <empty> <key>.
		node.satSize = witSize{
			dsat: satBound{
				valid: true, size: keyBytes + 1, count: 2,
			},
			sat: satBound{
				valid: true, size: keyBytes + sigSize, count: 2,
			},
		}

	case f_older, f_after:
		// Satisfied by an empty witness; not dissatisfiable.
		node.satSize = witSize{dsat: invalid, sat: zero}

	case f_sha256, f_hash256, f_ripemd160, f_hash160:
		// sat: <32-byte preimage>, dsat: <32-byte non-preimage>. Both
		// are a single 33-byte push (32 bytes plus a one-byte length
		// prefix).
		bound := satBound{valid: true, size: 33, count: 1}
		node.satSize = witSize{dsat: bound, sat: bound}

	case f_andor:
		// sat: satisfy X and Y, or dissatisfy X and satisfy Z. dsat:
		// dissatisfy X and Z.
		x, y, z := node.args[0], node.args[1], node.args[2]
		node.satSize = witSize{
			dsat: x.satSize.dsat.concat(z.satSize.dsat),
			sat: x.satSize.sat.concat(y.satSize.sat).fieldMax(
				x.satSize.dsat.concat(z.satSize.sat),
			),
		}

	case f_and_v:
		// and_v is not dissatisfiable; satisfaction concatenates both.
		x, y := node.args[0], node.args[1]
		node.satSize = witSize{
			dsat: invalid,
			sat:  x.satSize.sat.concat(y.satSize.sat),
		}

	case f_and_b:
		x, y := node.args[0], node.args[1]
		node.satSize = witSize{
			dsat: x.satSize.dsat.concat(y.satSize.dsat),
			sat:  x.satSize.sat.concat(y.satSize.sat),
		}

	case f_or_b:
		x, z := node.args[0], node.args[1]
		node.satSize = witSize{
			dsat: x.satSize.dsat.concat(z.satSize.dsat),
			sat: x.satSize.sat.concat(z.satSize.dsat).fieldMax(
				x.satSize.dsat.concat(z.satSize.sat),
			),
		}

	case f_or_c:
		x, z := node.args[0], node.args[1]
		node.satSize = witSize{
			dsat: invalid,
			sat: x.satSize.sat.fieldMax(
				x.satSize.dsat.concat(z.satSize.sat),
			),
		}

	case f_or_d:
		x, z := node.args[0], node.args[1]
		node.satSize = witSize{
			dsat: x.satSize.dsat.concat(z.satSize.dsat),
			sat: x.satSize.sat.fieldMax(
				x.satSize.dsat.concat(z.satSize.sat),
			),
		}

	case f_or_i:
		x, z := node.args[0], node.args[1]

		// or_i pushes one extra selector element on the witness
		// stack: a "1" (2 bytes) selects the left branch, a "0"
		// (1 byte) the right branch.
		with1 := func(b satBound) satBound {
			if !b.valid {
				return b
			}
			return satBound{
				valid: true,
				size:  b.size + 2,
				count: b.count + 1,
			}
		}
		with0 := func(b satBound) satBound {
			if !b.valid {
				return b
			}
			return satBound{
				valid: true,
				size:  b.size + 1,
				count: b.count + 1,
			}
		}
		node.satSize = witSize{
			dsat: with1(x.satSize.dsat).fieldMax(
				with0(z.satSize.dsat),
			),
			sat: with1(x.satSize.sat).fieldMax(
				with0(z.satSize.sat),
			),
		}

	case f_thresh:
		node.satSize = threshSatSize(node)

	case f_multi:
		k := int(node.args[0].num)

		// sat: <0> <sig>*k, dsat: <0> <0>*k, both pushing k+1
		// elements. The leading OP_0 is one empty element (1 byte).
		node.satSize = witSize{
			dsat: satBound{valid: true, size: 1 + k, count: k + 1},
			sat: satBound{
				valid: true,
				size:  1 + 73*k,
				count: k + 1,
			},
		}

	case f_multi_a:
		n := len(node.args) - 1
		k := int(node.args[0].num)

		// sat: k signatures (66 bytes each) and n-k empty elements.
		// dsat: n empty elements. Both push n elements.
		node.satSize = witSize{
			dsat: satBound{valid: true, size: n, count: n},
			sat: satBound{
				valid: true,
				size:  (n - k) + 66*k,
				count: n,
			},
		}

	case f_wrap_a, f_wrap_s, f_wrap_c, f_wrap_n:
		// These wrappers do not change the witness.
		node.satSize = node.args[0].satSize

	case f_wrap_d:
		x := node.args[0]

		// d: adds a "1" selector element to the child's
		// satisfaction and is dissatisfied by a single empty element.
		sat := x.satSize.sat
		if sat.valid {
			sat = satBound{
				valid: true,
				size:  sat.size + 1,
				count: sat.count + 1,
			}
		}
		node.satSize = witSize{
			dsat: satBound{valid: true, size: 1, count: 1},
			sat:  sat,
		}

	case f_wrap_v:
		x := node.args[0]

		// v: is not dissatisfiable; satisfaction passes through.
		node.satSize = witSize{dsat: invalid, sat: x.satSize.sat}

	case f_wrap_j:
		x := node.args[0]

		// j: is dissatisfied by a single empty element; satisfaction
		// passes through.
		node.satSize = witSize{
			dsat: satBound{valid: true, size: 1, count: 1},
			sat:  x.satSize.sat,
		}

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}

	return node, nil
}

// threshSatSize computes the worst-case satisfaction/dissatisfaction bounds of
// a thresh fragment. It mirrors rust-miniscript's threshold sizing: the
// dissatisfaction dissatisfies every sub expression, while the satisfaction is
// found per field by sorting the sub expressions by the size/count delta
// between their satisfaction and dissatisfaction and satisfying the k+1 with
// the largest delta. The k+1 (rather than k) is a conservative over-count
// carried over from the reference implementation, and is required for the
// values to match it.
func threshSatSize(node *AST) witSize {
	k := int(node.args[0].num)
	subs := node.args[1:]

	// Dissatisfaction dissatisfies every sub expression in order; if any
	// is not dissatisfiable, the whole thresh cannot be dissatisfied.
	dsat := satBound{valid: true, size: 0, count: 0}
	for _, arg := range subs {
		dsat = dsat.concat(arg.satSize.dsat)
	}

	// Each field (size, count) is optimised independently.
	satSizeVal, okSize := threshSatField(
		subs, k, func(b satBound) int { return b.size },
	)
	satCountVal, okCount := threshSatField(
		subs, k, func(b satBound) int { return b.count },
	)

	var sat satBound
	if okSize && okCount {
		sat = satBound{
			valid: true, size: satSizeVal, count: satCountVal,
		}
	}

	return witSize{dsat: dsat, sat: sat}
}

// threshSatField computes one field (selected by proj) of a thresh
// satisfaction. It sorts the sub expressions ascending by the field delta
// between their satisfaction and dissatisfaction (sub expressions that are not
// both satisfiable and dissatisfiable sort first), then satisfies the k+1 with
// the largest delta and dissatisfies the rest, summing the field. It returns
// false if a sub expression that must be (dis)satisfied cannot be.
func threshSatField(subs []*AST, k int, proj func(satBound) int) (int, bool) {
	type entry struct {
		sat, dsat satBound
		key       int
		hasKey    bool
	}

	entries := make([]entry, len(subs))
	for i, arg := range subs {
		s, d := arg.satSize.sat, arg.satSize.dsat
		e := entry{sat: s, dsat: d}
		if s.valid && d.valid {
			e.key = proj(s) - proj(d)
			e.hasKey = true
		}
		entries[i] = e
	}

	// Sort ascending by delta, with entries that have no delta (a sub
	// expression that is not both satisfiable and dissatisfiable) sorting
	// first, mirroring rust's ordering of None before Some. A stable sort
	// keeps the original order among equal keys.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].hasKey != entries[j].hasKey {
			return !entries[i].hasKey
		}
		if !entries[i].hasKey {
			return false
		}
		return entries[i].key < entries[j].key
	})

	// Walk the sorted entries in reverse (largest delta first): the first
	// k+1 are satisfied, the rest dissatisfied.
	total, n := 0, len(entries)
	for i := 0; i < n; i++ {
		e := entries[n-1-i]
		if i <= k {
			if !e.sat.valid {
				return 0, false
			}
			total += proj(e.sat)
		} else {
			if !e.dsat.valid {
				return 0, false
			}
			total += proj(e.dsat)
		}
	}

	return total, true
}

// ScriptLen returns the size in bytes of the script this expression compiles
// to.
func (a *AST) ScriptLen() int {
	return a.scriptLen
}

// MaxSatisfactionSize returns the maximum size in bytes of a satisfying
// witness: the sum of the sizes of all witness elements, each including its own
// length prefix, but excluding the prefix that encodes the number of elements.
// It returns an error if the expression cannot be satisfied.
func (a *AST) MaxSatisfactionSize() (int, error) {
	if !a.satSize.sat.valid {
		return 0, errImpossibleSatisfaction
	}
	return a.satSize.sat.size, nil
}

// MaxSatisfactionWitnessElements returns the maximum number of witness stack
// elements needed to satisfy the expression, including the witness script
// itself (hence the +1). It returns an error if the expression cannot be
// satisfied.
func (a *AST) MaxSatisfactionWitnessElements() (int, error) {
	if !a.satSize.sat.valid {
		return 0, errImpossibleSatisfaction
	}
	return a.satSize.sat.count + 1, nil
}
