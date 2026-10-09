package miniscript

import "fmt"

// execSize holds the maximum number of stack and altstack elements present at
// any point during execution, beyond the initial witness elements, when
// satisfying (sat) or dissatisfying (dsat) a (sub)expression.
//
// It feeds the consensus rule of both segwit contexts that caps the total
// number of stack elements (initial witness elements plus these) at 1000. The
// per-fragment values follow rust-miniscript's max_exec_stack_count, using a
// straightforward peak model: two sub expressions run in sequence take the max
// of their peaks, and if the first leaves its result on the stack while the
// second executes (OP_BOOLAND/OP_BOOLOR, or a threshold's running total), the
// second's peak is one higher.
//
// They deviate from rust's in the three places where rust's value is not the
// true peak: thresh (see threshExecStack), the OP_IFDUP of or_d and the <k> and
// <n> pushes of multi.
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

	// Sequential execution takes a peak, not a sum. Only a retained result
	// from the first fragment increases the second fragment's contribution.
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
		// The script pushes <k>, then the n public keys one at a time,
		// then <n>, all of which are on the stack when
		// OP_CHECKMULTISIG runs.
		n := len(node.args) - 1
		node.execStack = execSize{dsat: fixed(n + 2), sat: fixed(n + 2)}

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

		// or_d compiles to [X] OP_IFDUP OP_NOTIF [Z] OP_ENDIF. On the
		// path where X is satisfied, X leaves exactly one non-zero
		// element (its u property) which OP_IFDUP duplicates before
		// OP_NOTIF consumes the copy, so the peak of that path is at
		// least the two elements. On the path where X is dissatisfied
		// the top element is zero, which OP_IFDUP does not duplicate,
		// and OP_NOTIF pops it before Z runs.
		satX := x.execStack.sat
		if satX.valid {
			satX = maxInt{valid: true, value: max(satX.value, 2)}
		}

		node.execStack = execSize{
			dsat: seqExec(
				x.execStack.dsat, z.execStack.dsat, false,
			),
			sat: satX.or(seqExec(
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
// The sub expressions execute in order. The first starts the sum; every
// later one runs with the running total (one element) already on the stack, and
// the final `<k> OP_EQUAL` needs the total plus the pushed k (two elements). So
// the peak of a given (dis)satisfaction is the maximum, over the sub
// expressions, of each one's own peak plus one for the running total when it is
// not the first, and at least two for the final comparison.
//
// This bounds the execution-stack contribution independently of which witness
// is selected. It intentionally differs from rust-miniscript's order-dependent
// threshold estimate; the differential test documents those differences.
func threshExecStack(node *AST) execSize {
	n := len(node.args) - 1
	k := int(node.args[0].num)

	subSat := make([]maxInt, n)
	subDsat := make([]maxInt, n)
	for i, arg := range node.args[1:] {
		subSat[i], subDsat[i] = arg.execStack.sat, arg.execStack.dsat
	}

	// adjusted returns the contribution of one sub expression's
	// (dis)satisfaction to the peak: every sub expression but the first
	// runs with the running total already on the stack.
	adjusted := func(e maxInt, i int) int {
		if i > 0 {
			return e.value + 1
		}
		return e.value
	}

	// Dissatisfaction dissatisfies every sub expression.
	dsat := maxInt{valid: true, value: 2}
	for i := range subDsat {
		if !subDsat[i].valid {
			dsat = maxInt{}
			break
		}
		dsat.value = max(dsat.value, adjusted(subDsat[i], i))
	}

	// Satisfaction satisfies exactly k sub expressions, taking the worst
	// case over all such choices. Since the peak is a maximum over the sub
	// expressions rather than a sum, it is enough to know for each of them
	// whether some choice satisfies (or dissatisfies) it, so the choices
	// don't have to be enumerated.
	sel := newThreshSelection(k, subSat, subDsat)
	sat := maxInt{}
	if sel.possible {
		sat = maxInt{valid: true, value: 2}
		for i := range subSat {
			if sel.canSatisfy(i) {
				sat.value = max(sat.value, adjusted(
					subSat[i], i,
				))
			}
			if sel.canDissatisfy(i) {
				sat.value = max(sat.value, adjusted(
					subDsat[i], i,
				))
			}
		}
	}

	return execSize{dsat: dsat, sat: sat}
}

// maxExecStackSize returns the maximum number of stack elements pushed during
// execution (beyond the initial witness) to satisfy this script.
func (a *AST) maxExecStackSize() int {
	return a.execStack.sat.value
}
