package miniscript

import "fmt"

// execSize holds the maximum number of stack and altstack elements present at
// any point during execution, beyond the initial witness elements, when
// satisfying (sat) or dissatisfying (dsat) a (sub)expression.
//
// It is only relevant in the P2TR context, whose consensus rules cap the total
// number of stack elements (initial witness elements plus these) at 1000. The
// per-fragment values mirror rust-miniscript's max_exec_stack_count, using a
// straightforward peak model: two sub expressions run in sequence take the max
// of their peaks, and if the first leaves its result on the stack while the
// second executes (OP_BOOLAND/OP_BOOLOR, or a threshold's running total), the
// second's peak is one higher.
type execSize struct {
	dsat, sat maxInt
}

// seqExec combines the execution stack sizes of two sub expressions that run in
// sequence. If keepFirst is set, the first sub expression leaves a result on
// the stack while the second executes, so the second's peak counts one extra
// element. The result is invalid if either input is.
func seqExec(a, b maxInt, keepFirst bool) maxInt {
	if !a.valid || !b.valid {
		return maxInt{}
	}
	second := b.value
	if keepFirst {
		second++
	}
	return maxInt{valid: true, value: max(a.value, second)}
}

// computeExecStack computes the execSize of a node from that of its children.
// It is applied bottom-up as part of Parse.
func computeExecStack(node *AST) (*AST, error) {
	invalid := maxInt{valid: false}
	fixed := func(v int) maxInt { return maxInt{valid: true, value: v} }

	switch node.identifier {
	case f_0:
		node.execStack = execSize{dsat: fixed(1), sat: invalid}

	case f_1:
		node.execStack = execSize{dsat: invalid, sat: fixed(1)}

	case f_pk_k:
		node.execStack = execSize{dsat: fixed(1), sat: fixed(1)}

	case f_pk_h:
		// OP_DUP and the hash push.
		node.execStack = execSize{dsat: fixed(2), sat: fixed(2)}

	case f_older, f_after:
		node.execStack = execSize{dsat: invalid, sat: fixed(1)}

	case f_sha256, f_hash256, f_ripemd160, f_hash160:
		// Either <32-byte size> or <hash> <32-byte value>.
		node.execStack = execSize{dsat: fixed(2), sat: fixed(2)}

	case f_multi:
		// The n public keys are pushed one at a time.
		n := len(node.args) - 1
		node.execStack = execSize{dsat: fixed(n), sat: fixed(n)}

	case f_multi_a:
		// The two numbers before the final OP_NUMEQUAL.
		node.execStack = execSize{dsat: fixed(2), sat: fixed(2)}

	case f_andor:
		x, y, z := node.args[0], node.args[1], node.args[2]
		node.execStack = execSize{
			dsat: seqExec(
				x.execStack.dsat, z.execStack.dsat, false,
			),
			sat: seqExec(x.execStack.sat, y.execStack.sat, false).
				or(seqExec(
					x.execStack.dsat, z.execStack.sat,
					false,
				)),
		}

	case f_and_v:
		x, y := node.args[0], node.args[1]
		node.execStack = execSize{
			dsat: invalid,
			sat:  seqExec(x.execStack.sat, y.execStack.sat, false),
		}

	case f_and_b:
		x, y := node.args[0], node.args[1]
		node.execStack = execSize{
			dsat: seqExec(x.execStack.dsat, y.execStack.dsat, true),
			sat:  seqExec(x.execStack.sat, y.execStack.sat, true),
		}

	case f_or_b:
		x, z := node.args[0], node.args[1]
		node.execStack = execSize{
			dsat: seqExec(x.execStack.dsat, z.execStack.dsat, true),
			sat: seqExec(x.execStack.sat, z.execStack.dsat, true).
				or(seqExec(
					x.execStack.dsat, z.execStack.sat, true,
				)),
		}

	case f_or_c:
		x, z := node.args[0], node.args[1]
		node.execStack = execSize{
			dsat: invalid,
			sat: x.execStack.sat.or(seqExec(
				x.execStack.dsat, z.execStack.sat, false,
			)),
		}

	case f_or_d:
		x, z := node.args[0], node.args[1]
		node.execStack = execSize{
			dsat: seqExec(
				x.execStack.dsat, z.execStack.dsat, false,
			),
			sat: x.execStack.sat.or(seqExec(
				x.execStack.dsat, z.execStack.sat, false,
			)),
		}

	case f_or_i:
		x, z := node.args[0], node.args[1]
		node.execStack = execSize{
			dsat: x.execStack.dsat.or(z.execStack.dsat),
			sat:  x.execStack.sat.or(z.execStack.sat),
		}

	case f_thresh:
		node.execStack = threshExecStack(node)

	case f_wrap_a, f_wrap_s, f_wrap_c, f_wrap_n:
		// These wrappers do not change the peak execution stack size.
		node.execStack = node.args[0].execStack

	case f_wrap_d:
		x := node.args[0]
		sat := invalid
		if x.execStack.sat.valid {
			// OP_DUP OP_IF leaves at least the duplicated element.
			sat = fixed(max(1, x.execStack.sat.value))
		}
		node.execStack = execSize{dsat: fixed(1), sat: sat}

	case f_wrap_v:
		node.execStack = execSize{
			dsat: invalid,
			sat:  node.args[0].execStack.sat,
		}

	case f_wrap_j:
		node.execStack = execSize{
			dsat: fixed(1),
			sat:  node.args[0].execStack.sat,
		}

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}

	return node, nil
}

// threshExecStack computes the execSize of a thresh(k, X1, ..., Xn) fragment.
//
// The sub expressions execute in order. The first runs on a clean stack; every
// later one runs with the running total (one element) already on the stack, and
// the final `<k> OP_EQUAL` needs the total plus the pushed k (two elements). So
// the peak of a given (dis)satisfaction is the maximum, over the sub
// expressions, of each one's own peak plus one for the running total when it is
// not the first, and at least two for the final comparison.
//
// This is a tight, sound model of the true execution stack peak. (It does not
// match rust-miniscript's threshold value exactly: rust's is an
// order-dependent, internally inconsistent conservative estimate not worth
// replicating; see the differential test for details.)
func threshExecStack(node *AST) execSize {
	n := len(node.args) - 1
	k := int(node.args[0].num)

	// peak returns the execution stack peak for the choice of satisfied sub
	// expressions given by the predicate, or an invalid value if any
	// required (dis)satisfaction is unavailable.
	peak := func(satisfied func(i int) bool) maxInt {
		result := 2
		for i := 0; i < n; i++ {
			sub := node.args[i+1].execStack
			e := sub.dsat
			if satisfied(i) {
				e = sub.sat
			}
			if !e.valid {
				return maxInt{}
			}
			v := e.value
			if i > 0 {
				v++
			}
			result = max(result, v)
		}
		return maxInt{valid: true, value: result}
	}

	// Dissatisfaction dissatisfies every sub expression.
	dsat := peak(func(int) bool { return false })

	// Satisfaction satisfies exactly k sub expressions, taking the worst
	// case over all such choices.
	sat := maxInt{}
	for _, subset := range subsets(n, k) {
		sat = sat.or(peak(func(i int) bool {
			return containsInt(subset, i)
		}))
	}

	return execSize{dsat: dsat, sat: sat}
}

// maxExecStackSize returns the maximum number of stack elements pushed during
// execution (beyond the initial witness) to satisfy this script.
func (a *AST) maxExecStackSize() int {
	return a.execStack.sat.value
}
