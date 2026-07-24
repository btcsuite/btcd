package miniscript

import "fmt"

// stackSize holds the maximum number of witness stack elements required to
// satisfy (sat) or dissatisfy (dsat) a (sub)expression.
//
// The witness element count follows the same algebra as the op count in
// opcount.go: `and` (concatenating two witnesses) sums the counts, while `or`
// (choosing the worst-case branch) takes the maximum. The per-fragment
// constants below mirror the witness stacks produced by satisfy() in
// satisfactions.go, so this static bound matches what the satisfier actually
// generates.
type stackSize struct {
	// dsat is the number of witness stack elements needed to dissatisfy
	// the expression.
	dsat maxInt

	// sat is the number of witness stack elements needed to satisfy the
	// expression.
	sat maxInt
}

func computeStackSize(node *AST) (*AST, error) {
	zero := maxInt{valid: true, value: 0}
	one := maxInt{valid: true, value: 1}
	two := maxInt{valid: true, value: 2}
	invalid := maxInt{valid: false}

	switch node.identifier {
	case f_0:
		// Dissatisfied by an empty witness; cannot be satisfied.
		node.stackSize = stackSize{dsat: zero, sat: invalid}

	case f_1:
		// Satisfied by an empty witness; cannot be dissatisfied.
		node.stackSize = stackSize{dsat: invalid, sat: zero}

	case f_pk_k:
		// sat: <sig>, dsat: <empty>.
		node.stackSize = stackSize{dsat: one, sat: one}

	case f_pk_h:
		// sat: <sig> <key>, dsat: <empty> <key>.
		node.stackSize = stackSize{dsat: two, sat: two}

	case f_older, f_after:
		// Satisfied by an empty witness; not dissatisfiable.
		node.stackSize = stackSize{dsat: invalid, sat: zero}

	case f_sha256, f_hash256, f_ripemd160, f_hash160:
		// sat: <preimage>, dsat: <32-byte non-preimage>.
		node.stackSize = stackSize{dsat: one, sat: one}

	case f_andor:
		// This mirrors rust-miniscript's canonical satisfaction and
		// dissatisfaction: the dissatisfaction is always "dissatisfy X,
		// dissatisfy Z" (the else branch), not the malleable
		// "satisfy X, dissatisfy Y" path.
		x, y, z := node.args[0], node.args[1], node.args[2]
		node.stackSize = stackSize{
			dsat: z.stackSize.dsat.and(x.stackSize.dsat),
			sat: y.stackSize.sat.and(x.stackSize.sat).or(
				z.stackSize.sat.and(x.stackSize.dsat),
			),
		}

	case f_and_v:
		// and_v is not dissatisfiable (its first argument is a V-type
		// verify that aborts on failure), matching rust's dissat_data
		// of None.
		x, y := node.args[0], node.args[1]
		node.stackSize = stackSize{
			dsat: invalid,
			sat:  y.stackSize.sat.and(x.stackSize.sat),
		}

	case f_and_b:
		// The canonical dissatisfaction dissatisfies both sub
		// expressions; the "satisfy one, dissatisfy the other" paths
		// are malleable and are not counted (matching rust).
		x, y := node.args[0], node.args[1]
		node.stackSize = stackSize{
			dsat: y.stackSize.dsat.and(x.stackSize.dsat),
			sat:  y.stackSize.sat.and(x.stackSize.sat),
		}

	case f_or_b:
		// The canonical satisfaction satisfies exactly one branch; the
		// "satisfy both" path is malleable and is not counted (matching
		// rust).
		x, z := node.args[0], node.args[1]
		node.stackSize = stackSize{
			dsat: z.stackSize.dsat.and(x.stackSize.dsat),
			sat: z.stackSize.dsat.and(x.stackSize.sat).or(
				z.stackSize.sat.and(x.stackSize.dsat),
			),
		}

	case f_or_c:
		x, z := node.args[0], node.args[1]
		node.stackSize = stackSize{
			dsat: invalid,
			sat: x.stackSize.sat.or(
				z.stackSize.sat.and(x.stackSize.dsat),
			),
		}

	case f_or_d:
		x, z := node.args[0], node.args[1]
		node.stackSize = stackSize{
			dsat: z.stackSize.dsat.and(x.stackSize.dsat),
			sat: x.stackSize.sat.or(
				z.stackSize.sat.and(x.stackSize.dsat),
			),
		}

	case f_or_i:
		x, z := node.args[0], node.args[1]

		// or_i pushes one extra witness element, the branch selector.
		node.stackSize = stackSize{
			dsat: x.stackSize.dsat.and(one).or(
				z.stackSize.dsat.and(one),
			),
			sat: x.stackSize.sat.and(one).or(
				z.stackSize.sat.and(one),
			),
		}

	case f_thresh:
		k := int(node.args[0].num)
		n := len(node.args) - 1

		// Dissatisfaction dissatisfies every sub expression.
		dsat := zero
		for _, arg := range node.args[1:] {
			dsat = dsat.and(arg.stackSize.dsat)
		}

		// Satisfaction satisfies exactly k sub expressions and
		// dissatisfies the rest, taking the worst case over all such
		// choices.
		sat := invalid
		for _, subset := range subsets(n, k) {
			candidate := zero
			for i, arg := range node.args[1:] {
				if containsInt(subset, i) {
					candidate = arg.stackSize.sat.and(
						candidate,
					)
				} else {
					candidate = arg.stackSize.dsat.and(
						candidate,
					)
				}
			}
			sat = sat.or(candidate)
		}
		node.stackSize = stackSize{dsat: dsat, sat: sat}

	case f_multi:
		k := int(node.args[0].num)

		// sat: <0> <sig> ... <sig> (k sigs), dsat: <0> <0> ... <0>
		// (k zeros). Both push k+1 elements.
		count := maxInt{valid: true, value: k + 1}
		node.stackSize = stackSize{dsat: count, sat: count}

	case f_multi_a:
		n := len(node.args) - 1

		// Both sat and dsat push one element per key (a signature or an
		// empty element for the keys not signing), so n elements.
		count := maxInt{valid: true, value: n}
		node.stackSize = stackSize{dsat: count, sat: count}

	case f_wrap_a, f_wrap_s, f_wrap_c, f_wrap_n:
		// These wrappers do not add or remove witness elements.
		node.stackSize = node.args[0].stackSize

	case f_wrap_d:
		x := node.args[0]

		// d: is dissatisfied by a single <empty> element; satisfaction
		// adds the <1> selector element to the child's satisfaction.
		node.stackSize = stackSize{
			dsat: one,
			sat:  x.stackSize.sat.and(one),
		}

	case f_wrap_v:
		x := node.args[0]

		// v: is not dissatisfiable; satisfaction passes through.
		node.stackSize = stackSize{dsat: invalid, sat: x.stackSize.sat}

	case f_wrap_j:
		x := node.args[0]

		// j: is dissatisfied by a single <empty> element; satisfaction
		// passes through.
		node.stackSize = stackSize{dsat: one, sat: x.stackSize.sat}

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}

	return node, nil
}

// maxWitnessSize returns the maximum number of witness stack elements needed to
// satisfy this script. It does not include the witness script itself, which is
// pushed as a separate witness element in a P2WSH spend.
func (a *AST) maxWitnessSize() int {
	return a.stackSize.sat.value
}
