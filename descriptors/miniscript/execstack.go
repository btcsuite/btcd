package miniscript

import "fmt"

// The stack effect of the individual opcodes miniscript compiles to. OP_ELSE
// and OP_ENDIF have no stack effect.
var (
	// siEmpty is the empty script.
	siEmpty = trace(0, 0)

	// siPush is a single push opcode.
	siPush = trace(-1, 0)

	// siHash is a single hash opcode, siNop is OP_CHECKLOCKTIMEVERIFY or
	// OP_CHECKSEQUENCEVERIFY and si0NotEqual is OP_0NOTEQUAL.
	siHash      = trace(0, 0)
	siNop       = trace(0, 0)
	si0NotEqual = trace(0, 0)

	// siIf is OP_IF or OP_NOTIF and siBinaryOp is OP_BOOLAND, OP_BOOLOR or
	// OP_ADD.
	siIf       = trace(1, 1)
	siBinaryOp = trace(1, 1)

	// siDup is OP_DUP and siSize is OP_SIZE.
	siDup  = trace(-1, 0)
	siSize = trace(-1, 0)

	// siEqualVerify, siEqual, siCheckSig and siVerify are the opcodes of
	// the same name.
	siEqualVerify = trace(2, 2)
	siEqual       = trace(1, 1)
	siCheckSig    = trace(1, 1)
	siVerify      = trace(1, 1)
)

// satInfo describes the stack size effect of a set of script execution traces
// (sequences of executed opcodes). It is a port of Bitcoin Core's SatInfo (see
// Node::CalcStackSize in Core's miniscript.h):
//   - the zero value is the empty set, i.e. no such trace exists;
//   - netdiff is the largest difference, over all traces in the set, between
//     the stack size at the start and at the end of the trace (negative for a
//     net increase);
//   - exec is the largest difference, over all traces in the set, between the
//     stack size anywhere during the trace and at its end.
//
// The stack and the altstack are counted together, since that is what the
// consensus limit of 1000 elements applies to.
type satInfo struct {
	valid   bool
	netdiff int
	exec    int
}

// trace returns the set holding the single trace with the given netdiff and
// exec values.
func trace(netdiff, exec int) satInfo {
	return satInfo{valid: true, netdiff: netdiff, exec: exec}
}

// siIfDup is OP_IFDUP, which only duplicates a non-zero top element.
func siIfDup(nonZero bool) satInfo {
	if nonZero {
		return trace(-1, 0)
	}
	return trace(0, 0)
}

// or returns the union of the two sets of traces, for choosing between
// execution branches.
func (a satInfo) or(b satInfo) satInfo {
	if !a.valid {
		return b
	}
	if !b.valid {
		return a
	}

	return trace(max(a.netdiff, b.netdiff), max(a.exec, b.exec))
}

// then returns the set of traces formed by running any trace of a followed by
// any trace of b. The peak is either reached in b, or in a, in which case it
// is b's net difference higher than the end of b.
func (a satInfo) then(b satInfo) satInfo {
	if !a.valid || !b.valid {
		return satInfo{}
	}

	return trace(a.netdiff+b.netdiff, max(b.exec, b.netdiff+a.exec))
}

// seq concatenates the given sets of traces in order.
func seq(first satInfo, rest ...satInfo) satInfo {
	for _, next := range rest {
		first = first.then(next)
	}

	return first
}

// execSize holds the stack size effect of satisfying (sat) or dissatisfying
// (dsat) a (sub)expression. Since every satisfaction of a top-level expression
// ends with a single stack element, exec+1 of its sat is the largest number of
// stack and altstack elements present at any point of the execution, including
// the initial witness elements. That is the value the consensus limit of 1000
// elements applies to.
type execSize struct {
	dsat, sat satInfo
}

// both returns the execSize whose sat and dsat are the same set of traces.
func both(s satInfo) execSize {
	return execSize{dsat: s, sat: s}
}

// computeExecStack computes the execSize of a node from that of its children.
// It is applied bottom-up as part of Parse.
func computeExecStack(node *AST) (*AST, error) {
	none := satInfo{}

	switch node.identifier {
	case f_0:
		node.execStack = execSize{dsat: siPush, sat: none}

	case f_1:
		node.execStack = execSize{dsat: none, sat: siPush}

	case f_pk_k:
		node.execStack = both(siPush)

	case f_pk_h:
		node.execStack = both(seq(siDup, siHash, siPush, siEqualVerify))

	case f_older, f_after:
		node.execStack = execSize{dsat: none, sat: seq(siPush, siNop)}

	case f_sha256, f_hash256, f_ripemd160, f_hash160:
		node.execStack = execSize{
			dsat: none,
			sat: seq(
				siSize, siPush, siEqualVerify, siHash, siPush,
				siEqual,
			),
		}

	case f_multi:
		// multi(k, key1, ..., keyn) starts off with k+1 elements (a
		// zero and k signatures), reaches n+k+3 after pushing <k>, the
		// n keys and <n>, and ends with one. So it removes k elements
		// in total while reaching k+n+2 more than it ends with.
		k, n := int(node.args[0].num), len(node.args)-1
		node.execStack = both(trace(k, k+n+2))

	case f_multi_a:
		// multi_a(k, key1, ..., keyn) starts off with n elements (the
		// signatures or empty vectors), reaches one more after the
		// first key push and ends with one. So it removes n-1 elements
		// in total while reaching n more than it ends with.
		n := len(node.args) - 1
		node.execStack = both(trace(n-1, n))

	case f_andor:
		x, y, z := node.args[0].execStack, node.args[1].execStack,
			node.args[2].execStack
		node.execStack = execSize{
			dsat: seq(x.dsat, siIf, z.dsat),
			sat: seq(x.sat, siIf, y.sat).
				or(seq(x.dsat, siIf, z.sat)),
		}

	case f_and_v:
		x, y := node.args[0].execStack, node.args[1].execStack
		node.execStack = execSize{dsat: none, sat: x.sat.then(y.sat)}

	case f_and_b:
		x, y := node.args[0].execStack, node.args[1].execStack
		node.execStack = execSize{
			dsat: seq(x.dsat, y.dsat, siBinaryOp),
			sat:  seq(x.sat, y.sat, siBinaryOp),
		}

	case f_or_b:
		x, z := node.args[0].execStack, node.args[1].execStack
		node.execStack = execSize{
			dsat: seq(x.dsat, z.dsat, siBinaryOp),
			sat: x.sat.then(z.dsat).or(x.dsat.then(z.sat)).
				then(siBinaryOp),
		}

	case f_or_c:
		x, z := node.args[0].execStack, node.args[1].execStack
		node.execStack = execSize{
			dsat: none,
			sat:  x.sat.then(siIf).or(seq(x.dsat, siIf, z.sat)),
		}

	case f_or_d:
		// or_d compiles to [X] OP_IFDUP OP_NOTIF [Z] OP_ENDIF. On the
		// path where X is satisfied, OP_IFDUP duplicates its non-zero
		// result before OP_NOTIF consumes the copy. On the path where X
		// is dissatisfied the top element is zero, which OP_IFDUP does
		// not duplicate.
		x, z := node.args[0].execStack, node.args[1].execStack
		node.execStack = execSize{
			dsat: seq(x.dsat, siIfDup(false), siIf, z.dsat),
			sat: seq(x.sat, siIfDup(true), siIf).
				or(seq(x.dsat, siIfDup(false), siIf, z.sat)),
		}

	case f_or_i:
		x, z := node.args[0].execStack, node.args[1].execStack
		node.execStack = execSize{
			dsat: siIf.then(x.dsat.or(z.dsat)),
			sat:  siIf.then(x.sat.or(z.sat)),
		}

	case f_thresh:
		node.execStack = threshExecStack(node)

	case f_wrap_a, f_wrap_s, f_wrap_n:
		// The altstack counts towards the same limit as the stack, so
		// moving an element there and back has no effect.
		node.execStack = node.args[0].execStack

	case f_wrap_c:
		x := node.args[0].execStack
		node.execStack = execSize{
			dsat: x.dsat.then(siCheckSig),
			sat:  x.sat.then(siCheckSig),
		}

	case f_wrap_d:
		x := node.args[0].execStack
		node.execStack = execSize{
			dsat: siDup.then(siIf),
			sat:  seq(siDup, siIf, x.sat),
		}

	case f_wrap_v:
		x := node.args[0].execStack
		node.execStack = execSize{dsat: none, sat: x.sat.then(siVerify)}

	case f_wrap_j:
		x := node.args[0].execStack
		node.execStack = execSize{
			dsat: seq(siSize, si0NotEqual, siIf),
			sat:  seq(siSize, si0NotEqual, siIf, x.sat),
		}

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}

	return node, nil
}

// threshExecStack computes the execSize of a thresh(k, X1, ..., Xn) fragment,
// which compiles to [X1] ([X2] OP_ADD) ... ([Xn] OP_ADD) <k> OP_EQUAL.
//
// sats[j] holds the traces through the sub expressions processed so far that
// satisfy exactly j of them. Satisfying the thresh needs exactly k of them, and
// dissatisfying it needs none, so counts above k are never needed. This takes
// O(n*k) steps.
func threshExecStack(node *AST) execSize {
	k := int(node.args[0].num)

	sats := []satInfo{siEmpty}
	for i, arg := range node.args[1:] {
		add := siEmpty
		if i > 0 {
			add = siBinaryOp
		}

		sub := arg.execStack
		next := make([]satInfo, 0, min(len(sats)+1, k+1))
		next = append(next, seq(sats[0], sub.dsat, add))
		for j := 1; j < len(sats); j++ {
			next = append(
				next,
				sats[j].then(sub.dsat).
					or(sats[j-1].then(sub.sat)).then(add),
			)
		}
		if len(sats) <= k {
			next = append(next, seq(
				sats[len(sats)-1], sub.sat, add,
			))
		}
		sats = next
	}

	sat := satInfo{}
	if k < len(sats) {
		sat = seq(sats[k], siPush, siEqual)
	}

	return execSize{
		dsat: seq(sats[0], siPush, siEqual),
		sat:  sat,
	}
}

// maxExecStackSize returns the largest number of stack and altstack elements
// present at any point while executing a satisfaction of this node, including
// the initial witness elements. Every node but one of type V leaves its result
// on the stack. It is only meaningful if the node has a satisfaction.
func (a *AST) maxExecStackSize() int {
	if a.basicType == typeV {
		return a.execStack.sat.exec
	}

	return a.execStack.sat.exec + 1
}
